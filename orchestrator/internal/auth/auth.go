// Package auth autentica a los operadores de la consola: login con contraseña (bcrypt) y token JWT corto.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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

const (
	// MinSecretBytes es el largo mínimo del secreto de firma (HS256 con un secreto corto se puede adivinar).
	MinSecretBytes = 32
	issuer         = "policy-orchestrator"
	audience       = "ops-console"
	clockSkew      = 30 * time.Second
)

// Service emite y verifica los tokens de los operadores.
type Service struct {
	store     Store
	secret    []byte
	ttl       time.Duration
	now       func() time.Time
	dummyHash []byte
}

type claims struct {
	Name string `json:"name"`
	jwt.RegisteredClaims
}

// NewService valida el secreto y arma el servicio.
func NewService(store Store, secret []byte, ttl time.Duration) (*Service, error) {
	if len(secret) < MinSecretBytes {
		return nil, fmt.Errorf("signing secret must have at least %d bytes", MinSecretBytes)
	}
	// Hash de una contraseña que nadie conoce: se compara contra él cuando el usuario no existe, para que el
	// login tarde lo mismo y el tiempo de respuesta no revele qué usuarios existen.
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	return &Service{store: store, secret: secret, ttl: ttl, now: time.Now, dummyHash: dummy}, nil
}

// Login verifica las credenciales y emite un token.
func (s *Service) Login(ctx context.Context, username, password string) (Token, error) {
	op, err := s.store.FindOperator(ctx, username)
	if err != nil && !errors.Is(err, ErrOperatorNotFound) {
		return Token{}, fmt.Errorf("find operator: %w", err)
	}
	hash := s.dummyHash
	if err == nil {
		hash = []byte(op.PasswordHash)
	}
	// Siempre se ejecuta bcrypt, exista o no el usuario.
	mismatch := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil || mismatch != nil || !op.Active {
		return Token{}, ErrInvalidCredentials
	}

	now := s.now()
	expires := now.Add(s.ttl)
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return Token{}, fmt.Errorf("token id: %w", err)
	}
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Name: op.DisplayName,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   op.Username,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        hex.EncodeToString(jti),
		},
	}).SignedString(s.secret)
	if err != nil {
		return Token{}, fmt.Errorf("sign token: %w", err)
	}
	return Token{Value: value, ExpiresAt: expires}, nil
}

// Verify valida firma (solo HS256: un token con "alg": "none" o con otro algoritmo se rechaza), emisor,
// audiencia y vencimiento, con una tolerancia de reloj de 30 s.
func (s *Service) Verify(token string) (Operator, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(clockSkew),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil || c.Subject == "" {
		return Operator{}, ErrInvalidToken
	}
	return Operator{Username: c.Subject, DisplayName: c.Name}, nil
}

// Bootstrap crea el primer operador si no existe, para poder entrar a la consola sin gestión de usuarios.
// Si ya existe no lo toca: reiniciar el proceso no cambia su contraseña.
func (s *Service) Bootstrap(ctx context.Context, username, displayName, password string) error {
	if username == "" || len(password) < 12 {
		return errors.New("bootstrap operator needs a username and a password of at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.store.CreateOperatorIfMissing(ctx, StoredOperator{
		Username: username, DisplayName: displayName, PasswordHash: string(hash), Active: true,
	})
}
