package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
)

type fakeAuth struct{ valid map[string]string }

func (f fakeAuth) Login(_ context.Context, user, pass string) (auth.Token, error) {
	if f.valid[user] != pass || pass == "" {
		return auth.Token{}, auth.ErrInvalidCredentials
	}
	return auth.Token{Value: "token-" + user, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

// Verify acepta los tokens "token-<usuario>" que emite fakeAuth.
func (f fakeAuth) Verify(token string) (auth.Operator, error) {
	user, ok := strings.CutPrefix(token, "token-")
	if !ok || f.valid[user] == "" {
		return auth.Operator{}, auth.ErrInvalidToken
	}
	return auth.Operator{Username: user, DisplayName: "Ana"}, nil
}

var operators = fakeAuth{valid: map[string]string{"ana.perez": "clave"}}

func discard() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func loginMux(limiter *IPRateLimiter) *http.ServeMux {
	mux := http.NewServeMux()
	NewLogin(operators, limiter, discard()).Register(mux)
	return mux
}

func postLogin(mux *http.ServeMux, ip, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.RemoteAddr = ip + ":5000"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestLogin_ValidCredentialsReturnToken(t *testing.T) {
	rec := postLogin(loginMux(NewIPRateLimiter(5)), "10.0.0.1", `{"username":"ana.perez","password":"clave"}`)

	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&body) != nil || body.Token != "token-ana.perez" || body.ExpiresAt.IsZero() {
		t.Fatalf("status=%d body=%+v", rec.Code, body)
	}
}

func TestLogin_InvalidCredentialsAre401WithGenericMessage(t *testing.T) {
	rec := postLogin(loginMux(NewIPRateLimiter(5)), "10.0.0.1", `{"username":"ana.perez","password":"mala"}`)

	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid credentials") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogin_MalformedBodyIs400(t *testing.T) {
	rec := postLogin(loginMux(NewIPRateLimiter(5)), "10.0.0.1", `{no json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Contra la fuerza bruta: 5 intentos por minuto por IP; el sexto responde 429 con Retry-After.
// Otra IP no se ve afectada.
func TestLogin_RateLimitedPerIP(t *testing.T) {
	mux := loginMux(NewIPRateLimiter(5))
	for range 5 {
		postLogin(mux, "10.0.0.1", `{"username":"ana.perez","password":"mala"}`)
	}

	rec := postLogin(mux, "10.0.0.1", `{"username":"ana.perez","password":"clave"}`)
	other := postLogin(mux, "10.0.0.2", `{"username":"ana.perez","password":"clave"}`)

	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("sexto intento: status=%d Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if other.Code != http.StatusOK {
		t.Fatalf("otra IP: status = %d", other.Code)
	}
}

func protected(t *testing.T) http.HandlerFunc {
	t.Helper()
	return RequireOperator(operators)(func(w http.ResponseWriter, r *http.Request) {
		op, ok := OperatorFrom(r.Context())
		if !ok {
			t.Error("el handler protegido no recibió al operador")
		}
		_, _ = w.Write([]byte(op.Username))
	})
}

func TestRequireOperator(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"sin cabecera", "", http.StatusUnauthorized},
		{"sin Bearer", "token-ana.perez", http.StatusUnauthorized},
		{"token inválido", "Bearer token-intruso", http.StatusUnauthorized},
		{"token válido", "Bearer token-ana.perez", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()

			protected(t)(rec, req)

			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
			if c.want == http.StatusOK && rec.Body.String() != "ana.perez" {
				t.Fatalf("operador = %q", rec.Body.String())
			}
		})
	}
}

// fakeClock avanza 1 µs en cada lectura: cada intento tiene su propia hora, sin depender de la resolución del
// reloj del sistema (en Windows muchas lecturas seguidas devuelven la misma hora).
func fakeClock() func() time.Time {
	t := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Microsecond)
		return t
	}
}

func ipN(i int) string {
	return "10." + strconv.Itoa(i/65536) + "." + strconv.Itoa(i/256%256) + "." + strconv.Itoa(i%256)
}

// Al llenarse el registro se descartan las IP más antiguas, no una que está siendo frenada ahora:
// rotar IP no le devuelve el cupo a un atacante activo.
func TestRateLimiter_EvictionKeepsRecentlyBlockedIP(t *testing.T) {
	l := NewIPRateLimiter(5)
	l.now = fakeClock()
	for i := range maxTrackedIPs - 1 {
		l.Allow(ipN(i))
	}
	for range 5 {
		l.Allow("192.168.0.1")
	}
	for i := range 100 {
		l.Allow(ipN(maxTrackedIPs + i)) // fuerza desalojos
	}

	if ok, _ := l.Allow("192.168.0.1"); ok {
		t.Fatal("la IP frenada recuperó su cupo al desalojar")
	}
}

// Hallazgo de Antigravity (revisión A): bajo un ataque distribuido todas las IP están activas y la poda no
// libera nada. El registro igual no supera el máximo: se descarta la IP que lleva más tiempo sin intentar.
func TestRateLimiter_NeverExceedsMaxTrackedIPs(t *testing.T) {
	l := NewIPRateLimiter(5)
	l.now = fakeClock()
	for i := range maxTrackedIPs + 500 {
		l.Allow(ipN(i)) // cada IP consume cupo: ninguna queda inactiva
	}

	if n := l.tracked(); n > maxTrackedIPs {
		t.Fatalf("IP registradas = %d, máximo %d", n, maxTrackedIPs)
	}
}

// Barrido del login: ni la contraseña (aunque sea la equivocada) ni el token emitido llegan a los logs.
func TestLogin_NeverLogsPasswordsOrTokens(t *testing.T) {
	var logs bytes.Buffer
	mux := http.NewServeMux()
	NewLogin(operators, NewIPRateLimiter(5), slog.New(slog.NewJSONHandler(&logs, nil))).Register(mux)

	postLogin(mux, "10.0.0.1", `{"username":"ana.perez","password":"clave-equivocada-secreta"}`)
	ok := postLogin(mux, "10.0.0.1", `{"username":"ana.perez","password":"clave"}`)

	for _, secret := range []string{"clave-equivocada-secreta", `"clave"`, "token-ana.perez"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("%q aparece en los logs: %s", secret, logs.String())
		}
	}
	if ok.Code != http.StatusOK || logs.Len() == 0 {
		t.Fatalf("status=%d; el login fallido debía dejar un WARN", ok.Code)
	}
}
