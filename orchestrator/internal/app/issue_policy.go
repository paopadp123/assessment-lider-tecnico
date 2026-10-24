package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// IssuePolicy procesa el comando issue_policy: emite la póliza de una orden pagada a través de la ACL.
//
// Es idempotente por estado y nunca emite a ciegas: una orden que ya estaba en ISSUING (reentrega, o
// reintento tras un timeout) primero se consulta por externalRef, porque un timeout del emisor no prueba
// que la póliza no se emitió. Solo si la consulta confirma que no existe, se vuelve a emitir.
type IssuePolicy struct {
	repo           OrderRepository
	dead           DeadLetterStore
	issuer         PolicyIssuer
	log            *slog.Logger
	now            func() time.Time
	maxAttempts    int
	attemptTimeout time.Duration
	deferDelay     time.Duration
}

// NewIssuePolicy arma el caso de uso. deferDelay es la espera antes de volver a intentar cuando el
// circuit breaker está abierto; conviene que coincida con el tiempo que el breaker permanece abierto.
// El timeout por intento supera los 8 s que puede tardar el emisor legado en responder.
func NewIssuePolicy(repo OrderRepository, dead DeadLetterStore, issuer PolicyIssuer, log *slog.Logger, deferDelay time.Duration) *IssuePolicy {
	return &IssuePolicy{repo: repo, dead: dead, issuer: issuer, log: log, now: utcMicros,
		maxAttempts: 4, attemptTimeout: 10 * time.Second, deferDelay: deferDelay}
}

// MaxAttempts es el número de intentos antes de enviar el comando a la DLQ.
func (uc *IssuePolicy) MaxAttempts() int { return uc.maxAttempts }

// Execute procesa una entrega del comando. Los errores de base de datos se reintentan.
func (uc *IssuePolicy) Execute(ctx context.Context, d Delivery) CommandResult {
	log := uc.log.With("order_id", d.OrderID, "message_id", d.MessageID, "attempt", d.Attempt)

	o, err := uc.repo.Get(ctx, d.OrderID)
	if errors.Is(err, ErrOrderNotFound) {
		return uc.deadLetter(ctx, log, d, ReasonOrderNotFound, err)
	}
	if err != nil {
		return retry(log, "load order", err)
	}

	switch o.Status {
	case order.Paid:
		o, err = transition(ctx, uc.repo, uc.now, o, order.Issuing, "policy issuance started")
		if errors.Is(err, order.ErrInvalidTransition) {
			log.Info("policy issuance already started by another delivery")
			return CommandResult{Outcome: CommandDone}
		}
		if err != nil {
			return retry(log, "transition to issuing", err)
		}
		return uc.issue(ctx, log, d, o)
	case order.Issuing:
		return uc.verifyThenIssue(ctx, log, d, o)
	case order.Issued, order.RefundRequired:
		log.Info("policy issuance already processed", "status", o.Status)
		return CommandResult{Outcome: CommandDone}
	default:
		// Una orden que nunca llegó a PAID no se emite; si aparece aquí, el comando se encoló mal.
		log.Warn("policy issuance command for an unpaid order ignored", "status", o.Status)
		return CommandResult{Outcome: CommandDone}
	}
}

// verifyThenIssue consulta por externalRef antes de emitir: si la póliza ya existe, solo se registra.
func (uc *IssuePolicy) verifyThenIssue(ctx context.Context, log *slog.Logger, d Delivery, o order.Order) CommandResult {
	attemptCtx, cancel := context.WithTimeout(ctx, uc.attemptTimeout)
	number, err := uc.issuer.FindByExternalRef(attemptCtx, o.ID)
	cancel()

	switch {
	case err == nil:
		log.Info("policy found by external reference; not issuing again")
		return uc.markIssued(ctx, log, o, number)
	case errors.Is(err, ErrPolicyNotFound):
		return uc.issue(ctx, log, d, o)
	default:
		return uc.unavailable(ctx, log, d, "policy lookup failed", err)
	}
}

func (uc *IssuePolicy) issue(ctx context.Context, log *slog.Logger, d Delivery, o order.Order) CommandResult {
	attemptCtx, cancel := context.WithTimeout(ctx, uc.attemptTimeout)
	number, err := uc.issuer.Issue(attemptCtx, IssueRequest{
		ExternalRef: o.ID,
		QuoteID:     o.Quote.ID,
		Premium:     o.Quote.Premium,
		Currency:    o.Quote.Currency,
		Plate:       o.Plate,
		DocumentID:  o.DocumentID,
		VehicleYear: o.VehicleYear,
		CityCode:    o.CityCode,
	})
	cancel()

	switch {
	case err == nil:
		return uc.markIssued(ctx, log, o, number)
	case errors.Is(err, ErrIssueRejected):
		return uc.requireRefund(ctx, log, o, err)
	default:
		return uc.unavailable(ctx, log, d, "policy issue failed", err)
	}
}

// unavailable decide qué hacer cuando la capa de emisión no respondió. Con el breaker abierto no se
// consume un intento: la orden espera en ISSUING el tiempo que el breaker tarda en volver a probar.
func (uc *IssuePolicy) unavailable(ctx context.Context, log *slog.Logger, d Delivery, step string, err error) CommandResult {
	switch {
	case errors.Is(err, ErrIssuerCircuitOpen):
		log.Warn("policy issuance deferred", "reason", ReasonIssuerUnavailable, "retry_in", uc.deferDelay, "error", err)
		return CommandResult{Outcome: CommandDeferred, Reason: ReasonIssuerUnavailable, Delay: uc.deferDelay}
	case d.Attempt < uc.maxAttempts:
		return retry(log, step, err)
	default:
		return uc.deadLetter(ctx, log, d, ReasonIssuerUnavailable, err)
	}
}

func (uc *IssuePolicy) markIssued(ctx context.Context, log *slog.Logger, o order.Order, number order.PolicyNumber) CommandResult {
	o.PolicyNumber = number
	_, err := transition(ctx, uc.repo, uc.now, o, order.Issued, "policy issued")
	if errors.Is(err, order.ErrInvalidTransition) {
		log.Info("order already advanced by another delivery")
		return CommandResult{Outcome: CommandDone}
	}
	if err != nil {
		// La póliza ya existe en el emisor: el reintento la encuentra por externalRef y no la reemite.
		return retry(log, "mark issued", err)
	}
	log.Info("policy issued", "policy_number", number)
	return CommandResult{Outcome: CommandDone}
}

// requireRefund deja la orden en REFUND_REQUIRED para que operaciones decida el reverso.
// No se reembolsa automáticamente (ver README).
func (uc *IssuePolicy) requireRefund(ctx context.Context, log *slog.Logger, o order.Order, cause error) CommandResult {
	log.Error("policy issue rejected; refund required", "reason", ReasonIssueRejected, "error", cause)
	o.FailureReason = ReasonIssueRejected
	_, err := transition(ctx, uc.repo, uc.now, o, order.RefundRequired, ReasonIssueRejected)
	if err != nil && !errors.Is(err, order.ErrInvalidTransition) {
		return retry(log, "require refund", err)
	}
	return CommandResult{Outcome: CommandDone, Reason: ReasonIssueRejected}
}

// deadLetter registra el comando agotado. La orden se deja en ISSUING a propósito: los reintentos
// agotados por timeouts no prueban que la póliza no se emitió; la reconciliación consulta y decide.
func (uc *IssuePolicy) deadLetter(ctx context.Context, log *slog.Logger, d Delivery, reason string, cause error) CommandResult {
	log.Error("policy issuance exhausted", "reason", reason, "error", cause)
	dl := DeadLetter{MessageID: d.MessageID, Type: CommandIssuePolicy, OrderID: d.OrderID, Attempts: d.Attempt, Reason: reason}
	if err := uc.dead.RecordDeadLetter(ctx, dl); err != nil {
		return retry(log, "record dead letter", err)
	}
	return CommandResult{Outcome: CommandDeadLetter, Reason: reason}
}
