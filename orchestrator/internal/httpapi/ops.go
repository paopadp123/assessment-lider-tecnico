package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
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
	gapGrace  time.Duration
	closing   chan struct{}
	closeOnce sync.Once
}

// NewOps arma los handlers de operación.
func NewOps(actions OperatorActionRunner, events app.EventReader, log *slog.Logger) *Ops {
	return &Ops{actions: actions, events: events, log: log, poll: time.Second, heartbeat: 15 * time.Second, gapGrace: 10 * time.Second,
		closing: make(chan struct{})}
}

// CloseStreams termina los flujos SSE abiertos. Se registra con http.Server.RegisterOnShutdown: Shutdown
// espera a que terminen las conexiones activas, y un flujo SSE nunca termina por su cuenta.
func (h *Ops) CloseStreams() { h.closeOnce.Do(func() { close(h.closing) }) }

// Register registra las rutas protegidas con require.
func (h *Ops) Register(mux *http.ServeMux, require func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("POST /orders/{id}/actions/retry-issuance", require(h.action(OperatorActionRunner.RetryIssuance)))
	mux.HandleFunc("POST /orders/{id}/actions/mark-for-reversal", require(h.action(OperatorActionRunner.MarkForReversal)))
	mux.HandleFunc("GET /orders/{id}/events", require(h.listEvents))
	mux.HandleFunc("GET /events/stream", require(h.stream))
}

type eventResponse struct {
	ID         int64         `json:"id"`
	OrderID    order.OrderID `json:"orderId"`
	From       order.Status  `json:"from,omitempty"`
	To         order.Status  `json:"to"`
	Reason     string        `json:"reason"`
	Actor      string        `json:"actor"` // "system" si no lo ejecutó un operador
	OccurredAt time.Time     `json:"occurredAt"`
}

func toEventResponse(e app.StoredEvent) eventResponse {
	actor := e.Actor
	if actor == "" {
		actor = "system"
	}
	return eventResponse{ID: e.ID, OrderID: e.OrderID, From: e.From, To: e.To, Reason: e.Reason, Actor: actor, OccurredAt: e.OccurredAt.UTC()}
}

// action adapta una acción operativa a un handler. run es una expresión de método: recibe el runner como
// primer argumento, así ambas acciones comparten la validación y el mapeo de errores.
func (h *Ops) action(run func(OperatorActionRunner, context.Context, order.OrderID, string) (order.Order, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := orderIDFrom(w, r)
		if !ok {
			return
		}
		op, ok := OperatorFrom(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing operator"})
			return
		}
		o, err := run(h.actions, r.Context(), id, op.Username)
		switch {
		case errors.Is(err, app.ErrOrderNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
		case errors.Is(err, app.ErrActionNotAllowed), errors.Is(err, order.ErrInvalidTransition):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case err != nil:
			h.internalError(w, r, err)
		default:
			writeJSON(w, http.StatusOK, toResponse(o))
		}
	}
}

func (h *Ops) listEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := orderIDFrom(w, r)
	if !ok {
		return
	}
	events, err := h.events.ListEvents(r.Context(), id)
	switch {
	case errors.Is(err, app.ErrOrderNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
	case err != nil:
		h.internalError(w, r, err)
	default:
		out := make([]eventResponse, 0, len(events))
		for _, e := range events {
			out = append(out, toEventResponse(e))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// stream envía por SSE cada cambio de estado de cualquier orden. Lee de order_events por ID, así que funciona
// igual con varias réplicas y permite reanudar: el navegador reenvía Last-Event-ID al reconectar y recibe lo
// que se perdió. Sin Last-Event-ID arranca en el presente.
func (h *Ops) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	last, err := h.startingEventID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid Last-Event-ID"})
		return
	}
	// El servidor corta las respuestas largas (WriteTimeout); un flujo SSE debe quedar abierto.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // que un proxy no acumule los eventos
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	cursor := &streamCursor{last: last, grace: h.gapGrace}
	poll := time.NewTicker(h.poll)
	defer poll.Stop()
	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.closing:
			return
		case <-heartbeat.C:
			// Un comentario mantiene viva la conexión a través de balanceadores con timeout por inactividad.
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case <-poll.C:
			events, err := h.events.EventsAfter(ctx, cursor.last, 100)
			if err != nil {
				if ctx.Err() == nil {
					h.log.ErrorContext(ctx, "read events for stream", "error", err)
				}
				continue
			}
			ready := cursor.take(events, time.Now())
			for _, e := range ready {
				data, _ := json.Marshal(toEventResponse(e))
				if _, err := fmt.Fprintf(w, "id: %d\nevent: order-status\ndata: %s\n\n", e.ID, data); err != nil {
					return
				}
			}
			if len(ready) > 0 && rc.Flush() != nil {
				return
			}
		}
	}
}

func (h *Ops) startingEventID(r *http.Request) (int64, error) {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		return strconv.ParseInt(v, 10, 64)
	}
	return h.events.LatestEventID(r.Context())
}

func orderIDFrom(w http.ResponseWriter, r *http.Request) (order.OrderID, bool) {
	id := r.PathValue("id")
	if !uuidFormat.MatchString(id) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
		return "", false
	}
	return order.OrderID(id), true
}

func (h *Ops) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
}

// streamCursor decide qué eventos entregar. Los IDs de order_events se asignan al insertar, no al confirmar:
// el evento 5 puede confirmarse después que el 6. Si el flujo entregara el 6 y avanzara, el 5 se perdería.
// Por eso, ante un hueco, espera a que se llene; si no se llena en grace (fue un rollback), sigue.
type streamCursor struct {
	last     int64
	grace    time.Duration
	gapSince time.Time
}

// take devuelve, en orden, los eventos que se pueden entregar sin saltar ninguno pendiente de confirmar.
func (c *streamCursor) take(events []app.StoredEvent, now time.Time) []app.StoredEvent {
	var out []app.StoredEvent
	for _, e := range events {
		if e.ID <= c.last {
			continue
		}
		if e.ID != c.last+1 {
			if c.gapSince.IsZero() {
				c.gapSince = now
			}
			if now.Sub(c.gapSince) < c.grace {
				break // el hueco puede ser una transacción que aún no confirma
			}
		}
		out = append(out, e)
		c.last, c.gapSince = e.ID, time.Time{}
	}
	return out
}
