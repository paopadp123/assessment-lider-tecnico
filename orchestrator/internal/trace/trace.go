// Package trace maneja el correlation-id: un identificador por compra que acompaña a la orden en todos sus
// saltos (HTTP, outbox, RabbitMQ, terceros) y en todos sus logs.
package trace

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"regexp"
)

// Header es la cabecera HTTP que transporta el correlation-id.
const Header = "X-Correlation-ID"

// validID acota lo que se acepta de afuera: el id termina en los logs, así que nada de espacios, comillas ni
// saltos de línea (no se puede falsificar una línea de log) y un largo máximo.
var validID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

type key struct{}

// New genera un correlation-id nuevo (UUID v4).
func New() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand no falla en las plataformas soportadas
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Valid indica si un correlation-id recibido de afuera se puede usar.
func Valid(id string) bool { return validID.MatchString(id) }

// WithID devuelve un contexto que transporta el correlation-id.
func WithID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

// ID devuelve el correlation-id del contexto, o "".
func ID(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}

// Logger devuelve base con el correlation-id del contexto como atributo, si hay uno.
func Logger(ctx context.Context, base *slog.Logger) *slog.Logger {
	if id := ID(ctx); id != "" {
		return base.With("correlation_id", id)
	}
	return base
}
