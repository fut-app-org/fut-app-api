package api

import (
	"testing"
	"time"
)

func TestAmountInReais(t *testing.T) {
	if got, want := amountInReais(8050), "80.50"; got != want {
		t.Errorf("amountInReais() = %q, want %q", got, want)
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

func TestPixExpirationRejectsExpiredCharge(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	if _, err := pixExpiration(now, "2026-07-24"); err == nil {
		t.Error("pixExpiration() error = nil, want error")
	}
}
