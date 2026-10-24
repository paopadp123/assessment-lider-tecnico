package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const uniqueViolation = "23505"

// querier lo cumplen el pool y una transacción: permite leer órdenes dentro o fuera de una transacción.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// OrderRepository implementa app.OrderRepository sobre PostgreSQL.
type OrderRepository struct {
	pool *pgxpool.Pool
}

var _ app.OrderRepository = (*OrderRepository)(nil)

// NewOrderRepository crea el repositorio sobre un pool de conexiones.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// Create inserta la orden, su evento inicial y la clave de idempotencia en una transacción.
// Si otra solicitud con la misma clave ganó la carrera, la PK de idempotency_keys hace fallar
// esta transacción y se devuelve la orden ganadora (o un conflicto si la solicitud difiere).
func (r *OrderRepository) Create(ctx context.Context, key app.IdempotencyKey, o order.Order, ev order.Event) (order.Order, bool, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (status, plate, document_id, vehicle_year, city_code, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			o.Status, o.Plate.Raw(), o.DocumentID.Raw(), o.VehicleYear, o.CityCode, o.CreatedAt, o.UpdatedAt,
		).Scan(&o.ID); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO idempotency_keys (key, request_hash, order_id) VALUES ($1, $2, $3)`,
			key.Key, key.RequestHash, o.ID); err != nil {
			return fmt.Errorf("insert idempotency key: %w", err)
		}
		return insertEvent(ctx, tx, o.ID, ev)
	})
	if err == nil {
		return o, true, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		return order.Order{}, false, err
	}
	existing, err := r.byIdempotencyKey(ctx, key)
	if err != nil {
		return order.Order{}, false, err
	}
	return existing, false, nil
}

func (r *OrderRepository) byIdempotencyKey(ctx context.Context, key app.IdempotencyKey) (order.Order, error) {
	var (
		hash    string
		orderID order.OrderID
	)
	err := r.pool.QueryRow(ctx,
		`SELECT request_hash, order_id::text FROM idempotency_keys WHERE key = $1`, key.Key,
	).Scan(&hash, &orderID)
	if err != nil {
		return order.Order{}, fmt.Errorf("load idempotency key: %w", err)
	}
	if hash != key.RequestHash {
		return order.Order{}, app.ErrIdempotencyConflict
	}
	return r.Get(ctx, orderID)
}

// SaveTransition guarda el nuevo estado, su evento y los comandos de la outbox en la misma transacción.
func (r *OrderRepository) SaveTransition(ctx context.Context, o order.Order, ev order.Event, cmds ...app.Command) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := saveTransition(ctx, tx, o, ev); err != nil {
			return err
		}
		return insertCommands(ctx, tx, cmds)
	})
}

// Get devuelve una orden por id.
func (r *OrderRepository) Get(ctx context.Context, id order.OrderID) (order.Order, error) {
	return loadOrder(ctx, r.pool, "id = $1", id)
}

// saveTransition actualiza la orden solo si sigue en el estado de origen del evento (control optimista)
// y registra el evento. Debe ejecutarse dentro de una transacción.
func saveTransition(ctx context.Context, tx pgx.Tx, o order.Order, ev order.Event) error {
	var quoteID, premium, currency *string
	var validUntil *time.Time
	if o.Quote != nil {
		quoteID, premium, currency, validUntil = &o.Quote.ID, &o.Quote.Premium, &o.Quote.Currency, &o.Quote.ValidUntil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE orders
		SET status = $2, quote_id = $3, premium = $4::text::numeric, currency = $5, quote_valid_until = $6,
		    payment_id = NULLIF($7, ''), failure_reason = NULLIF($8, ''), updated_at = $9
		WHERE id = $1 AND status = $10`,
		o.ID, o.Status, quoteID, premium, currency, validUntil, o.PaymentID, o.FailureReason, o.UpdatedAt, ev.From)
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}
	// La condición sobre el estado anterior evita pisar un cambio concurrente.
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: order %s is no longer in %s", order.ErrInvalidTransition, o.ID, ev.From)
	}
	return insertEvent(ctx, tx, o.ID, ev)
}

// loadOrder lee una orden con el filtro indicado; suffix permite agregar "FOR UPDATE" dentro de una transacción.
func loadOrder(ctx context.Context, q querier, where string, arg any, suffix ...string) (order.Order, error) {
	var (
		o                                     order.Order
		plate, doc                            string
		quoteID, premium, currency, paymentID *string
		validUntil                            *time.Time
		failureReason                         *string
	)
	sql := `
		SELECT id::text, status, plate, document_id, vehicle_year, city_code,
		       quote_id, premium::text, currency, quote_valid_until, payment_id, failure_reason, created_at, updated_at
		FROM orders WHERE ` + where
	for _, s := range suffix {
		sql += " " + s
	}
	err := q.QueryRow(ctx, sql, arg).Scan(&o.ID, &o.Status, &plate, &doc, &o.VehicleYear, &o.CityCode,
		&quoteID, &premium, &currency, &validUntil, &paymentID, &failureReason, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Order{}, app.ErrOrderNotFound
	}
	if err != nil {
		return order.Order{}, fmt.Errorf("load order: %w", err)
	}

	// Los datos guardados ya se validaron al crear la orden.
	if o.Plate, err = order.NewPlate(plate); err != nil {
		return order.Order{}, fmt.Errorf("stored plate: %w", err)
	}
	if o.DocumentID, err = order.NewDocumentID(doc); err != nil {
		return order.Order{}, fmt.Errorf("stored document: %w", err)
	}
	if quoteID != nil {
		o.Quote = &order.Quote{ID: *quoteID, Premium: deref(premium), Currency: deref(currency)}
		if validUntil != nil {
			o.Quote.ValidUntil = *validUntil
		}
	}
	o.PaymentID = order.PaymentID(deref(paymentID))
	o.FailureReason = deref(failureReason)
	return o, nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, orderID order.OrderID, ev order.Event) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO order_events (order_id, from_status, to_status, reason, occurred_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5)`,
		orderID, ev.From, ev.To, ev.Reason, ev.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert order event: %w", err)
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
