package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const maxBodyBytes = 16 << 10

var (
	uuidFormat = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Solo ASCII visible: sin espacios, saltos de línea ni caracteres de control.
	idempotencyKeyFormat = regexp.MustCompile(`^[\x21-\x7E]{1,255}$`)
)

// OrderCreator es el caso de uso de creación que necesita el handler.
type OrderCreator interface {
	Execute(ctx context.Context, idempotencyKey string, in order.NewOrderInput) (app.CreateOrderResult, error)
}

// OrderReader permite consultar una orden.
type OrderReader interface {
	Get(ctx context.Context, id order.OrderID) (order.Order, error)
}

// Orders expone los endpoints de órdenes.
type Orders struct {
	creator OrderCreator
	reader  OrderReader
	log     *slog.Logger
}

// NewOrders crea los handlers de órdenes.
func NewOrders(creator OrderCreator, reader OrderReader, log *slog.Logger) *Orders {
	return &Orders{creator: creator, reader: reader, log: log}
}

// Register registra las rutas en el mux.
func (h *Orders) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.create)
	mux.HandleFunc("GET /orders/{id}", h.get)
}

type createOrderRequest struct {
	Plate       string `json:"plate"`
	VehicleYear int    `json:"vehicleYear"`
	CityCode    string `json:"cityCode"`
	DocumentID  string `json:"documentId"`
}

type quoteResponse struct {
	ID         string      `json:"id"`
	Premium    json.Number `json:"premium"`
	Currency   string      `json:"currency"`
	ValidUntil time.Time   `json:"validUntil"`
}

// orderResponse nunca incluye el documento y devuelve la placa enmascarada.
type orderResponse struct {
	ID            order.OrderID  `json:"id"`
	Status        order.Status   `json:"status"`
	Plate         string         `json:"plate"`
	VehicleYear   int            `json:"vehicleYear"`
	CityCode      string         `json:"cityCode"`
	Quote         *quoteResponse `json:"quote,omitempty"`
	FailureReason string         `json:"failureReason,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

type errorResponse struct {
	Error  string       `json:"error"`
	Fields []fieldError `json:"fields,omitempty"`
}

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func toFieldErrors(verr order.ValidationErrors) []fieldError {
	out := make([]fieldError, len(verr))
	for i, e := range verr {
		out[i] = fieldError{Field: e.Field, Message: e.Message}
	}
	return out
}

func (h *Orders) create(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !idempotencyKeyFormat.MatchString(key) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Idempotency-Key header is required: 1 to 255 visible ASCII characters"})
		return
	}

	var req createOrderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	res, err := h.creator.Execute(r.Context(), key, order.NewOrderInput{
		Plate:       req.Plate,
		DocumentID:  req.DocumentID,
		VehicleYear: req.VehicleYear,
		CityCode:    req.CityCode,
	})
	var verr order.ValidationErrors
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "validation failed", Fields: toFieldErrors(verr)})
	case errors.Is(err, app.ErrIdempotencyConflict):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
	case err != nil:
		h.internalError(w, r, err)
	case res.Created:
		writeJSON(w, http.StatusCreated, toResponse(res.Order))
	default:
		writeJSON(w, http.StatusOK, toResponse(res.Order))
	}
}

func (h *Orders) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidFormat.MatchString(id) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
		return
	}
	o, err := h.reader.Get(r.Context(), order.OrderID(id))
	switch {
	case errors.Is(err, app.ErrOrderNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toResponse(o))
	}
}

// internalError registra el detalle y responde un mensaje genérico: los errores internos no se filtran al cliente.
func (h *Orders) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
}

func toResponse(o order.Order) orderResponse {
	resp := orderResponse{
		ID:            o.ID,
		Status:        o.Status,
		Plate:         o.Plate.String(),
		VehicleYear:   o.VehicleYear,
		CityCode:      o.CityCode,
		FailureReason: o.FailureReason,
		CreatedAt:     o.CreatedAt.UTC(),
		UpdatedAt:     o.UpdatedAt.UTC(),
	}
	if o.Quote != nil {
		resp.Quote = &quoteResponse{
			ID:         o.Quote.ID,
			Premium:    json.Number(o.Quote.Premium),
			Currency:   o.Quote.Currency,
			ValidUntil: o.Quote.ValidUntil.UTC(),
		}
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // si el cliente ya se fue, no hay a quién reportarle el error
}
