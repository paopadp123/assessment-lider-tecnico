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

// pendingOrder deja en el repositorio una orden que espera el pago "pay-1".
func pendingOrder(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo, id := quotedInRepo(t)
	newRequestPayment(repo, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))
	if o, _ := repo.Get(context.Background(), id); o.Status != order.PaymentPending {
		t.Fatalf("preparación: status=%s", o.Status)
	}
	return repo, id
}

func approved() PaymentEvent {
	return PaymentEvent{PaymentID: "pay-1", Status: order.PaymentApproved, OccurredAt: time.Now()}
}

func TestWebhook_ApprovedMovesOrderToPaid(t *testing.T) {
	repo, id := pendingOrder(t)

	res, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), approved())

	if err != nil || res.Outcome != PaymentApplied || res.OrderID != id {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Paid {
		t.Fatalf("status = %s, want PAID", o.Status)
	}
}

func TestWebhook_RejectedMovesOrderToFailedWithReason(t *testing.T) {
	repo, id := pendingOrder(t)
	ev := approved()
	ev.Status = order.PaymentRejected

	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	if o, _ := repo.Get(context.Background(), id); o.Status != order.Failed || o.FailureReason != order.ReasonPaymentRejected {
		t.Fatalf("status=%s reason=%q", o.Status, o.FailureReason)
	}
}

// Regla crítica: el PSP reenvía el mismo evento; solo el primero tiene efecto.
func TestWebhook_DuplicateHasNoSecondEffect(t *testing.T) {
	repo, _ := pendingOrder(t)
	uc := NewHandlePaymentWebhook(repo, discardLogger())
	if _, err := uc.Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}
	before := len(repo.events)

	res, err := uc.Execute(context.Background(), approved())

	if err != nil || res.Outcome != PaymentDuplicate || len(repo.events) != before {
		t.Fatalf("res=%+v err=%v eventos %d→%d", res, err, before, len(repo.events))
	}
}

// Si la decisión de dominio falla por algo que no es un conflicto, no queda nada: ni el evento ni el cambio.
func TestWebhook_ApplyErrorLeavesNoTrace(t *testing.T) {
	repo, id := pendingOrder(t)
	events, commands := len(repo.events), len(repo.commands)
	failing := func(*order.Order, PaymentEvent) (order.Event, []Command, error) {
		return order.Event{}, nil, errors.New("boom")
	}

	_, err := repo.RecordPaymentEvent(context.Background(), approved(), failing)

	if err == nil {
		t.Fatal("se esperaba error")
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.PaymentPending || len(repo.events) != events || len(repo.payEvents) != 0 || len(repo.commands) != commands {
		t.Fatalf("quedó rastro: status=%s eventos %d→%d eventos de pago=%d", o.Status, events, len(repo.events), len(repo.payEvents))
	}
}

func TestWebhook_UnknownPaymentIsKeptForLater(t *testing.T) {
	res, err := NewHandlePaymentWebhook(newFakeRepo(), discardLogger()).Execute(context.Background(), approved())

	if err != nil || res.Outcome != PaymentPendingOrder {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// Un rechazo que llega después de que la orden quedó pagada se registra como anomalía y no cambia la orden.
func TestWebhook_ContradictoryEventIsAnomalyAndLoggedAsWarning(t *testing.T) {
	repo, id := pendingOrder(t)
	var logs bytes.Buffer
	uc := NewHandlePaymentWebhook(repo, slog.New(slog.NewJSONHandler(&logs, nil)))
	if _, err := uc.Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}
	rejected := approved()
	rejected.Status = order.PaymentRejected

	res, err := uc.Execute(context.Background(), rejected)

	if err != nil || res.Outcome != PaymentAnomaly {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Paid {
		t.Fatalf("status = %s, la anomalía no debe cambiar la orden", o.Status)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), `"payment_id":"pay-1"`) {
		t.Fatalf("falta el WARN de la anomalía: %s", logs.String())
	}
}

// commandsFor devuelve los comandos de la outbox de un tipo para una orden.
func commandsFor(repo *fakeRepo, id order.OrderID, typ CommandType) int {
	n := 0
	for _, c := range repo.commands {
		if c.OrderID == id && c.Type == typ {
			n++
		}
	}
	return n
}

// Regla crítica: al quedar PAID se encola la emisión en la misma operación que el cambio de estado.
func TestWebhook_ApprovedEnqueuesPolicyIssue(t *testing.T) {
	repo, id := pendingOrder(t)

	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}

	if n := commandsFor(repo, id, CommandIssuePolicy); n != 1 {
		t.Fatalf("comandos issue_policy = %d, want 1", n)
	}
}

func TestWebhook_RejectedDoesNotEnqueuePolicyIssue(t *testing.T) {
	repo, id := pendingOrder(t)
	rejected := approved()
	rejected.Status = order.PaymentRejected

	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), rejected); err != nil {
		t.Fatal(err)
	}

	if n := commandsFor(repo, id, CommandIssuePolicy); n != 0 {
		t.Fatalf("comandos issue_policy = %d; un pago rechazado no se emite", n)
	}
}

// Un webhook duplicado no encola una segunda emisión.
func TestWebhook_DuplicateApprovedEnqueuesIssueOnce(t *testing.T) {
	repo, id := pendingOrder(t)
	uc := NewHandlePaymentWebhook(repo, discardLogger())
	for range 2 {
		if _, err := uc.Execute(context.Background(), approved()); err != nil {
			t.Fatal(err)
		}
	}

	if n := commandsFor(repo, id, CommandIssuePolicy); n != 1 {
		t.Fatalf("comandos issue_policy = %d, want 1", n)
	}
}

// El otro camino a PAID: el APPROVED llegó antes de guardar PAYMENT_PENDING y se aplica al guardarlo.
func TestEarlyApproved_EnqueuesPolicyIssueWhenAppliedLater(t *testing.T) {
	repo, id := quotedInRepo(t)
	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}

	newRequestPayment(repo, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if o, _ := repo.Get(context.Background(), id); o.Status != order.Paid {
		t.Fatalf("status = %s, want PAID", o.Status)
	}
	if n := commandsFor(repo, id, CommandIssuePolicy); n != 1 {
		t.Fatalf("comandos issue_policy = %d, want 1", n)
	}
}
