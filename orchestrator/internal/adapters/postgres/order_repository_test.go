package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// testPool conecta a la base de pruebas. Sin TEST_DATABASE_URL se omite en local,
// pero en CI falla: las pruebas de integración no pueden saltarse en silencio.
// Como vacía las tablas, se niega a correr sobre una base cuyo nombre no termine en _test.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL es obligatoria en CI")
		}
		t.Skip("TEST_DATABASE_URL no definida: se omiten las pruebas de integración")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if db := pool.Config().ConnConfig.Database; !strings.HasSuffix(db, "_test") {
		t.Fatalf("la base %q no termina en _test: las pruebas vacían tablas y no se ejecutan sobre ella", db)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE order_events, idempotency_keys, orders"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

func newOrder(t *testing.T) order.Order {
	t.Helper()
	o, err := order.NewOrder(order.NewOrderInput{Plate: "ABC123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	o.CreatedAt, o.UpdatedAt = now, now
	return o
}

func createdEvent() order.Event {
	return order.Event{To: order.Created, Reason: "order created", OccurredAt: time.Now()}
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Regla crítica: con solicitudes concurrentes y la misma clave se crea una sola orden.
func TestCreate_ConcurrentSameKeyCreatesSingleOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	key := app.IdempotencyKey{Key: "concurrent-key", RequestHash: "hash-a"}

	const workers = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
		ids     = map[order.OrderID]bool{}
	)
	o := newOrder(t) // fuera de las goroutines: t.Fatal solo puede llamarse desde la goroutine de la prueba
	for range workers {
		wg.Go(func() {
			o, isNew, err := repo.Create(context.Background(), key, o, createdEvent())
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids[o.ID] = true
			if isNew {
				created++
			}
		})
	}
	wg.Wait()

	if created != 1 || len(ids) != 1 {
		t.Fatalf("created=%d ids distintos=%d, want 1 y 1", created, len(ids))
	}
	if n := countRows(t, pool, "orders"); n != 1 {
		t.Fatalf("órdenes en base = %d, want 1", n)
	}
}

func TestCreate_SameKeyDifferentRequestIsConflict(t *testing.T) {
	repo := NewOrderRepository(testPool(t))
	ctx := context.Background()
	if _, _, err := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "a"}, newOrder(t), createdEvent()); err != nil {
		t.Fatal(err)
	}

	_, _, err := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "b"}, newOrder(t), createdEvent())

	if !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatalf("err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestSaveTransition_PersistsStateQuoteAndEvent(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o, _, err := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "a"}, newOrder(t), createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	validUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	o.Quote = &order.Quote{ID: "q-1", Premium: "850000.50", Currency: "COP", ValidUntil: validUntil}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())

	if err := repo.SaveTransition(ctx, o, ev); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != order.Quoted || got.Quote == nil || got.Quote.Premium != "850000.50" || !got.Quote.ValidUntil.Equal(validUntil) {
		t.Fatalf("orden leída = %+v quote=%+v", got, got.Quote)
	}
	if got.Plate.Raw() != "ABC123" || got.DocumentID.Raw() != "1020304050" {
		t.Fatal("datos de identidad no recuperados")
	}
	if n := countRows(t, pool, "order_events"); n != 2 {
		t.Fatalf("eventos = %d, want 2", n)
	}
}

// Si otro proceso ya cambió el estado, la transición se rechaza y no se registra un evento huérfano.
func TestSaveTransition_RejectsStaleState(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o, _, _ := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "a"}, newOrder(t), createdEvent())
	stale := o
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	if err := repo.SaveTransition(ctx, o, ev); err != nil {
		t.Fatal(err)
	}

	staleEv, _ := stale.Transition(order.Failed, "late failure", time.Now())
	err := repo.SaveTransition(ctx, stale, staleEv)

	if !errors.Is(err, order.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if n := countRows(t, pool, "order_events"); n != 2 {
		t.Fatalf("eventos = %d, want 2", n)
	}
}

// Regla crítica: estado y evento son atómicos. Si el evento no se puede guardar, el estado no cambia.
func TestSaveTransition_EventFailureRollsBackState(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o, _, err := repo.Create(ctx, app.IdempotencyKey{Key: "k", RequestHash: "a"}, newOrder(t), createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := o.Transition(order.Quoted, "quote received", time.Now())
	ev.To = "BOGUS" // viola el CHECK de order_events después de que el UPDATE ya se ejecutó

	if err := repo.SaveTransition(ctx, o, ev); err == nil {
		t.Fatal("se esperaba un error al guardar el evento")
	}

	got, err := repo.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != order.Created {
		t.Fatalf("status = %s: el cambio de estado debió revertirse", got.Status)
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := NewOrderRepository(testPool(t))

	_, err := repo.Get(context.Background(), "00000000-0000-0000-0000-000000000000")

	if !errors.Is(err, app.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	pool := testPool(t) // ya migró una vez
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("segunda migración: %v", err)
	}
}
