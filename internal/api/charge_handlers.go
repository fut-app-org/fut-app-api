package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

	s.store.LogActivity(r.Context(), &user.ID, "charges_generated",
		fmt.Sprintf("%s gerou %d cobranças de %s para %s",
			user.Name, batch.UserCount, formatCentsBR(batch.IndividualAmountCents), batch.ReferenceMonth))
	writeJSON(w, http.StatusCreated, batch)
}

const defaultReminderTemplate = "Olá, {{nome}}. A mensalidade de {{mes_referencia}}, no valor de {{valor}}, ainda está pendente. O prazo para pagamento termina em {{data_vencimento}}."

func (s *Server) handleWhatsAppReminder(w http.ResponseWriter, r *http.Request) {
	charge, err := s.store.ChargeByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if charge.Status != "pending" && charge.Status != "overdue" {
		writeError(w, http.StatusConflict, "só é possível lembrar cobranças pendentes ou vencidas")
		return
	}

	recipient, err := s.store.UserByID(r.Context(), charge.UserID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	phone, err := normalizeWhatsAppNumber(recipient.Phone)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	dueDate, err := time.Parse("2006-01-02", charge.DueDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "vencimento inválido na cobrança")
		return
	}
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	message := reminderMessage(settings["reminder_template"], recipient.Name, charge, dueDate)
	link := "https://wa.me/" + phone + "?text=" + url.QueryEscape(message)
	admin := currentUser(r)
	s.store.LogActivity(r.Context(), &admin.ID, "whatsapp_reminder_prepared",
		fmt.Sprintf("%s preparou lembrete de WhatsApp para %s", admin.Name, recipient.Name))

	writeJSON(w, http.StatusOK, map[string]string{
		"url":     link,
		"message": message,
	})
}

func reminderMessage(template, recipientName string, charge store.Charge, dueDate time.Time) string {
	if template == "" {
		template = defaultReminderTemplate
	}
	replacements := map[string]string{
		"{{nome}}":            recipientName,
		"{{mes_referencia}}":  charge.ReferenceMonth,
		"{{valor}}":           formatCentsBR(charge.AmountCents),
		"{{data_vencimento}}": dueDate.Format("02/01/2006"),
		"{{codigo_pix}}":      charge.PixPayload,
	}
	for placeholder, value := range replacements {
		template = strings.ReplaceAll(template, placeholder, value)
	}
	return template
}

func normalizeWhatsAppNumber(phone string) (string, error) {
	trimmed := strings.TrimSpace(phone)
	var digits strings.Builder
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	number := digits.String()
	if (strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "00")) && !strings.HasPrefix(number, "55") {
		return "", errors.New("informe um WhatsApp brasileiro com DDD")
	}
	switch len(number) {
	case 10, 11:
		return "55" + number, nil
	case 12, 13:
		if strings.HasPrefix(number, "55") {
			return number, nil
		}
	}
	return "", errors.New("informe um WhatsApp brasileiro com DDD")
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
