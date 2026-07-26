// Package mercadopago implements the small part of the Mercado Pago Orders API
// used by the application. It deliberately keeps the payment provider behind a
// narrow interface so the rest of the billing code is independent of its HTTP API.
package mercadopago

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ordersURL = "https://api.mercadopago.com/v1/orders"

// Client calls the Mercado Pago Orders API using a private access token.
type Client struct {
	accessToken string
	ordersURL   string
	httpClient  *http.Client
}

// APIError identifies a non-success response from Mercado Pago.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("mercado pago respondeu com HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("mercado pago respondeu com HTTP %d: %s", e.StatusCode, e.Message)
}

func New(accessToken string) *Client {
	return newClient(accessToken, ordersURL, &http.Client{Timeout: 15 * time.Second})
}

func newClient(accessToken, ordersURL string, httpClient *http.Client) *Client {
	return &Client{
		accessToken: strings.TrimSpace(accessToken),
		ordersURL:   strings.TrimRight(ordersURL, "/"),
		httpClient:  httpClient,
	}
}

func (c *Client) Enabled() bool {
	return c.accessToken != ""
}

// CreatePixOrderInput contains the local charge data that identifies a Pix order.
type CreatePixOrderInput struct {
	Amount             string
	ChargeID           string
	PayerEmail         string
	PayerFirstName     string
	ExpirationDuration string
	IdempotencyKey     string
}

// Order is the Mercado Pago representation relevant to a local charge.
type Order struct {
	ID                string `json:"id"`
	ExternalReference string `json:"external_reference"`
	Status            string `json:"status"`
	StatusDetail      string `json:"status_detail"`
	Transactions      struct {
		Payments []Payment `json:"payments"`
	} `json:"transactions"`
}

// Payment contains the Pix data returned by the Orders API.
type Payment struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	StatusDetail  string `json:"status_detail"`
	PaymentMethod struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		TicketURL    string `json:"ticket_url"`
		QRCode       string `json:"qr_code"`
		QRCodeBase64 string `json:"qr_code_base64"`
	} `json:"payment_method"`
}

func (c *Client) CreatePixOrder(ctx context.Context, in CreatePixOrderInput) (Order, error) {
	if !c.Enabled() {
		return Order{}, errors.New("mercado pago não está configurado")
	}
	if in.Amount == "" || in.ChargeID == "" || in.PayerEmail == "" || in.ExpirationDuration == "" || in.IdempotencyKey == "" {
		return Order{}, errors.New("dados incompletos para criar cobrança Pix")
	}

	body := struct {
		Type              string `json:"type"`
		TotalAmount       string `json:"total_amount"`
		ExternalReference string `json:"external_reference"`
		ProcessingMode    string `json:"processing_mode"`
		Transactions      struct {
			Payments []struct {
				Amount         string `json:"amount"`
				ExpirationTime string `json:"expiration_time"`
				PaymentMethod  struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"payment_method"`
			} `json:"payments"`
		} `json:"transactions"`
		Payer struct {
			Email     string `json:"email"`
			FirstName string `json:"first_name,omitempty"`
		} `json:"payer"`
	}{
		Type:              "online",
		TotalAmount:       in.Amount,
		ExternalReference: in.ChargeID,
		ProcessingMode:    "automatic",
	}
	payment := struct {
		Amount         string `json:"amount"`
		ExpirationTime string `json:"expiration_time"`
		PaymentMethod  struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"payment_method"`
	}{
		Amount:         in.Amount,
		ExpirationTime: in.ExpirationDuration,
	}
	payment.PaymentMethod.ID = "pix"
	payment.PaymentMethod.Type = "bank_transfer"
	body.Transactions.Payments = append(body.Transactions.Payments, payment)
	body.Payer.Email = in.PayerEmail
	body.Payer.FirstName = in.PayerFirstName

	var order Order
	if err := c.doJSON(ctx, http.MethodPost, c.ordersURL, in.IdempotencyKey, body, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) OrderByID(ctx context.Context, orderID string) (Order, error) {
	if !c.Enabled() {
		return Order{}, errors.New("mercado pago não está configurado")
	}
	if orderID == "" {
		return Order{}, errors.New("ID da order ausente")
	}

	var order Order
	if err := c.doJSON(ctx, http.MethodGet, c.ordersURL+"/"+orderID, "", nil, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) doJSON(ctx context.Context, method, url, idempotencyKey string, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("codificando requisição do Mercado Pago: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("criando requisição para Mercado Pago: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chamando Mercado Pago: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		response, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return fmt.Errorf("lendo erro do Mercado Pago: %w", err)
		}
		var providerError struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(response, &providerError)
		message := strings.TrimSpace(providerError.Message)
		if message == "" {
			message = strings.TrimSpace(providerError.Error)
		}
		return &APIError{StatusCode: resp.StatusCode, Message: message}
	}
	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("lendo resposta do Mercado Pago: %w", err)
	}
	return nil
}

// PixPayment returns the Pix transaction carried by an order.
func (o Order) PixPayment() (Payment, bool) {
	for _, payment := range o.Transactions.Payments {
		if payment.PaymentMethod.ID == "pix" {
			return payment, true
		}
	}
	return Payment{}, false
}

// IsApproved reports whether the provider has definitively credited the Pix.
func (o Order) IsApproved() bool {
	if o.StatusDetail == "accredited" {
		return true
	}
	payment, ok := o.PixPayment()
	return ok && payment.StatusDetail == "accredited"
}
