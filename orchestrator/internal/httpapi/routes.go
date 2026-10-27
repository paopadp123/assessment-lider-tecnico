package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/api"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// Router lo cumple *http.ServeMux. Permite registrar las rutas en un recolector durante las pruebas.
type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// Handlers reúne los handlers del orquestador para registrarlos en un solo lugar.
type Handlers struct {
	Orders   *Orders
	Webhooks *Webhooks
	Login    *Login
	Ops      *Ops
	Console  *Console
	Driver   *Driver
	Breakers http.HandlerFunc
	Ready    http.HandlerFunc
	Require  func(http.HandlerFunc) http.HandlerFunc
}

// Routes registra todas las rutas. Lo usan main y la prueba que verifica que cada ruta esté en el contrato:
// una ruta nueva que no se documente hace fallar esa prueba.
func Routes(r Router, h Handlers) {
	r.HandleFunc("GET /health", Health) // alias de /health/live, para quien ya lo usa
	r.HandleFunc("GET /health/live", Health)
	r.HandleFunc("GET /health/ready", h.Ready)
	r.HandleFunc("GET /openapi.yaml", OpenAPI)
	r.HandleFunc("GET /ops/breakers", h.Require(h.Breakers))
	h.Orders.Register(r)
	h.Webhooks.Register(r)
	h.Login.Register(r)
	h.Ops.Register(r, h.Require)
	h.Console.Register(r, h.Require)
	h.Driver.Register(r)
}

// WithCorrelation asegura que cada solicitud tenga un correlation-id: usa el del cliente si es válido o
// genera uno, lo deja en el contexto y lo devuelve en la respuesta. Un valor inválido se reemplaza (no se
// propaga a los logs).
func WithCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(trace.Header)
		if !trace.Valid(id) {
			id = trace.New()
		}
		w.Header().Set(trace.Header, id)
		next.ServeHTTP(w, r.WithContext(trace.WithID(r.Context(), id)))
	})
}

// Check verifica una dependencia para el readiness.
type Check struct {
	Name  string
	Probe func(ctx context.Context) error
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// readyTimeout acota cada verificación: un readiness que se cuelga es peor que uno que responde 503.
const readyTimeout = 2 * time.Second

// Ready responde 200 si todas las dependencias responden y 503 si alguna falla, con el nombre de la que falló.
// El detalle del error no se expone (podría revelar direcciones internas); queda en el log de cada componente.
// Live solo dice que el proceso vive; Ready decide si el balanceador le envía tráfico.
func Ready(checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := readyResponse{Status: "ready", Checks: map[string]string{}}
		for _, c := range checks {
			ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
			err := c.Probe(ctx)
			cancel()
			if err != nil {
				resp.Status, resp.Checks[c.Name] = "not_ready", "down"
				continue
			}
			resp.Checks[c.Name] = "up"
		}
		status := http.StatusOK
		if resp.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, resp)
	}
}

// OpenAPI sirve el contrato. La consola genera sus tipos TypeScript desde aquí.
func OpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(api.Spec)
}
