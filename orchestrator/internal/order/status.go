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

// CanTransitionTo indica si el flujo permite pasar de s a next.
func (s Status) CanTransitionTo(next Status) bool { return false }

// IsTerminal indica si la orden ya no admite cambios de estado.
func (s Status) IsTerminal() bool { return false }
