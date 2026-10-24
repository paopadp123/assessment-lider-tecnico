// Package auth autentica a los operadores de la consola: login con contraseña (bcrypt) y token JWT corto.
package auth

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials es la única respuesta ante un login fallido: no distingue un usuario inexistente,
	// inactivo o con otra contraseña, para no revelar qué usuarios existen.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken indica un token ausente, mal firmado, de otro emisor o vencido.
	ErrInvalidToken = errors.New("invalid token")
	// ErrOperatorNotFound lo devuelve el Store cuando el usuario no existe.
	ErrOperatorNotFound = errors.New("operator not found")
)

// Operator es la identidad de quien ejecuta una acción operativa.
type Operator struct {
	Username    string
	DisplayName string
}

// StoredOperator es un operador tal como se guarda.
type StoredOperator struct {
	Username     string
	DisplayName  string
	PasswordHash string
	Active       bool
}

// Store es el puerto de persistencia de operadores.
type Store interface {
	// FindOperator devuelve el operador o ErrOperatorNotFound.
	FindOperator(ctx context.Context, username string) (StoredOperator, error)
	// CreateOperatorIfMissing crea el operador si no existe; si existe, no lo modifica.
	CreateOperatorIfMissing(ctx context.Context, op StoredOperator) error
}

// Token es un JWT emitido tras un login correcto.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// Service emite y verifica los tokens de los operadores.
type Service struct {
	store  Store
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// MinSecretBytes es el largo mínimo del secreto de firma (HS256 con un secreto corto se puede adivinar).
const MinSecretBytes = 32

// NewService valida el secreto y arma el servicio.
func NewService(store Store, secret []byte, ttl time.Duration) (*Service, error) {
	return &Service{store: store, secret: secret, ttl: ttl, now: time.Now}, nil
}

// Login verifica las credenciales y emite un token.
func (s *Service) Login(ctx context.Context, username, password string) (Token, error) {
	return Token{}, errors.New("not implemented")
}

// Verify valida el token y devuelve el operador.
func (s *Service) Verify(token string) (Operator, error) {
	return Operator{}, errors.New("not implemented")
}

// Bootstrap crea el primer operador si no existe, para poder entrar a la consola sin gestión de usuarios.
func (s *Service) Bootstrap(ctx context.Context, username, displayName, password string) error {
	return errors.New("not implemented")
}
