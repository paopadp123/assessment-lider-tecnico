package order

import "time"

// Quote es la cotización vigente de la aseguradora para una orden.
type Quote struct {
	ID         string
	Premium    string // decimal exacto como texto, para no perder precisión con float
	Currency   string
	ValidUntil time.Time
}

// Order es una compra de póliza y su estado actual.
type Order struct {
	ID            string
	Status        Status
	Plate         Plate
	DocumentID    DocumentID
	VehicleYear   int
	CityCode      string
	Quote         *Quote
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Event registra un cambio de estado; alimenta la línea de tiempo de la orden.
type Event struct {
	From       Status
	To         Status
	Reason     string
	OccurredAt time.Time
}

// NewOrderInput son los datos de la solicitud de compra, sin validar.
type NewOrderInput struct {
	Plate       string
	DocumentID  string
	VehicleYear int
	CityCode    string
}

// NewOrder valida la solicitud y crea una orden en estado CREATED.
func NewOrder(in NewOrderInput) (Order, error) { return Order{}, nil }

// Transition cambia el estado de la orden si el flujo lo permite y devuelve el evento a registrar.
func (o *Order) Transition(to Status, reason string, now time.Time) (Event, error) {
	return Event{}, nil
}
