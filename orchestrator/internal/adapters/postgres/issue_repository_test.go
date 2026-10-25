package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// applyAndIssue replica la decisión de app: al quedar PAID se dispara la emisión.
func applyAndIssue(o *order.Order, ev app.PaymentEvent) (order.Event, []app.Command, error) {
	e, err := o.ApplyPayment(ev.Status, time.Now())
	if err != nil || o.Status != order.Paid {
		return e, nil, err
	}
	return e, []app.Command{{Type: app.CommandIssuePolicy, OrderID: o.ID}}, nil
}

func issueCommands(t *testing.T, repo *OrderRepository, id order.OrderID) int {
	t.Helper()
	return countWhere(t, repo.pool, "SELECT count(*) FROM outbox WHERE order_id = $1 AND type = 'issue_policy'", id)
}

// Regla crítica: el cambio a PAID y el comando issue_policy quedan en la misma transacción.
func TestRecordPaymentEvent_SavesTriggeredCommand(t *testing.T) {
	repo := NewOrderRepository(testPool(t))
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")

	if _, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), applyAndIssue); err != nil {
		t.Fatal(err)
	}

	if n := issueCommands(t, repo, o.ID); n != 1 {
		t.Fatalf("comandos issue_policy = %d, want 1", n)
	}
}

// El otro camino a PAID: el evento temprano se aplica al guardar PAYMENT_PENDING y también encola la emisión.
func TestMarkPaymentPending_EarlyApprovalSavesTriggeredCommand(t *testing.T) {
	repo := NewOrderRepository(testPool(t))
	o := quotedOrder(t, repo, "k")
	if _, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), applyAndIssue); err != nil {
		t.Fatal(err)
	}

	o.PaymentID = "pay-1"
	ev, _ := o.Transition(order.PaymentPending, "payment requested", time.Now())
	final, err := repo.MarkPaymentPending(context.Background(), o, ev, applyAndIssue)

	if err != nil || final.Status != order.Paid {
		t.Fatalf("status=%s err=%v", final.Status, err)
	}
	if n := issueCommands(t, repo, o.ID); n != 1 {
		t.Fatalf("comandos issue_policy = %d, want 1", n)
	}
}

// Si el comando no se puede guardar, tampoco queda el pago aplicado: no existe "PAID sin emisión encolada".
func TestRecordPaymentEvent_CommandFailureRollsBackPayment(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")
	broken := func(o *order.Order, ev app.PaymentEvent) (order.Event, []app.Command, error) {
		e, err := o.ApplyPayment(ev.Status, time.Now())
		return e, []app.Command{{Type: "unknown_command", OrderID: o.ID}}, err // lo rechaza el CHECK de la outbox
	}

	_, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), broken)

	if err == nil {
		t.Fatal("se esperaba error")
	}
	if got, _ := repo.Get(context.Background(), o.ID); got.Status != order.PaymentPending {
		t.Fatalf("status = %s; el pago no debía quedar aplicado", got.Status)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM payment_events WHERE payment_id = 'pay-1'"); n != 0 {
		t.Fatalf("eventos de pago = %d; el evento tampoco debía quedar", n)
	}
}

func TestSaveTransition_PersistsPolicyNumber(t *testing.T) {
	repo := NewOrderRepository(testPool(t))
	ctx := context.Background()
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")
	if _, err := repo.RecordPaymentEvent(ctx, paymentEvent("pay-1", order.PaymentApproved), applyAndIssue); err != nil {
		t.Fatal(err)
	}
	o, _ = repo.Get(ctx, o.ID)
	for _, step := range []struct {
		to     order.Status
		policy order.PolicyNumber
	}{{order.Issuing, ""}, {order.Issued, "POL-77"}} {
		o.PolicyNumber = step.policy
		ev, err := o.Transition(step.to, "test", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveTransition(ctx, o, ev); err != nil {
			t.Fatalf("save %s: %v", step.to, err)
		}
	}

	got, err := repo.Get(ctx, o.ID)

	if err != nil || got.Status != order.Issued || got.PolicyNumber != "POL-77" {
		t.Fatalf("orden = %s / %q err=%v", got.Status, got.PolicyNumber, err)
	}
}
