// Package psp implementa el puerto app.PaymentGateway contra la pasarela de pagos HTTP.
package psp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const maxResponseBytes = 64 << 10

// Client llama a POST /v1/payments. Los timeouts los controla el contexto de cada intento.
type Client struct {
	baseURL string
	http    *http.Client
}

var _ app.PaymentGateway = (*Client)(nil)

// NewClient crea el cliente del PSP.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type paymentRequest struct {
	QuoteID     string      `json:"quoteId"`
	Amount      json.Number `json:"amount"`
	Currency    string      `json:"currency"`
	CustomerRef string      `json:"customerRef"`
}

type paymentResponse struct {
	PaymentID string `json:"paymentId"`
	Status    string `json:"status"`
}

// CreatePayment crea la intención de pago. Envía Idempotency-Key = orderId para que un reintento
// tras un timeout no cree un segundo cobro. Supuesto documentado: el PSP respeta esa clave
// (ampliación del contrato del Anexo B; el simulador la implementa). 5xx, 429, errores de red y respuestas inválidas son
// ErrPaymentUnavailable (se reintentan); el resto de 4xx es ErrPaymentRequestRejected.
func (c *Client) CreatePayment(ctx context.Context, req app.PaymentRequest) (order.PaymentID, error) {
	body, err := json.Marshal(paymentRequest{
		QuoteID:     req.QuoteID,
		Amount:      json.Number(req.Amount),
		Currency:    req.Currency,
		CustomerRef: string(req.OrderID),
	})
	if err != nil {
		return "", fmt.Errorf("encode payment request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/payments", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build payment request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", string(req.OrderID))

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrPaymentUnavailable, err)
	}
	defer func() {
		// Drenar el cuerpo permite que net/http reutilice la conexión keep-alive.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 500, resp.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("%w: status %d", app.ErrPaymentUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		return "", fmt.Errorf("%w: status %d", app.ErrPaymentRequestRejected, resp.StatusCode)
	case resp.StatusCode != http.StatusAccepted:
		return "", fmt.Errorf("%w: unexpected status %d", app.ErrPaymentUnavailable, resp.StatusCode)
	}

	var out paymentResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: invalid response body: %w", app.ErrPaymentUnavailable, err)
	}
	if out.PaymentID == "" || out.Status != "PENDING" {
		return "", fmt.Errorf("%w: unexpected payment response (id %q, status %q)", app.ErrPaymentUnavailable, out.PaymentID, out.Status)
	}
	return order.PaymentID(out.PaymentID), nil
}
