package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// Motivos de falla visibles para operaciones.
const (
	ReasonQuoteUnavailable       = "QUOTE_UNAVAILABLE"
	ReasonQuoteRejected          = "QUOTE_REJECTED"
	ReasonPaymentUnavailable     = "PAYMENT_UNAVAILABLE"
	ReasonPaymentRequestRejected = "PAYMENT_REQUEST_REJECTED"
)

// processingBudget limita cotizar y guardar el resultado, aun desligado de la cancelación del cliente.
const processingBudget = 30 * time.Second

var quotePolicy = retryPolicy{maxAttempts: 3, attemptTimeout: 4 * time.Second, baseBackoff: 200 * time.Millisecond}

// CreateOrder es el caso de uso que inicia una compra: crea la orden de forma idempotente y la cotiza.
type CreateOrder struct {
	repo   OrderRepository
	quoter Quoter
	psp    PaymentGateway
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
func NewCreateOrder(repo OrderRepository, quoter Quoter, psp PaymentGateway, log *slog.Logger) *CreateOrder {
	return &CreateOrder{repo: repo, quoter: quoter, psp: psp, log: log, now: utcMicros, sleep: sleepCtx}
}

func utcMicros() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Execute procesa la solicitud de compra identificada por idempotencyKey.
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
	procCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processingBudget)
	defer cancel()
	quoted, err := uc.quote(procCtx, saved)
	if err != nil {
		return CreateOrderResult{}, err
	}
	return CreateOrderResult{Order: quoted, Created: true}, nil
}

func (uc *CreateOrder) quote(ctx context.Context, o order.Order) (order.Order, error) {
	req := QuoteRequest{Plate: o.Plate.Raw(), VehicleYear: o.VehicleYear, CityCode: o.CityCode, DocumentID: o.DocumentID.Raw()}
	q, attempts, err := withRetry(ctx, quotePolicy, uc.sleep, isTransientQuote,
		func(ctx context.Context) (order.Quote, error) { return uc.quoter.Quote(ctx, req) })
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

func isTransientQuote(err error) bool {
	return errors.Is(err, ErrQuoteUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

// requestHash es la huella de la solicitud normalizada: detecta una clave reutilizada con otros datos.
func requestHash(o order.Order) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d|%s", o.Plate.Raw(), o.DocumentID.Raw(), o.VehicleYear, o.CityCode))
	return hex.EncodeToString(sum[:])
}
