package api

import (
	"testing"
	"time"

	"futdarapaziada/api/internal/store"
)

func TestValidateMatchChargeInput(t *testing.T) {
	tests := []struct {
		name       string
		month      string
		title      string
		totalCents int64
		userIDs    []string
		wantErr    bool
	}{
		{name: "válido com mês", month: "2026-09", title: "Quadra society", totalCents: 30000, userIDs: []string{"a", "b"}},
		{name: "válido sem mês", title: "Quadra society", totalCents: 30000, userIDs: []string{"a"}},
		{name: "título vazio", title: "  ", totalCents: 30000, userIDs: []string{"a"}, wantErr: true},
		{name: "total zerado", title: "Quadra", totalCents: 0, userIDs: []string{"a"}, wantErr: true},
		{name: "total negativo", title: "Quadra", totalCents: -100, userIDs: []string{"a"}, wantErr: true},
		{name: "sem participantes", title: "Quadra", totalCents: 30000, userIDs: nil, wantErr: true},
		{name: "mês inválido", month: "2026/09", title: "Quadra", totalCents: 30000, userIDs: []string{"a"}, wantErr: true},
		{name: "mês curto", month: "2026-9", title: "Quadra", totalCents: 30000, userIDs: []string{"a"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMatchChargeInput(tt.month, tt.title, tt.totalCents, tt.userIDs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMatchChargeInput() error = %v, wantErr %v", err, tt.wantErr)
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
