package issuer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// BreakerSettings configura el circuit breaker hacia la capa de emisión.
type BreakerSettings struct {
	FailureThreshold uint32        // fallas transitorias seguidas que abren el circuito
	OpenTimeout      time.Duration // tiempo abierto antes de dejar pasar una llamada de prueba (half-open)
}

// Breaker envuelve un app.PolicyIssuer con un circuit breaker. Emisión y consulta comparten el circuito:
// van al mismo servicio, y si está caído, tampoco tiene sentido consultarlo.
//
// Cerrado: las llamadas pasan. Tras FailureThreshold fallas transitorias seguidas se abre y responde
// ErrIssuerCircuitOpen sin llamar, lo que le da aire al emisor para recuperarse. Pasado OpenTimeout
// queda medio abierto (half-open): deja pasar una llamada de prueba; si sale bien se cierra, si falla
// se vuelve a abrir.
type Breaker struct {
	inner app.PolicyIssuer
	cb    *gobreaker.CircuitBreaker[order.PolicyNumber]
}

var _ app.PolicyIssuer = (*Breaker)(nil)

// NewBreaker arma el decorador. Los cambios de estado se registran: son la señal para alertar.
func NewBreaker(inner app.PolicyIssuer, s BreakerSettings, log *slog.Logger) *Breaker {
	cb := gobreaker.NewCircuitBreaker[order.PolicyNumber](gobreaker.Settings{
		Name:        "policy-issuer",
		MaxRequests: 1,
		Timeout:     s.OpenTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= s.FailureThreshold },
		// Un rechazo de negocio o una póliza inexistente son respuestas sanas: la ACL contestó.
		IsSuccessful: func(err error) bool {
			return err == nil || errors.Is(err, app.ErrIssueRejected) || errors.Is(err, app.ErrPolicyNotFound)
		},
		// Una cancelación propia (p. ej. el apagado del proceso) no dice nada de la salud de la ACL.
		IsExcluded: func(err error) bool { return errors.Is(err, context.Canceled) },
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed", "breaker", name, "from", from.String(), "to", to.String())
		},
	})
	return &Breaker{inner: inner, cb: cb}
}

// Status es el estado actual del breaker, para operación.
type Status struct {
	Name                string
	State               string
	ConsecutiveFailures uint32
}

// Status devuelve el estado actual del breaker.
func (b *Breaker) Status() Status {
	return Status{Name: b.cb.Name(), State: b.cb.State().String(), ConsecutiveFailures: b.cb.Counts().ConsecutiveFailures}
}

// Issue emite a través del breaker.
func (b *Breaker) Issue(ctx context.Context, req app.IssueRequest) (order.PolicyNumber, error) {
	return b.call(func() (order.PolicyNumber, error) { return b.inner.Issue(ctx, req) })
}

// FindByExternalRef consulta a través del breaker.
func (b *Breaker) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	return b.call(func() (order.PolicyNumber, error) { return b.inner.FindByExternalRef(ctx, ref) })
}

// call traduce el rechazo del breaker (abierto, o half-open con la llamada de prueba en curso) al error del puerto.
func (b *Breaker) call(fn func() (order.PolicyNumber, error)) (order.PolicyNumber, error) {
	n, err := b.cb.Execute(fn)
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return "", fmt.Errorf("%w: %w", app.ErrIssuerCircuitOpen, err)
	}
	return n, err
}
