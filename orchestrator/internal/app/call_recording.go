package app

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// Los decoradores de este archivo envuelven los puertos de los terceros y registran cada intento
// (tercero, operación, resultado, duración) para que la consola muestre qué respondió cada uno.
// No cambian el resultado: si el registro falla, solo queda un WARN.

// maxCallDetail recorta el detalle guardado de cada llamada.
const maxCallDetail = 300

// recordTimeout limita cuánto puede esperar la compra por el registro de una llamada.
const recordTimeout = 2 * time.Second

// callLog arma y guarda el registro de un intento. Lo comparten los tres decoradores.
type callLog struct {
	rec CallRecorder
	log *slog.Logger
	now func() time.Time
}

// track devuelve la función que, al terminar el intento, lo registra con su resultado y su duración.
func (l callLog) track(ctx context.Context, id order.OrderID, party ThirdParty, op CallOperation) func(error) {
	started := l.now()
	return func(err error) {
		c := ThirdPartyCall{OrderID: id, Party: party, Operation: op, Outcome: classify(err),
			Duration: l.now().Sub(started), StartedAt: started.UTC()}
		if err != nil {
			c.Detail = truncate(err.Error(), maxCallDetail)
		}
		// El intento pudo terminar por su propio timeout: el registro usa un contexto aparte, con su límite.
		recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
		defer cancel()
		if recErr := l.rec.RecordCall(recCtx, c); recErr != nil {
			trace.Logger(ctx, l.log).Warn("third-party call not recorded", "order_id", id, "party", party,
				"operation", op, "error", recErr)
		}
	}
}

// RecordingQuoter registra cada intento de cotización.
type RecordingQuoter struct {
	inner Quoter
	callLog
}

// NewRecordingQuoter envuelve el cotizador.
func NewRecordingQuoter(inner Quoter, rec CallRecorder, log *slog.Logger) *RecordingQuoter {
	return &RecordingQuoter{inner: inner, callLog: callLog{rec: rec, log: log, now: time.Now}}
}

// Quote cotiza y registra el intento.
func (q *RecordingQuoter) Quote(ctx context.Context, req QuoteRequest) (order.Quote, error) {
	done := q.track(ctx, req.OrderID, PartyQuoter, OpQuote)
	quote, err := q.inner.Quote(ctx, req)
	done(err)
	return quote, err
}

// RecordingPaymentGateway registra cada intento de crear la intención de pago.
type RecordingPaymentGateway struct {
	inner PaymentGateway
	callLog
}

// NewRecordingPaymentGateway envuelve el PSP.
func NewRecordingPaymentGateway(inner PaymentGateway, rec CallRecorder, log *slog.Logger) *RecordingPaymentGateway {
	return &RecordingPaymentGateway{inner: inner, callLog: callLog{rec: rec, log: log, now: time.Now}}
}

// CreatePayment pide el pago y registra el intento.
func (p *RecordingPaymentGateway) CreatePayment(ctx context.Context, req PaymentRequest) (order.PaymentID, error) {
	done := p.track(ctx, req.OrderID, PartyPSP, OpCreatePayment)
	id, err := p.inner.CreatePayment(ctx, req)
	done(err)
	return id, err
}

// RecordingPolicyIssuer registra cada emisión y cada consulta. Envuelve al breaker, así que también queda
// registrado cuando no se llamó porque el circuito estaba abierto.
type RecordingPolicyIssuer struct {
	inner PolicyIssuer
	callLog
}

// NewRecordingPolicyIssuer envuelve la capa de emisión.
func NewRecordingPolicyIssuer(inner PolicyIssuer, rec CallRecorder, log *slog.Logger) *RecordingPolicyIssuer {
	return &RecordingPolicyIssuer{inner: inner, callLog: callLog{rec: rec, log: log, now: time.Now}}
}

// Issue emite y registra el intento.
func (i *RecordingPolicyIssuer) Issue(ctx context.Context, req IssueRequest) (order.PolicyNumber, error) {
	done := i.track(ctx, req.ExternalRef, PartyIssuer, OpIssue)
	n, err := i.inner.Issue(ctx, req)
	done(err)
	return n, err
}

// FindByExternalRef consulta y registra el intento.
func (i *RecordingPolicyIssuer) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	done := i.track(ctx, ref, PartyIssuer, OpFindPolicy)
	n, err := i.inner.FindByExternalRef(ctx, ref)
	done(err)
	return n, err
}

// classify traduce el error de un adaptador al resultado que ve la consola. El orden importa: un timeout
// llega envuelto en ErrIssuerUnavailable, y debe verse como TIMEOUT.
func classify(err error) CallOutcome {
	switch {
	case err == nil:
		return CallOK
	case errors.Is(err, ErrIssuerCircuitOpen):
		return CallCircuitOpen
	case errors.Is(err, ErrPolicyNotFound):
		return CallNotFound
	case errors.Is(err, ErrQuoteRejected), errors.Is(err, ErrPaymentRequestRejected), errors.Is(err, ErrIssueRejected):
		return CallRejected
	case errors.Is(err, context.DeadlineExceeded):
		return CallTimeout
	default:
		return CallUnavailable
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Recorta en un límite de carácter: cortar un carácter UTF-8 a la mitad lo rompe en la base.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
