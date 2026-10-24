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
	ReasonOrderNotFound          = "ORDER_NOT_FOUND"
	ReasonIssueRejected          = "ISSUE_REJECTED"
	ReasonIssuerUnavailable      = "ISSUER_UNAVAILABLE"
)

// processingBudget limita cotizar y guardar, aun desligado de la cancelación del cliente:
// si el cotizador o la base se congelan, la goroutine no queda colgada para siempre.
const processingBudget = 15 * time.Second

var quotePolicy = retryPolicy{maxAttempts: 3, attemptTimeout: 4 * time.Second, baseBackoff: 200 * time.Millisecond}

// CreateOrder es el caso de uso que inicia una compra: crea la orden de forma idempotente y la cotiza.
// La intención de pago no se pide aquí: queda como comando en la outbox, en la misma transacción que QUOTED.
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
	req := QuoteRequest{Plate: o.Plate.Raw(), VehicleYear: o.VehicleYear, CityCode: o.CityCode, DocumentID: o.DocumentID.Raw()}
	q, attempts, err := withRetry(ctx, quotePolicy, uc.sleep, isTransient(ErrQuoteUnavailable),
		func(ctx context.Context) (order.Quote, error) { return uc.quoter.Quote(ctx, req) })
	if err != nil {
		reason := ReasonQuoteUnavailable
		if errors.Is(err, ErrQuoteRejected) {
			reason = ReasonQuoteRejected
		}
		uc.log.Error("quote failed", "order_id", o.ID, "attempts", attempts, "reason", reason, "error", err)
		o.FailureReason = reason
		return transition(ctx, uc.repo, uc.now, o, order.Failed, reason)
	}
	o.Quote = &q
	uc.log.Info("order quoted", "order_id", o.ID, "attempts", attempts)
	return transition(ctx, uc.repo, uc.now, o, order.Quoted, "quote received",
		Command{Type: CommandRequestPayment, OrderID: o.ID})
}

// transition aplica el cambio de estado y lo guarda junto con su evento y sus comandos.
func transition(ctx context.Context, repo OrderRepository, now func() time.Time, o order.Order, to order.Status, reason string, cmds ...Command) (order.Order, error) {
	ev, err := o.Transition(to, reason, now())
	if err != nil {
		return order.Order{}, err
	}
	if err := repo.SaveTransition(ctx, o, ev, cmds...); err != nil {
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
// Una orden que queda PAID dispara la emisión de la póliza; un rechazo no dispara nada.
func applyPayment(now func() time.Time) ApplyPaymentFunc {
	return func(o *order.Order, ev PaymentEvent) (order.Event, []Command, error) {
		applied, err := o.ApplyPayment(ev.Status, now())
		if err != nil || o.Status != order.Paid {
			return applied, nil, err
		}
		return applied, []Command{{Type: CommandIssuePolicy, OrderID: o.ID}}, nil
	}
}

// requestHash es la huella de la solicitud normalizada: detecta una clave reutilizada con otros datos.
func requestHash(o order.Order) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d|%s", o.Plate.Raw(), o.DocumentID.Raw(), o.VehicleYear, o.CityCode))
	return hex.EncodeToString(sum[:])
}
