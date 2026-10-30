package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// DriverAlertPanic es el único tipo de alerta del prototipo móvil (HU-20): el botón de pánico.
const DriverAlertPanic = "PANIC"

// ErrDriverAlertConflict indica que el teléfono reutilizó el id de una alerta con otro contenido.
var ErrDriverAlertConflict = errors.New("driver alert id reused with different content")

// DriverAlert es una alerta de la app del conductor. El ID lo genera el teléfono al crearla, antes de
// intentar enviarla: es la clave de idempotencia de los reintentos desde la cola offline.
type DriverAlert struct {
	ID         string
	Plate      order.Plate
	TripID     string
	Type       string
	OccurredAt time.Time
}

// DriverAlertStore guarda las alertas una sola vez por ID.
type DriverAlertStore interface {
	// SaveDriverAlert devuelve created=false si la alerta ya estaba guardada con el mismo contenido, y
	// ErrDriverAlertConflict si el ID existe con otro contenido.
	SaveDriverAlert(ctx context.Context, a DriverAlert) (created bool, err error)
}

// ReportDriverAlert registra una alerta del conductor.
type ReportDriverAlert struct {
	store DriverAlertStore
	log   *slog.Logger
}

// NewReportDriverAlert crea el caso de uso.
func NewReportDriverAlert(store DriverAlertStore, log *slog.Logger) *ReportDriverAlert {
	return &ReportDriverAlert{store: store, log: log}
}

// Execute guarda la alerta. Un reintento de la misma alerta (la cola offline del teléfono) no la duplica.
func (uc *ReportDriverAlert) Execute(ctx context.Context, a DriverAlert) (created bool, err error) {
	created, err = uc.store.SaveDriverAlert(ctx, a)
	if err != nil {
		return false, fmt.Errorf("save driver alert: %w", err)
	}
	log := uc.log.With("alert_id", a.ID, "plate", a.Plate, "trip_id", a.TripID, "type", a.Type)
	if !created {
		log.Info("driver alert retried (already received)")
		return false, nil
	}
	// WARN: es la señal que operaciones vigila. Un reintento del teléfono no la repite.
	log.Warn("driver alert received", "occurred_at", a.OccurredAt)
	return true, nil
}
