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

// pendingOrder deja en el repositorio una orden que espera el pago "pay-1".
func pendingOrder(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo := newFakeRepo()
	res, err := newUseCase(repo, &fakeQuoter{}, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)
	if err != nil || res.Order.Status != order.PaymentPending {
		t.Fatalf("preparación: status=%s err=%v", res.Order.Status, err)
	}
	return repo, res.Order.ID
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
