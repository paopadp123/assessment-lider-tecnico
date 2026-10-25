package app

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// retryPolicy define cuántas veces y con qué ritmo se reintenta una llamada a un tercero.
type retryPolicy struct {
	maxAttempts    int
	attemptTimeout time.Duration
	baseBackoff    time.Duration
}

// withRetry ejecuta call hasta maxAttempts veces mientras el error sea transitorio.
// Entre intentos espera un backoff exponencial con jitter completo (al azar entre 0 y base·2ⁿ)
// para que muchas órdenes que fallan juntas no reintenten al mismo tiempo.
// Devuelve el resultado, cuántos intentos se usaron y el último error.
func withRetry[T any](
	ctx context.Context,
	p retryPolicy,
	sleep func(context.Context, time.Duration) error,
	isTransient func(error) bool,
	call func(context.Context) (T, error),
) (T, int, error) {
	var zero T
	var lastErr error
	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, p.attemptTimeout)
		result, err := call(attemptCtx)
		cancel()
		if err == nil {
			return result, attempt, nil
		}
		lastErr = err
		if !isTransient(err) {
			return zero, attempt, err
		}
		if attempt < p.maxAttempts {
			if err := sleep(ctx, Backoff(p.baseBackoff, attempt)); err != nil {
				return zero, attempt, err
			}
		}
	}
	return zero, p.maxAttempts, fmt.Errorf("after %d attempts: %w", p.maxAttempts, lastErr)
}

// Backoff es la espera antes del reintento que sigue al intento attempt (desde 1):
// backoff exponencial con jitter completo, un valor al azar en [0, base·2^(attempt-1)).
// El tope de 2^20 evita desbordes si attempt crece sin control.
func Backoff(base time.Duration, attempt int) time.Duration {
	exp := min(max(attempt-1, 0), 20)
	return rand.N(base << exp)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
