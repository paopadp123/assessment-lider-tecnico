package quoter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

var req = app.QuoteRequest{Plate: "ABC123", VehicleYear: 2020, CityCode: "11001", DocumentID: "1020304050"}

func server(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got quoteRequest
		if r.Method != http.MethodPost || r.URL.Path != "/v1/quotes" || json.NewDecoder(r.Body).Decode(&got) != nil || got.Plate != "ABC123" {
			t.Errorf("solicitud inesperada: %s %s %+v", r.Method, r.URL.Path, got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, srv.Client())
}

func TestQuote_Success(t *testing.T) {
	c := server(t, http.StatusOK, `{"quoteId":"q-1","premium":850000.5,"currency":"COP","validUntil":"2099-11-01T00:00:00Z"}`)

	q, err := c.Quote(context.Background(), req)

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if q.ID != "q-1" || q.Premium != "850000.50" || q.Currency != "COP" || q.ValidUntil.Location() != time.UTC {
		t.Fatalf("cotización = %+v", q)
	}
}

// El OrderID solo sirve para registrar la llamada: no se le envía al cotizador.
func TestQuote_DoesNotSendOrderID(t *testing.T) {
	var fields map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Errorf("cuerpo: %v", err)
		}
		_, _ = w.Write([]byte(`{"quoteId":"q-1","premium":1,"currency":"COP","validUntil":"2099-11-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)
	withOrder := req
	withOrder.OrderID = "4f1c2a7e-0000-4000-8000-000000000001"

	if _, err := NewClient(srv.URL, srv.Client()).Quote(context.Background(), withOrder); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 || fields["orderId"] != nil || fields["OrderID"] != nil {
		t.Errorf("campos enviados = %v, se esperaban solo plate, vehicleYear, cityCode y documentId", fields)
	}
}

func TestQuote_ErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"500 es transitorio", 500, `{}`, app.ErrQuoteUnavailable},
		{"503 es transitorio", 503, ``, app.ErrQuoteUnavailable},
		{"429 es transitorio", 429, ``, app.ErrQuoteUnavailable},
		{"400 es rechazo", 400, `{"error":"bad plate"}`, app.ErrQuoteRejected},
		{"cuerpo no JSON", 200, `<html>`, app.ErrQuoteUnavailable},
		{"sin quoteId", 200, `{"premium":1,"currency":"COP","validUntil":"2026-11-01T00:00:00Z"}`, app.ErrQuoteUnavailable},
		{"prima negativa", 200, `{"quoteId":"q","premium":-1,"currency":"COP","validUntil":"2026-11-01T00:00:00Z"}`, app.ErrQuoteUnavailable},
		{"moneda inválida", 200, `{"quoteId":"q","premium":1,"currency":"pesos","validUntil":"2099-11-01T00:00:00Z"}`, app.ErrQuoteUnavailable},
		{"cotización vencida", 200, `{"quoteId":"q","premium":1,"currency":"COP","validUntil":"2020-01-01T00:00:00Z"}`, app.ErrQuoteUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server(t, tt.status, tt.body).Quote(context.Background(), req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestQuote_TimeoutIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := NewClient(srv.URL, srv.Client()).Quote(ctx, req)

	if !errors.Is(err, app.ErrQuoteUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrQuoteUnavailable causado por DeadlineExceeded", err)
	}
}
