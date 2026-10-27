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

type fakeAlertStore struct {
	saved map[string]DriverAlert
	err   error
}

func (s *fakeAlertStore) SaveDriverAlert(_ context.Context, a DriverAlert) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if prev, ok := s.saved[a.ID]; ok {
		if prev != a {
			return false, ErrDriverAlertConflict
		}
		return false, nil
	}
	s.saved[a.ID] = a
	return true, nil
}

func panicAlert(t *testing.T) DriverAlert {
	t.Helper()
	plate, err := order.NewPlate("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	return DriverAlert{ID: "6f1c2a7e-0000-4000-8000-000000000001", Plate: plate, TripID: "trip-1",
		Type: DriverAlertPanic, OccurredAt: time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)}
}

func TestReportDriverAlert_SavesOnceAndWarnsWithTheMaskedPlate(t *testing.T) {
	var logs bytes.Buffer
	store := &fakeAlertStore{saved: map[string]DriverAlert{}}
	uc := NewReportDriverAlert(store, slog.New(slog.NewJSONHandler(&logs, nil)))
	alert := panicAlert(t)

	created, err := uc.Execute(context.Background(), alert)
	if err != nil || !created {
		t.Fatalf("primera alerta: created=%v err=%v", created, err)
	}
	again, err := uc.Execute(context.Background(), alert)
	if err != nil || again {
		t.Fatalf("reintento de la misma alerta: created=%v err=%v; no debía crear otra", again, err)
	}

	if len(store.saved) != 1 {
		t.Fatalf("alertas guardadas = %d, se esperaba 1", len(store.saved))
	}
	out := logs.String()
	if strings.Count(out, `"level":"WARN"`) != 1 || !strings.Contains(out, alert.ID) {
		t.Fatalf("se esperaba un solo WARN con el id de la alerta (el reintento no alerta otra vez): %s", out)
	}
	if strings.Contains(out, "ABC123") {
		t.Fatalf("la placa quedó en claro en los logs: %s", out)
	}
}

func TestReportDriverAlert_PropagatesConflictsAndStoreErrors(t *testing.T) {
	store := &fakeAlertStore{saved: map[string]DriverAlert{}}
	uc := NewReportDriverAlert(store, slog.New(slog.DiscardHandler))
	alert := panicAlert(t)
	if _, err := uc.Execute(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	other := alert
	other.TripID = "trip-2"

	if _, err := uc.Execute(context.Background(), other); !errors.Is(err, ErrDriverAlertConflict) {
		t.Fatalf("mismo id con otro contenido: err = %v, se esperaba ErrDriverAlertConflict", err)
	}

	boom := errors.New("db down")
	if _, err := NewReportDriverAlert(&fakeAlertStore{err: boom}, slog.New(slog.DiscardHandler)).Execute(context.Background(), alert); !errors.Is(err, boom) {
		t.Fatalf("error de la base: err = %v", err)
	}
}
