package mercadopago

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePixOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/orders" {
			t.Fatalf("request = %s %s, want POST /v1/orders", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("X-Idempotency-Key"); got != "charge-123" {
			t.Errorf("X-Idempotency-Key = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["external_reference"] != "charge-123" {
			t.Errorf("external_reference = %v", body["external_reference"])
		}
		if body["total_amount"] != "42.50" {
			t.Errorf("total_amount = %v", body["total_amount"])
		}
		payer, ok := body["payer"].(map[string]any)
		if !ok || payer["first_name"] != "APRO" {
			t.Errorf("payer.first_name = %v, want APRO", payer["first_name"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"ORD123",
			"external_reference":"charge-123",
			"status":"action_required",
			"status_detail":"waiting_transfer",
			"transactions":{"payments":[{
				"id":"PAY123",
				"payment_method":{"id":"pix","type":"bank_transfer","qr_code":"pix-code","ticket_url":"https://ticket","qr_code_base64":"image"}
			}]}
		}`))
	}))
	defer server.Close()

	client := newClient("test-token", server.URL+"/v1/orders", server.Client())
	order, err := client.CreatePixOrder(context.Background(), CreatePixOrderInput{
		Amount:             "42.50",
		ChargeID:           "charge-123",
		PayerEmail:         "buyer@testuser.com",
		PayerFirstName:     "APRO",
		ExpirationDuration: "PT48H0M",
		IdempotencyKey:     "charge-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	payment, ok := order.PixPayment()
	if !ok || payment.PaymentMethod.QRCode != "pix-code" {
		t.Errorf("PixPayment() = %#v, %t", payment, ok)
	}
}

func TestOrderIsApproved(t *testing.T) {
	order := Order{Status: "processed", StatusDetail: "accredited"}
	if !order.IsApproved() {
		t.Error("IsApproved() = false, want true")
	}

	order.StatusDetail = "waiting_transfer"
	if order.IsApproved() {
		t.Error("IsApproved() = true for a pending order")
	}
}
