package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const stuckAfter = 5 * time.Minute

func newTestReconciler(repo *fakeRepo, quoter Quoter, issuer PolicyIssuer, logs *bytes.Buffer) *Reconciler {
	r := NewReconciler(repo, repo, quoter, issuer, slog.New(slog.NewJSONHandler(logs, nil)), stuckAfter)
	r.sleep = func(context.Context, time.Duration) error { return nil }
	return r
}

// stuck deja la orden sin avanzar desde hace más del umbral.
func stuck(repo *fakeRepo, id order.OrderID) { repo.age(id, stuckAfter+time.Minute) }

// nextRound simula que pasó otro intervalo: la orden vuelve a ser elegible para el reconciliador.
func nextRound(repo *fakeRepo) { repo.reconciled = map[order.OrderID]time.Time{} }

func reconcileOnce(t *testing.T, r *Reconciler) int {
	t.Helper()
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return n
}

// createdInRepo deja una orden en CREATED, como queda si el proceso murió antes de guardar la cotización.
func createdInRepo(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo := newFakeRepo()
	o, err := order.NewOrder(validInput)
	if err != nil {
		t.Fatal(err)
	}
	o.CreatedAt, o.UpdatedAt = time.Now(), time.Now()
	saved, _, err := repo.Create(context.Background(), IdempotencyKey{Key: "k", RequestHash: "h"}, o,
		order.Event{To: order.Created, Reason: "order created", OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return repo, saved.ID
}

func issuingInRepo(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo, id := paidInRepo(t)
	if _, err := transition(context.Background(), repo, time.Now, orderIn(t, repo, id), order.Issuing, "policy issuance started"); err != nil {
		t.Fatal(err)
	}
	return repo, id
}

func TestReconcile_RecentOrdersAreNotTouched(t *testing.T) {
	repo, id := quotedInRepo(t)
	before := commandsFor(repo, id, CommandRequestPayment)

	n := reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{}))

	if n != 0 || commandsFor(repo, id, CommandRequestPayment) != before {
		t.Fatalf("revisadas=%d; una orden reciente no está atascada", n)
	}
}

// Pendiente de HU-01: si el proceso murió después de crear la orden, se vuelve a cotizar (no hay dinero en juego).
func TestReconcile_StuckCreatedIsQuotedAgain(t *testing.T) {
	repo, id := createdInRepo(t)
	stuck(repo, id)

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{}))

	if o := orderIn(t, repo, id); o.Status != order.Quoted || o.Quote == nil {
		t.Fatalf("orden = %s", o.Status)
	}
	if n := commandsFor(repo, id, CommandRequestPayment); n != 1 {
		t.Fatalf("comandos request_payment = %d, want 1", n)
	}
}

func TestReconcile_StuckCreatedFailsWhenQuoteIsRejected(t *testing.T) {
	repo, id := createdInRepo(t)
	stuck(repo, id)

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{errs: []error{ErrQuoteRejected}}, newFakeIssuer(), &bytes.Buffer{}))

	if o := orderIn(t, repo, id); o.Status != order.Failed || o.FailureReason != ReasonQuoteRejected {
		t.Fatalf("orden = %s / %s", o.Status, o.FailureReason)
	}
}

// Pendiente de HU-02: un cobro que el PSP aceptó sin que se guardara PAYMENT_PENDING se recupera
// reencolando request_payment (misma Idempotency-Key ⇒ mismo paymentId, sin doble cobro).
func TestReconcile_StuckQuotedReenqueuesPaymentRequest(t *testing.T) {
	repo, id := quotedInRepo(t)
	stuck(repo, id)

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{}))

	if n := commandsFor(repo, id, CommandRequestPayment); n != 2 {
		t.Fatalf("comandos request_payment = %d, want 2 (el original y el reencolado)", n)
	}
	if o := orderIn(t, repo, id); o.Status != order.Quoted {
		t.Fatalf("status = %s; reencolar no cambia el estado", o.Status)
	}
}

// El PSP no tiene consulta: una orden esperando el webhook solo se hace visible.
func TestReconcile_StuckPaymentPendingIsOnlyReported(t *testing.T) {
	repo, id := pendingOrder(t)
	stuck(repo, id)
	commands := len(repo.commands)
	var logs bytes.Buffer

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &logs))

	if o := orderIn(t, repo, id); o.Status != order.PaymentPending || len(repo.commands) != commands {
		t.Fatalf("status = %s, comandos %d→%d; no debía cambiar nada", o.Status, commands, len(repo.commands))
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), string(id)) {
		t.Fatalf("falta el WARN de orden atascada: %s", logs.String())
	}
}

func TestReconcile_StuckPaidReenqueuesIssue(t *testing.T) {
	repo, id := paidInRepo(t)
	stuck(repo, id)

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{}))

	if n := commandsFor(repo, id, CommandIssuePolicy); n != 2 {
		t.Fatalf("comandos issue_policy = %d, want 2", n)
	}
}

// Regla crítica del enunciado: antes de reintentar se consulta al emisor. Si la póliza existe, se registra.
func TestReconcile_StuckIssuingWithPolicyIsMarkedIssued(t *testing.T) {
	repo, id := issuingInRepo(t)
	stuck(repo, id)
	issuer := newFakeIssuer()
	issuer.policies[id] = "POL-9"

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, issuer, &bytes.Buffer{}))

	if o := orderIn(t, repo, id); o.Status != order.Issued || o.PolicyNumber != "POL-9" {
		t.Fatalf("orden = %s / %q", o.Status, o.PolicyNumber)
	}
	if issuer.issueCalls != 0 {
		t.Fatalf("issue=%d; el reconciliador nunca emite directamente", issuer.issueCalls)
	}
}

func TestReconcile_StuckIssuingWithoutPolicyReenqueuesAndCountsTheRound(t *testing.T) {
	repo, id := issuingInRepo(t)
	stuck(repo, id)
	issued := commandsFor(repo, id, CommandIssuePolicy)

	reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{}))

	if o := orderIn(t, repo, id); o.Status != order.Issuing {
		t.Fatalf("status = %s, want ISSUING", o.Status)
	}
	if commandsFor(repo, id, CommandIssuePolicy) != issued+1 || repo.issueMisses[id] != 1 {
		t.Fatalf("comandos %d→%d, rondas fallidas = %d", issued, commandsFor(repo, id, CommandIssuePolicy), repo.issueMisses[id])
	}
}

// Regla crítica (decisión del usuario): a la 3.ª ronda en que la consulta confirma que la póliza no
// existe, la emisión falló en definitiva → REFUND_REQUIRED, sin reembolso automático.
func TestReconcile_ThirdMissRequiresRefund(t *testing.T) {
	repo, id := issuingInRepo(t)
	r := newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{})
	issued := commandsFor(repo, id, CommandIssuePolicy)

	for round := 1; round <= 3; round++ {
		nextRound(repo)
		stuck(repo, id)
		reconcileOnce(t, r)
	}

	if o := orderIn(t, repo, id); o.Status != order.RefundRequired || o.FailureReason != ReasonIssueFailedDefinitively {
		t.Fatalf("orden = %s / %q", o.Status, o.FailureReason)
	}
	if n := commandsFor(repo, id, CommandIssuePolicy) - issued; n != 2 {
		t.Fatalf("reencolados = %d, want 2 (rondas 1 y 2)", n)
	}
}

// Sin respuesta confiable del emisor no se cuenta la ronda ni se decide nada.
func TestReconcile_IssuerUnavailableDoesNotCountTheRound(t *testing.T) {
	for _, lookupErr := range []error{ErrIssuerCircuitOpen, ErrIssuerUnavailable} {
		repo, id := issuingInRepo(t)
		stuck(repo, id)
		issuer := newFakeIssuer()
		issuer.findErrs = []error{lookupErr}
		issued := commandsFor(repo, id, CommandIssuePolicy)

		reconcileOnce(t, newTestReconciler(repo, &fakeQuoter{}, issuer, &bytes.Buffer{}))

		if repo.issueMisses[id] != 0 || commandsFor(repo, id, CommandIssuePolicy) != issued || orderIn(t, repo, id).Status != order.Issuing {
			t.Fatalf("%v: rondas=%d comandos %d→%d", lookupErr, repo.issueMisses[id], issued, commandsFor(repo, id, CommandIssuePolicy))
		}
	}
}

// Una orden ya revisada no se vuelve a tomar hasta que pase otro intervalo.
func TestReconcile_ClaimedOrderIsNotTakenTwiceInTheSameInterval(t *testing.T) {
	repo, id := paidInRepo(t)
	stuck(repo, id)
	r := newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{})
	reconcileOnce(t, r)

	if n := reconcileOnce(t, r); n != 0 {
		t.Fatalf("revisadas = %d en la segunda pasada, want 0", n)
	}
}

// partialStore devuelve una orden tomada junto con un error de lectura de otra.
type partialStore struct {
	*fakeRepo
	err error
}

func (p partialStore) ClaimStuck(ctx context.Context, before time.Time, limit int) ([]StuckOrder, error) {
	got, _ := p.fakeRepo.ClaimStuck(ctx, before, limit)
	return got, p.err
}

// Si una orden no se pudo leer, las demás se reconcilian igual y el error se informa.
func TestReconcile_PartialClaimStillProcessesTheRest(t *testing.T) {
	repo, id := paidInRepo(t)
	stuck(repo, id)
	r := newTestReconciler(repo, &fakeQuoter{}, newFakeIssuer(), &bytes.Buffer{})
	r.store = partialStore{fakeRepo: repo, err: errors.New("load stuck order x: boom")}

	n, err := r.RunOnce(context.Background())

	if n != 1 || err == nil || commandsFor(repo, id, CommandIssuePolicy) != 2 {
		t.Fatalf("revisadas=%d err=%v comandos=%d", n, err, commandsFor(repo, id, CommandIssuePolicy))
	}
}
