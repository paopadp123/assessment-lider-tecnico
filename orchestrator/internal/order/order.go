package order

import (
	"fmt"
	"regexp"
	"time"
)

const minVehicleYear = 1980

// Supuesto documentado: cityCode es el código DANE del municipio (5 dígitos, p. ej. 11001 = Bogotá).
var cityCodeFormat = regexp.MustCompile(`^[0-9]{5}$`)

// Quote es la cotización vigente de la aseguradora para una orden.
type Quote struct {
	ID         string
	Premium    string // decimal exacto como texto, para no perder precisión con float
	Currency   string
	ValidUntil time.Time
}

// Order es una compra de póliza y su estado actual.
type Order struct {
	ID            OrderID
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
// Devuelve todos los campos inválidos a la vez, no solo el primero.
func NewOrder(in NewOrderInput) (Order, error) {
	var errs ValidationErrors

	plate, err := NewPlate(in.Plate)
	if err != nil {
		errs = append(errs, ValidationError{Field: "plate", Message: err.Error()})
	}
	doc, err := NewDocumentID(in.DocumentID)
	if err != nil {
		errs = append(errs, ValidationError{Field: "documentId", Message: err.Error()})
	}
	if maxYear := time.Now().Year() + 1; in.VehicleYear < minVehicleYear || in.VehicleYear > maxYear {
		errs = append(errs, ValidationError{
			Field:   "vehicleYear",
			Message: fmt.Sprintf("must be between %d and %d", minVehicleYear, maxYear),
		})
	}
	if !cityCodeFormat.MatchString(in.CityCode) {
		errs = append(errs, ValidationError{Field: "cityCode", Message: "must be a 5-digit DANE code"})
	}
	if len(errs) > 0 {
		return Order{}, errs
	}

	return Order{
		Status:      Created,
		Plate:       plate,
		DocumentID:  doc,
		VehicleYear: in.VehicleYear,
		CityCode:    in.CityCode,
	}, nil
}

// Transition cambia el estado de la orden si el flujo lo permite y devuelve el evento a registrar.
// Si la transición no es válida, la orden no se modifica.
func (o *Order) Transition(to Status, reason string, now time.Time) (Event, error) {
	if !o.Status.CanTransitionTo(to) {
		return Event{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, o.Status, to)
	}
	ev := Event{From: o.Status, To: to, Reason: reason, OccurredAt: now}
	o.Status = to
	o.UpdatedAt = now
	return ev, nil
}
