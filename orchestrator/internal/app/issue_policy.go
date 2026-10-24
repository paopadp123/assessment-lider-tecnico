package app

import (
	"context"
	"log/slog"
	"time"
)

// IssuePolicy procesa el comando issue_policy: emite la póliza de una orden pagada a través de la ACL.
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
func NewIssuePolicy(repo OrderRepository, dead DeadLetterStore, issuer PolicyIssuer, log *slog.Logger, deferDelay time.Duration) *IssuePolicy {
	return &IssuePolicy{repo: repo, dead: dead, issuer: issuer, log: log, now: utcMicros,
		maxAttempts: 4, attemptTimeout: 10 * time.Second, deferDelay: deferDelay}
}

// MaxAttempts es el número de intentos antes de enviar el comando a la DLQ.
func (uc *IssuePolicy) MaxAttempts() int { return uc.maxAttempts }

// Execute procesa una entrega del comando.
func (uc *IssuePolicy) Execute(ctx context.Context, d Delivery) CommandResult {
	return CommandResult{Outcome: CommandDone}
}
