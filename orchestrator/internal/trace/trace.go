// Package trace maneja el correlation-id: un identificador por compra que acompaña a la orden en todos sus
// saltos (HTTP, outbox, RabbitMQ, terceros) y en todos sus logs.
package trace

import (
	"context"
	"log/slog"
)

// Header es la cabecera HTTP que transporta el correlation-id.
const Header = "X-Correlation-ID"

// New genera un correlation-id nuevo.
func New() string { return "" }

// Valid indica si un correlation-id recibido de afuera se puede usar.
func Valid(id string) bool { return false }

// WithID devuelve un contexto que transporta el correlation-id.
func WithID(ctx context.Context, id string) context.Context { return ctx }

// ID devuelve el correlation-id del contexto, o "".
func ID(ctx context.Context) string { return "" }

// Logger devuelve base con el correlation-id del contexto como atributo, si hay uno.
func Logger(ctx context.Context, base *slog.Logger) *slog.Logger { return base }
