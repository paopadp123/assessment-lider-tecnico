package httpapi

import (
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

func ids(evs []app.StoredEvent) []int64 {
	out := make([]int64, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.ID)
	}
	return out
}

func evs(idList ...int64) []app.StoredEvent {
	out := make([]app.StoredEvent, 0, len(idList))
	for _, id := range idList {
		out = append(out, app.StoredEvent{ID: id})
	}
	return out
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Regla crítica (hallazgo del QA): los IDs se asignan al insertar, no al confirmar. Si la transacción del
// evento 3 confirma después que la del 4, el flujo no puede saltar el 3: espera a que aparezca.
func TestCursor_WaitsForGapToBeFilled(t *testing.T) {
	now := time.Now()
	c := &streamCursor{last: 0, grace: 10 * time.Second}

	first := c.take(evs(1, 2, 4), now)
	second := c.take(evs(3, 4), now.Add(time.Second))

	if !equal(ids(first), []int64{1, 2}) || !equal(ids(second), []int64{3, 4}) || c.last != 4 {
		t.Fatalf("primera=%v segunda=%v último=%d", ids(first), ids(second), c.last)
	}
}

// Un hueco que nunca se llena es un rollback: pasado el margen, el flujo sigue.
func TestCursor_SkipsGapAfterGrace(t *testing.T) {
	now := time.Now()
	c := &streamCursor{last: 2, grace: 10 * time.Second}

	held := c.take(evs(4, 5), now)
	still := c.take(evs(4, 5), now.Add(9*time.Second))
	released := c.take(evs(4, 5), now.Add(11*time.Second))

	if len(held) != 0 || len(still) != 0 || !equal(ids(released), []int64{4, 5}) || c.last != 5 {
		t.Fatalf("retenidos=%v %v liberados=%v último=%d", ids(held), ids(still), ids(released), c.last)
	}
}

// El margen se cuenta por hueco: después de saltar uno, un hueco nuevo vuelve a esperar.
func TestCursor_EachGapGetsItsOwnGrace(t *testing.T) {
	now := time.Now()
	c := &streamCursor{last: 0, grace: 10 * time.Second}
	c.take(evs(2), now)
	c.take(evs(2), now.Add(11*time.Second)) // salta el 1

	got := c.take(evs(4), now.Add(12*time.Second))

	if len(got) != 0 || c.last != 2 {
		t.Fatalf("entregó %v sin esperar el hueco del 3", ids(got))
	}
}
