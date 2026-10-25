package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// makeStuck deja la orden sin cambios desde hace 10 minutos.
func makeStuck(t *testing.T, pool *pgxpool.Pool, id order.OrderID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE orders SET updated_at = now() - interval '10 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func fiveMinutesAgo() time.Time { return time.Now().Add(-5 * time.Minute) }

func claimedIDs(stuck []app.StuckOrder) []string {
	ids := make([]string, 0, len(stuck))
	for _, s := range stuck {
		ids = append(ids, string(s.Order.ID))
	}
	sort.Strings(ids)
	return ids
}

func TestClaimStuck_TakesOnlyStuckOrdersInRecoverableStates(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	stuckQuoted := quotedOrder(t, repo, "k1")
	makeStuck(t, pool, stuckQuoted.ID)
	quotedOrder(t, repo, "k2") // reciente: no está atascada
	failed := quotedOrder(t, repo, "k3")
	failed.FailureReason = "PAYMENT_UNAVAILABLE"
	ev, _ := failed.Transition(order.Failed, "x", time.Now())
	if err := repo.SaveTransition(ctx, failed, ev); err != nil {
		t.Fatal(err)
	}
	makeStuck(t, pool, failed.ID) // terminal: nunca se reconcilia

	got, err := repo.ClaimStuck(ctx, fiveMinutesAgo(), 10)

	if err != nil || len(got) != 1 || got[0].Order.ID != stuckQuoted.ID || got[0].Order.Status != order.Quoted {
		t.Fatalf("tomadas = %v, err=%v", claimedIDs(got), err)
	}
	// Quedó marcada: dentro del mismo intervalo no se vuelve a tomar.
	if again, _ := repo.ClaimStuck(ctx, fiveMinutesAgo(), 10); len(again) != 0 {
		t.Fatalf("segunda pasada tomó %v", claimedIDs(again))
	}
}

// Regla crítica: con varias réplicas, dos reconciliadores nunca toman la misma orden (SKIP LOCKED).
func TestClaimStuck_ConcurrentReconcilersNeverShareAnOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	const total = 30
	for i := range total {
		o := quotedOrder(t, repo, fmt.Sprintf("k-%d", i))
		makeStuck(t, pool, o.ID)
	}

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				got, err := repo.ClaimStuck(context.Background(), fiveMinutesAgo(), 3)
				if err != nil {
					t.Error(err)
					return
				}
				if len(got) == 0 {
					return
				}
				mu.Lock()
				for _, id := range claimedIDs(got) {
					seen[id]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != total {
		t.Fatalf("órdenes tomadas = %d, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("la orden %s la tomaron %d reconciliadores", id, n)
		}
	}
}

func TestEnqueue_AddsCommandsWithoutChangingTheOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	o := quotedOrder(t, repo, "k")

	if err := repo.Enqueue(context.Background(), app.Command{Type: app.CommandRequestPayment, OrderID: o.ID}); err != nil {
		t.Fatal(err)
	}

	if n := countWhere(t, pool, "SELECT count(*) FROM outbox WHERE order_id = $1 AND type = 'request_payment'", o.ID); n != 1 {
		t.Fatalf("comandos = %d", n)
	}
	if got, _ := repo.Get(context.Background(), o.ID); got.Status != order.Quoted {
		t.Fatalf("status = %s", got.Status)
	}
}

// Las rondas sin póliza se cuentan, y cualquier cambio de estado (p. ej. un operador reintenta) las reinicia.
func TestRecordIssueMiss_CountsAndResetsOnTransition(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o := quotedOrder(t, repo, "k")
	for want := 1; want <= 2; want++ {
		if n, err := repo.RecordIssueMiss(ctx, o.ID); err != nil || n != want {
			t.Fatalf("ronda %d: n=%d err=%v", want, n, err)
		}
	}

	o.PaymentID = "pay-1"
	ev, _ := o.Transition(order.PaymentPending, "payment requested", time.Now())
	if _, err := repo.MarkPaymentPending(ctx, o, ev, applyDomain); err != nil {
		t.Fatal(err)
	}
	makeStuck(t, pool, o.ID)
	got, _ := repo.ClaimStuck(ctx, fiveMinutesAgo(), 10)

	if len(got) != 1 || got[0].IssueMisses != 0 {
		t.Fatalf("rondas tras el cambio de estado = %+v", got)
	}
	if _, err := repo.RecordIssueMiss(ctx, "4f1c2a7e-0000-4000-8000-000000000999"); !errors.Is(err, app.ErrOrderNotFound) {
		t.Fatalf("orden inexistente: err = %v", err)
	}
}

// La línea de tiempo guarda quién ejecutó cada acción; los cambios del sistema quedan sin actor.
func TestEvents_TimelineKeepsActorAndOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o := quotedOrder(t, repo, "k")
	o.FailureReason = "MARKED_FOR_REVERSAL"
	ev, _ := o.Transition(order.Failed, "manual", time.Now())
	ev.Actor = "ana.perez"
	if err := repo.SaveTransition(ctx, o, ev); err != nil {
		t.Fatal(err)
	}

	timeline, err := repo.ListEvents(ctx, o.ID)

	if err != nil || len(timeline) != 3 {
		t.Fatalf("eventos = %d, err=%v", len(timeline), err)
	}
	if timeline[0].To != order.Created || timeline[0].Actor != "" || timeline[2].To != order.Failed || timeline[2].Actor != "ana.perez" {
		t.Fatalf("línea de tiempo = %+v", timeline)
	}
	if !(timeline[0].ID < timeline[1].ID && timeline[1].ID < timeline[2].ID) || timeline[2].OrderID != o.ID {
		t.Fatalf("IDs = %d, %d, %d", timeline[0].ID, timeline[1].ID, timeline[2].ID)
	}
	if _, err := repo.ListEvents(ctx, "4f1c2a7e-0000-4000-8000-000000000999"); !errors.Is(err, app.ErrOrderNotFound) {
		t.Fatalf("orden inexistente: err = %v", err)
	}
}

// El flujo SSE lee por ID: entrega en orden, respeta el límite y reanuda desde cualquier punto.
func TestEventsAfter_PagesInOrderAcrossOrders(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	quotedOrder(t, repo, "k1")
	quotedOrder(t, repo, "k2") // 4 eventos en total, de dos órdenes

	latest, err := repo.LatestEventID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := repo.EventsAfter(ctx, 0, 100)
	page, _ := repo.EventsAfter(ctx, all[1].ID, 1)

	if len(all) != 4 || all[3].ID != latest {
		t.Fatalf("eventos = %d, último = %d / %d", len(all), all[len(all)-1].ID, latest)
	}
	if len(page) != 1 || page[0].ID != all[2].ID {
		t.Fatalf("página = %+v", page)
	}
	if empty, _ := repo.EventsAfter(ctx, latest, 10); len(empty) != 0 {
		t.Fatalf("después del último hay %d eventos", len(empty))
	}
}
