package api

import (
	"futdarapaziada/api/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPushRoutesRequireSession(t *testing.T) {
	s := NewServer(config.Config{JWTSecret: []byte("test")}, nil, nil)
	for _, test := range []struct{ method, path string }{{"GET", "/api/push/config"}, {"POST", "/api/push/subscriptions/status"}, {"POST", "/api/push/subscriptions"}, {"DELETE", "/api/push/subscriptions"}} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`)))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", test.method, test.path, w.Code)
		}
	}
}
func TestPushConfigNeverExposesPrivateKey(t *testing.T) {
	s := NewServer(config.Config{VAPIDPublicKey: "public", VAPIDPrivateKey: "private-secret", VAPIDSubject: "mailto:admin@example.com"}, nil, nil)
	w := httptest.NewRecorder()
	s.handlePushConfig(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":true`) || strings.Contains(w.Body.String(), "private-secret") {
		t.Fatal(w.Body.String())
	}
}
func TestPushSubscribeRejectsUnsafeEndpointsBeforeDatabase(t *testing.T) {
	s := NewServer(config.Config{VAPIDPublicKey: "public", VAPIDPrivateKey: "private", VAPIDSubject: "mailto:admin@example.com"}, nil, nil)
	w := httptest.NewRecorder()
	s.handleSubscribePush(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"endpoint":"http://127.0.0.1:8080","keys":{"auth":"x","p256dh":"x"}}`)))
	if w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
}
func TestPushSubscribeDisabledWithoutKeys(t *testing.T) {
	s := NewServer(config.Config{}, nil, nil)
	w := httptest.NewRecorder()
	s.handleSubscribePush(w, httptest.NewRequest("POST", "/", strings.NewReader(`{}`)))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
