package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"futdarapaziada/api/internal/busdays"
	"futdarapaziada/api/internal/notify"
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

// reminderData carrega a cobrança, o destinatário e monta telefone e mensagem
// do lembrete. Em caso de erro já escreve a resposta HTTP e retorna ok=false.
func (s *Server) reminderData(w http.ResponseWriter, r *http.Request) (charge store.Charge, recipient store.User, phone, message string, ok bool) {
	charge, err := s.store.ChargeByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return charge, recipient, "", "", false
	}
	if charge.Status != "pending" && charge.Status != "overdue" {
		writeError(w, http.StatusConflict, "só é possível lembrar cobranças pendentes ou vencidas")
		return charge, recipient, "", "", false
	}

	recipient, err = s.store.UserByID(r.Context(), charge.UserID)
	if err != nil {
		writeStoreError(w, err)
		return charge, recipient, "", "", false
	}
	phone, err = notify.NormalizeNumber(recipient.Phone)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return charge, recipient, "", "", false
	}

	dueDate, err := time.Parse("2006-01-02", charge.DueDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "vencimento inválido na cobrança")
		return charge, recipient, "", "", false
	}
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return charge, recipient, "", "", false
	}

	return charge, recipient, phone, reminderMessage(settings["reminder_template"], recipient.Name, charge, dueDate), true
}

func (s *Server) handleWhatsAppReminder(w http.ResponseWriter, r *http.Request) {
	charge, _, phone, message, ok := s.reminderData(w, r)
	if !ok {
		return
	}

	link := "https://wa.me/" + phone + "?text=" + url.QueryEscape(message)
	admin := currentUser(r)
	s.store.LogActivity(r.Context(), &admin.ID, "whatsapp_reminder_prepared",
		fmt.Sprintf("%s preparou lembrete de WhatsApp para %s", admin.Name, charge.UserName))

	writeJSON(w, http.StatusOK, map[string]string{
		"url":     link,
		"message": message,
	})
}

// handleWhatsAppSend dispara o lembrete direto pelo WhatsApp (Evolution Go),
// sem depender do wa.me/WhatsApp Web, e registra o envio em notifications.
func (s *Server) handleWhatsAppSend(w http.ResponseWriter, r *http.Request) {
	charge, recipient, phone, message, ok := s.reminderData(w, r)
	if !ok {
		return
	}

	admin := currentUser(r)
	notificationID, err := s.store.ScheduleNotification(r.Context(), recipient.ID, &charge.ID, phone, message, time.Now())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	providerID, err := s.sender.Send(phone, message)
	if err != nil {
		_ = s.store.MarkNotificationFailed(r.Context(), notificationID, err.Error())
		s.store.LogActivity(r.Context(), &admin.ID, "whatsapp_reminder_failed",
			fmt.Sprintf("falha no lembrete de WhatsApp para %s: %v", recipient.Name, err))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("falha ao enviar WhatsApp: %v", err))
		return
	}

	_ = s.store.MarkNotificationSent(r.Context(), notificationID, providerID)
	s.store.LogActivity(r.Context(), &admin.ID, "whatsapp_reminder_sent",
		fmt.Sprintf("%s enviou lembrete de WhatsApp para %s", admin.Name, recipient.Name))

	writeJSON(w, http.StatusOK, map[string]string{
		"message":             message,
		"provider_message_id": providerID,
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
