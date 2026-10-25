package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte(strings.Repeat("s", MinSecretBytes))

type memStore struct {
	mu  sync.Mutex
	ops map[string]StoredOperator
}

func newMemStore() *memStore { return &memStore{ops: map[string]StoredOperator{}} }

func (m *memStore) FindOperator(_ context.Context, username string) (StoredOperator, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.ops[username]
	if !ok {
		return StoredOperator{}, ErrOperatorNotFound
	}
	return op, nil
}

func (m *memStore) CreateOperatorIfMissing(_ context.Context, op StoredOperator) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.ops[op.Username]; !ok {
		m.ops[op.Username] = op
	}
	return nil
}

func newTestService(t *testing.T) (*Service, *memStore) {
	t.Helper()
	store := newMemStore()
	svc, err := NewService(store, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Bootstrap(context.Background(), "ana.perez", "Ana Pérez", "clave-segura-1"); err != nil {
		t.Fatal(err)
	}
	return svc, store
}

func TestNewService_RejectsShortSecret(t *testing.T) {
	if _, err := NewService(newMemStore(), []byte("corto"), time.Hour); err == nil {
		t.Fatal("un secreto corto debe rechazarse al iniciar")
	}
}

func TestBootstrap_StoresHashNotPassword(t *testing.T) {
	_, store := newTestService(t)

	op := store.ops["ana.perez"]

	if !op.Active || op.PasswordHash == "" || strings.Contains(op.PasswordHash, "clave-segura-1") {
		t.Fatalf("operador guardado = %+v", op)
	}
}

func TestLogin_ValidCredentialsReturnVerifiableToken(t *testing.T) {
	svc, _ := newTestService(t)

	tok, err := svc.Login(context.Background(), "ana.perez", "clave-segura-1")
	if err != nil {
		t.Fatal(err)
	}
	op, err := svc.Verify(tok.Value)

	if err != nil || op != (Operator{Username: "ana.perez", DisplayName: "Ana Pérez"}) {
		t.Fatalf("operador = %+v, err=%v", op, err)
	}
	if d := time.Until(tok.ExpiresAt); d < 59*time.Minute || d > time.Hour+time.Minute {
		t.Fatalf("vence en %v, want ~1 h", d)
	}
}

// El login no revela qué usuarios existen: mismo error para contraseña mala, usuario inexistente o inactivo.
func TestLogin_FailuresAreIndistinguishable(t *testing.T) {
	svc, store := newTestService(t)
	inactive := store.ops["ana.perez"]
	inactive.Username, inactive.Active = "inactivo", false
	store.ops["inactivo"] = inactive

	for _, c := range []struct{ user, pass string }{
		{"ana.perez", "otra-clave"},
		{"no-existe", "clave-segura-1"},
		{"inactivo", "clave-segura-1"},
	} {
		if _, err := svc.Login(context.Background(), c.user, c.pass); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", c.user, err)
		}
	}
}

// Contra un usuario inexistente o inactivo igual se ejecuta bcrypt: si respondiera al instante, el tiempo de
// respuesta revelaría qué usuarios existen. Se compara la mediana de varios intentos para no depender de un
// pico de la máquina.
func TestLogin_FailuresTakeAsLongAsWrongPassword(t *testing.T) {
	svc, store := newTestService(t)
	inactive := store.ops["ana.perez"]
	inactive.Username, inactive.Active = "inactivo", false
	store.ops["inactivo"] = inactive
	median := func(user string) time.Duration {
		var d []time.Duration
		for range 5 {
			start := time.Now()
			_, _ = svc.Login(context.Background(), user, "otra-clave")
			d = append(d, time.Since(start))
		}
		slices.Sort(d)
		return d[len(d)/2]
	}
	known := median("ana.perez")

	for _, user := range []string{"no-existe", "inactivo"} {
		if got := median(user); got < known/2 {
			t.Errorf("%s: %v vs. contraseña mala: %v; la diferencia revela si el usuario existe", user, got, known)
		}
	}
}

func TestVerify_RejectsTamperedExpiredOrForeignTokens(t *testing.T) {
	svc, _ := newTestService(t)
	tok, _ := svc.Login(context.Background(), "ana.perez", "clave-segura-1")

	expired, _ := newTestService(t)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	old, _ := expired.Login(context.Background(), "ana.perez", "clave-segura-1")

	other, _ := NewService(newMemStore(), []byte(strings.Repeat("x", MinSecretBytes)), time.Hour)
	_ = other.Bootstrap(context.Background(), "ana.perez", "Ana", "clave-segura-1")
	foreign, _ := other.Login(context.Background(), "ana.perez", "clave-segura-1")

	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "ana.perez", "exp": time.Now().Add(time.Hour).Unix()})
	unsigned, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)

	cases := map[string]string{
		"alterado":     tok.Value[:len(tok.Value)-2] + "xx",
		"vencido":      old.Value,
		"otra firma":   foreign.Value,
		"alg none":     unsigned,
		"vacío":        "",
		"no es un JWT": "abc",
	}
	for name, value := range cases {
		if _, err := svc.Verify(value); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v, want ErrInvalidToken", name, err)
		}
	}
}

// El primer operador se crea una sola vez: reiniciar el proceso no cambia su contraseña.
func TestBootstrap_DoesNotOverwriteExistingOperator(t *testing.T) {
	svc, store := newTestService(t)
	hash := store.ops["ana.perez"].PasswordHash

	if err := svc.Bootstrap(context.Background(), "ana.perez", "Ana", "otra-clave-larga"); err != nil {
		t.Fatal(err)
	}

	if store.ops["ana.perez"].PasswordHash != hash {
		t.Fatal("Bootstrap pisó la contraseña de un operador existente")
	}
}
