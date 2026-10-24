package httpapi

import (
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

// Con muchas IP distintas, el limitador descarta las que no tienen intentos recientes, no a todas:
// una IP que está siendo frenada sigue frenada.
func TestRateLimiter_PruningKeepsActiveLimits(t *testing.T) {
	l := NewIPRateLimiter(5)
	for range 5 {
		l.Allow("10.0.0.1")
	}
	for i := range maxTrackedIPs + 10 {
		l.Allow("192.168." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256))
	}

	if ok, _ := l.Allow("10.0.0.1"); ok {
		t.Fatal("la IP frenada recuperó su cupo al podar el mapa")
	}
}
