package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// paymentIDFormat excluye "|", el separador del texto firmado: así ningún id puede desplazar
// los límites entre campos y reutilizar una firma legítima con otra lectura.
var paymentIDFormat = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// PaymentEventHandler es el caso de uso que procesa un resultado de pago verificado.
type PaymentEventHandler interface {
	Execute(ctx context.Context, ev app.PaymentEvent) (app.PaymentEventResult, error)
}

// Webhooks expone el endpoint que recibe los resultados de pago del PSP.
type Webhooks struct {
	handler  PaymentEventHandler
	verifier *SignatureVerifier
	log      *slog.Logger
}

// NewWebhooks crea el handler de webhooks.
func NewWebhooks(handler PaymentEventHandler, verifier *SignatureVerifier, log *slog.Logger) *Webhooks {
	return &Webhooks{handler: handler, verifier: verifier, log: log}
}

// Register registra la ruta en el mux.
func (h *Webhooks) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/payments", h.payments)
}

type paymentWebhook struct {
	PaymentID  string `json:"paymentId"`
	Status     string `json:"status"`
	OccurredAt string `json:"occurredAt"`
	Signature  string `json:"signature"`
}

type webhookResponse struct {
	Outcome app.PaymentOutcome `json:"outcome"`
}

// payments valida el formato (400), luego la firma (401) y solo entonces procesa el evento.
// Duplicados, eventos tempranos y anomalías responden 200 para que el PSP deje de reenviar;
// un error interno responde 500 para que el PSP reintente.
func (h *Webhooks) payments(w http.ResponseWriter, r *http.Request) {
	var body paymentWebhook
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	status, statusErr := order.ParsePaymentStatus(body.Status)
	occurredAt, timeErr := time.Parse(time.RFC3339Nano, body.OccurredAt)
	if !paymentIDFormat.MatchString(body.PaymentID) || statusErr != nil || timeErr != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid payment event"})
		return
	}
	// Firma ausente o inválida es un problema de autenticación (401), no de formato.
	if body.Signature == "" || !h.verifier.Verify(body.PaymentID, body.Status, body.OccurredAt, body.Signature) {
		h.log.WarnContext(r.Context(), "payment webhook with invalid signature", "payment_id", body.PaymentID)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid signature"})
		return
	}

	res, err := h.handler.Execute(r.Context(), app.PaymentEvent{
		PaymentID:  order.PaymentID(body.PaymentID),
		Status:     status,
		OccurredAt: occurredAt.UTC(),
	})
	if err != nil {
		h.log.ErrorContext(r.Context(), "payment webhook failed", "payment_id", body.PaymentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, webhookResponse{Outcome: res.Outcome})
}
