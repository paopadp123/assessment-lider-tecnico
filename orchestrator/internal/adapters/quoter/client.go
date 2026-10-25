// Package quoter implementa el puerto app.Quoter contra el cotizador HTTP de la aseguradora.
package quoter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

const maxResponseBytes = 64 << 10

var (
	premiumFormat  = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)
	currencyFormat = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Client llama a POST /v1/quotes. Los timeouts los controla el contexto de cada intento.
type Client struct {
	baseURL string
	http    *http.Client
}

var _ app.Quoter = (*Client)(nil)

// NewClient crea el cliente del cotizador.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type quoteRequest struct {
	Plate       string `json:"plate"`
	VehicleYear int    `json:"vehicleYear"`
	CityCode    string `json:"cityCode"`
	DocumentID  string `json:"documentId"`
}

type quoteResponse struct {
	QuoteID    string      `json:"quoteId"`
	Premium    json.Number `json:"premium"`
	Currency   string      `json:"currency"`
	ValidUntil time.Time   `json:"validUntil"`
}

// Quote solicita la cotización. 5xx, 429, errores de red y respuestas inválidas son ErrQuoteUnavailable
// (se reintentan); el resto de 4xx es ErrQuoteRejected (reintentar no cambia el resultado).
func (c *Client) Quote(ctx context.Context, req app.QuoteRequest) (order.Quote, error) {
	// Campo por campo: OrderID es interno y no se le envía al cotizador.
	body, err := json.Marshal(quoteRequest{Plate: req.Plate, VehicleYear: req.VehicleYear, CityCode: req.CityCode, DocumentID: req.DocumentID})
	if err != nil {
		return order.Quote{}, fmt.Errorf("encode quote request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/quotes", bytes.NewReader(body))
	if err != nil {
		return order.Quote{}, fmt.Errorf("build quote request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if id := trace.ID(ctx); id != "" {
		httpReq.Header.Set(trace.Header, id) // el tercero puede cruzar sus logs con los de la compra
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return order.Quote{}, fmt.Errorf("%w: %w", app.ErrQuoteUnavailable, err)
	}
	defer func() {
		// Drenar el cuerpo permite que net/http reutilice la conexión keep-alive.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 500, resp.StatusCode == http.StatusTooManyRequests:
		return order.Quote{}, fmt.Errorf("%w: status %d", app.ErrQuoteUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		return order.Quote{}, fmt.Errorf("%w: status %d", app.ErrQuoteRejected, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return order.Quote{}, fmt.Errorf("%w: unexpected status %d", app.ErrQuoteUnavailable, resp.StatusCode)
	}

	var out quoteResponse
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return order.Quote{}, fmt.Errorf("%w: invalid response body: %w", app.ErrQuoteUnavailable, err)
	}
	if err := validate(out, time.Now()); err != nil {
		return order.Quote{}, fmt.Errorf("%w: %w", app.ErrQuoteUnavailable, err)
	}
	return order.Quote{
		ID:         out.QuoteID,
		Premium:    twoDecimals(out.Premium.String()),
		Currency:   out.Currency,
		ValidUntil: out.ValidUntil.UTC().Truncate(time.Microsecond),
	}, nil
}

// twoDecimals normaliza la prima a dos decimales ("850000.5" → "850000.50"), igual que numeric(14,2),
// para que la respuesta original y la repetida por idempotencia sean idénticas.
func twoDecimals(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	return whole + "." + (frac + "00")[:2]
}

// validate rechaza respuestas incompletas o una cotización ya vencida: con ella el cliente pagaría un precio caducado.
func validate(q quoteResponse, now time.Time) error {
	switch {
	case q.QuoteID == "":
		return fmt.Errorf("missing quoteId")
	case !premiumFormat.MatchString(q.Premium.String()):
		return fmt.Errorf("invalid premium %q", q.Premium)
	case !currencyFormat.MatchString(q.Currency):
		return fmt.Errorf("invalid currency %q", q.Currency)
	case q.ValidUntil.IsZero():
		return fmt.Errorf("missing validUntil")
	case !q.ValidUntil.After(now):
		return fmt.Errorf("quote expired at %s", q.ValidUntil.Format(time.RFC3339))
	}
	return nil
}
