package api

import (
	"testing"
	"time"

	"futdarapaziada/api/internal/store"
)

func TestNormalizeWhatsAppNumber(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		want    string
		wantErr bool
	}{
		{name: "celular com DDD", phone: "(11) 98765-4321", want: "5511987654321"},
		{name: "número com país", phone: "+55 11 98765-4321", want: "5511987654321"},
		{name: "fixo com DDD", phone: "1133334444", want: "551133334444"},
		{name: "sem DDD", phone: "987654321", wantErr: true},
		{name: "outro país", phone: "+1 415 555 2671", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeWhatsAppNumber(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeWhatsAppNumber(%q) error = %v, wantErr %v", tt.phone, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("normalizeWhatsAppNumber(%q) = %q, want %q", tt.phone, got, tt.want)
			}
		})
	}
}

func TestReminderMessage(t *testing.T) {
	charge := store.Charge{
		ReferenceMonth: "2026-07",
		AmountCents:    12500,
		PixPayload:     "pix-copia-e-cola",
	}
	got := reminderMessage(
		"Oi, {{nome}}: {{mes_referencia}} custa {{valor}}, vence {{data_vencimento}}. PIX: {{codigo_pix}}",
		"João", charge, time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC),
	)
	want := "Oi, João: 2026-07 custa R$ 125,00, vence 07/07/2026. PIX: pix-copia-e-cola"
	if got != want {
		t.Errorf("reminderMessage() = %q, want %q", got, want)
	}
}
