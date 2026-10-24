package order

import (
	"errors"
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

// ErrPaymentConflict indica un resultado de pago que no corresponde al estado de la orden.
var ErrPaymentConflict = errors.New("payment result conflicts with order status")

// ParsePaymentStatus valida el estado de pago recibido del PSP.
func ParsePaymentStatus(s string) (PaymentStatus, error) { return "", nil }

// ApplyPayment lleva la orden al estado que corresponde al resultado del pago.
func (o *Order) ApplyPayment(result PaymentStatus, now time.Time) (Event, error) {
	return Event{}, nil
}
