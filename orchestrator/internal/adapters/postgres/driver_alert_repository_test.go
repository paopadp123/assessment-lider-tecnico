package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

func driverAlert(t *testing.T) app.DriverAlert {
	t.Helper()
	plate, err := order.NewPlate("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	return app.DriverAlert{ID: "6f1c2a7e-0000-4000-8000-000000000001", Plate: plate, TripID: "trip-1",
		Type: app.DriverAlertPanic, OccurredAt: time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)}
}

func TestDriverAlerts_SavedOnceEvenWithConcurrentRetries(t *testing.T) {
	pool := testPool(t)
	repo := NewDriverAlertRepository(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE driver_alerts"); err != nil {
		t.Fatal(err)
	}
	alert := driverAlert(t)

	// La cola del teléfono puede reenviar la misma alerta mientras la primera solicitud sigue en curso.
	var wg sync.WaitGroup
	created := make(chan bool, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.SaveDriverAlert(ctx, alert)
			if err != nil {
				t.Errorf("reintento concurrente: %v", err)
			}
			created <- ok
		}()
	}
	wg.Wait()
	close(created)

	firsts := 0
	for ok := range created {
		if ok {
			firsts++
		}
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM driver_alerts WHERE id = $1", alert.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if firsts != 1 || rows != 1 {
		t.Fatalf("created=true en %d solicitudes y %d filas; se esperaba 1 y 1", firsts, rows)
	}
}

func TestDriverAlerts_SameIDWithOtherContentIsAConflict(t *testing.T) {
	pool := testPool(t)
	repo := NewDriverAlertRepository(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE driver_alerts"); err != nil {
		t.Fatal(err)
	}
	alert := driverAlert(t)
	if _, err := repo.SaveDriverAlert(ctx, alert); err != nil {
		t.Fatal(err)
	}
	other := alert
	other.TripID = "trip-2"

	if _, err := repo.SaveDriverAlert(ctx, other); !errors.Is(err, app.ErrDriverAlertConflict) {
		t.Fatalf("err = %v, se esperaba ErrDriverAlertConflict", err)
	}
	var plate string
	if err := pool.QueryRow(ctx, "SELECT plate FROM driver_alerts WHERE id = $1", alert.ID).Scan(&plate); err != nil || plate != "ABC123" {
		t.Fatalf("placa guardada = %q (err %v); se guarda en claro, como en orders", plate, err)
	}
}
