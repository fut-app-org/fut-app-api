package busdays

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func TestAddBusinessDays(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		n    int
		want time.Time
	}{
		// Exemplo do requisito 6.4: gerada na segunda, vence no 5º dia útil (segunda seguinte).
		{"segunda + 5 úteis", date(2026, time.July, 13), 5, date(2026, time.July, 20)},
		{"terça + 5 úteis", date(2026, time.July, 14), 5, date(2026, time.July, 21)},
		{"sexta + 1 útil pula o fim de semana", date(2026, time.July, 17), 1, date(2026, time.July, 20)},
		{"sábado + 1 útil", date(2026, time.July, 18), 1, date(2026, time.July, 20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddBusinessDays(tt.from, tt.n)
			if got.Format("2006-01-02") != tt.want.Format("2006-01-02") {
				t.Errorf("AddBusinessDays(%s, %d) = %s, quer %s",
					tt.from.Format("2006-01-02"), tt.n, got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

func TestSubBusinessDays(t *testing.T) {
	// Lembrete um dia útil antes de uma segunda-feira cai na sexta anterior.
	got := SubBusinessDays(date(2026, time.July, 20), 1)
	if got.Format("2006-01-02") != "2026-07-17" {
		t.Errorf("SubBusinessDays(segunda, 1) = %s, quer 2026-07-17", got.Format("2006-01-02"))
	}
}
