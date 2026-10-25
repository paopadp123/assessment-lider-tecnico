package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// Los decoradores de este archivo envuelven los puertos de los terceros y registran cada intento
// (tercero, operación, resultado, duración) para que la consola muestre qué respondió cada uno.
// No cambian el resultado: si el registro falla, solo queda un WARN.

// RecordingQuoter registra cada intento de cotización.
type RecordingQuoter struct {
	inner Quoter
	rec   CallRecorder
	log   *slog.Logger
	now   func() time.Time
}

// NewRecordingQuoter envuelve el cotizador.
func NewRecordingQuoter(inner Quoter, rec CallRecorder, log *slog.Logger) *RecordingQuoter {
	return &RecordingQuoter{inner: inner, rec: rec, log: log, now: time.Now}
}

// Quote cotiza y registra el intento.
func (q *RecordingQuoter) Quote(ctx context.Context, req QuoteRequest) (order.Quote, error) {
	return q.inner.Quote(ctx, req)
}

// RecordingPaymentGateway registra cada intento de crear la intención de pago.
type RecordingPaymentGateway struct {
	inner PaymentGateway
	rec   CallRecorder
	log   *slog.Logger
	now   func() time.Time
}

// NewRecordingPaymentGateway envuelve el PSP.
func NewRecordingPaymentGateway(inner PaymentGateway, rec CallRecorder, log *slog.Logger) *RecordingPaymentGateway {
	return &RecordingPaymentGateway{inner: inner, rec: rec, log: log, now: time.Now}
}

// CreatePayment pide el pago y registra el intento.
func (p *RecordingPaymentGateway) CreatePayment(ctx context.Context, req PaymentRequest) (order.PaymentID, error) {
	return p.inner.CreatePayment(ctx, req)
}

// RecordingPolicyIssuer registra cada emisión y cada consulta. Envuelve al breaker, así que también queda
// registrado cuando no se llamó porque el circuito estaba abierto.
type RecordingPolicyIssuer struct {
	inner PolicyIssuer
	rec   CallRecorder
	log   *slog.Logger
	now   func() time.Time
}

// NewRecordingPolicyIssuer envuelve la capa de emisión.
func NewRecordingPolicyIssuer(inner PolicyIssuer, rec CallRecorder, log *slog.Logger) *RecordingPolicyIssuer {
	return &RecordingPolicyIssuer{inner: inner, rec: rec, log: log, now: time.Now}
}

// Issue emite y registra el intento.
func (i *RecordingPolicyIssuer) Issue(ctx context.Context, req IssueRequest) (order.PolicyNumber, error) {
	return i.inner.Issue(ctx, req)
}

// FindByExternalRef consulta y registra el intento.
func (i *RecordingPolicyIssuer) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	return i.inner.FindByExternalRef(ctx, ref)
}

// maxCallDetail recorta el detalle guardado de cada llamada.
const maxCallDetail = 300

// classify traduce el error de un adaptador al resultado que ve la consola.
func classify(error) CallOutcome { return "" }
