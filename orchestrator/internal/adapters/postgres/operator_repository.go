package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
)

// OperatorRepository implementa auth.Store sobre PostgreSQL.
type OperatorRepository struct {
	pool *pgxpool.Pool
}

var _ auth.Store = (*OperatorRepository)(nil)

// NewOperatorRepository crea el repositorio de operadores.
func NewOperatorRepository(pool *pgxpool.Pool) *OperatorRepository { return &OperatorRepository{pool: pool} }

// FindOperator busca un operador por usuario.
func (r *OperatorRepository) FindOperator(ctx context.Context, username string) (auth.StoredOperator, error) {
	return auth.StoredOperator{}, errors.New("not implemented")
}

// CreateOperatorIfMissing crea el operador si no existe.
func (r *OperatorRepository) CreateOperatorIfMissing(ctx context.Context, op auth.StoredOperator) error {
	return errors.New("not implemented")
}
