package trace

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_GeneratesValidUniqueIDs(t *testing.T) {
	a, b := New(), New()

	if !Valid(a) || !Valid(b) || a == b {
		t.Fatalf("ids = %q, %q", a, b)
	}
}

// El id viene de afuera y termina en los logs: se acota el largo y los caracteres para que no se pueda
// inyectar una línea de log falsa ni inflar los registros.
func TestValid_RejectsUnsafeIDs(t *testing.T) {
	cases := map[string]string{
		"vacío":          "",
		"muy largo":      strings.Repeat("a", 65),
		"salto de línea": "abc\nlevel=ERROR",
		"espacio":        "abc def",
		"comillas":       `abc"`,
	}
	for name, id := range cases {
		if Valid(id) {
			t.Errorf("%s: %q no debía ser válido", name, id)
		}
	}
	for _, id := range []string{"4f1c2a7e-0000-4000-8000-000000000001", "req_123.abc:9"} {
		if !Valid(id) {
			t.Errorf("%q debía ser válido", id)
		}
	}
}

func TestContextRoundTripAndLogger(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := WithID(context.Background(), "corr-1")

	Logger(ctx, base).Info("hola")
	Logger(context.Background(), base).Info("sin id")

	if ID(ctx) != "corr-1" || ID(context.Background()) != "" {
		t.Fatalf("ID = %q / %q", ID(ctx), ID(context.Background()))
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(lines[0], `"correlation_id":"corr-1"`) || strings.Contains(lines[1], "correlation_id") {
		t.Fatalf("logs = %s", buf.String())
	}
}
