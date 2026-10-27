package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

// DriverAlertRepository implementa app.DriverAlertStore sobre PostgreSQL.
type DriverAlertRepository struct {
	pool *pgxpool.Pool
}

var _ app.DriverAlertStore = (*DriverAlertRepository)(nil)

// NewDriverAlertRepository crea el repositorio de alertas del conductor.
func NewDriverAlertRepository(pool *pgxpool.Pool) *DriverAlertRepository {
	return &DriverAlertRepository{pool: pool}
}

// SaveDriverAlert guarda la alerta una sola vez por id.
func (r *DriverAlertRepository) SaveDriverAlert(ctx context.Context, a app.DriverAlert) (bool, error) {
	return false, errors.New("not implemented")
}
