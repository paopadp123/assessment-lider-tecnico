package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

var (
	_ app.ReconcileStore = (*OrderRepository)(nil)
	_ app.EventReader    = (*OrderRepository)(nil)
)

// ClaimStuck marca y devuelve órdenes atascadas en una sola sentencia. FOR UPDATE SKIP LOCKED hace que dos
// reconciliadores en paralelo tomen órdenes distintas sin esperarse; reconciled_at deja la orden fuera de las
// siguientes pasadas hasta que vuelva a cumplirse el umbral. El lock dura solo esta sentencia: las llamadas a
// terceros del reconciliador ocurren después, sin locks tomados.
func (r *OrderRepository) ClaimStuck(ctx context.Context, before time.Time, limit int) ([]app.StuckOrder, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE orders SET reconciled_at = now()
		WHERE id IN (
			SELECT id FROM orders
			WHERE status IN ('CREATED', 'QUOTED', 'PAYMENT_PENDING', 'PAID', 'ISSUING')
			  AND updated_at < $1
			  AND (reconciled_at IS NULL OR reconciled_at < $1)
			ORDER BY updated_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED)
		RETURNING id::text, reconcile_misses`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("claim stuck orders: %w", err)
	}
	type claimed struct {
		id     order.OrderID
		misses int
	}
	ids, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (claimed, error) {
		var c claimed
		err := row.Scan(&c.id, &c.misses)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("read stuck orders: %w", err)
	}

	// Si una lectura falla, se devuelven las demás: las ya marcadas no deben esperar otro intervalo completo.
	out := make([]app.StuckOrder, 0, len(ids))
	var errs []error
	for _, c := range ids {
		o, err := r.Get(ctx, c.id)
		if err != nil {
			errs = append(errs, fmt.Errorf("load stuck order %s: %w", c.id, err))
			continue
		}
		out = append(out, app.StuckOrder{Order: o, IssueMisses: c.misses})
	}
	return out, errors.Join(errs...)
}

// Enqueue guarda comandos en la outbox sin cambiar el estado de la orden.
func (r *OrderRepository) Enqueue(ctx context.Context, cmds ...app.Command) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error { return insertCommands(ctx, tx, cmds) })
}

// RecordIssueMiss suma una ronda sin póliza y devuelve el total.
func (r *OrderRepository) RecordIssueMiss(ctx context.Context, id order.OrderID) (int, error) {
	var misses int
	err := r.pool.QueryRow(ctx,
		`UPDATE orders SET reconcile_misses = reconcile_misses + 1 WHERE id = $1 RETURNING reconcile_misses`, id).Scan(&misses)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrOrderNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("record issue miss: %w", err)
	}
	return misses, nil
}

const eventColumns = `id, order_id::text, COALESCE(from_status, ''), to_status, reason, COALESCE(actor, ''), occurred_at`

func scanEvent(row pgx.CollectableRow) (app.StoredEvent, error) {
	var e app.StoredEvent
	err := row.Scan(&e.ID, &e.OrderID, &e.From, &e.To, &e.Reason, &e.Actor, &e.OccurredAt)
	return e, err
}

// ListEvents devuelve la línea de tiempo de una orden en orden de ocurrencia.
func (r *OrderRepository) ListEvents(ctx context.Context, id order.OrderID) ([]app.StoredEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+eventColumns+` FROM order_events WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	events, err := pgx.CollectRows(rows, scanEvent)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	if len(events) == 0 {
		// Toda orden tiene al menos su evento CREATED: sin eventos, la orden no existe.
		return nil, app.ErrOrderNotFound
	}
	return events, nil
}

// EventsAfter devuelve hasta limit eventos de cualquier orden con ID mayor que afterID, en orden.
func (r *OrderRepository) EventsAfter(ctx context.Context, afterID int64, limit int) ([]app.StoredEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+eventColumns+` FROM order_events WHERE id > $1 ORDER BY id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("events after: %w", err)
	}
	events, err := pgx.CollectRows(rows, scanEvent)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	return events, nil
}

// LatestEventID devuelve el ID del último evento guardado, o 0.
func (r *OrderRepository) LatestEventID(ctx context.Context) (int64, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM order_events`).Scan(&id); err != nil {
		return 0, fmt.Errorf("latest event id: %w", err)
	}
	return id, nil
}
