package app

import (
	"testing"
	"time"
)

// El jitter completo debe quedar siempre dentro de [0, base·2^(n-1)) y repartirse en ese rango:
// si todos los valores fueran iguales, los reintentos volverían a llegar juntos.
func TestBackoff_FullJitterStaysWithinExponentialBound(t *testing.T) {
	const base = 100 * time.Millisecond
	for attempt := 1; attempt <= 4; attempt++ {
		limit := base << (attempt - 1)
		seen := map[time.Duration]bool{}
		for range 500 {
			d := Backoff(base, attempt)
			if d < 0 || d >= limit {
				t.Fatalf("intento %d: %v fuera de [0, %v)", attempt, d, limit)
			}
			seen[d] = true
		}
		if len(seen) < 50 {
			t.Fatalf("intento %d: solo %d valores distintos; el jitter no reparte la carga", attempt, len(seen))
		}
	}
}

func TestBackoff_CapsTheExponent(t *testing.T) {
	if d := Backoff(time.Millisecond, 1_000); d >= time.Millisecond<<21 {
		t.Fatalf("sin tope: %v", d)
	}
}
