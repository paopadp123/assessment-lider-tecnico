package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
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
)

// CommandResult es el resultado de procesar un comando, con el motivo para logs y headers.
type CommandResult struct {
	Outcome CommandOutcome
	Reason  string
}

// Delivery es un comando recibido del broker.
type Delivery struct {
	MessageID string
	OrderID   order.OrderID
	Attempt   int // empieza en 1
}

// RequestPayment procesa el comando request_payment: crea la intención de pago en el PSP.
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

// Execute procesa una entrega del comando.
func (uc *RequestPayment) Execute(ctx context.Context, d Delivery) CommandResult {
	return CommandResult{}
}
