package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// OperatorActions son las acciones manuales de operaciones sobre una orden. Cada una queda en la línea de
// tiempo con el operador que la ejecutó.
type OperatorActions struct {
	repo OrderRepository
	log  *slog.Logger
	now  func() time.Time
}

// NewOperatorActions arma las acciones operativas.
func NewOperatorActions(repo OrderRepository, log *slog.Logger) *OperatorActions {
	return &OperatorActions{repo: repo, log: log, now: utcMicros}
}

// RetryIssuance vuelve a encolar la emisión de una orden en ISSUING o REFUND_REQUIRED.
func (a *OperatorActions) RetryIssuance(ctx context.Context, id order.OrderID, operator string) (order.Order, error) {
	return order.Order{}, errors.New("not implemented")
}

// MarkForReversal deja una orden pagada sin póliza en REFUND_REQUIRED para que se ejecute el reverso.
func (a *OperatorActions) MarkForReversal(ctx context.Context, id order.OrderID, operator string) (order.Order, error) {
	return order.Order{}, errors.New("not implemented")
}
