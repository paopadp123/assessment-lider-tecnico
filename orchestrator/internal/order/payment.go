package order

import (
	"errors"
	"fmt"
	"time"
)

// PaymentID identifica un pago en la pasarela (PSP).
type PaymentID string

// PaymentStatus es el resultado definitivo de un pago informado por el PSP.
type PaymentStatus string

const (
	PaymentApproved PaymentStatus = "APPROVED"
	PaymentRejected PaymentStatus = "REJECTED"
)

// ReasonPaymentRejected es el motivo de falla cuando el PSP rechaza el pago.
const ReasonPaymentRejected = "PAYMENT_REJECTED"

// ErrPaymentConflict indica un resultado de pago que no corresponde al estado de la orden
// (p. ej. un rechazo que llega después de que la orden quedó pagada). No cambia la orden:
// operaciones lo revisa como anomalía.
var ErrPaymentConflict = errors.New("payment result conflicts with order status")

// ParsePaymentStatus valida el estado de pago recibido del PSP.
func ParsePaymentStatus(s string) (PaymentStatus, error) {
	switch st := PaymentStatus(s); st {
	case PaymentApproved, PaymentRejected:
		return st, nil
	default:
		return "", fmt.Errorf("invalid payment status %q", s)
	}
}

// ApplyPayment lleva la orden al estado que corresponde al resultado del pago.
// Solo una orden que espera el pago (PAYMENT_PENDING) puede cambiar; cualquier otro caso es ErrPaymentConflict.
func (o *Order) ApplyPayment(result PaymentStatus, now time.Time) (Event, error) {
	if o.Status != PaymentPending {
		return Event{}, fmt.Errorf("%w: %s received while order is %s", ErrPaymentConflict, result, o.Status)
	}
	if result == PaymentRejected {
		ev, err := o.Transition(Failed, ReasonPaymentRejected, now)
		if err == nil {
			o.FailureReason = ReasonPaymentRejected
		}
		return ev, err
	}
	return o.Transition(Paid, "payment approved", now)
}
