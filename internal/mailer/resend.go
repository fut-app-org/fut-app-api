// Package mailer sends transactional email through Resend.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const resendEndpoint = "https://api.resend.com/emails"

type Resend struct {
	apiKey   string
	from     string
	client   *http.Client
	endpoint string
}

func NewResend(apiKey, from string) *Resend {
	return newResend(apiKey, from, &http.Client{Timeout: 15 * time.Second}, resendEndpoint)
}

func newResend(apiKey, from string, client *http.Client, endpoint string) *Resend {
	return &Resend{
		apiKey:   strings.TrimSpace(apiKey),
		from:     strings.TrimSpace(from),
		client:   client,
		endpoint: endpoint,
	}
}

func (r *Resend) Enabled() bool { return r.apiKey != "" && r.from != "" }

func (r *Resend) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	body, err := json.Marshal(map[string]string{
		"from":    r.from,
		"to":      to,
		"subject": "Redefina sua senha \u2014 Fut da Rapaziada",
		"html": `<p>Recebemos uma solicita&ccedil;&atilde;o para redefinir sua senha.</p>` +
			`<p><a href="` + resetURL + `">Redefinir senha</a></p>` +
			`<p>Este link expira em 30 minutos. Se voc&ecirc; n&atilde;o solicitou, ignore este e-mail.</p>`,
	})
	if err != nil {
		return fmt.Errorf("codificando e-mail: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("criando requisiÃ§Ã£o Resend: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("enviando e-mail Resend: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Resend respondeu com HTTP %d", resp.StatusCode)
	}
	return nil
}
