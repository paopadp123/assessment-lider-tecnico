package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

var (
	_ app.OutboxStore     = (*OrderRepository)(nil)
	_ app.DeadLetterStore = (*OrderRepository)(nil)
)

func insertCommands(ctx context.Context, tx pgx.Tx, cmds []app.Command) error {
	for _, c := range cmds {
		// El comando copia el correlation-id de su orden: quien lo encole (creación, webhook, reconciliador,
		// operador) no necesita conocerlo.
		tag, err := tx.Exec(ctx, `
			INSERT INTO outbox (type, order_id, correlation_id)
			SELECT $1, id, correlation_id FROM orders WHERE id = $2`, string(c.Type), string(c.OrderID))
		if err != nil {
			return fmt.Errorf("insert outbox command: %w", err)
		}
		// Con INSERT ... SELECT una orden inexistente no daría error: se insertarían cero filas en silencio.
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("insert outbox command: %w", app.ErrOrderNotFound)
		}
	}
	return nil
}

// PublishPending toma hasta limit mensajes pendientes con FOR UPDATE SKIP LOCKED: varios despachadores
// pueden correr en paralelo sin tomar el mismo mensaje ni esperarse entre sí. El lock dura lo que dura
// la publicación del lote; por eso los lotes son pequeños.
func (r *OrderRepository) PublishPending(ctx context.Context, limit int, publish func(context.Context, app.OutboxMessage) error) (int, error) {
	published := 0
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, message_id::text, type, order_id::text, attempts, COALESCE(correlation_id, '')
			FROM outbox
			WHERE published_at IS NULL
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return fmt.Errorf("load outbox: %w", err)
		}
		msgs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (app.OutboxMessage, error) {
			var m app.OutboxMessage
			err := row.Scan(&m.ID, &m.MessageID, &m.Type, &m.OrderID, &m.Attempts, &m.CorrelationID)
			return m, err
		})
		if err != nil {
			return fmt.Errorf("read outbox: %w", err)
		}

		for _, m := range msgs {
			if pubErr := publish(ctx, m); pubErr != nil {
				if _, err := tx.Exec(ctx,
					`UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`, m.ID, pubErr.Error()); err != nil {
					return fmt.Errorf("record publish failure: %w", err)
				}
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now(), last_error = NULL WHERE id = $1`, m.ID); err != nil {
				return fmt.Errorf("mark published: %w", err)
			}
			published++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, nil
}

// RecordDeadLetter es idempotente por message_id: una reentrega no duplica el registro.
func (r *OrderRepository) RecordDeadLetter(ctx context.Context, dl app.DeadLetter) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO dead_letters (message_id, type, order_id, attempts, reason)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (message_id) DO NOTHING`,
		dl.MessageID, string(dl.Type), string(dl.OrderID), dl.Attempts, dl.Reason)
	if err != nil {
		return fmt.Errorf("record dead letter: %w", err)
	}
	return nil
}
