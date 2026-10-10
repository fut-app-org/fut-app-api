package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	webpush "github.com/SherClockHolmes/webpush-go"
	"io"
	"net/http"
	"strings"
	"testing"
)

func validSubscription(t *testing.T) Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var s Subscription
	s.Endpoint = "https://fcm.googleapis.com/fcm/send/test"
	s.Keys.P256dh = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return s
}
func TestSubscriptionValidation(t *testing.T) {
	s := validSubscription(t)
	for _, endpoint := range []string{"https://fcm.googleapis.com/x", "https://updates.push.services.mozilla.com/x", "https://web.push.apple.com/x", "https://wns2.notify.windows.com/x"} {
		s.Endpoint = endpoint
		if err := ValidateSubscription(s); err != nil {
			t.Errorf("valid endpoint %s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"http://fcm.googleapis.com/x", "https://127.0.0.1/x", "https://localhost/x", "https://fcm.googleapis.com.evil.test/x", "https://user@fcm.googleapis.com/x", "https://fcm.googleapis.com:8080/x", "https://example.com/x"} {
		s.Endpoint = endpoint
		if ValidateSubscription(s) == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	s = validSubscription(t)
	s.Keys.Auth = "bad"
	if ValidateSubscription(s) == nil {
		t.Fatal("bad auth accepted")
	}
	s = validSubscription(t)
	s.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if ValidateSubscription(s) == nil {
		t.Fatal("invalid curve point accepted")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestEncryptedSendAndProviderErrors(t *testing.T) {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{201, 410, 503} {
		c := New(public, private, "mailto:admin@example.com")
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != "POST" || r.Header.Get("Authorization") == "" || r.Header.Get("Content-Encoding") != "aes128gcm" {
				t.Fatal("missing Web Push encryption/authentication")
			}
			authorization := r.Header.Get("Authorization")
			token := strings.Split(strings.TrimPrefix(authorization, "vapid t="), ",")[0]
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Fatal("invalid VAPID token")
			}
			claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil {
				t.Fatal(err)
			}
			var claims map[string]any
			if err = json.Unmarshal(claimsJSON, &claims); err != nil {
				t.Fatal(err)
			}
			if claims["sub"] != "mailto:admin@example.com" {
				t.Fatalf("invalid VAPID subject: %v", claims["sub"])
			}
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "Confirmar presença") {
				t.Fatal("unencrypted payload")
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})
		got, err := c.Send(context.Background(), validSubscription(t), Message{Title: "Confirmar presença"}, 60)
		if got != status || (err == nil) != (status == 201) {
			t.Fatalf("status %d: got %d, %v", status, got, err)
		}
	}
}
