package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mercadopago/sdk-go/pkg/webhook"

	"futdarapaziada/api/internal/mercadopago"
	"futdarapaziada/api/internal/store"
)

const mercadoPagoSandboxPayerEmail = "test_user_br@testuser.com"

// handleCreatePixCharge returns an existing Pix or creates one for the logged
// in user. Creating on first access avoids creating external orders that no
// player will ever see, while the idempotency key prevents duplicates.
func (s *Server) handleCreatePixCharge(w http.ResponseWriter, r *http.Request) {
	charge, err := s.store.ChargeByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if charge.UserID != currentUser(r).ID {
		writeError(w, http.StatusNotFound, "cobrança não encontrada")
		return
	}
	if charge.Status != "pending" && charge.Status != "overdue" {
		writeError(w, http.StatusConflict, "esta cobrança não aceita mais pagamentos")
		return
	}
	if charge.PixPayload != "" {
		writeJSON(w, http.StatusOK, charge)
		return
	}
	if !s.mercadoPago.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "a integração com Mercado Pago ainda não está configurada")
		return
	}

	user, err := s.store.UserByID(r.Context(), charge.UserID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	payerEmail := user.Email
	payerFirstName := user.Name
	if s.cfg.MercadoPagoTestMode {
		payerEmail = mercadoPagoSandboxPayerEmail
		payerFirstName = "APRO"
	}
	expiration, err := pixExpiration(time.Now(), charge.DueDate)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	order, err := s.mercadoPago.CreatePixOrder(r.Context(), mercadopago.CreatePixOrderInput{
		Amount:             amountInReais(charge.AmountCents),
		ChargeID:           charge.ID,
		PayerEmail:         payerEmail,
		PayerFirstName:     payerFirstName,
		ExpirationDuration: expiration,
		IdempotencyKey:     "fut-app-charge-" + charge.ID,
	})
	if err != nil {
		log.Printf("criando Pix para cobrança %s: %v", charge.ID, err)
		writeError(w, http.StatusBadGateway, "não foi possível gerar o Pix agora; tente novamente")
		return
	}
	payment, ok := order.PixPayment()
	if !ok || payment.PaymentMethod.QRCode == "" || order.ID == "" {
		log.Printf("Mercado Pago retornou order Pix incompleta para cobrança %s", charge.ID)
		writeError(w, http.StatusBadGateway, "o Mercado Pago não retornou os dados do Pix")
		return
	}
	if err := s.store.SaveMercadoPagoOrder(r.Context(), charge.ID, order.ID, payment.ID,
		order.Status, order.StatusDetail, payment.PaymentMethod.QRCode,
		payment.PaymentMethod.TicketURL, payment.PaymentMethod.QRCodeBase64); err != nil {
		writeStoreError(w, err)
		return
	}

	charge, err = s.store.ChargeByID(r.Context(), charge.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, charge)
}

// handleMercadoPagoWebhook accepts only signed order notifications. The order
// is always fetched again from Mercado Pago before local payment state changes.
func (s *Server) handleMercadoPagoWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.mercadoPago.Enabled() || s.cfg.MercadoPagoWebhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "integração com Mercado Pago não configurada")
		return
	}

	orderID := r.URL.Query().Get("data.id")
	if err := webhook.ValidateSignature(
		r.Header.Get("x-signature"),
		r.Header.Get("x-request-id"),
		orderID,
		s.cfg.MercadoPagoWebhookSecret,
	); err != nil {
		log.Printf("webhook Mercado Pago rejeitado: %v", err)
		writeError(w, http.StatusUnauthorized, "assinatura de webhook inválida")
		return
	}
	if orderID == "" {
		writeError(w, http.StatusBadRequest, "notificação sem ID da order")
		return
	}

	order, err := s.mercadoPago.OrderByID(r.Context(), orderID)
	if err != nil {
		log.Printf("consultando order Mercado Pago %s: %v", orderID, err)
		writeError(w, http.StatusBadGateway, "não foi possível consultar a order")
		return
	}
	if order.ExternalReference == "" {
		log.Printf("webhook Mercado Pago ignorado: order %s sem referência externa", orderID)
		w.WriteHeader(http.StatusOK)
		return
	}

	charge, err := s.store.ChargeByID(r.Context(), order.ExternalReference)
	if errors.Is(err, store.ErrNotFound) {
		log.Printf("webhook Mercado Pago ignorado: cobrança %s não encontrada", order.ExternalReference)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	payment, ok := order.PixPayment()
	if !ok || charge.PixPayload == "" {
		log.Printf("webhook Mercado Pago ignorado: order %s não corresponde a um Pix local", orderID)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.store.UpdateMercadoPagoStatus(r.Context(), charge.ID, order.ID, payment.ID, order.Status, order.StatusDetail); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Printf("webhook Mercado Pago ignorado: order %s não corresponde à cobrança %s", order.ID, charge.ID)
			w.WriteHeader(http.StatusOK)
			return
		}
		writeStoreError(w, err)
		return
	}

	if !order.IsApproved() {
		w.WriteHeader(http.StatusOK)
		return
	}

	charge, changed, reactivated, err := s.store.MarkMercadoPagoPaid(r.Context(), charge.ID, order.ID, payment.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if changed {
		s.store.LogActivity(r.Context(), nil, "payment",
			fmt.Sprintf("%s pagou a mensalidade de %s via PIX", charge.UserName, charge.ReferenceMonth))
	}
	if reactivated {
		s.store.LogActivity(r.Context(), nil, "user_reactivated",
			fmt.Sprintf("%s foi reativado após confirmação do pagamento", charge.UserName))
	}
	w.WriteHeader(http.StatusOK)
}

func amountInReais(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func pixExpiration(now time.Time, dueDate string) (string, error) {
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return "", errors.New("vencimento inválido na cobrança")
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return "", fmt.Errorf("carregando fuso horário brasileiro: %w", err)
	}

	deadline := time.Date(due.Year(), due.Month(), due.Day(), 23, 59, 0, 0, location)
	duration := deadline.Sub(now.In(location))
	// A cobrança pode estar vencida, mas ainda deve poder ser quitada. Nesse caso,
	// o Pix recebe um novo prazo de 24 horas, dentro do intervalo aceito pelo
	// Mercado Pago. Para cobranças muito futuras, limitamos o prazo a 30 dias.
	if duration < 30*time.Minute {
		duration = 24 * time.Hour
	}
	if duration > 30*24*time.Hour {
		duration = 30 * 24 * time.Hour
	}

	minutes := int(duration.Round(time.Minute).Minutes())
	hours := minutes / 60
	minutes %= 60
	return fmt.Sprintf("PT%dH%dM", hours, minutes), nil
}
