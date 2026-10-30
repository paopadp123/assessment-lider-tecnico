package postgres

import (
	"context"
	"fmt"
	"time"

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

// SaveDriverAlert guarda la alerta una sola vez por id. La clave primaria decide entre reintentos
// concurrentes (no un SELECT previo, que dejaría pasar a dos a la vez). Si el id ya existe, compara el
// contenido: igual es un reintento; distinto es un conflicto.
func (r *DriverAlertRepository) SaveDriverAlert(ctx context.Context, a app.DriverAlert) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO driver_alerts (id, plate, trip_id, type, occurred_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING`,
		a.ID, a.Plate.Raw(), a.TripID, a.Type, a.OccurredAt)
	if err != nil {
		return false, fmt.Errorf("insert driver alert: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	var plate, tripID, typ string
	var occurredAt time.Time
	err = r.pool.QueryRow(ctx, `SELECT plate, trip_id, type, occurred_at FROM driver_alerts WHERE id = $1`, a.ID).
		Scan(&plate, &tripID, &typ, &occurredAt)
	if err != nil {
		return false, fmt.Errorf("read existing driver alert: %w", err)
	}
	// timestamptz guarda microsegundos: se compara con esa precisión.
	if plate != a.Plate.Raw() || tripID != a.TripID || typ != a.Type || !occurredAt.Equal(a.OccurredAt.Truncate(time.Microsecond)) {
		return false, app.ErrDriverAlertConflict
	}
	return false, nil
}
