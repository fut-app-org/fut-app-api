package api

import (
	"testing"
	"time"

	"futdarapaziada/api/internal/store"
)

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
