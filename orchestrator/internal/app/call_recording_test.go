package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// fakeCallRecorder guarda las llamadas en memoria; err simula una base caída.
type fakeCallRecorder struct {
	mu     sync.Mutex
	calls  []ThirdPartyCall
	ctxErr []error
	err    error
}

func (f *fakeCallRecorder) RecordCall(ctx context.Context, c ThirdPartyCall) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxErr = append(f.ctxErr, ctx.Err())
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, c)
	return nil
}

// stepClock avanza 250 ms en cada lectura: la duración registrada de un intento es exacta.
func stepClock() func() time.Time {
	t := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(250 * time.Millisecond)
		return t
	}
}

const callOrder = order.OrderID("4f1c2a7e-0000-4000-8000-0000000000aa")

func TestRecordingQuoter_RecordsEachAttemptWithOrderAndDuration(t *testing.T) {
	rec := &fakeCallRecorder{}
	q := NewRecordingQuoter(&fakeQuoter{errs: []error{fmt.Errorf("%w: status 500", ErrQuoteUnavailable)}}, rec, discardLogger())
	q.now = stepClock()

	_, err1 := q.Quote(context.Background(), QuoteRequest{OrderID: callOrder})
	_, err2 := q.Quote(context.Background(), QuoteRequest{OrderID: callOrder})

	if err1 == nil || err2 != nil {
		t.Fatalf("el decorador cambió el resultado: %v, %v", err1, err2)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("se registraron %d llamadas, se esperaban 2", len(rec.calls))
	}
	first, second := rec.calls[0], rec.calls[1]
	if first.OrderID != callOrder || first.Party != PartyQuoter || first.Operation != OpQuote {
		t.Errorf("identificación incorrecta: %+v", first)
	}
	if first.Outcome != CallUnavailable || !strings.Contains(first.Detail, "status 500") {
		t.Errorf("primer intento: %s %q, se esperaba UNAVAILABLE con el código", first.Outcome, first.Detail)
	}
	if second.Outcome != CallOK || second.Detail != "" {
		t.Errorf("segundo intento: %s %q, se esperaba OK sin detalle", second.Outcome, second.Detail)
	}
	if first.Duration != 250*time.Millisecond || first.StartedAt.IsZero() {
		t.Errorf("duración %v e inicio %v", first.Duration, first.StartedAt)
	}
}

func TestRecordingPaymentGateway_RecordsAttempt(t *testing.T) {
	rec := &fakeCallRecorder{}
	p := NewRecordingPaymentGateway(&fakePSP{errs: []error{fmt.Errorf("%w: status 400", ErrPaymentRequestRejected)}}, rec, discardLogger())

	_, err := p.CreatePayment(context.Background(), PaymentRequest{OrderID: callOrder})

	if !errors.Is(err, ErrPaymentRequestRejected) {
		t.Fatalf("err = %v", err)
	}
	if len(rec.calls) != 1 || rec.calls[0].Party != PartyPSP || rec.calls[0].Operation != OpCreatePayment ||
		rec.calls[0].Outcome != CallRejected || rec.calls[0].OrderID != callOrder {
		t.Fatalf("llamada registrada: %+v", rec.calls)
	}
}

func TestRecordingPolicyIssuer_RecordsIssueAndLookup(t *testing.T) {
	rec := &fakeCallRecorder{}
	inner := newFakeIssuer()
	i := NewRecordingPolicyIssuer(inner, rec, discardLogger())

	if _, err := i.FindByExternalRef(context.Background(), callOrder); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("consulta: %v", err)
	}
	if _, err := i.Issue(context.Background(), IssueRequest{ExternalRef: callOrder}); err != nil {
		t.Fatalf("emisión: %v", err)
	}

	want := []struct {
		op      CallOperation
		outcome CallOutcome
	}{{OpFindPolicy, CallNotFound}, {OpIssue, CallOK}}
	if len(rec.calls) != len(want) {
		t.Fatalf("llamadas: %+v", rec.calls)
	}
	for n, w := range want {
		c := rec.calls[n]
		if c.Party != PartyIssuer || c.Operation != w.op || c.Outcome != w.outcome || c.OrderID != callOrder {
			t.Errorf("llamada %d: %+v, se esperaba %s %s", n, c, w.op, w.outcome)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want CallOutcome
	}{
		{nil, CallOK},
		{fmt.Errorf("%w: code %q", ErrIssueRejected, "E-102"), CallRejected},
		{fmt.Errorf("%w: status 422", ErrQuoteRejected), CallRejected},
		{fmt.Errorf("%w: status 400", ErrPaymentRequestRejected), CallRejected},
		{fmt.Errorf("%w: unexpected status 503", ErrIssuerUnavailable), CallUnavailable},
		{fmt.Errorf("%w: %w", ErrIssuerUnavailable, context.DeadlineExceeded), CallTimeout},
		{fmt.Errorf("%w: breaker open", ErrIssuerCircuitOpen), CallCircuitOpen},
		{ErrPolicyNotFound, CallNotFound},
		{errors.New("algo inesperado"), CallUnavailable},
	}
	for _, c := range cases {
		if got := classify(c.err); got != c.want {
			t.Errorf("classify(%v) = %q, se esperaba %q", c.err, got, c.want)
		}
	}
}

// Si la base no guarda el registro, la compra sigue: el resultado del tercero no cambia y queda un WARN.
func TestRecording_FailureToRecordDoesNotChangeTheResult(t *testing.T) {
	var logs bytes.Buffer
	rec := &fakeCallRecorder{err: errors.New("db down")}
	q := NewRecordingQuoter(&fakeQuoter{}, rec, slog.New(slog.NewJSONHandler(&logs, nil)))

	got, err := q.Quote(context.Background(), QuoteRequest{OrderID: callOrder})

	if err != nil || got.ID != "q-1" {
		t.Fatalf("Quote = %+v, %v", got, err)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), "db down") {
		t.Errorf("se esperaba un WARN con la causa: %s", logs.String())
	}
}

// Un intento que venció su tiempo se registra igual: el registro no usa el contexto cancelado del intento.
func TestRecording_RecordsEvenWhenTheAttemptContextExpired(t *testing.T) {
	rec := &fakeCallRecorder{}
	i := NewRecordingPolicyIssuer(newFakeIssuer(), rec, discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = i.FindByExternalRef(ctx, callOrder)

	if len(rec.calls) != 1 || rec.ctxErr[0] != nil {
		t.Fatalf("llamadas %d, contexto del registro: %v", len(rec.calls), rec.ctxErr)
	}
}

// El detalle es el error del adaptador; se recorta para que un error enorme no llene la tabla.
func TestRecording_TruncatesLongDetail(t *testing.T) {
	rec := &fakeCallRecorder{}
	long := fmt.Errorf("%w: %s", ErrQuoteUnavailable, strings.Repeat("x", 1000))
	q := NewRecordingQuoter(&fakeQuoter{errs: []error{long}}, rec, discardLogger())

	_, _ = q.Quote(context.Background(), QuoteRequest{OrderID: callOrder})

	if len(rec.calls) != 1 {
		t.Fatalf("se registraron %d llamadas, se esperaba 1", len(rec.calls))
	}
	if n := len(rec.calls[0].Detail); n > maxCallDetail {
		t.Errorf("detalle de %d caracteres, máximo %d", n, maxCallDetail)
	}
}
