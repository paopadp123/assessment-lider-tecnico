package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

func applyDomain(o *order.Order, ev app.PaymentEvent) (order.Event, []app.Command, error) {
	e, err := o.ApplyPayment(ev.Status, time.Now())
	return e, nil, err
}

// quotedOrder deja en la base una orden cotizada lista para pedir el pago.
func quotedOrder(t *testing.T, repo *OrderRepository, key string) order.Order {
	t.Helper()
	ctx := context.Background()
	o, _, err := repo.Create(ctx, app.IdempotencyKey{Key: key, RequestHash: "h"}, newOrder(t), createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	o.Quote = &order.Quote{ID: "q-1", Premium: "850000.00", Currency: "COP", ValidUntil: time.Now().Add(time.Hour)}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	if err := repo.SaveTransition(ctx, o, ev); err != nil {
		t.Fatal(err)
	}
	return o
}

func markPending(t *testing.T, repo *OrderRepository, o order.Order, pid order.PaymentID) order.Order {
	t.Helper()
	o.PaymentID = pid
	ev, _ := o.Transition(order.PaymentPending, "payment requested", time.Now())
	final, err := repo.MarkPaymentPending(context.Background(), o, ev, applyDomain)
	if err != nil {
		t.Fatalf("mark pending: %v", err)
	}
	return final
}

func paymentEvent(pid order.PaymentID, st order.PaymentStatus) app.PaymentEvent {
	return app.PaymentEvent{PaymentID: pid, Status: st, OccurredAt: time.Now().UTC()}
}

func countWhere(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRecordPaymentEvent_AppliesToPendingOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")

	res, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), applyDomain)

	if err != nil || res.Outcome != app.PaymentApplied || res.OrderID != o.ID {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got, _ := repo.Get(context.Background(), o.ID); got.Status != order.Paid || got.PaymentID != "pay-1" {
		t.Fatalf("orden = %s / %s", got.Status, got.PaymentID)
	}
}

// Regla crítica: un reenvío del PSP no tiene segundo efecto, garantizado por el índice único.
func TestRecordPaymentEvent_DuplicateHasNoEffect(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")
	ctx := context.Background()
	if _, err := repo.RecordPaymentEvent(ctx, paymentEvent("pay-1", order.PaymentApproved), applyDomain); err != nil {
		t.Fatal(err)
	}

	res, err := repo.RecordPaymentEvent(ctx, paymentEvent("pay-1", order.PaymentApproved), applyDomain)

	if err != nil || res.Outcome != app.PaymentDuplicate {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM order_events WHERE order_id = $1 AND to_status = 'PAID'", o.ID); n != 1 {
		t.Fatalf("eventos PAID = %d, want 1", n)
	}
}

// Caso exigido: APPROVED llega antes de que la orden quede en PAYMENT_PENDING.
func TestPaymentApprovedBeforePending_IsAppliedWhenPendingIsSaved(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := quotedOrder(t, repo, "k")

	res, err := repo.RecordPaymentEvent(context.Background(), paymentEvent("pay-1", order.PaymentApproved), applyDomain)
	if err != nil || res.Outcome != app.PaymentPendingOrder {
		t.Fatalf("webhook temprano: res=%+v err=%v", res, err)
	}

	final := markPending(t, repo, o, "pay-1")

	if final.Status != order.Paid {
		t.Fatalf("status = %s, want PAID", final.Status)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM payment_events WHERE applied_at IS NULL"); n != 0 {
		t.Fatalf("eventos sin aplicar = %d", n)
	}
}

func TestRecordPaymentEvent_ContradictoryEventIsAnomaly(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := markPending(t, repo, quotedOrder(t, repo, "k"), "pay-1")
	ctx := context.Background()
	if _, err := repo.RecordPaymentEvent(ctx, paymentEvent("pay-1", order.PaymentApproved), applyDomain); err != nil {
		t.Fatal(err)
	}

	res, err := repo.RecordPaymentEvent(ctx, paymentEvent("pay-1", order.PaymentRejected), applyDomain)

	if err != nil || res.Outcome != app.PaymentAnomaly {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got, _ := repo.Get(ctx, o.ID); got.Status != order.Paid {
		t.Fatalf("status = %s, la anomalía no debe cambiar la orden", got.Status)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM payment_events WHERE outcome = 'ANOMALY'"); n != 1 {
		t.Fatalf("anomalías registradas = %d, want 1", n)
	}
}

// Regla crítica: el webhook y el guardado de PAYMENT_PENDING compiten al mismo tiempo.
// Con cualquier intercalado, la orden termina PAID exactamente una vez y ningún evento queda sin aplicar.
func TestPaymentRace_WebhookAndPendingConcurrently(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	const rounds = 25

	for i := range rounds {
		pid := order.PaymentID(fmt.Sprintf("race-%d", i))
		o := quotedOrder(t, repo, fmt.Sprintf("race-key-%d", i))
		o.PaymentID = pid
		pendingEv, _ := o.Transition(order.PaymentPending, "payment requested", time.Now())

		var wg sync.WaitGroup
		var markErr, recordErr error
		wg.Go(func() {
			_, markErr = repo.MarkPaymentPending(context.Background(), o, pendingEv, applyDomain)
		})
		wg.Go(func() {
			_, recordErr = repo.RecordPaymentEvent(context.Background(), paymentEvent(pid, order.PaymentApproved), applyDomain)
		})
		wg.Wait()
		if markErr != nil || recordErr != nil {
			t.Fatalf("ronda %d: mark=%v record=%v", i, markErr, recordErr)
		}

		got, err := repo.Get(context.Background(), o.ID)
		if err != nil || got.Status != order.Paid {
			t.Fatalf("ronda %d: status=%s err=%v", i, got.Status, err)
		}
		if n := countWhere(t, pool, "SELECT count(*) FROM order_events WHERE order_id = $1 AND to_status = 'PAID'", o.ID); n != 1 {
			t.Fatalf("ronda %d: eventos PAID = %d", i, n)
		}
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM payment_events WHERE applied_at IS NULL"); n != 0 {
		t.Fatalf("eventos sin aplicar = %d", n)
	}
}
