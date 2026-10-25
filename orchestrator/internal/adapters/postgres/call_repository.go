package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// RecordCall guarda un intento con un tercero.
func (r *OrderRepository) RecordCall(ctx context.Context, c app.ThirdPartyCall) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO third_party_calls (order_id, party, operation, outcome, detail, duration_ms, started_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)`,
		c.OrderID, c.Party, c.Operation, c.Outcome, c.Detail, c.Duration.Milliseconds(), c.StartedAt)
	if err != nil {
		return fmt.Errorf("record third-party call: %w", err)
	}
	return nil
}

// ListCalls devuelve los intentos de una orden en orden de ocurrencia, o app.ErrOrderNotFound.
func (r *OrderRepository) ListCalls(ctx context.Context, id order.OrderID) ([]app.StoredCall, error) {
	// A diferencia de los eventos, una orden puede no tener llamadas todavía: se verifica que exista.
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1)`, id).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check order: %w", err)
	}
	if !exists {
		return nil, app.ErrOrderNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, order_id::text, party, operation, outcome, COALESCE(detail, ''), duration_ms, started_at
		FROM third_party_calls WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("list third-party calls: %w", err)
	}
	calls, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (app.StoredCall, error) {
		var c app.StoredCall
		var ms int64
		err := row.Scan(&c.ID, &c.OrderID, &c.Party, &c.Operation, &c.Outcome, &c.Detail, &ms, &c.StartedAt)
		c.Duration = time.Duration(ms) * time.Millisecond
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("read third-party calls: %w", err)
	}
	if calls == nil {
		calls = []app.StoredCall{}
	}
	return calls, nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ListOrders devuelve una página del listado de la consola: de la más reciente a la más antigua, con
// paginación por cursor (keyset). Pide una fila de más para saber si hay otra página sin un COUNT.
func (r *OrderRepository) ListOrders(ctx context.Context, f app.OrderFilter) (app.OrderPage, error) {
	statuses := make([]string, len(f.Statuses))
	for i, s := range f.Statuses {
		statuses[i] = string(s)
	}
	q := strings.TrimSpace(f.Query)
	var byID *string // el id solo se compara si la búsqueda tiene forma de UUID (si no, el cast fallaría)
	if uuidPattern.MatchString(q) {
		byID = &q
	}
	var afterAt *time.Time
	var afterID *string
	if f.Cursor != nil {
		afterAt, afterID = &f.Cursor.CreatedAt, (*string)(&f.Cursor.ID)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT `+orderColumns+` FROM orders
		WHERE (cardinality($1::text[]) = 0 OR status = ANY ($1))
		  AND ($2 = '' OR id = $3::uuid OR payment_id = $2 OR policy_number = $2 OR correlation_id = $2
		       OR plate = upper(replace(replace($2, ' ', ''), '-', '')))
		  AND ($4::timestamptz IS NULL OR (created_at, id) < ($4, $5::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $6`,
		statuses, q, byID, afterAt, afterID, f.Limit+1)
	if err != nil {
		return app.OrderPage{}, fmt.Errorf("list orders: %w", err)
	}
	orders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (order.Order, error) { return scanOrder(row) })
	if err != nil {
		return app.OrderPage{}, fmt.Errorf("read orders: %w", err)
	}

	page := app.OrderPage{Orders: orders}
	if len(orders) > f.Limit {
		page.Orders = orders[:f.Limit]
		last := page.Orders[f.Limit-1]
		page.Next = &app.OrderCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if page.Orders == nil {
		page.Orders = []order.Order{}
	}
	return page, nil
}
