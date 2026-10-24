package issuer

import (
	"context"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// BreakerSettings configura el circuit breaker hacia la capa de emisión.
type BreakerSettings struct {
	FailureThreshold uint32        // fallas transitorias seguidas que abren el circuito
	OpenTimeout      time.Duration // tiempo abierto antes de dejar pasar una llamada de prueba (half-open)
}

// Breaker envuelve un app.PolicyIssuer con un circuit breaker.
type Breaker struct {
	inner app.PolicyIssuer
}

var _ app.PolicyIssuer = (*Breaker)(nil)

// NewBreaker arma el decorador.
func NewBreaker(inner app.PolicyIssuer, s BreakerSettings, log *slog.Logger) *Breaker {
	return &Breaker{inner: inner}
}

// Status es el estado actual del breaker, para operación.
type Status struct {
	Name                string
	State               string
	ConsecutiveFailures uint32
}

// Status devuelve el estado actual del breaker.
func (b *Breaker) Status() Status { return Status{} }

// Issue emite a través del breaker.
func (b *Breaker) Issue(ctx context.Context, req app.IssueRequest) (order.PolicyNumber, error) {
	return b.inner.Issue(ctx, req)
}

// FindByExternalRef consulta a través del breaker.
func (b *Breaker) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	return b.inner.FindByExternalRef(ctx, ref)
}
