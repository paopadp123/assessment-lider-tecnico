package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// quotedWithCommand deja una orden en QUOTED con su comando request_payment en la outbox.
func quotedWithCommand(t *testing.T, repo *OrderRepository, key string) order.Order {
	t.Helper()
	o, _, err := repo.Create(context.Background(), app.IdempotencyKey{Key: key, RequestHash: "h"}, newOrder(t), createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	o.Quote = &order.Quote{ID: "q-1", Premium: "850000.00", Currency: "COP", ValidUntil: time.Now().Add(time.Hour)}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	if err := repo.SaveTransition(context.Background(), o, ev, app.Command{Type: app.CommandRequestPayment, OrderID: o.ID}); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestSaveTransition_WritesOutboxCommandInSameTransaction(t *testing.T) {
	pool := testPool(t)
	o := quotedWithCommand(t, NewOrderRepository(pool), "k")

	if n := countWhere(t, pool, "SELECT count(*) FROM outbox WHERE order_id = $1 AND type = 'request_payment' AND published_at IS NULL", o.ID); n != 1 {
		t.Fatalf("comandos en la outbox = %d, want 1", n)
	}
}

// Regla crítica del patrón outbox: estado, evento y comando son una sola unidad. El comando es lo último
// que se escribe; si falla, el UPDATE de la orden y el evento ya escritos deben revertirse.
func TestSaveTransition_OutboxFailureRollsBackStateAndEvent(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o, _, err := repo.Create(context.Background(), app.IdempotencyKey{Key: "k", RequestHash: "h"}, newOrder(t), createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())

	err = repo.SaveTransition(context.Background(), o, ev, app.Command{Type: "BOGUS", OrderID: o.ID}) // viola el CHECK de outbox
	if err == nil {
		t.Fatal("se esperaba error al escribir la outbox")
	}

	if got, _ := repo.Get(context.Background(), o.ID); got.Status != order.Created {
		t.Fatalf("status = %s: el cambio de estado debió revertirse", got.Status)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM order_events WHERE order_id = $1", o.ID); n != 1 {
		t.Fatalf("eventos = %d: solo debe quedar el de creación", n)
	}
	if n := countWhere(t, pool, "SELECT count(*) FROM outbox"); n != 0 {
		t.Fatalf("comandos en la outbox = %d", n)
	}
}

func TestPublishPending_MarksConfirmedAndKeepsFailed(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ok := quotedWithCommand(t, repo, "k1")
	bad := quotedWithCommand(t, repo, "k2")
	ctx := context.Background()

	n, err := repo.PublishPending(ctx, 10, func(_ context.Context, m app.OutboxMessage) error {
		if m.OrderID == bad.ID {
			return errors.New("broker did not confirm")
		}
		return nil
	})

	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if c := countWhere(t, pool, "SELECT count(*) FROM outbox WHERE order_id = $1 AND published_at IS NOT NULL", ok.ID); c != 1 {
		t.Fatal("el confirmado no quedó marcado")
	}
	if c := countWhere(t, pool, "SELECT count(*) FROM outbox WHERE order_id = $1 AND published_at IS NULL AND attempts = 1 AND last_error IS NOT NULL", bad.ID); c != 1 {
		t.Fatal("el fallido debía quedar pendiente con su intento y su error")
	}

	if n, _ := repo.PublishPending(ctx, 10, func(context.Context, app.OutboxMessage) error { return nil }); n != 1 {
		t.Fatalf("segundo ciclo publicó %d, want 1", n)
	}
}

// Dos despachadores en paralelo: SKIP LOCKED hace que cada mensaje se publique una sola vez.
func TestPublishPending_ConcurrentDispatchersDoNotPublishTwice(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	const messages = 30
	for i := range messages {
		quotedWithCommand(t, repo, fmt.Sprintf("k-%d", i))
	}

	var (
		mu   sync.Mutex
		seen = map[string]int{}
		wg   sync.WaitGroup
	)
	publish := func(_ context.Context, m app.OutboxMessage) error {
		time.Sleep(5 * time.Millisecond) // mantiene el lock mientras el otro despachador intenta
		mu.Lock()
		defer mu.Unlock()
		seen[m.MessageID]++
		return nil
	}
	for range 2 {
		wg.Go(func() {
			for {
				n, err := repo.PublishPending(context.Background(), 5, publish)
				if err != nil {
					t.Errorf("publish: %v", err)
					return
				}
				if n == 0 {
					return
				}
			}
		})
	}
	wg.Wait()

	if len(seen) != messages {
		t.Fatalf("mensajes publicados = %d, want %d", len(seen), messages)
	}
	for id, times := range seen {
		if times != 1 {
			t.Fatalf("mensaje %s publicado %d veces", id, times)
		}
	}
}

func TestRecordDeadLetter_IsIdempotent(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	dl := app.DeadLetter{MessageID: "msg-1", Type: app.CommandRequestPayment, OrderID: "o-1", Attempts: 4, Reason: "PAYMENT_UNAVAILABLE"}

	for range 2 {
		if err := repo.RecordDeadLetter(context.Background(), dl); err != nil {
			t.Fatal(err)
		}
	}

	if n := countWhere(t, pool, "SELECT count(*) FROM dead_letters"); n != 1 {
		t.Fatalf("dead letters = %d, want 1", n)
	}
}
