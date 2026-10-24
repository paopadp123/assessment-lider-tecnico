package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// lockPayment serializa, dentro de la transacción, todo lo que toca un mismo pago.
// Sin este lock, un webhook y el guardado de PAYMENT_PENDING que corren a la vez podrían no verse
// entre sí (cada uno lee antes de que el otro confirme) y el evento quedaría sin aplicar.
func lockPayment(ctx context.Context, tx pgx.Tx, id order.PaymentID) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", string(id)); err != nil {
		return fmt.Errorf("lock payment: %w", err)
	}
	return nil
}

// MarkPaymentPending guarda PAYMENT_PENDING con su paymentId y aplica, en orden de ocurrencia,
// los eventos de ese pago que llegaron antes.
func (r *OrderRepository) MarkPaymentPending(ctx context.Context, o order.Order, ev order.Event, apply app.ApplyPaymentFunc) (order.Order, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockPayment(ctx, tx, o.PaymentID); err != nil {
			return err
		}
		if err := saveTransition(ctx, tx, o, ev); err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `
			SELECT id, status, occurred_at FROM payment_events
			WHERE payment_id = $1 AND applied_at IS NULL
			ORDER BY occurred_at, id`, string(o.PaymentID))
		if err != nil {
			return fmt.Errorf("load early payment events: %w", err)
		}
		type early struct {
			id int64
			ev app.PaymentEvent
		}
		var pending []early
		for rows.Next() {
			e := early{ev: app.PaymentEvent{PaymentID: o.PaymentID}}
			if err := rows.Scan(&e.id, &e.ev.Status, &e.ev.OccurredAt); err != nil {
				rows.Close()
				return fmt.Errorf("scan payment event: %w", err)
			}
			pending = append(pending, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read payment events: %w", err)
		}

		for _, e := range pending {
			if _, err := applyEvent(ctx, tx, &o, e.id, e.ev, apply); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return order.Order{}, err
	}
	return o, nil
}

// RecordPaymentEvent guarda el evento y lo aplica si la orden ya espera ese pago.
func (r *OrderRepository) RecordPaymentEvent(ctx context.Context, ev app.PaymentEvent, apply app.ApplyPaymentFunc) (app.PaymentEventResult, error) {
	var res app.PaymentEventResult
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockPayment(ctx, tx, ev.PaymentID); err != nil {
			return err
		}

		var eventID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO payment_events (payment_id, status, occurred_at) VALUES ($1, $2, $3)
			ON CONFLICT (payment_id, status) DO NOTHING
			RETURNING id`, string(ev.PaymentID), string(ev.Status), ev.OccurredAt).Scan(&eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			res = app.PaymentEventResult{Outcome: app.PaymentDuplicate}
			return nil
		}
		if err != nil {
			return fmt.Errorf("insert payment event: %w", err)
		}

		o, err := loadOrder(ctx, tx, "payment_id = $1", string(ev.PaymentID), "FOR UPDATE")
		if errors.Is(err, app.ErrOrderNotFound) {
			res = app.PaymentEventResult{Outcome: app.PaymentPendingOrder}
			return nil
		}
		if err != nil {
			return err
		}

		outcome, err := applyEvent(ctx, tx, &o, eventID, ev, apply)
		if err != nil {
			return err
		}
		res = app.PaymentEventResult{Outcome: outcome, OrderID: o.ID, CorrelationID: o.CorrelationID}
		return nil
	})
	if err != nil {
		return app.PaymentEventResult{}, err
	}
	return res, nil
}

// applyEvent ejecuta la decisión de dominio y deja constancia del resultado en payment_events.
// Los comandos que dispara el cambio (p. ej. issue_policy al quedar PAID) entran a la outbox en la misma
// transacción. Un conflicto de dominio no es un error: se marca como anomalía y la orden no cambia.
func applyEvent(ctx context.Context, tx pgx.Tx, o *order.Order, eventID int64, ev app.PaymentEvent, apply app.ApplyPaymentFunc) (app.PaymentOutcome, error) {
	candidate := *o
	transition, cmds, err := apply(&candidate, ev)
	outcome := app.PaymentApplied
	switch {
	case errors.Is(err, order.ErrPaymentConflict):
		outcome = app.PaymentAnomaly
	case err != nil:
		return "", err
	default:
		if err := saveTransition(ctx, tx, candidate, transition); err != nil {
			return "", err
		}
		if err := insertCommands(ctx, tx, cmds); err != nil {
			return "", err
		}
		*o = candidate
	}
	if _, err := tx.Exec(ctx,
		`UPDATE payment_events SET applied_at = now(), outcome = $2 WHERE id = $1`, eventID, string(outcome)); err != nil {
		return "", fmt.Errorf("mark payment event: %w", err)
	}
	return outcome, nil
}
