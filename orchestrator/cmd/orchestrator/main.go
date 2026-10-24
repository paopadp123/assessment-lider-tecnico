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
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/postgres"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/psp"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/quoter"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/rabbitmq"
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
	pspURL, err := requiredEnv("PSP_URL")
	if err != nil {
		return err
	}
	webhookSecret, err := requiredEnv("PSP_WEBHOOK_SECRET")
	if err != nil {
		return err
	}
	verifier, err := httpapi.NewSignatureVerifier(webhookSecret)
	if err != nil {
		return fmt.Errorf("PSP_WEBHOOK_SECRET: %w", err)
	}
	rabbitURL, err := requiredEnv("RABBITMQ_URL")
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

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()
	topo := rabbitmq.Topology{RetryLevels: 3}
	topoCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open topology channel: %w", err)
	}
	if err := topo.Declare(topoCh); err != nil {
		return fmt.Errorf("declare topology: %w", err)
	}
	_ = topoCh.Close()
	publisher, err := rabbitmq.NewPublisher(conn, topo)
	if err != nil {
		return err
	}
	defer publisher.Close()

	repo := postgres.NewOrderRepository(pool)
	quoteClient := quoter.NewClient(quoterURL, &http.Client{})
	pspClient := psp.NewClient(pspURL, &http.Client{})
	createOrder := app.NewCreateOrder(repo, quoteClient, logger)
	handlePayment := app.NewHandlePaymentWebhook(repo, logger)
	requestPayment := app.NewRequestPayment(repo, repo, pspClient, logger)
	dispatcher := app.NewDispatcher(repo, publisher, logger)
	worker := rabbitmq.NewWorker(conn, topo, requestPayment, logger, time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", httpapi.Health)
	httpapi.NewOrders(createOrder, repo, logger).Register(mux)
	httpapi.NewWebhooks(handlePayment, verifier, logger).Register(mux)

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second, // corta clientes que envían el cuerpo a goteo (slowloris)
		WriteTimeout:      20 * time.Second, // cubre la cotización con sus reintentos; el pago ya no va en la petición
	}

	// API, despachador de la outbox y worker de pagos corren juntos en este proceso. Si cualquiera falla
	// (p. ej. se cae la conexión con RabbitMQ), se detiene todo y el orquestador de contenedores lo reinicia.
	// En producción serían despliegues separados que escalan por separado (ver ADR 0002).
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		logger.Info("orchestrator listening", "addr", addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	})
	g.Go(func() error { return dispatcher.Run(gctx) })
	publisherClosed := publisher.Closed()
	g.Go(func() error {
		select {
		case <-gctx.Done():
			return nil
		case err := <-publisherClosed:
			return fmt.Errorf("rabbitmq publisher channel closed: %v", err)
		}
	})
	g.Go(func() error { return worker.Run(gctx) })
	g.Go(func() error {
		<-gctx.Done()
		logger.Info("shutting down")
		// Termina las solicitudes en curso antes de salir, para no dejar órdenes a medias.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})
	return g.Wait()
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
