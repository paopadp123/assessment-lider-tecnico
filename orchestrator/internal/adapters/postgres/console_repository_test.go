package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// seedOrder crea una orden con la placa y el instante de creación indicados, y la deja en status con los
// identificadores dados (paymentId, póliza, correlation-id) escritos directo en la tabla.
func seedOrder(t *testing.T, pool *pgxpool.Pool, plate string, createdAt time.Time, status order.Status, paymentID, policy, corr string) order.Order {
	t.Helper()
	ctx := context.Background()
	o, err := order.NewOrder(order.NewOrderInput{Plate: plate, DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"})
	if err != nil {
		t.Fatal(err)
	}
	o.CreatedAt, o.UpdatedAt = createdAt, createdAt
	saved, _, err := NewOrderRepository(pool).Create(ctx, app.IdempotencyKey{Key: "k-" + plate, RequestHash: plate}, o, createdEvent())
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE orders SET status = $2, payment_id = NULLIF($3, ''), policy_number = NULLIF($4, ''),
		correlation_id = NULLIF($5, '') WHERE id = $1`, saved.ID, status, paymentID, policy, corr)
	if err != nil {
		t.Fatal(err)
	}
	saved.Status = status
	return saved
}

func ids(orders []order.Order) []order.OrderID {
	out := make([]order.OrderID, len(orders))
	for i, o := range orders {
		out[i] = o.ID
	}
	return out
}

func TestCalls_RecordedAndListedInOrder(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()
	o := seedOrder(t, pool, "ABC123", time.Now().UTC(), order.Issuing, "", "", "")
	start := time.Now().UTC().Truncate(time.Microsecond)

	calls := []app.ThirdPartyCall{
		{OrderID: o.ID, Party: app.PartyIssuer, Operation: app.OpIssue, Outcome: app.CallTimeout,
			Detail: "policy issuer unavailable: context deadline exceeded", Duration: 10 * time.Second, StartedAt: start},
		{OrderID: o.ID, Party: app.PartyIssuer, Operation: app.OpFindPolicy, Outcome: app.CallOK,
			Duration: 1500 * time.Millisecond, StartedAt: start.Add(10 * time.Second)},
	}
	for _, c := range calls {
		if err := repo.RecordCall(ctx, c); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := repo.ListCalls(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("llamadas = %d, se esperaban 2", len(got))
	}
	if got[0].ID >= got[1].ID {
		t.Errorf("no vienen en orden de ocurrencia: %d, %d", got[0].ID, got[1].ID)
	}
	for i, c := range calls {
		g := got[i].ThirdPartyCall
		if g.OrderID != c.OrderID || g.Party != c.Party || g.Operation != c.Operation || g.Outcome != c.Outcome ||
			g.Detail != c.Detail || g.Duration != c.Duration || !g.StartedAt.Equal(c.StartedAt) {
			t.Errorf("llamada %d leída = %+v, se guardó %+v", i, g, c)
		}
	}
}

func TestCalls_UnknownOrderAndEmptyList(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	ctx := context.Background()

	if _, err := repo.ListCalls(ctx, "4f1c2a7e-0000-4000-8000-0000000000ff"); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("orden inexistente: err = %v, se esperaba ErrOrderNotFound", err)
	}
	o := seedOrder(t, pool, "ABC123", time.Now().UTC(), order.Created, "", "", "")
	got, err := repo.ListCalls(ctx, o.ID)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("orden sin llamadas: %v, %v (se esperaba una lista vacía, no nil)", got, err)
	}
}

func TestListOrders_NewestFirstAndFilteredByStatus(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	base := time.Now().UTC().Truncate(time.Microsecond)
	oldest := seedOrder(t, pool, "AAA111", base, order.Issued, "", "", "")
	middle := seedOrder(t, pool, "BBB222", base.Add(time.Minute), order.Issuing, "", "", "")
	newest := seedOrder(t, pool, "CCC333", base.Add(2*time.Minute), order.RefundRequired, "", "", "")

	all, err := repo.ListOrders(context.Background(), app.OrderFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(all.Orders), []order.OrderID{newest.ID, middle.ID, oldest.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("orden del listado = %v, se esperaba %v", got, want)
	}
	if all.Next != nil {
		t.Errorf("una sola página no debe tener cursor: %+v", all.Next)
	}

	stuck, err := repo.ListOrders(context.Background(), app.OrderFilter{Statuses: []order.Status{order.Issuing, order.RefundRequired}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(stuck.Orders), []order.OrderID{newest.ID, middle.ID}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("filtro por estado = %v, se esperaba %v", got, want)
	}
	if all.Orders[0].Plate.Raw() != "CCC333" || all.Orders[0].DocumentID.Raw() == "" {
		t.Error("el listado no carga la orden completa")
	}
}

func TestListOrders_SearchByIdentifiers(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	base := time.Now().UTC()
	target := seedOrder(t, pool, "XYZ987", base, order.Issued, "pay-77", "POL-2026-000077", "corr-77")
	seedOrder(t, pool, "AAA111", base.Add(time.Second), order.Issued, "pay-11", "POL-2026-000011", "corr-11")

	for _, q := range []string{string(target.ID), "pay-77", "POL-2026-000077", "corr-77", "XYZ987", "xyz987"} {
		page, err := repo.ListOrders(context.Background(), app.OrderFilter{Query: q, Limit: 10})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if len(page.Orders) != 1 || page.Orders[0].ID != target.ID {
			t.Errorf("buscar %q devolvió %v, se esperaba solo %s", q, ids(page.Orders), target.ID)
		}
	}
	page, err := repo.ListOrders(context.Background(), app.OrderFilter{Query: "no-existe", Limit: 10})
	if err != nil || len(page.Orders) != 0 || page.Orders == nil {
		t.Errorf("búsqueda sin resultados: %v, %v (se esperaba lista vacía, no nil)", page.Orders, err)
	}
}

// El cursor sigue donde terminó la página, aunque varias órdenes compartan el mismo instante de creación.
func TestListOrders_PaginatesWithCursor(t *testing.T) {
	pool := testPool(t)
	repo := NewOrderRepository(pool)
	same := time.Now().UTC().Truncate(time.Microsecond)
	var all []order.OrderID
	for _, plate := range []string{"AAA111", "BBB222", "CCC333", "DDD444", "EEE555"} {
		all = append(all, seedOrder(t, pool, plate, same, order.Issued, "", "", "").ID)
	}

	var seen []order.OrderID
	f := app.OrderFilter{Limit: 2}
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("la paginación no termina")
		}
		page, err := repo.ListOrders(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, ids(page.Orders)...)
		if page.Next == nil {
			break
		}
		f.Cursor = page.Next
	}
	if len(seen) != len(all) {
		t.Fatalf("se vieron %d órdenes, hay %d: %v", len(seen), len(all), seen)
	}
	unique := map[order.OrderID]bool{}
	for _, id := range seen {
		unique[id] = true
	}
	if len(unique) != len(all) {
		t.Errorf("la paginación repitió órdenes: %v", seen)
	}
}
