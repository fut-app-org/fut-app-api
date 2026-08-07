package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EvolutionSender envia mensagens por uma instância Evolution Go
// (evoapicloud/evolution-go) usando o token da instância como apikey.
type EvolutionSender struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewEvolutionSender(baseURL, apiKey string) EvolutionSender {
	return EvolutionSender{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s EvolutionSender) Send(phone, message string) (string, error) {
	body, err := json.Marshal(map[string]string{"number": phone, "text": message})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, s.baseURL+"/send/text", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("evolution-go: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("evolution-go: lendo resposta: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("evolution-go: status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	return extractMessageID(respBody), nil
}

// extractMessageID faz o melhor esforço para achar o id da mensagem na
// resposta (o formato varia entre versões do evolution-go).
func extractMessageID(body []byte) string {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "evolution"
	}
	for _, key := range []string{"Id", "id", "messageId", "MessageId"} {
		if v, ok := parsed[key].(string); ok && v != "" {
			return v
		}
	}
	if key, ok := parsed["key"].(map[string]any); ok {
		if v, ok := key["id"].(string); ok && v != "" {
			return v
		}
	}
	return "evolution"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// NormalizeNumber converte telefone solto ("(11) 98765-4321") para o formato
// internacional sem "+" usado pelo WhatsApp ("5511987654321"). Aceita apenas
// números brasileiros com DDD.
func NormalizeNumber(phone string) (string, error) {
	trimmed := strings.TrimSpace(phone)
	var digits strings.Builder
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	number := digits.String()
	if (strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "00")) && !strings.HasPrefix(number, "55") {
		return "", errors.New("informe um WhatsApp brasileiro com DDD")
	}
	switch len(number) {
	case 10, 11:
		return "55" + number, nil
	case 12, 13:
		if strings.HasPrefix(number, "55") {
			return number, nil
		}
	}
	return "", errors.New("informe um WhatsApp brasileiro com DDD")
}
