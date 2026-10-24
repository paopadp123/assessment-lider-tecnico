// Package app contiene los casos de uso del orquestador y define los puertos (interfaces)
// que implementan los adaptadores de base de datos, HTTP y terceros.
package app

import (
	"context"
	"errors"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

var (
	// ErrQuoteUnavailable indica una falla transitoria del cotizador (5xx, timeout, respuesta inválida): se reintenta.
	ErrQuoteUnavailable = errors.New("quote service unavailable")
	// ErrQuoteRejected indica que el cotizador rechazó la solicitud (4xx): reintentar no cambia el resultado.
	ErrQuoteRejected = errors.New("quote rejected")
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
	// Get devuelve una orden por id o ErrOrderNotFound.
	Get(ctx context.Context, id order.OrderID) (order.Order, error)
}
