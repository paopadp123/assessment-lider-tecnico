// Package app contiene los casos de uso del orquestador y define los puertos (interfaces)
// que implementan los adaptadores de base de datos, HTTP y terceros.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

var (
	// ErrQuoteUnavailable indica una falla transitoria del cotizador (5xx, timeout, respuesta inválida): se reintenta.
	ErrQuoteUnavailable = errors.New("quote service unavailable")
	// ErrQuoteRejected indica que el cotizador rechazó la solicitud (4xx): reintentar no cambia el resultado.
	ErrQuoteRejected = errors.New("quote rejected")
	// ErrPaymentUnavailable indica una falla transitoria del PSP al crear la intención de pago: se reintenta.
	ErrPaymentUnavailable = errors.New("payment service unavailable")
	// ErrPaymentRequestRejected indica que el PSP rechazó la intención de pago (4xx).
	ErrPaymentRequestRejected = errors.New("payment request rejected")
	// ErrIdempotencyConflict indica una Idempotency-Key reutilizada con una solicitud distinta.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	// ErrOrderNotFound indica que la orden no existe.
	ErrOrderNotFound = errors.New("order not found")
)

// QuoteRequest son los datos que exige el cotizador de la aseguradora.
type QuoteRequest struct {
	Plate       string
	VehicleYear int
	CityCode    string
	DocumentID  string
}

// Quoter es el puerto hacia el cotizador de la aseguradora.
type Quoter interface {
	Quote(ctx context.Context, req QuoteRequest) (order.Quote, error)
}

// PaymentRequest es la intención de pago que se envía al PSP.
type PaymentRequest struct {
	OrderID  order.OrderID // también se usa como clave de idempotencia ante el PSP
	QuoteID  string
	Amount   string
	Currency string
}

// PaymentGateway es el puerto hacia la pasarela de pagos (PSP).
type PaymentGateway interface {
	CreatePayment(ctx context.Context, req PaymentRequest) (order.PaymentID, error)
}

// PaymentEvent es un resultado de pago recibido por webhook, ya con la firma verificada.
type PaymentEvent struct {
	PaymentID  order.PaymentID
	Status     order.PaymentStatus
	OccurredAt time.Time
}

// PaymentOutcome describe qué pasó con un evento de pago recibido.
type PaymentOutcome string

const (
	// PaymentApplied: la orden cambió de estado con este evento.
	PaymentApplied PaymentOutcome = "APPLIED"
	// PaymentDuplicate: el evento ya se había recibido; no tuvo efecto.
	PaymentDuplicate PaymentOutcome = "DUPLICATE"
	// PaymentPendingOrder: la orden aún no espera este pago; el evento queda guardado y se aplica después.
	PaymentPendingOrder PaymentOutcome = "PENDING_ORDER"
	// PaymentAnomaly: el evento contradice el estado de la orden; se guarda para operaciones y no cambia nada.
	PaymentAnomaly PaymentOutcome = "ANOMALY"
)

// PaymentEventResult es el resultado de registrar un evento de pago.
type PaymentEventResult struct {
	Outcome PaymentOutcome
	OrderID order.OrderID // vacío si la orden aún no se conoce
}

// ApplyPaymentFunc decide el cambio de estado de una orden ante un evento de pago.
// Devuelve order.ErrPaymentConflict si el evento no corresponde al estado de la orden.
type ApplyPaymentFunc func(o *order.Order, ev PaymentEvent) (order.Event, error)

// IdempotencyKey asocia la clave enviada por el cliente con la huella de su solicitud.
type IdempotencyKey struct {
	Key         string
	RequestHash string
}

// OrderRepository es el puerto de persistencia de órdenes.
type OrderRepository interface {
	// Create guarda en una sola transacción la orden, su evento inicial y la clave de idempotencia.
	// Si la clave ya existe con la misma huella, devuelve la orden existente y created=false.
	// Si existe con otra huella, devuelve ErrIdempotencyConflict.
	Create(ctx context.Context, key IdempotencyKey, o order.Order, ev order.Event) (saved order.Order, created bool, err error)
	// SaveTransition guarda el nuevo estado de la orden (con su cotización o motivo de falla) y su evento en una transacción.
	SaveTransition(ctx context.Context, o order.Order, ev order.Event) error
	// MarkPaymentPending guarda PAYMENT_PENDING con su paymentId y, en la misma transacción y bajo un lock por pago,
	// aplica con apply los eventos de ese pago que hayan llegado antes. Devuelve la orden final.
	MarkPaymentPending(ctx context.Context, o order.Order, ev order.Event, apply ApplyPaymentFunc) (order.Order, error)
	// RecordPaymentEvent guarda el evento (los duplicados no tienen efecto) y, bajo el mismo lock por pago,
	// lo aplica con apply si la orden ya espera ese pago.
	RecordPaymentEvent(ctx context.Context, ev PaymentEvent, apply ApplyPaymentFunc) (PaymentEventResult, error)
	// Get devuelve una orden por id o ErrOrderNotFound.
	Get(ctx context.Context, id order.OrderID) (order.Order, error)
}
