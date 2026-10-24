package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// Motivos de falla visibles para operaciones.
const (
	ReasonQuoteUnavailable = "QUOTE_UNAVAILABLE"
	ReasonQuoteRejected    = "QUOTE_REJECTED"
)

const (
	quoteMaxAttempts = 3
	// El cotizador tarda hasta 3 s según su contrato; 4 s por intento deja margen sin esperar indefinidamente.
	quoteAttemptTimeout = 4 * time.Second
	quoteBaseBackoff    = 200 * time.Millisecond
	// processingBudget limita cotizar y guardar el resultado, aun desligado de la cancelación del cliente:
	// si la base se congela, la goroutine no queda colgada para siempre.
	processingBudget = 30 * time.Second
)

// CreateOrder es el caso de uso que inicia una compra: crea la orden de forma idempotente y la cotiza.
type CreateOrder struct {
	repo   OrderRepository
	quoter Quoter
	log    *slog.Logger
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
}

// CreateOrderResult es la orden resultante; Created es false si la solicitud ya se había procesado.
type CreateOrderResult struct {
	Order   order.Order
	Created bool
}

// NewCreateOrder arma el caso de uso con sus puertos.
func NewCreateOrder(repo OrderRepository, quoter Quoter, log *slog.Logger) *CreateOrder {
	return &CreateOrder{repo: repo, quoter: quoter, log: log, now: utcMicros, sleep: sleepCtx}
}

// utcMicros usa la misma precisión y zona que guarda PostgreSQL: así la respuesta original
// y la repetida por idempotencia son idénticas.
func utcMicros() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Execute procesa la solicitud de compra identificada por idempotencyKey.
// Una falla del cotizador no es un error de la solicitud: la orden queda en FAILED con su motivo.
func (uc *CreateOrder) Execute(ctx context.Context, idempotencyKey string, in order.NewOrderInput) (CreateOrderResult, error) {
	o, err := order.NewOrder(in)
	if err != nil {
		return CreateOrderResult{}, err
	}

	now := uc.now()
	o.CreatedAt, o.UpdatedAt = now, now
	created := order.Event{To: order.Created, Reason: "order created", OccurredAt: now}
	key := IdempotencyKey{Key: idempotencyKey, RequestHash: requestHash(o)}

	saved, isNew, err := uc.repo.Create(ctx, key, o, created)
	if err != nil {
		return CreateOrderResult{}, fmt.Errorf("create order: %w", err)
	}
	if !isNew {
		return CreateOrderResult{Order: saved}, nil
	}
	uc.log.Info("order created", "order_id", saved.ID, "plate", saved.Plate)

	// La orden ya existe: si el cliente se desconecta, la cotización debe terminar igual
	// para no dejarla atascada en CREATED, pero con un límite de tiempo propio.
	procCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processingBudget)
	defer cancel()
	quoted, err := uc.quote(procCtx, saved)
	if err != nil {
		return CreateOrderResult{}, err
	}
	return CreateOrderResult{Order: quoted, Created: true}, nil
}

func (uc *CreateOrder) quote(ctx context.Context, o order.Order) (order.Order, error) {
	req := QuoteRequest{
		Plate:       o.Plate.Raw(),
		VehicleYear: o.VehicleYear,
		CityCode:    o.CityCode,
		DocumentID:  o.DocumentID.Raw(),
	}

	q, attempts, err := uc.quoteWithRetry(ctx, req)
	if err != nil {
		reason := ReasonQuoteUnavailable
		if errors.Is(err, ErrQuoteRejected) {
			reason = ReasonQuoteRejected
		}
		uc.log.Error("quote failed", "order_id", o.ID, "attempts", attempts, "reason", reason, "error", err)
		o.FailureReason = reason
		return uc.transition(ctx, o, order.Failed, reason)
	}

	o.Quote = &q
	uc.log.Info("order quoted", "order_id", o.ID, "attempts", attempts)
	return uc.transition(ctx, o, order.Quoted, "quote received")
}

// quoteWithRetry reintenta solo las fallas transitorias, con backoff exponencial y jitter completo
// para que los reintentos de muchas órdenes no golpeen al cotizador al mismo tiempo.
func (uc *CreateOrder) quoteWithRetry(ctx context.Context, req QuoteRequest) (order.Quote, int, error) {
	var lastErr error
	for attempt := 1; attempt <= quoteMaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, quoteAttemptTimeout)
		q, err := uc.quoter.Quote(attemptCtx, req)
		cancel()
		if err == nil {
			return q, attempt, nil
		}
		lastErr = err
		if !isTransient(err) {
			return order.Quote{}, attempt, err
		}
		if attempt < quoteMaxAttempts {
			backoff := quoteBaseBackoff << (attempt - 1)
			if err := uc.sleep(ctx, rand.N(backoff)); err != nil {
				return order.Quote{}, attempt, err
			}
		}
	}
	return order.Quote{}, quoteMaxAttempts, fmt.Errorf("after %d attempts: %w", quoteMaxAttempts, lastErr)
}

func (uc *CreateOrder) transition(ctx context.Context, o order.Order, to order.Status, reason string) (order.Order, error) {
	ev, err := o.Transition(to, reason, uc.now())
	if err != nil {
		return order.Order{}, err
	}
	if err := uc.repo.SaveTransition(ctx, o, ev); err != nil {
		return order.Order{}, fmt.Errorf("save transition to %s: %w", to, err)
	}
	return o, nil
}

func isTransient(err error) bool {
	return errors.Is(err, ErrQuoteUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

// requestHash es la huella de la solicitud normalizada: detecta una clave reutilizada con otros datos.
func requestHash(o order.Order) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d|%s", o.Plate.Raw(), o.DocumentID.Raw(), o.VehicleYear, o.CityCode))
	return hex.EncodeToString(sum[:])
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
