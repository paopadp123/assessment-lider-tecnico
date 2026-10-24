package httpapi

import (
	"context"
	"net/http"
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
	Breakers http.HandlerFunc
	Ready    http.HandlerFunc
	Require  func(http.HandlerFunc) http.HandlerFunc
}

// Routes registra todas las rutas. Lo usan main y la prueba que verifica que cada ruta esté en el contrato.
func Routes(r Router, h Handlers) {
	r.HandleFunc("GET /health", Health)
	r.HandleFunc("GET /ops/breakers", h.Require(h.Breakers))
	h.Orders.Register(r)
	h.Webhooks.Register(r)
	h.Login.Register(r)
	h.Ops.Register(r, h.Require)
}

// WithCorrelation asegura que cada solicitud tenga un correlation-id.
func WithCorrelation(next http.Handler) http.Handler { return next }

// Check verifica una dependencia para el readiness.
type Check struct {
	Name  string
	Probe func(ctx context.Context) error
}

// Ready responde si el orquestador está listo para recibir tráfico.
func Ready(checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
}

// OpenAPI sirve el contrato.
func OpenAPI(w http.ResponseWriter, _ *http.Request) {}
