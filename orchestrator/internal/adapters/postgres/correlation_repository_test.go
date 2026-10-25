package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// El correlation-id se guarda con la compra y todo comando de esa orden lo hereda en la outbox, sin
// importar quién lo encole (creación, webhook, reconciliador u operador).
func TestCorrelationID_StoredWithOrderAndInheritedByCommands(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o := newOrder(t)
	o.CorrelationID = "corr-1"
	o, _, err := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "h"}, o, createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	o.Quote = &order.Quote{ID: "q-1", Premium: "1.00", Currency: "COP", ValidUntil: time.Now().Add(time.Hour)}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	if err := repo.SaveTransition(ctx, o, ev, app.Command{Type: app.CommandRequestPayment, OrderID: o.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Enqueue(ctx, app.Command{Type: app.CommandRequestPayment, OrderID: o.ID}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(ctx, o.ID)
	if err != nil || got.CorrelationID != "corr-1" {
		t.Fatalf("orden: correlation-id = %q, err=%v", got.CorrelationID, err)
	}
	var seen []string
	if _, err := repo.PublishPending(ctx, 10, func(_ context.Context, m app.OutboxMessage) error {
		seen = append(seen, m.CorrelationID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "corr-1" || seen[1] != "corr-1" {
		t.Fatalf("comandos publicados con correlation-id %v", seen)
	}
}

// El webhook no trae el correlation-id: el repositorio lo devuelve desde la orden.
func TestRecordPaymentEvent_ReturnsOrderCorrelationID(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := newOrder(t)
	o.CorrelationID = "corr-1"
	o, _, err := repo.Create(context.Background(), app.IdempotencyKey{Key: "k", RequestHash: "h"}, o, createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	o.Quote = &order.Quote{ID: "q-1", Premium: "1.00", Currency: "COP", ValidUntil: time.Now().Add(time.Hour)}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	if err := repo.SaveTransition(context.Background(), o, ev); err != nil {
		t.Fatal(err)
	}
	markPending(t, repo, o, "pay-1")

	res, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), applyDomain)

	if err != nil || res.CorrelationID != "corr-1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// Un comando para una orden que no existe es un error, no un comando que desaparece en silencio.
func TestEnqueue_UnknownOrderIsAnError(t *testing.T) {
	repo := NewOrderRepository(testPool(t))

	err := repo.Enqueue(context.Background(), app.Command{Type: app.CommandIssuePolicy, OrderID: "4f1c2a7e-0000-4000-8000-000000000999"})

	if !errors.Is(err, app.ErrOrderNotFound) {
		t.Fatalf("err = %v", err)
	}
}
