package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// errOperatorRequired protege la regla de auditoría: ninguna acción manual se ejecuta sin saber quién la pidió.
var errOperatorRequired = errors.New("operator is required")

// OperatorActions son las acciones manuales de operaciones sobre una orden. Cada una queda en la línea de
// tiempo con el operador que la ejecutó, en la misma transacción que el cambio.
type OperatorActions struct {
	repo OrderRepository
	log  *slog.Logger
	now  func() time.Time
}

// NewOperatorActions arma las acciones operativas.
func NewOperatorActions(repo OrderRepository, log *slog.Logger) *OperatorActions {
	return &OperatorActions{repo: repo, log: log, now: utcMicros}
}

// RetryIssuance vuelve a encolar la emisión de una orden en ISSUING o REFUND_REQUIRED. No emite directo:
// el worker consulta por externalRef antes de emitir, así que reintentar nunca duplica una póliza.
func (a *OperatorActions) RetryIssuance(ctx context.Context, id order.OrderID, operator string) (order.Order, error) {
	o, err := a.load(ctx, id, operator)
	if err != nil {
		return order.Order{}, err
	}
	const reason = "retry issuance requested"
	issue := Command{Type: CommandIssuePolicy, OrderID: o.ID}

	var ev order.Event
	switch o.Status {
	case order.RefundRequired:
		o.FailureReason = ""
		if ev, err = o.Transition(order.Issuing, reason, a.now()); err != nil {
			return order.Order{}, err
		}
	case order.Issuing:
		// Sin cambio de estado, pero la acción queda en la línea de tiempo como cualquier otra.
		now := a.now()
		o.UpdatedAt = now
		ev = order.Event{From: order.Issuing, To: order.Issuing, Reason: reason, OccurredAt: now}
	default:
		return order.Order{}, fmt.Errorf("%w: retry issuance from %s", ErrActionNotAllowed, o.Status)
	}
	return a.save(ctx, o, ev, operator, issue)
}

// MarkForReversal deja una orden pagada sin póliza en REFUND_REQUIRED para que se ejecute el reverso.
// El reverso en sí lo ejecuta operaciones fuera del sistema: no hay reembolso automático.
func (a *OperatorActions) MarkForReversal(ctx context.Context, id order.OrderID, operator string) (order.Order, error) {
	o, err := a.load(ctx, id, operator)
	if err != nil {
		return order.Order{}, err
	}
	if o.Status != order.Paid && o.Status != order.Issuing {
		return order.Order{}, fmt.Errorf("%w: mark for reversal from %s", ErrActionNotAllowed, o.Status)
	}
	o.FailureReason = ReasonMarkedForReversal
	ev, err := o.Transition(order.RefundRequired, ReasonMarkedForReversal, a.now())
	if err != nil {
		return order.Order{}, err
	}
	return a.save(ctx, o, ev, operator)
}

func (a *OperatorActions) load(ctx context.Context, id order.OrderID, operator string) (order.Order, error) {
	if operator == "" {
		return order.Order{}, errOperatorRequired
	}
	return a.repo.Get(ctx, id)
}

// save guarda el cambio con el operador como actor. Si otra operación cambió la orden entre la lectura y el
// guardado, el control optimista lo rechaza con ErrInvalidTransition (la API responde 409).
func (a *OperatorActions) save(ctx context.Context, o order.Order, ev order.Event, operator string, cmds ...Command) (order.Order, error) {
	ev.Actor = operator
	if err := a.repo.SaveTransition(ctx, o, ev, cmds...); err != nil {
		return order.Order{}, fmt.Errorf("save operator action: %w", err)
	}
	a.log.Info("operator action executed", "order_id", o.ID, "operator", operator, "reason", ev.Reason, "status", o.Status)
	return o, nil
}
