package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

type fakePaymentHandler struct {
	res    app.PaymentEventResult
	err    error
	called bool
	got    app.PaymentEvent
}

func (f *fakePaymentHandler) Execute(_ context.Context, ev app.PaymentEvent) (app.PaymentEventResult, error) {
	f.called, f.got = true, ev
	return f.res, f.err
}

func webhookBody(t *testing.T, v *SignatureVerifier, paymentID, status, occurredAt string) string {
	t.Helper()
	return fmt.Sprintf(`{"paymentId":%q,"status":%q,"occurredAt":%q,"signature":%q}`,
		paymentID, status, occurredAt, v.Sign(paymentID, status, occurredAt))
}

func postWebhook(t *testing.T, h *fakePaymentHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	v, _ := NewSignatureVerifier(testSecret)
	mux := http.NewServeMux()
	NewWebhooks(h, v, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/payments", strings.NewReader(body)))
	return rec
}

func TestPaymentWebhook(t *testing.T) {
	v, _ := NewSignatureVerifier(testSecret)
	const at = "2026-10-24T10:00:00.123Z"
	valid := webhookBody(t, v, "pay-1", "APPROVED", at)
	tampered := strings.Replace(valid, `"status":"APPROVED"`, `"status":"REJECTED"`, 1)

	tests := []struct {
		name        string
		body        string
		handler     *fakePaymentHandler
		wantStatus  int
		wantInBody  string
		wantInvoked bool
	}{
		{"JSON inválido", `{`, &fakePaymentHandler{}, http.StatusBadRequest, "invalid JSON", false},
		{"sin paymentId", webhookBody(t, v, "", "APPROVED", at), &fakePaymentHandler{}, http.StatusBadRequest, "invalid payment event", false},
		{"paymentId con el separador de la firma", webhookBody(t, v, "pay-1|APPROVED", "APPROVED", at), &fakePaymentHandler{}, http.StatusBadRequest, "invalid payment event", false},
		{"estado desconocido", webhookBody(t, v, "pay-1", "PENDING", at), &fakePaymentHandler{}, http.StatusBadRequest, "invalid payment event", false},
		{"fecha inválida", webhookBody(t, v, "pay-1", "APPROVED", "ayer"), &fakePaymentHandler{}, http.StatusBadRequest, "invalid payment event", false},
		{"sin firma", `{"paymentId":"pay-1","status":"APPROVED","occurredAt":"` + at + `"}`, &fakePaymentHandler{}, http.StatusUnauthorized, "invalid signature", false},
		{"firma de otro contenido", tampered, &fakePaymentHandler{}, http.StatusUnauthorized, "invalid signature", false},
		{"aplicado", valid, &fakePaymentHandler{res: app.PaymentEventResult{Outcome: app.PaymentApplied}}, http.StatusOK, `"outcome":"APPLIED"`, true},
		{"duplicado responde 200", valid, &fakePaymentHandler{res: app.PaymentEventResult{Outcome: app.PaymentDuplicate}}, http.StatusOK, `"outcome":"DUPLICATE"`, true},
		{"anomalía responde 200", valid, &fakePaymentHandler{res: app.PaymentEventResult{Outcome: app.PaymentAnomaly}}, http.StatusOK, `"outcome":"ANOMALY"`, true},
		{"error interno pide reintento", valid, &fakePaymentHandler{err: errors.New("db down")}, http.StatusInternalServerError, `"internal error"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postWebhook(t, tt.handler, tt.body)
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Fatalf("status=%d body=%s, want %d con %s", rec.Code, rec.Body, tt.wantStatus, tt.wantInBody)
			}
			if tt.handler.called != tt.wantInvoked {
				t.Fatalf("caso de uso invocado = %v, want %v", tt.handler.called, tt.wantInvoked)
			}
		})
	}
}

func TestPaymentWebhook_PassesVerifiedEvent(t *testing.T) {
	v, _ := NewSignatureVerifier(testSecret)
	h := &fakePaymentHandler{res: app.PaymentEventResult{Outcome: app.PaymentApplied}}

	postWebhook(t, h, webhookBody(t, v, "pay-1", "REJECTED", "2026-10-24T05:00:00-05:00"))

	if h.got.PaymentID != "pay-1" || h.got.Status != order.PaymentRejected || h.got.OccurredAt.Hour() != 10 {
		t.Fatalf("evento recibido = %+v", h.got)
	}
}
