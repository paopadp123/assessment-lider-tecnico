package issuer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// scriptedIssuer devuelve el error configurado (nil = éxito) y cuenta las llamadas que le llegan.
type scriptedIssuer struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (s *scriptedIssuer) set(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *scriptedIssuer) Issue(context.Context, app.IssueRequest) (order.PolicyNumber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return "POL-1", nil
}

func (s *scriptedIssuer) FindByExternalRef(context.Context, order.OrderID) (order.PolicyNumber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return "POL-1", nil
}

const (
	threshold   = 3
	openTimeout = 100 * time.Millisecond
)

// newTestBreaker deja el circuito abierto una hora: con un tiempo corto, una pausa de la máquina entre llamadas
// lo pasaría a half-open y la prueba fallaría sin que el código esté mal (pasó en la suite completa).
func newTestBreaker(inner app.PolicyIssuer) *Breaker { return newTimedBreaker(inner, time.Hour) }

// newTimedBreaker es para las pruebas de half-open, que necesitan que el tiempo abierto venza.
func newTimedBreaker(inner app.PolicyIssuer, open time.Duration) *Breaker {
	return NewBreaker(inner, BreakerSettings{FailureThreshold: threshold, OpenTimeout: open},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func issue(b *Breaker) error {
	_, err := b.Issue(context.Background(), app.IssueRequest{ExternalRef: ref})
	return err
}

// Regla crítica: tras N fallas transitorias seguidas el circuito se abre y deja de llamar a la ACL.
func TestBreaker_OpensAfterConsecutiveTransientFailures(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTestBreaker(inner)
	for range threshold {
		if err := issue(b); !errors.Is(err, app.ErrIssuerUnavailable) {
			t.Fatalf("antes de abrir: err = %v", err)
		}
	}

	err := issue(b)

	if !errors.Is(err, app.ErrIssuerCircuitOpen) {
		t.Fatalf("err = %v, want ErrIssuerCircuitOpen", err)
	}
	if inner.calls != threshold {
		t.Fatalf("llamadas a la ACL = %d; con el circuito abierto no debe llamarse", inner.calls)
	}
}

// Los timeouts también cuentan como falla del tercero.
func TestBreaker_TimeoutsCountAsFailures(t *testing.T) {
	inner := &scriptedIssuer{err: context.DeadlineExceeded}
	b := newTestBreaker(inner)
	for range threshold {
		_ = issue(b)
	}

	if err := issue(b); !errors.Is(err, app.ErrIssuerCircuitOpen) {
		t.Fatalf("err = %v, want ErrIssuerCircuitOpen", err)
	}
}

// Un rechazo de negocio o una póliza inexistente son respuestas sanas de la ACL: no abren el circuito.
func TestBreaker_BusinessAnswersDoNotOpen(t *testing.T) {
	for _, answer := range []error{app.ErrIssueRejected, app.ErrPolicyNotFound} {
		inner := &scriptedIssuer{err: answer}
		b := newTestBreaker(inner)
		for range threshold * 2 {
			_ = issue(b)
		}

		if err := issue(b); !errors.Is(err, answer) {
			t.Fatalf("err = %v, want %v (el circuito no debía abrirse)", err, answer)
		}
	}
}

// Un éxito reinicia la cuenta: solo abren las fallas seguidas.
func TestBreaker_SuccessResetsConsecutiveFailures(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTestBreaker(inner)
	for range threshold - 1 {
		_ = issue(b)
	}
	inner.set(nil)
	_ = issue(b)
	inner.set(app.ErrIssuerUnavailable)
	for range threshold - 1 {
		_ = issue(b)
	}

	if err := issue(b); errors.Is(err, app.ErrIssuerCircuitOpen) {
		t.Fatal("el circuito se abrió sin fallas seguidas suficientes")
	}
}

// Pasado el tiempo abierto, deja pasar una llamada de prueba (half-open); si sale bien, se cierra.
func TestBreaker_HalfOpenProbeClosesOnSuccess(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTimedBreaker(inner, openTimeout)
	for range threshold {
		_ = issue(b)
	}
	inner.set(nil)
	time.Sleep(openTimeout + 20*time.Millisecond)

	if err := issue(b); err != nil {
		t.Fatalf("llamada de prueba: err = %v", err)
	}
	if err := issue(b); err != nil {
		t.Fatalf("tras cerrarse: err = %v", err)
	}
}

// Si la llamada de prueba falla, el circuito vuelve a abrirse de inmediato.
func TestBreaker_HalfOpenProbeReopensOnFailure(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTimedBreaker(inner, openTimeout)
	for range threshold {
		_ = issue(b)
	}
	time.Sleep(openTimeout + 20*time.Millisecond)
	_ = issue(b)
	calls := inner.calls

	err := issue(b)

	if !errors.Is(err, app.ErrIssuerCircuitOpen) || inner.calls != calls {
		t.Fatalf("err = %v, llamadas %d→%d", err, calls, inner.calls)
	}
}

// La consulta va por el mismo circuito: si la ACL está caída, tampoco se consulta.
func TestBreaker_LookupSharesTheCircuit(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTestBreaker(inner)
	for range threshold {
		_ = issue(b)
	}

	_, err := b.FindByExternalRef(context.Background(), ref)

	if !errors.Is(err, app.ErrIssuerCircuitOpen) {
		t.Fatalf("err = %v, want ErrIssuerCircuitOpen", err)
	}
}

// Cancelar el contexto propio (apagado del proceso) no es culpa de la ACL: no abre el circuito.
func TestBreaker_OwnCancellationDoesNotOpen(t *testing.T) {
	inner := &scriptedIssuer{err: context.Canceled}
	b := newTestBreaker(inner)
	for range threshold * 2 {
		_ = issue(b)
	}

	if err := issue(b); errors.Is(err, app.ErrIssuerCircuitOpen) {
		t.Fatal("una cancelación propia abrió el circuito")
	}
}

// Operación ve el estado del breaker: cerrado al inicio, abierto tras la caída.
func TestBreaker_StatusIsExposed(t *testing.T) {
	inner := &scriptedIssuer{err: app.ErrIssuerUnavailable}
	b := newTestBreaker(inner)
	if s := b.Status(); s.State != "closed" || s.Name != "policy-issuer" {
		t.Fatalf("estado inicial = %+v", s)
	}
	for range threshold - 1 {
		_ = issue(b)
	}
	if s := b.Status(); s.State != "closed" || s.ConsecutiveFailures != threshold-1 {
		t.Fatalf("antes de abrir = %+v, want closed con %d fallas", s, threshold-1)
	}
	_ = issue(b)

	// Al cambiar de estado, gobreaker reinicia los contadores: el estado abierto es la señal.
	if s := b.Status(); s.State != "open" {
		t.Fatalf("estado = %+v, want open", s)
	}
}
