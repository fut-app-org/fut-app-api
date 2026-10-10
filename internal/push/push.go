// Package push sends encrypted Web Push messages to opted-in devices.
package push

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type Subscription = webpush.Subscription

type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Only browser push services are accepted, never arbitrary URLs on our network.
func ValidateSubscription(s Subscription) error {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || len(s.Endpoint) > 4096 {
		return errors.New("endereço de notificação inválido")
	}
	h := strings.ToLower(u.Hostname())
	allowed := h == "fcm.googleapis.com" || h == "updates.push.services.mozilla.com" || h == "web.push.apple.com" || strings.HasSuffix(h, ".push.apple.com") || strings.HasSuffix(h, ".notify.windows.com")
	if !allowed {
		return errors.New("serviço de notificação não suportado")
	}
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil {
		return errors.New("chave de notificação inválida")
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return errors.New("chave de notificação inválida")
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return errors.New("autenticação de notificação inválida")
	}
	return nil
}

type Client struct {
	public, private, subject string
	http                     *http.Client
}

func New(public, private, subject string) *Client {
	return &Client{public: public, private: private, subject: subject, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Enabled() bool {
	return c != nil && c.public != "" && c.private != "" && c.subject != ""
}
func (c *Client) Send(ctx context.Context, s Subscription, m Message, ttl int) (int, error) {
	if !c.Enabled() {
		return 0, errors.New("push não configurado")
	}
	if err := ValidateSubscription(s); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &s, &webpush.Options{HTTPClient: c.http, Subscriber: c.subject, VAPIDPublicKey: c.public, VAPIDPrivateKey: c.private, TTL: ttl})
	if err != nil {
		return 0, errors.New("falha ao acessar serviço push")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("serviço push retornou HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
