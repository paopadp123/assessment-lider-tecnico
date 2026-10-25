package postgres

import (
	"context"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// RecordCall guarda un intento con un tercero.
func (r *OrderRepository) RecordCall(ctx context.Context, c app.ThirdPartyCall) error {
	return nil
}

// ListCalls devuelve los intentos de una orden en orden de ocurrencia, o app.ErrOrderNotFound.
func (r *OrderRepository) ListCalls(ctx context.Context, id order.OrderID) ([]app.StoredCall, error) {
	return nil, nil
}

// ListOrders devuelve una página del listado de la consola.
func (r *OrderRepository) ListOrders(ctx context.Context, f app.OrderFilter) (app.OrderPage, error) {
	return app.OrderPage{}, nil
}
