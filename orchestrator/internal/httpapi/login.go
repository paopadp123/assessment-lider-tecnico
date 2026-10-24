package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

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

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (h *Login) login(w http.ResponseWriter, r *http.Request) {
	if ok, wait := h.limiter.Allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "too many login attempts"})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	tok, err := h.auth.Login(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		// Sin el usuario en el log: un intento fallido puede traer la contraseña escrita en el campo equivocado.
		h.log.WarnContext(r.Context(), "operator login failed", "ip", clientIP(r))
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
	case err != nil:
		h.log.ErrorContext(r.Context(), "login failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
	default:
		writeJSON(w, http.StatusOK, loginResponse{Token: tok.Value, ExpiresAt: tok.ExpiresAt})
	}
}

type operatorKey struct{}

// RequireOperator exige "Authorization: Bearer <token>" válido y deja al operador en el contexto. La identidad
// de quien ejecuta una acción sale siempre de aquí, nunca del cuerpo de la solicitud.
func RequireOperator(v TokenVerifier) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing bearer token"})
				return
			}
			op, err := v.Verify(token)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token"})
				return
			}
			next(w, r.WithContext(context.WithValue(r.Context(), operatorKey{}, op)))
		}
	}
}

// OperatorFrom devuelve el operador autenticado de la solicitud.
func OperatorFrom(ctx context.Context) (auth.Operator, bool) {
	op, ok := ctx.Value(operatorKey{}).(auth.Operator)
	return op, ok
}

// clientIP usa la IP de la conexión. No confía en X-Forwarded-For: cualquiera puede enviarla para saltarse el
// límite. Detrás de un balanceador habría que leerla solo de él (ver ADR 0005).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// IPRateLimiter limita los intentos por IP con un token bucket por dirección. Vive en memoria de cada réplica:
// con N réplicas el límite efectivo es N veces mayor (documentado en el ADR 0005).
type IPRateLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	entries map[string]*ipEntry
	now     func() time.Time
}

type ipEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// maxTrackedIPs acota la memoria si llegan intentos desde muchas IP distintas.
const maxTrackedIPs = 10_000

// NewIPRateLimiter permite perMinute intentos por minuto por IP.
func NewIPRateLimiter(perMinute int) *IPRateLimiter {
	return &IPRateLimiter{limit: rate.Every(time.Minute / time.Duration(perMinute)), burst: perMinute, entries: map[string]*ipEntry{}, now: time.Now}
}

// Allow indica si la IP puede intentar ahora y, si no, cuánto debe esperar.
func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	now := l.now()
	e, ok := l.entries[ip]
	if !ok {
		l.makeRoomLocked(now)
		e = &ipEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[ip] = e
	}
	e.lastSeen = now
	l.mu.Unlock()

	res := e.lim.ReserveN(now, 1)
	if wait := res.DelayFrom(now); wait > 0 {
		res.CancelAt(now) // un intento rechazado no consume del cupo
		return false, wait
	}
	return true, 0
}

// makeRoomLocked mantiene el registro por debajo de maxTrackedIPs. Primero descarta las IP con el cupo
// completo (sin intentos recientes): olvidarlas no cambia nada, y las que están siendo frenadas se conservan.
// Si todas están activas (ataque distribuido), descarta la que lleva más tiempo sin intentar: la memoria
// queda acotada aunque esa IP recupere su cupo (hallazgo de la revisión A).
func (l *IPRateLimiter) makeRoomLocked(now time.Time) {
	if len(l.entries) < maxTrackedIPs {
		return
	}
	for ip, e := range l.entries {
		if e.lim.TokensAt(now) >= float64(l.burst) {
			delete(l.entries, ip)
		}
	}
	if len(l.entries) < maxTrackedIPs {
		return
	}
	var oldestIP string
	var oldest time.Time
	for ip, e := range l.entries {
		if oldestIP == "" || e.lastSeen.Before(oldest) {
			oldestIP, oldest = ip, e.lastSeen
		}
	}
	delete(l.entries, oldestIP)
}

// tracked devuelve cuántas IP se están siguiendo.
func (l *IPRateLimiter) tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
