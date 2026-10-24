package order

import (
	"errors"
	"testing"
	"time"
)

var allStatuses = []Status{Created, Quoted, PaymentPending, Paid, Issuing, Issued, Failed, RefundRequired}

// allowed es la especificación del flujo de compra, escrita de forma independiente a la implementación.
var allowed = map[[2]Status]bool{
	{Created, Quoted}:         true,
	{Created, Failed}:         true,
	{Quoted, PaymentPending}:  true,
	{Quoted, Failed}:          true,
	{PaymentPending, Paid}:    true,
	{PaymentPending, Failed}:  true,
	{Paid, Issuing}:           true,
	{Paid, RefundRequired}:    true,
	{Issuing, Issued}:         true,
	{Issuing, RefundRequired}: true,
	// Operaciones puede reintentar la emisión de una orden marcada para reembolso, y si la póliza
	// aparece (p. ej. el emisor terminó de emitir tarde) la verdad es ISSUED: aún no se reembolsó nada.
	{RefundRequired, Issuing}: true,
	{RefundRequired, Issued}:  true,
}

func TestCanTransitionTo_AllCombinations(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := allowed[[2]Status{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestTerminalStatuses(t *testing.T) {
	for _, s := range []Status{Issued, Failed} {
		if !s.IsTerminal() {
			t.Errorf("%s debería ser terminal", s)
		}
	}
	for _, s := range []Status{Created, Quoted, PaymentPending, Paid, Issuing, RefundRequired} {
		if s.IsTerminal() {
			t.Errorf("%s no debería ser terminal", s)
		}
	}
}

func TestTransition_ValidReturnsEventAndUpdatesOrder(t *testing.T) {
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	o := Order{Status: Created}

	ev, err := o.Transition(Quoted, "quote received", now)

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if o.Status != Quoted || !o.UpdatedAt.Equal(now) {
		t.Fatalf("orden no actualizada: status=%s updatedAt=%v", o.Status, o.UpdatedAt)
	}
	if ev.From != Created || ev.To != Quoted || ev.Reason != "quote received" || !ev.OccurredAt.Equal(now) {
		t.Fatalf("evento incorrecto: %+v", ev)
	}
}

func TestTransition_InvalidIsRejectedAndOrderUnchanged(t *testing.T) {
	o := Order{Status: Created}

	_, err := o.Transition(Issued, "skip", time.Now())

	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if o.Status != Created {
		t.Fatalf("la orden cambió a %s tras una transición inválida", o.Status)
	}
}
