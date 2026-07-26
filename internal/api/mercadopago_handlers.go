package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
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
		updatedCharge, err := s.refreshMercadoPagoCharge(r.Context(), charge)
		if err != nil {
			log.Printf("reconciliando Pix da cobranÃ§a %s: %v", charge.ID, err)
		} else {
			charge = updatedCharge
		}
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
	idempotencyKey, err := mercadoPagoIdempotencyKey()
	if err != nil {
		log.Printf("gerando chave de idempotência Mercado Pago: %v", err)
		writeError(w, http.StatusInternalServerError, "não foi possível preparar o Pix")
		return
	}

	order, err := s.mercadoPago.CreatePixOrder(r.Context(), mercadopago.CreatePixOrderInput{
		Amount:             amountInReais(charge.AmountCents),
		ChargeID:           charge.ID,
		PayerEmail:         payerEmail,
		PayerFirstName:     payerFirstName,
		ExpirationDuration: expiration,
		IdempotencyKey:     idempotencyKey,
	})
	if err != nil {
		log.Printf("criando Pix para cobrança %s: %v", charge.ID, err)
		var providerError *mercadopago.APIError
		if s.cfg.MercadoPagoTestMode && errors.As(err, &providerError) {
			message := providerError.Message
			if message == "" {
				message = fmt.Sprintf("HTTP %d", providerError.StatusCode)
			}
			writeError(w, http.StatusBadGateway, "Mercado Pago sandbox recusou o Pix: "+message)
			return
		}
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
		mercadoPagoWebhookDataID(orderID),
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

// refreshMercadoPagoCharge reconciles a Pix previously generated by this API.
// It is a fallback for a delayed or temporarily rejected webhook.
func (s *Server) refreshMercadoPagoCharge(ctx context.Context, charge store.Charge) (store.Charge, error) {
	if charge.ProviderOrderID == "" || !s.mercadoPago.Enabled() {
		return charge, nil
	}

	order, err := s.mercadoPago.OrderByID(ctx, charge.ProviderOrderID)
	if err != nil {
		return charge, err
	}
	if order.ID != charge.ProviderOrderID || order.ExternalReference != charge.ID {
		return charge, errors.New("order retornada pelo Mercado Pago nÃ£o corresponde Ã  cobranÃ§a")
	}
	payment, ok := order.PixPayment()
	if !ok {
		return charge, errors.New("order retornada pelo Mercado Pago nÃ£o contÃ©m pagamento Pix")
	}
	if err := s.store.UpdateMercadoPagoStatus(ctx, charge.ID, order.ID, payment.ID, order.Status, order.StatusDetail); err != nil {
		return charge, err
	}
	if !order.IsApproved() {
		return s.store.ChargeByID(ctx, charge.ID)
	}

	updatedCharge, changed, reactivated, err := s.store.MarkMercadoPagoPaid(ctx, charge.ID, order.ID, payment.ID)
	if err != nil {
		return charge, err
	}
	if changed {
		s.store.LogActivity(ctx, nil, "payment",
			fmt.Sprintf("%s pagou a mensalidade de %s via PIX", updatedCharge.UserName, updatedCharge.ReferenceMonth))
	}
	if reactivated {
		s.store.LogActivity(ctx, nil, "user_reactivated",
			fmt.Sprintf("%s foi reativado apÃ³s confirmaÃ§Ã£o do pagamento", updatedCharge.UserName))
	}
	return updatedCharge, nil
}

func amountInReais(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// mercadoPagoWebhookDataID normalizes an alphanumeric order ID for the
// Mercado Pago signature manifest. The provider requires this field in lower
// case, while Orders API IDs are returned in upper case.
func mercadoPagoWebhookDataID(orderID string) string {
	return strings.ToLower(orderID)
}

func mercadoPagoIdempotencyKey() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
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
