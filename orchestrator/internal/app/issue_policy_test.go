package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const testDeferDelay = 30 * time.Second

// paidInRepo deja una orden pagada, como la deja el webhook APPROVED.
func paidInRepo(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo, id := pendingOrder(t)
	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Paid {
		t.Fatalf("preparación: status=%s", o.Status)
	}
	return repo, id
}

func newIssuePolicy(repo *fakeRepo, issuer PolicyIssuer, logs *bytes.Buffer) *IssuePolicy {
	return NewIssuePolicy(repo, repo, issuer, slog.New(slog.NewJSONHandler(logs, nil)), testDeferDelay)
}

func orderIn(t *testing.T, repo *fakeRepo, id order.OrderID) order.Order {
	t.Helper()
	o, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestIssuePolicy_SuccessMovesToIssuedWithPolicyNumber(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()

	res := newIssuePolicy(repo, issuer, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Reason)
	}
	if o := orderIn(t, repo, id); o.Status != order.Issued || o.PolicyNumber != "POL-1" {
		t.Fatalf("orden = %s / %q", o.Status, o.PolicyNumber)
	}
	if issuer.issueCalls != 1 || issuer.findCalls != 0 {
		t.Fatalf("llamadas: issue=%d find=%d; una orden recién pagada no necesita consulta previa", issuer.issueCalls, issuer.findCalls)
	}
	got := issuer.got[0]
	if got.ExternalRef != id || got.QuoteID != "q-1" || got.Premium != "850000.00" || got.Currency != "COP" ||
		got.Plate.Raw() == "" || got.DocumentID.Raw() == "" || got.VehicleYear == 0 || got.CityCode == "" {
		t.Fatalf("solicitud de emisión = %+v", got)
	}
	// La línea de tiempo muestra el paso por ISSUING y la emisión.
	last := repo.events[len(repo.events)-2:]
	if last[0].To != order.Issuing || last[1].To != order.Issued {
		t.Fatalf("eventos finales = %+v", last)
	}
}

// Regla crítica: pago aprobado + emisión rechazada en definitiva → REFUND_REQUIRED, sin reintentar.
func TestIssuePolicy_BusinessRejectionRequiresRefund(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{ErrIssueRejected}

	res := newIssuePolicy(repo, issuer, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone {
		t.Fatalf("outcome = %s; un rechazo de negocio no se reintenta", res.Outcome)
	}
	if o := orderIn(t, repo, id); o.Status != order.RefundRequired || o.FailureReason != ReasonIssueRejected {
		t.Fatalf("orden = %s / %q", o.Status, o.FailureReason)
	}
}

func TestIssuePolicy_TransientFailureIsRetriedAndOrderStaysIssuing(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{ErrIssuerUnavailable}

	res := newIssuePolicy(repo, issuer, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandRetry {
		t.Fatalf("outcome = %s, want RETRY", res.Outcome)
	}
	if o := orderIn(t, repo, id); o.Status != order.Issuing {
		t.Fatalf("status = %s, want ISSUING", o.Status)
	}
}

// Regla crítica del enunciado: un timeout no autoriza a reemitir. El emisor sí emitió; el reintento
// consulta por externalRef, encuentra la póliza y la orden queda ISSUED con una sola emisión.
func TestIssuePolicy_TimeoutButIssuedIsFoundBeforeReissuing(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{context.DeadlineExceeded}
	issuer.issuedOnTimeout = true
	uc := newIssuePolicy(repo, issuer, &bytes.Buffer{})

	if res := uc.Execute(context.Background(), delivery(id, 1)); res.Outcome != CommandRetry {
		t.Fatalf("primer intento: outcome = %s, want RETRY", res.Outcome)
	}
	res := uc.Execute(context.Background(), delivery(id, 2))

	if res.Outcome != CommandDone {
		t.Fatalf("segundo intento: outcome = %s (%s)", res.Outcome, res.Reason)
	}
	if o := orderIn(t, repo, id); o.Status != order.Issued || o.PolicyNumber != "POL-1" {
		t.Fatalf("orden = %s / %q", o.Status, o.PolicyNumber)
	}
	if issuer.issueCalls != 1 || issuer.emissions != 1 {
		t.Fatalf("issue=%d emisiones=%d; el reintento no debía volver a emitir", issuer.issueCalls, issuer.emissions)
	}
}

// Si el proceso murió después de guardar ISSUING y antes de llamar, la reentrega consulta, no encuentra
// la póliza y emite.
func TestIssuePolicy_RedeliveryInIssuingWithoutPolicyIssues(t *testing.T) {
	repo, id := paidInRepo(t)
	if _, err := transition(context.Background(), repo, time.Now, orderIn(t, repo, id), order.Issuing, "issuance started"); err != nil {
		t.Fatal(err)
	}
	issuer := newFakeIssuer()

	res := newIssuePolicy(repo, issuer, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone || orderIn(t, repo, id).Status != order.Issued {
		t.Fatalf("outcome = %s, status = %s", res.Outcome, orderIn(t, repo, id).Status)
	}
	if issuer.findCalls != 1 || issuer.issueCalls != 1 {
		t.Fatalf("llamadas: find=%d issue=%d; debía consultar antes de emitir", issuer.findCalls, issuer.issueCalls)
	}
}

// Si la consulta falla, no se emite a ciegas: se reintenta más tarde.
func TestIssuePolicy_FailedLookupDoesNotIssueBlindly(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{context.DeadlineExceeded}
	issuer.findErrs = []error{ErrIssuerUnavailable}
	uc := newIssuePolicy(repo, issuer, &bytes.Buffer{})
	uc.Execute(context.Background(), delivery(id, 1))

	res := uc.Execute(context.Background(), delivery(id, 2))

	if res.Outcome != CommandRetry {
		t.Fatalf("outcome = %s, want RETRY", res.Outcome)
	}
	if issuer.issueCalls != 1 {
		t.Fatalf("issue=%d; sin confirmar con la consulta no se vuelve a emitir", issuer.issueCalls)
	}
}

// Regla crítica: con el breaker abierto la compra no se pierde. Queda en ISSUING y el comando se
// reprograma sin consumir intentos, incluso en el último intento (no va a la DLQ).
func TestIssuePolicy_CircuitOpenDefersWithoutConsumingAttempts(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{ErrIssuerCircuitOpen}
	var logs bytes.Buffer
	uc := newIssuePolicy(repo, issuer, &logs)

	res := uc.Execute(context.Background(), delivery(id, uc.MaxAttempts()))

	if res.Outcome != CommandDeferred || res.Delay != testDeferDelay {
		t.Fatalf("resultado = %+v, want DEFERRED con espera %v", res, testDeferDelay)
	}
	if o := orderIn(t, repo, id); o.Status != order.Issuing {
		t.Fatalf("status = %s, want ISSUING", o.Status)
	}
	if len(repo.deadLetters) != 0 {
		t.Fatal("un breaker abierto no debe mandar la orden a la DLQ")
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), ReasonIssuerUnavailable) {
		t.Fatalf("falta el WARN observable de emisor no disponible: %s", logs.String())
	}
}

// Regla crítica: agotados los reintentos, el comando va a la DLQ y la orden SIGUE en ISSUING. Un timeout no
// prueba que la póliza no se emitió: la decide la reconciliación (HU-05), no se marca REFUND_REQUIRED.
func TestIssuePolicy_ExhaustedGoesToDeadLetterAndStaysIssuing(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	uc := newIssuePolicy(repo, issuer, &bytes.Buffer{})
	issuer.issueErrs = []error{ErrIssuerUnavailable}
	uc.Execute(context.Background(), delivery(id, 1))
	issuer.findErrs = []error{ErrIssuerUnavailable}

	res := uc.Execute(context.Background(), delivery(id, uc.MaxAttempts()))

	if res.Outcome != CommandDeadLetter || res.Reason != ReasonIssuerUnavailable {
		t.Fatalf("resultado = %+v", res)
	}
	if o := orderIn(t, repo, id); o.Status != order.Issuing {
		t.Fatalf("status = %s, want ISSUING", o.Status)
	}
	if len(repo.deadLetters) != 1 || repo.deadLetters[0].Type != CommandIssuePolicy || repo.deadLetters[0].Reason != ReasonIssuerUnavailable {
		t.Fatalf("dead letters = %+v", repo.deadLetters)
	}
}

func TestIssuePolicy_AlreadyIssuedIsNoOp(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	uc := newIssuePolicy(repo, issuer, &bytes.Buffer{})
	uc.Execute(context.Background(), delivery(id, 1))
	calls := issuer.issueCalls + issuer.findCalls

	res := uc.Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone || orderIn(t, repo, id).Status != order.Issued || issuer.issueCalls+issuer.findCalls != calls {
		t.Fatalf("outcome = %s, llamadas %d→%d", res.Outcome, calls, issuer.issueCalls+issuer.findCalls)
	}
}

func TestIssuePolicy_UnknownOrderGoesToDeadLetter(t *testing.T) {
	repo := newFakeRepo()

	res := newIssuePolicy(repo, newFakeIssuer(), &bytes.Buffer{}).Execute(context.Background(), delivery("missing", 1))

	if res.Outcome != CommandDeadLetter || res.Reason != ReasonOrderNotFound {
		t.Fatalf("resultado = %+v", res)
	}
}

// Ningún dato sensible en claro en los logs, ni siquiera en los errores.
func TestIssuePolicy_LogsDoNotExposePlateOrDocument(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{ErrIssueRejected}
	var logs bytes.Buffer

	newIssuePolicy(repo, issuer, &logs).Execute(context.Background(), delivery(id, 1))

	o := orderIn(t, repo, id)
	if logs.Len() == 0 || strings.Contains(logs.String(), o.Plate.Raw()) || strings.Contains(logs.String(), o.DocumentID.Raw()) {
		t.Fatalf("logs vacíos o con datos sensibles: %s", logs.String())
	}
}

// Un comando de emisión para una orden que no está pagada no emite y queda en WARN para operaciones.
func TestIssuePolicy_UnpaidOrderIsIgnoredWithWarning(t *testing.T) {
	repo, id := pendingOrder(t)
	issuer := newFakeIssuer()
	var logs bytes.Buffer

	res := newIssuePolicy(repo, issuer, &logs).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone || issuer.issueCalls+issuer.findCalls != 0 {
		t.Fatalf("outcome = %s, llamadas = %d", res.Outcome, issuer.issueCalls+issuer.findCalls)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Fatalf("falta el WARN: %s", logs.String())
	}
}
