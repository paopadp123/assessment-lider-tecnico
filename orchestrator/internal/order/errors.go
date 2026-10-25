package order

import (
	"errors"
	"strings"
)

// ErrInvalidTransition indica un cambio de estado que el flujo no permite.
var ErrInvalidTransition = errors.New("invalid status transition")

// ValidationError describe un campo de entrada inválido.
type ValidationError struct {
	Field   string
	Message string
}

// ValidationErrors agrupa todos los campos inválidos de una solicitud.
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	parts := make([]string, len(v))
	for i, e := range v {
		parts[i] = e.Field + ": " + e.Message
	}
	return "validation failed: " + strings.Join(parts, "; ")
}
