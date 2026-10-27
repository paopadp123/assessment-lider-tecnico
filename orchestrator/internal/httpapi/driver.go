package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// tripIDFormat: el id de viaje lo genera el teléfono; solo caracteres seguros para logs.
var tripIDFormat = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// maxClockSkew: el reloj de un teléfono puede estar mal y una alerta de pánico no se rechaza por eso (el
// servidor guarda además received_at con su propia hora). Solo una fecha absurda es un dato inválido.
const maxClockSkew = 24 * time.Hour

// DriverAlertReporter es el caso de uso que registra una alerta del conductor.
type DriverAlertReporter interface {
	Execute(ctx context.Context, a app.DriverAlert) (created bool, err error)
}

// Driver expone la API de la app del conductor (prototipo móvil, HU-20).
//
// Autenticación de prototipo: un token compartido de dispositivo (DRIVER_DEVICE_TOKEN). En producción cada
// vehículo tendría su propio certificado (AWS IoT Core, docs/architecture.md); un token dentro de una app
// instalada en miles de teléfonos no es un secreto.
type Driver struct {
	reporter DriverAlertReporter
	token    string
	log      *slog.Logger
}

// NewDriver crea los handlers de la app del conductor. Con token vacío, todas las solicitudes responden 401.
func NewDriver(reporter DriverAlertReporter, token string, log *slog.Logger) *Driver {
	return &Driver{reporter: reporter, token: token, log: log}
}

// Register registra las rutas en el mux.
func (h *Driver) Register(mux Router) {
	mux.HandleFunc("POST /driver/alerts", h.alerts)
}

type driverAlertRequest struct {
	AlertID    string `json:"alertId"`
	Plate      string `json:"plate"`
	TripID     string `json:"tripId"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurredAt"`
}

type driverAlertResponse struct {
	AlertID string `json:"alertId"`
	Status  string `json:"status"`
}

// alerts recibe una alerta: 401 sin el token del dispositivo, 400 si es inválida, 201 la primera vez,
// 200 en un reintento de la misma alerta, 409 si el id se reutiliza con otro contenido y 500 si no se pudo
// guardar (el teléfono la conserva en su cola y la reintenta).
func (h *Driver) alerts(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid device token"})
		return
	}
	var body driverAlertRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields() // el contrato declara additionalProperties: false
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	alert, err := parseDriverAlert(body, time.Now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	created, err := h.reporter.Execute(r.Context(), alert)
	switch {
	case errors.Is(err, app.ErrDriverAlertConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "alertId already used with different content"})
	case err != nil:
		trace.Logger(r.Context(), h.log).Error("driver alert not saved", "alert_id", alert.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
	case created:
		writeJSON(w, http.StatusCreated, driverAlertResponse{AlertID: alert.ID, Status: "RECEIVED"})
	default:
		writeJSON(w, http.StatusOK, driverAlertResponse{AlertID: alert.ID, Status: "RECEIVED"})
	}
}

// authorized compara el token en tiempo constante. Sin token configurado, nadie está autorizado.
func (h *Driver) authorized(r *http.Request) bool {
	sent, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return h.token != "" && ok && subtle.ConstantTimeCompare([]byte(sent), []byte(h.token)) == 1
}

func parseDriverAlert(b driverAlertRequest, now time.Time) (app.DriverAlert, error) {
	plate, err := order.NewPlate(b.Plate)
	occurredAt, timeErr := time.Parse(time.RFC3339Nano, b.OccurredAt)
	switch {
	case !uuidFormat.MatchString(b.AlertID):
		return app.DriverAlert{}, errors.New("alertId must be a UUID")
	case err != nil:
		return app.DriverAlert{}, errors.New("invalid plate")
	case !tripIDFormat.MatchString(b.TripID):
		return app.DriverAlert{}, errors.New("invalid tripId")
	case b.Type != app.DriverAlertPanic:
		return app.DriverAlert{}, errors.New("type must be PANIC")
	case timeErr != nil:
		return app.DriverAlert{}, errors.New("occurredAt must be RFC 3339")
	case occurredAt.After(now.Add(maxClockSkew)):
		return app.DriverAlert{}, errors.New("occurredAt is in the future")
	}
	return app.DriverAlert{ID: strings.ToLower(b.AlertID), Plate: plate, TripID: b.TripID, Type: b.Type, OccurredAt: occurredAt.UTC()}, nil
}
