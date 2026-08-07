package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEvolutionSenderSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/send/text" {
			t.Errorf("request inesperado: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("apikey"); got != "token-da-instancia" {
			t.Errorf("apikey = %q, esperado %q", got, "token-da-instancia")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificando body: %v", err)
		}
		if body["number"] != "5511987654321" || body["text"] != "olá" {
			t.Errorf("body = %v", body)
		}
		w.Write([]byte(`{"Id":"MSG123"}`))
	}))
	defer server.Close()

	sender := NewEvolutionSender(server.URL, "token-da-instancia")
	id, err := sender.Send("5511987654321", "olá")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "MSG123" {
		t.Errorf("id = %q, esperado %q", id, "MSG123")
	}
}

func TestEvolutionSenderReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"instance disconnected"}`))
	}))
	defer server.Close()

	sender := NewEvolutionSender(server.URL, "token")
	_, err := sender.Send("5511987654321", "olá")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "instance disconnected") {
		t.Errorf("erro = %v", err)
	}
}

func TestExtractMessageID(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "Id direto", body: `{"Id":"ABC"}`, want: "ABC"},
		{name: "id minúsculo", body: `{"id":"abc"}`, want: "abc"},
		{name: "key.id aninhado", body: `{"key":{"id":"K1"}}`, want: "K1"},
		{name: "sem id", body: `{"ok":true}`, want: "evolution"},
		{name: "json inválido", body: `oops`, want: "evolution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractMessageID([]byte(tt.body)); got != tt.want {
				t.Errorf("extractMessageID = %q, esperado %q", got, tt.want)
			}
		})
	}
}
