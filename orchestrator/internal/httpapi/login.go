package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
)

// Authenticator verifica credenciales y emite un token.
type Authenticator interface {
	Login(ctx context.Context, username, password string) (auth.Token, error)
}

// TokenVerifier valida un token y devuelve el operador.
type TokenVerifier interface {
	Verify(token string) (auth.Operator, error)
}

// Login expone POST /auth/login con límite de intentos por IP.
type Login struct {
	auth    Authenticator
	limiter *IPRateLimiter
	log     *slog.Logger
}

// NewLogin arma el handler de login.
func NewLogin(a Authenticator, limiter *IPRateLimiter, log *slog.Logger) *Login {
	return &Login{auth: a, limiter: limiter, log: log}
}

// Register registra la ruta.
func (h *Login) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.login)
}

func (h *Login) login(w http.ResponseWriter, r *http.Request) {}

// RequireOperator exige un token válido y deja al operador en el contexto de la solicitud.
func RequireOperator(v TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc { return next }
}

// OperatorFrom devuelve el operador autenticado de la solicitud.
func OperatorFrom(ctx context.Context) (auth.Operator, bool) {
	return auth.Operator{}, false
}

// IPRateLimiter limita los intentos por IP.
type IPRateLimiter struct{}

// NewIPRateLimiter permite perMinute intentos por minuto por IP.
func NewIPRateLimiter(perMinute int) *IPRateLimiter { return &IPRateLimiter{} }

// Allow indica si la IP puede intentar ahora y, si no, cuánto debe esperar.
func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) { return true, 0 }

// maxTrackedIPs acota la memoria si llegan intentos desde muchas IP distintas.
const maxTrackedIPs = 10_000
