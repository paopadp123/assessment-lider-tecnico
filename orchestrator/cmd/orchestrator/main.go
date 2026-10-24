// Command orchestrator arranca el servicio orquestador de compra de pólizas.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/postgres"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/quoter"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("orchestrator stopped", "error", err)
		os.Exit(1)
	}
}

// run arma los adaptadores alrededor de los casos de uso y atiende hasta recibir SIGINT/SIGTERM.
func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	quoterURL, err := requiredEnv("QUOTER_URL")
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	repo := postgres.NewOrderRepository(pool)
	quoteClient := quoter.NewClient(quoterURL, &http.Client{})
	createOrder := app.NewCreateOrder(repo, quoteClient, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", httpapi.Health)
	httpapi.NewOrders(createOrder, repo, logger).Register(mux)

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second, // corta clientes que envían el cuerpo a goteo (slowloris)
		// Cubre el peor caso de cotización: 3 intentos de hasta 4 s más el backoff.
		WriteTimeout: 20 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("orchestrator listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	// Termina las solicitudes en curso antes de salir, para no dejar órdenes a medias.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func requiredEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("missing required environment variable %s", key)
	}
	return v, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
