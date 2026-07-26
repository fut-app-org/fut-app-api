package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/mercadopago/sdk-go/pkg/webhook"
)

func TestAmountInReais(t *testing.T) {
	if got, want := amountInReais(8050), "80.50"; got != want {
		t.Errorf("amountInReais() = %q, want %q", got, want)
	}
}

func TestMercadoPagoIdempotencyKey(t *testing.T) {
	key, err := mercadoPagoIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 36 || strings.Count(key, "-") != 4 {
		t.Errorf("mercadoPagoIdempotencyKey() = %q, want UUID", key)
	}
	if key[14] != '4' {
		t.Errorf("mercadoPagoIdempotencyKey() = %q, want UUID v4", key)
	}
}

func TestMercadoPagoWebhookDataIDVerifiesUppercaseOrderID(t *testing.T) {
	const (
		secret    = "test-secret"
		orderID   = "ORDTST01KYFVH4FFSFKN60VKZEHP62E1"
		requestID = "request-123"
		timestamp = "1704908010"
	)

	manifest := "id:ordtst01kyfvh4ffsfkn60vkzehp62e1;request-id:" + requestID + ";ts:" + timestamp + ";"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(manifest))
	signature := "ts=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))

	err := webhook.ValidateSignature(signature, requestID, mercadoPagoWebhookDataID(orderID), secret)
	if err != nil {
		t.Fatalf("ValidateSignature() = %v, want nil", err)
	}
}

func TestPixExpiration(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	got, err := pixExpiration(now, "2026-07-28")
	if err != nil {
		t.Fatal(err)
	}
	if got != "PT85H59M" {
		t.Errorf("pixExpiration() = %q, want PT85H59M", got)
	}
}

func TestPixExpirationRenewsExpiredCharge(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	got, err := pixExpiration(now, "2026-07-24")
	if err != nil {
		t.Fatal(err)
	}
	if got != "PT24H0M" {
		t.Errorf("pixExpiration() = %q, want PT24H0M", got)
	}
}
