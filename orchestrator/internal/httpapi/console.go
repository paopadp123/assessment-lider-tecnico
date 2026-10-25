package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	maxQueryLength  = 100
)

// Console expone las consultas de la consola de operaciones: el listado de órdenes y las llamadas a
// terceros de cada una. Las dos exigen un operador autenticado.
type Console struct {
	orders app.OrderLister
	calls  app.CallReader
	log    *slog.Logger
}

// NewConsole arma los handlers de consulta de la consola.
func NewConsole(orders app.OrderLister, calls app.CallReader, log *slog.Logger) *Console {
	return &Console{orders: orders, calls: calls, log: log}
}

// Register registra las rutas protegidas con require.
func (h *Console) Register(mux Router, require func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /orders", require(h.list))
	mux.HandleFunc("GET /orders/{id}/calls", require(h.listCalls))
}

type orderListResponse struct {
	Items      []orderResponse `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type callResponse struct {
	ID         int64             `json:"id"`
	Party      app.ThirdParty    `json:"party"`
	Operation  app.CallOperation `json:"operation"`
	Outcome    app.CallOutcome   `json:"outcome"`
	Detail     string            `json:"detail,omitempty"`
	DurationMs int64             `json:"durationMs"`
	StartedAt  time.Time         `json:"startedAt"`
}

func (h *Console) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, orderListResponse{Items: []orderResponse{}})
}

func (h *Console) listCalls(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []callResponse{})
}

// encodeCursor y decodeCursor convierten el cursor en un texto opaco para la URL.
func encodeCursor(c app.OrderCursor) string { return "" }

func decodeCursor(s string) (app.OrderCursor, error) {
	return app.OrderCursor{ID: order.OrderID(s)}, nil
}
