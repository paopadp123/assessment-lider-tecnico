package httpapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
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

func (h *Console) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseOrderFilter(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	page, err := h.orders.ListOrders(r.Context(), f)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	out := orderListResponse{Items: make([]orderResponse, 0, len(page.Orders))}
	for _, o := range page.Orders {
		out.Items = append(out.Items, toResponse(o))
	}
	if page.Next != nil {
		out.NextCursor = encodeCursor(*page.Next)
	}
	writeJSON(w, http.StatusOK, out)
}

// parseOrderFilter valida la consulta: ?status= (repetible), ?q=, ?limit= y ?cursor=.
func parseOrderFilter(v url.Values) (app.OrderFilter, error) {
	f := app.OrderFilter{Limit: defaultPageSize, Query: strings.TrimSpace(v.Get("q"))}
	for _, s := range v["status"] {
		st := order.Status(s)
		if !st.Valid() {
			return app.OrderFilter{}, fmt.Errorf("invalid status %q", s)
		}
		f.Statuses = append(f.Statuses, st)
	}
	if len(f.Query) > maxQueryLength {
		return app.OrderFilter{}, fmt.Errorf("q must be at most %d characters", maxQueryLength)
	}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageSize {
			return app.OrderFilter{}, fmt.Errorf("limit must be between 1 and %d", maxPageSize)
		}
		f.Limit = n
	}
	if raw := v.Get("cursor"); raw != "" {
		c, err := decodeCursor(raw)
		if err != nil {
			return app.OrderFilter{}, errors.New("invalid cursor")
		}
		f.Cursor = &c
	}
	return f, nil
}

func (h *Console) listCalls(w http.ResponseWriter, r *http.Request) {
	id, ok := orderIDFrom(w, r)
	if !ok {
		return
	}
	calls, err := h.calls.ListCalls(r.Context(), id)
	switch {
	case errors.Is(err, app.ErrOrderNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
	case err != nil:
		h.internalError(w, r, err)
	default:
		out := make([]callResponse, 0, len(calls))
		for _, c := range calls {
			out = append(out, callResponse{ID: c.ID, Party: c.Party, Operation: c.Operation, Outcome: c.Outcome,
				Detail: c.Detail, DurationMs: c.Duration.Milliseconds(), StartedAt: c.StartedAt.UTC()})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (h *Console) internalError(w http.ResponseWriter, r *http.Request, err error) {
	trace.Logger(r.Context(), h.log).Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
}

// encodeCursor y decodeCursor convierten el cursor en un texto opaco para la URL ("<instante>|<id>" en
// base64url). Opaco a propósito: el cliente no debe armarlo ni depender de su forma.
func encodeCursor(c app.OrderCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + string(c.ID)))
}

func decodeCursor(s string) (app.OrderCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return app.OrderCursor{}, err
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok || !uuidFormat.MatchString(id) {
		return app.OrderCursor{}, errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return app.OrderCursor{}, err
	}
	return app.OrderCursor{CreatedAt: t, ID: order.OrderID(id)}, nil
}
