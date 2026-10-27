package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

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

func (h *Driver) alerts(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
