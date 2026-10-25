// Package postgres implementa los puertos de persistencia sobre PostgreSQL con pgx.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// migrationLockID identifica el advisory lock que serializa las migraciones entre réplicas.
const migrationLockID = 7_340_001

// Migrate aplica en orden las migraciones pendientes, cada una en su transacción.
// El advisory lock evita que dos réplicas que arrancan a la vez migren en paralelo.
// Es un lock de sesión: por eso la conexión se saca del pool y se cierra al final,
// así el lock se libera siempre y nunca vuelve al pool una sesión que lo retenga.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	pooled, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	conn := pooled.Hijack()
	defer conn.Close(context.WithoutCancel(ctx)) //nolint:errcheck // cerrar la sesión libera el lock

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)

	for _, file := range files {
		if err := applyMigration(ctx, conn, file); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, file string) error {
	var applied bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", file).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %s: %w", file, err)
	}
	if applied {
		return nil
	}

	sql, err := migrations.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", file, err)
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", file); err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
		return nil
	})
}
