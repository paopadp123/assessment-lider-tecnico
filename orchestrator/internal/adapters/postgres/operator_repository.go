package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
)

// OperatorRepository implementa auth.Store sobre PostgreSQL.
type OperatorRepository struct {
	pool *pgxpool.Pool
}

var _ auth.Store = (*OperatorRepository)(nil)

// NewOperatorRepository crea el repositorio de operadores.
func NewOperatorRepository(pool *pgxpool.Pool) *OperatorRepository {
	return &OperatorRepository{pool: pool}
}

// FindOperator busca un operador por usuario.
func (r *OperatorRepository) FindOperator(ctx context.Context, username string) (auth.StoredOperator, error) {
	var op auth.StoredOperator
	err := r.pool.QueryRow(ctx,
		`SELECT username, display_name, password_hash, active FROM operators WHERE username = $1`, username,
	).Scan(&op.Username, &op.DisplayName, &op.PasswordHash, &op.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.StoredOperator{}, auth.ErrOperatorNotFound
	}
	if err != nil {
		return auth.StoredOperator{}, fmt.Errorf("find operator: %w", err)
	}
	return op, nil
}

// CreateOperatorIfMissing crea el operador si no existe; si existe, no lo modifica.
func (r *OperatorRepository) CreateOperatorIfMissing(ctx context.Context, op auth.StoredOperator) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO operators (username, display_name, password_hash, active) VALUES ($1, $2, $3, $4)
		ON CONFLICT (username) DO NOTHING`, op.Username, op.DisplayName, op.PasswordHash, op.Active)
	if err != nil {
		return fmt.Errorf("create operator: %w", err)
	}
	return nil
}
