// Package order contiene el dominio de la compra de pólizas: la orden, sus estados y sus reglas.
// No hace E/S: no conoce HTTP, base de datos ni terceros.
package order

// Status es el estado de una orden dentro del flujo de compra.
type Status string

const (
	Created        Status = "CREATED"
	Quoted         Status = "QUOTED"
	PaymentPending Status = "PAYMENT_PENDING"
	Paid           Status = "PAID"
	Issuing        Status = "ISSUING"
	Issued         Status = "ISSUED"
	Failed         Status = "FAILED"
	RefundRequired Status = "REFUND_REQUIRED"
)

// transitions es la única fuente de verdad del flujo. Una vez pagada, la orden nunca pasa a FAILED:
// si la emisión no se logra, queda en REFUND_REQUIRED para que operaciones decida el reverso.
var transitions = map[Status][]Status{
	Created:        {Quoted, Failed},
	Quoted:         {PaymentPending, Failed},
	PaymentPending: {Paid, Failed},
	Paid:           {Issuing, RefundRequired},
	Issuing:        {Issued, RefundRequired},
}

// CanTransitionTo indica si el flujo permite pasar de s a next.
func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// IsTerminal indica si la orden ya no admite cambios de estado.
func (s Status) IsTerminal() bool { return len(transitions[s]) == 0 }
