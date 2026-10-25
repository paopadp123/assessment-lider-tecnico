package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// CommandOutcome le dice al worker qué hacer con el mensaje después de procesarlo.
type CommandOutcome string

const (
	// CommandDone: terminado (con éxito, rechazo de negocio o porque ya se había procesado); se confirma (ack).
	CommandDone CommandOutcome = "DONE"
	// CommandRetry: falla transitoria; se reprograma con backoff.
	CommandRetry CommandOutcome = "RETRY"
	// CommandDeadLetter: agotó los reintentos o no se puede procesar; va a la DLQ.
	CommandDeadLetter CommandOutcome = "DEAD_LETTER"
	// CommandDeferred: el tercero no está disponible (circuit breaker abierto) y no se intentó; se reprograma
	// tras Delay sin consumir un intento, para que una caída larga no mande las órdenes a la DLQ.
	CommandDeferred CommandOutcome = "DEFERRED"
)

// CommandResult es el resultado de procesar un comando, con el motivo para logs y headers.
type CommandResult struct {
	Outcome CommandOutcome
	Reason  string
	Delay   time.Duration // solo para CommandDeferred
}

// Delivery es un comando recibido del broker.
type Delivery struct {
	MessageID string
	OrderID   order.OrderID
	Attempt   int // empieza en 1
}

// RequestPayment procesa el comando request_payment: crea la intención de pago en el PSP.
//
// Es idempotente por estado: solo actúa si la orden sigue en QUOTED. Una segunda entrega del mismo
// mensaje (el broker entrega al menos una vez) encuentra la orden ya avanzada y termina sin efecto.
// Si el proceso murió después de que el PSP aceptara, la reentrega envía la misma Idempotency-Key,
// el PSP devuelve el mismo paymentId y la orden avanza sin un segundo cobro.
type RequestPayment struct {
	repo           OrderRepository
	dead           DeadLetterStore
	psp            PaymentGateway
	log            *slog.Logger
	now            func() time.Time
	maxAttempts    int
	attemptTimeout time.Duration
}

// NewRequestPayment arma el caso de uso.
func NewRequestPayment(repo OrderRepository, dead DeadLetterStore, psp PaymentGateway, log *slog.Logger) *RequestPayment {
	return &RequestPayment{repo: repo, dead: dead, psp: psp, log: log, now: utcMicros, maxAttempts: 4, attemptTimeout: 4 * time.Second}
}

// MaxAttempts es el número de intentos antes de enviar el comando a la DLQ.
func (uc *RequestPayment) MaxAttempts() int { return uc.maxAttempts }

// Execute procesa una entrega del comando. Los errores de infraestructura (base de datos) se devuelven
// como reintento: el mensaje nunca se pierde por una caída temporal de la base.
func (uc *RequestPayment) Execute(ctx context.Context, d Delivery) CommandResult {
	log := trace.Logger(ctx, uc.log).With("order_id", d.OrderID, "message_id", d.MessageID, "attempt", d.Attempt)

	o, err := uc.repo.Get(ctx, d.OrderID)
	if errors.Is(err, ErrOrderNotFound) {
		return uc.deadLetter(ctx, log, d, ReasonOrderNotFound, err)
	}
	if err != nil {
		return retry(log, "load order", err)
	}
	if o.Status != order.Quoted {
		log.Info("payment request already processed", "status", o.Status)
		return CommandResult{Outcome: CommandDone}
	}

	attemptCtx, cancel := context.WithTimeout(ctx, uc.attemptTimeout)
	paymentID, err := uc.psp.CreatePayment(attemptCtx, PaymentRequest{
		OrderID: o.ID, QuoteID: o.Quote.ID, Amount: o.Quote.Premium, Currency: o.Quote.Currency,
	})
	cancel()

	switch {
	case err == nil:
		return uc.markPending(ctx, log, o, paymentID)
	case errors.Is(err, ErrPaymentRequestRejected):
		log.Error("payment request rejected", "error", err)
		return uc.fail(ctx, log, o, ReasonPaymentRequestRejected, CommandDone)
	case d.Attempt < uc.maxAttempts:
		return retry(log, "payment request failed", err)
	default:
		return uc.deadLetter(ctx, log, d, ReasonPaymentUnavailable, err)
	}
}

func (uc *RequestPayment) markPending(ctx context.Context, log *slog.Logger, o order.Order, paymentID order.PaymentID) CommandResult {
	o.PaymentID = paymentID
	ev, err := o.Transition(order.PaymentPending, "payment requested", uc.now())
	if err != nil {
		return retry(log, "transition to payment pending", err)
	}
	final, err := uc.repo.MarkPaymentPending(ctx, o, ev, applyPayment(uc.now))
	if errors.Is(err, order.ErrInvalidTransition) {
		log.Info("order already advanced by another delivery")
		return CommandResult{Outcome: CommandDone}
	}
	if err != nil {
		return retry(log, "mark payment pending", err)
	}
	log.Info("payment requested", "payment_id", paymentID, "status", final.Status)
	return CommandResult{Outcome: CommandDone}
}

// deadLetter registra el comando agotado y, si la orden existe, la deja en FAILED para operaciones.
// Ambos pasos son idempotentes: una reentrega después de una caída no duplica nada.
func (uc *RequestPayment) deadLetter(ctx context.Context, log *slog.Logger, d Delivery, reason string, cause error) CommandResult {
	log.Error("payment request exhausted", "reason", reason, "error", cause)
	dl := DeadLetter{MessageID: d.MessageID, Type: CommandRequestPayment, OrderID: d.OrderID, Attempts: d.Attempt, Reason: reason}
	if err := uc.dead.RecordDeadLetter(ctx, dl); err != nil {
		return retry(log, "record dead letter", err)
	}
	if reason == ReasonOrderNotFound {
		return CommandResult{Outcome: CommandDeadLetter, Reason: reason}
	}
	o, err := uc.repo.Get(ctx, d.OrderID)
	if err != nil {
		return retry(log, "load order", err)
	}
	if o.Status != order.Quoted {
		return CommandResult{Outcome: CommandDeadLetter, Reason: reason}
	}
	return uc.fail(ctx, log, o, reason, CommandDeadLetter)
}

func (uc *RequestPayment) fail(ctx context.Context, log *slog.Logger, o order.Order, reason string, outcome CommandOutcome) CommandResult {
	o.FailureReason = reason
	_, err := transition(ctx, uc.repo, uc.now, o, order.Failed, reason)
	if err != nil && !errors.Is(err, order.ErrInvalidTransition) {
		return retry(log, "fail order", err)
	}
	return CommandResult{Outcome: outcome, Reason: reason}
}

func retry(log *slog.Logger, step string, err error) CommandResult {
	log.Warn("command will be retried", "step", step, "error", err)
	return CommandResult{Outcome: CommandRetry, Reason: fmt.Sprintf("%s: %v", step, err)}
}
