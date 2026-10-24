package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// Motivos de falla visibles para operaciones.
const (
	ReasonQuoteUnavailable = "QUOTE_UNAVAILABLE"
	ReasonQuoteRejected    = "QUOTE_REJECTED"
)

// CreateOrder es el caso de uso que inicia una compra: crea la orden de forma idempotente y la cotiza.
type CreateOrder struct {
	repo   OrderRepository
	quoter Quoter
	log    *slog.Logger
	sleep  func(context.Context, time.Duration) error
}

// CreateOrderResult es la orden resultante; Created es false si la solicitud ya se había procesado.
type CreateOrderResult struct {
	Order   order.Order
	Created bool
}

// NewCreateOrder arma el caso de uso con sus puertos.
func NewCreateOrder(repo OrderRepository, quoter Quoter, log *slog.Logger) *CreateOrder {
	return &CreateOrder{repo: repo, quoter: quoter, log: log}
}

// Execute procesa la solicitud de compra identificada por idempotencyKey.
func (uc *CreateOrder) Execute(ctx context.Context, idempotencyKey string, in order.NewOrderInput) (CreateOrderResult, error) {
	return CreateOrderResult{}, errors.New("not implemented")
}
