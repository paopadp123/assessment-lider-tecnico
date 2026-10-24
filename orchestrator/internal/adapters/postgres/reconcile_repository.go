package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

var (
	_ app.ReconcileStore = (*OrderRepository)(nil)
	_ app.EventReader    = (*OrderRepository)(nil)
)

// ClaimStuck toma órdenes atascadas.
func (r *OrderRepository) ClaimStuck(ctx context.Context, before time.Time, limit int) ([]app.StuckOrder, error) {
	return nil, errors.New("not implemented")
}

// Enqueue guarda comandos sin cambiar el estado.
func (r *OrderRepository) Enqueue(ctx context.Context, cmds ...app.Command) error {
	return errors.New("not implemented")
}

// RecordIssueMiss suma una ronda sin póliza.
func (r *OrderRepository) RecordIssueMiss(ctx context.Context, id order.OrderID) (int, error) {
	return 0, errors.New("not implemented")
}

// ListEvents devuelve la línea de tiempo de una orden.
func (r *OrderRepository) ListEvents(ctx context.Context, id order.OrderID) ([]app.StoredEvent, error) {
	return nil, errors.New("not implemented")
}

// EventsAfter devuelve los eventos posteriores a afterID.
func (r *OrderRepository) EventsAfter(ctx context.Context, afterID int64, limit int) ([]app.StoredEvent, error) {
	return nil, errors.New("not implemented")
}

// LatestEventID devuelve el último ID de evento.
func (r *OrderRepository) LatestEventID(ctx context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}
