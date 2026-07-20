package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"futdarapaziada/api/internal/busdays"
	"futdarapaziada/api/internal/store"
)

func (s *Server) handleMyCharges(w http.ResponseWriter, r *http.Request) {
	charges, err := s.store.ChargesByUser(r.Context(), currentUser(r).ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(charges))
}

func (s *Server) handleAdminCharges(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")

	batch, err := s.store.BatchByMonth(r.Context(), month)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"batch": nil, "charges": []store.Charge{}})
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	charges, err := s.store.ChargesByMonth(r.Context(), batch.ReferenceMonth)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batch": batch, "charges": orEmpty(charges)})
}

func (s *Server) handleGenerateCharges(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Month            string `json:"month"` // YYYY-MM
		TotalAmountCents int64  `json:"total_amount_cents"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Month) != 7 || !strings.Contains(body.Month, "-") || body.TotalAmountCents <= 0 {
		writeError(w, http.StatusBadRequest, "informe month (YYYY-MM) e total_amount_cents > 0")
		return
	}

	user := currentUser(r)
	dueDate := busdays.AddBusinessDays(time.Now(), 5)
	batch, err := s.store.GenerateBatch(r.Context(), body.Month, body.TotalAmountCents, dueDate, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// Guarda o total como padrão do próximo mês e agenda o lembrete de WhatsApp
	// para um dia útil antes do vencimento.
	_ = s.store.SetSetting(r.Context(), "monthly_total_cents", fmt.Sprint(body.TotalAmountCents))
	s.scheduleReminders(r, batch)

	s.store.LogActivity(r.Context(), &user.ID, "charges_generated",
		fmt.Sprintf("%s gerou %d cobranças de %s para %s",
			user.Name, batch.UserCount, formatCentsBR(batch.IndividualAmountCents), batch.ReferenceMonth))
	writeJSON(w, http.StatusCreated, batch)
}

func (s *Server) scheduleReminders(r *http.Request, batch store.ChargeBatch) {
	due, err := time.Parse("2006-01-02", batch.DueDate)
	if err != nil {
		return
	}
	// 9h da manhã do dia útil anterior ao vencimento.
	remindAt := busdays.SubBusinessDays(due, 1)
	remindAt = time.Date(remindAt.Year(), remindAt.Month(), remindAt.Day(), 9, 0, 0, 0, time.Local)

	template, errSettings := s.store.Settings(r.Context())
	if errSettings != nil {
		return
	}
	charges, err := s.store.ChargesByMonth(r.Context(), batch.ReferenceMonth)
	if err != nil {
		return
	}
	for _, c := range charges {
		user, err := s.store.UserByID(r.Context(), c.UserID)
		if err != nil || user.Phone == "" {
			continue
		}
		msg := template["reminder_template"]
		msg = strings.ReplaceAll(msg, "{{nome}}", user.Name)
		msg = strings.ReplaceAll(msg, "{{mes_referencia}}", c.ReferenceMonth)
		msg = strings.ReplaceAll(msg, "{{valor}}", formatCentsBR(c.AmountCents))
		msg = strings.ReplaceAll(msg, "{{data_vencimento}}", due.Format("02/01/2006"))
		msg = strings.ReplaceAll(msg, "{{codigo_pix}}", c.PixPayload)
		chargeID := c.ID
		_ = s.store.ScheduleNotification(r.Context(), c.UserID, &chargeID, user.Phone, msg, remindAt)
	}
}

func (s *Server) handleMarkPaid(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Method string `json:"method"` // "pix" ou "manual"
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Method == "" {
		body.Method = "manual"
	}

	user := currentUser(r)
	charge, reactivated, err := s.store.MarkChargePaid(r.Context(), r.PathValue("id"), body.Method, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.store.LogActivity(r.Context(), &user.ID, "payment",
		fmt.Sprintf("%s pagou a mensalidade de %s · registrado por %s",
			charge.UserName, charge.ReferenceMonth, user.Name))
	if reactivated {
		s.store.LogActivity(r.Context(), &user.ID, "user_reactivated",
			fmt.Sprintf("%s foi reativado após confirmação do pagamento", charge.UserName))
	}
	writeJSON(w, http.StatusOK, charge)
}

func (s *Server) handleCancelCharge(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetChargeStatus(r.Context(), r.PathValue("id"), "cancelled"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleExemptCharge(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetChargeStatus(r.Context(), r.PathValue("id"), "exempt"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// formatCentsBR formata centavos como "R$ 80,00".
func formatCentsBR(cents int64) string {
	return fmt.Sprintf("R$ %d,%02d", cents/100, cents%100)
}
