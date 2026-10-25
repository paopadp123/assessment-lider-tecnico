package psp

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

var req = app.PaymentRequest{OrderID: "4f1c2a7e-0000-4000-8000-000000000001", QuoteID: "q-1", Amount: "850000.00", Currency: "COP"}

func server(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments" || json.NewDecoder(r.Body).Decode(&got) != nil {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") != string(req.OrderID) {
			t.Errorf("Idempotency-Key = %q, want el orderId", r.Header.Get("Idempotency-Key"))
		}
		if got["quoteId"] != "q-1" || got["amount"] != 850000.0 || got["currency"] != "COP" || got["customerRef"] != string(req.OrderID) {
			t.Errorf("cuerpo inesperado: %v", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, srv.Client())
}

func TestCreatePayment_Accepted(t *testing.T) {
	id, err := server(t, http.StatusAccepted, `{"paymentId":"pay-1","status":"PENDING"}`).CreatePayment(context.Background(), req)

	if err != nil || id != "pay-1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestCreatePayment_ErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"500 es transitorio", 500, ``, app.ErrPaymentUnavailable},
		{"429 es transitorio", 429, ``, app.ErrPaymentUnavailable},
		{"400 es rechazo", 400, `{"error":"invalid amount"}`, app.ErrPaymentRequestRejected},
		{"200 en vez de 202", 200, `{"paymentId":"pay-1","status":"PENDING"}`, app.ErrPaymentUnavailable},
		{"cuerpo no JSON", 202, `<html>`, app.ErrPaymentUnavailable},
		{"sin paymentId", 202, `{"status":"PENDING"}`, app.ErrPaymentUnavailable},
		{"estado distinto de PENDING", 202, `{"paymentId":"pay-1","status":"APPROVED"}`, app.ErrPaymentUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server(t, tt.status, tt.body).CreatePayment(context.Background(), req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreatePayment_TimeoutIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := NewClient(srv.URL, srv.Client()).CreatePayment(ctx, req)

	if !errors.Is(err, app.ErrPaymentUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrPaymentUnavailable causado por DeadlineExceeded", err)
	}
}
