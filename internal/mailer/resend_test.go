package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendPasswordReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["from"] != "Fut <nao-responda@example.com>" {
			t.Errorf("from = %q", body["from"])
		}
		if body["to"] != "player@example.com" {
			t.Errorf("to = %q", body["to"])
		}
		if body["html"] == "" {
			t.Error("html is empty")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	mailer := newResend("test-key", "Fut <nao-responda@example.com>", server.Client(), server.URL)
	if err := mailer.SendPasswordReset(context.Background(), "player@example.com", "https://example.com/redefinir-senha?token=test"); err != nil {
		t.Fatal(err)
	}
}

func TestSendPasswordResetReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	mailer := newResend("test-key", "Fut <nao-responda@example.com>", server.Client(), server.URL)
	if err := mailer.SendPasswordReset(context.Background(), "player@example.com", "https://example.com"); err == nil {
		t.Fatal("SendPasswordReset() error = nil, want provider error")
	}
}
