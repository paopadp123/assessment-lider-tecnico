package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// OperatorActionRunner ejecuta las acciones operativas.
type OperatorActionRunner interface {
	RetryIssuance(ctx context.Context, id order.OrderID, operator string) (order.Order, error)
	MarkForReversal(ctx context.Context, id order.OrderID, operator string) (order.Order, error)
}

// Ops expone las rutas de la consola de operaciones; todas exigen un operador autenticado.
type Ops struct {
	actions   OperatorActionRunner
	events    app.EventReader
	log       *slog.Logger
	poll      time.Duration
	heartbeat time.Duration
}

// NewOps arma los handlers de operación.
func NewOps(actions OperatorActionRunner, events app.EventReader, log *slog.Logger) *Ops {
	return &Ops{actions: actions, events: events, log: log, poll: time.Second, heartbeat: 15 * time.Second}
}

// Register registra las rutas protegidas con require.
func (h *Ops) Register(mux *http.ServeMux, require func(http.HandlerFunc) http.HandlerFunc) {}

// CloseStreams termina los flujos SSE abiertos.
func (h *Ops) CloseStreams() {}

// streamCursor decide qué eventos entregar sin saltar ninguno pendiente de confirmar.
type streamCursor struct {
	last  int64
	grace time.Duration
}

func (c *streamCursor) take(events []app.StoredEvent, now time.Time) []app.StoredEvent { return nil }
