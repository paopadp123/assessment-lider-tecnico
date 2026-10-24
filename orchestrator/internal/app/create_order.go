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

// processingBudget limita cotizar, pedir el pago y guardar, aun desligado de la cancelación del cliente:
// si un tercero o la base se congelan, la goroutine no queda colgada para siempre.
// Cubre el peor caso de ambos terceros con sus reintentos (~25 s).
const processingBudget = 35 * time.Second

var (
	quotePolicy   = retryPolicy{maxAttempts: 3, attemptTimeout: 4 * time.Second, baseBackoff: 200 * time.Millisecond}
	paymentPolicy = retryPolicy{maxAttempts: 3, attemptTimeout: 4 * time.Second, baseBackoff: 200 * time.Millisecond}
)

// CreateOrder es el caso de uso que inicia una compra: crea la orden de forma idempotente,
// la cotiza y crea la intención de pago.
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

// utcMicros usa la misma precisión y zona que guarda PostgreSQL: así la respuesta original
// y la repetida por idempotencia son idénticas.
func utcMicros() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Execute procesa la solicitud de compra identificada por idempotencyKey.
// Una falla de un tercero no es un error de la solicitud: la orden queda en FAILED con su motivo.
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

	// La orden ya existe: si el cliente se desconecta, el proceso debe terminar igual
	// para no dejarla atascada, pero con un límite de tiempo propio.
	procCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processingBudget)
	defer cancel()

	current, err := uc.quote(procCtx, saved)
	if err != nil || current.Status != order.Quoted {
		return CreateOrderResult{Order: current, Created: true}, err
	}
	current, err = uc.requestPayment(procCtx, current)
	if err != nil {
		return CreateOrderResult{}, err
	}
	return CreateOrderResult{Order: current, Created: true}, nil
}

func (uc *CreateOrder) quote(ctx context.Context, o order.Order) (order.Order, error) {
	req := QuoteRequest{Plate: o.Plate.Raw(), VehicleYear: o.VehicleYear, CityCode: o.CityCode, DocumentID: o.DocumentID.Raw()}
	q, attempts, err := withRetry(ctx, quotePolicy, uc.sleep, isTransient(ErrQuoteUnavailable),
		func(ctx context.Context) (order.Quote, error) { return uc.quoter.Quote(ctx, req) })
	if err != nil {
		reason := ReasonQuoteUnavailable
		if errors.Is(err, ErrQuoteRejected) {
			reason = ReasonQuoteRejected
		}
		return uc.fail(ctx, o, "quote failed", reason, attempts, err)
	}
	o.Quote = &q
	uc.log.Info("order quoted", "order_id", o.ID, "attempts", attempts)
	return uc.transition(ctx, o, order.Quoted, "quote received")
}

// requestPayment crea la intención de pago y guarda PAYMENT_PENDING. Si el PSP ya confirmó el pago
// por webhook antes de este guardado, el repositorio lo aplica en la misma transacción.
func (uc *CreateOrder) requestPayment(ctx context.Context, o order.Order) (order.Order, error) {
	req := PaymentRequest{OrderID: o.ID, QuoteID: o.Quote.ID, Amount: o.Quote.Premium, Currency: o.Quote.Currency}
	paymentID, attempts, err := withRetry(ctx, paymentPolicy, uc.sleep, isTransient(ErrPaymentUnavailable),
		func(ctx context.Context) (order.PaymentID, error) { return uc.psp.CreatePayment(ctx, req) })
	if err != nil {
		reason := ReasonPaymentUnavailable
		if errors.Is(err, ErrPaymentRequestRejected) {
			reason = ReasonPaymentRequestRejected
		}
		return uc.fail(ctx, o, "payment request failed", reason, attempts, err)
	}

	o.PaymentID = paymentID
	ev, err := o.Transition(order.PaymentPending, "payment requested", uc.now())
	if err != nil {
		return order.Order{}, err
	}
	final, err := uc.repo.MarkPaymentPending(ctx, o, ev, applyPayment(uc.now))
	if err != nil {
		return order.Order{}, fmt.Errorf("mark payment pending: %w", err)
	}
	uc.log.Info("payment requested", "order_id", o.ID, "payment_id", paymentID, "attempts", attempts, "status", final.Status)
	return final, nil
}

func (uc *CreateOrder) fail(ctx context.Context, o order.Order, msg, reason string, attempts int, cause error) (order.Order, error) {
	uc.log.Error(msg, "order_id", o.ID, "attempts", attempts, "reason", reason, "error", cause)
	o.FailureReason = reason
	return uc.transition(ctx, o, order.Failed, reason)
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

// isTransient arma el criterio de reintento de un tercero: su error transitorio o un timeout del intento.
func isTransient(unavailable error) func(error) bool {
	return func(err error) bool {
		return errors.Is(err, unavailable) || errors.Is(err, context.DeadlineExceeded)
	}
}

// applyPayment es la decisión de dominio que el repositorio ejecuta dentro de su transacción.
func applyPayment(now func() time.Time) ApplyPaymentFunc {
	return func(o *order.Order, ev PaymentEvent) (order.Event, error) {
		return o.ApplyPayment(ev.Status, now())
	}
}

// requestHash es la huella de la solicitud normalizada: detecta una clave reutilizada con otros datos.
func requestHash(o order.Order) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d|%s", o.Plate.Raw(), o.DocumentID.Raw(), o.VehicleYear, o.CityCode))
	return hex.EncodeToString(sum[:])
}
