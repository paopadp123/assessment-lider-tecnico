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

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/issuer"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/postgres"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/psp"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/quoter"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/adapters/rabbitmq"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
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
	issuerURL, err := requiredEnv("ISSUER_URL")
	if err != nil {
		return err
	}
	jwtSecret, err := requiredEnv("JWT_SECRET")
	if err != nil {
		return err
	}
	stuckAfter, err := durationEnv("STUCK_AFTER", 5*time.Minute)
	if err != nil {
		return err
	}
	reconcileEvery, err := durationEnv("RECONCILE_EVERY", time.Minute)
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
	// El breaker se abre tras 5 fallas seguidas y prueba de nuevo a los 30 s. Las emisiones que llegan con el
	// circuito abierto esperan en la cola ese mismo tiempo, sin consumir intentos (ver ADR 0003).
	const issuerOpenTimeout = 30 * time.Second
	policyIssuer := issuer.NewBreaker(issuer.NewClient(issuerURL, &http.Client{}),
		issuer.BreakerSettings{FailureThreshold: 5, OpenTimeout: issuerOpenTimeout}, logger)
	issuePolicy := app.NewIssuePolicy(repo, repo, policyIssuer, logger, issuerOpenTimeout)
	dispatcher := app.NewDispatcher(repo, publisher, logger)
	reconciler := app.NewReconciler(repo, repo, quoteClient, policyIssuer, logger, stuckAfter)
	operatorActions := app.NewOperatorActions(repo, logger)

	authService, err := auth.NewService(postgres.NewOperatorRepository(pool), []byte(jwtSecret), time.Hour)
	if err != nil {
		return fmt.Errorf("JWT_SECRET: %w", err)
	}
	if err := bootstrapOperator(ctx, authService); err != nil {
		return err
	}
	requireOperator := httpapi.RequireOperator(authService)
	paymentWorker := rabbitmq.NewWorker(conn, topo, app.CommandRequestPayment, requestPayment, logger, time.Second)
	issueWorker := rabbitmq.NewWorker(conn, topo, app.CommandIssuePolicy, issuePolicy, logger, time.Second)

	ops := httpapi.NewOps(operatorActions, repo, logger)
	mux := http.NewServeMux()
	httpapi.Routes(mux, httpapi.Handlers{
		Orders:   httpapi.NewOrders(createOrder, repo, logger),
		Webhooks: httpapi.NewWebhooks(handlePayment, verifier, logger),
		Login:    httpapi.NewLogin(authService, httpapi.NewIPRateLimiter(5), logger),
		Ops:      ops,
		Breakers: httpapi.Breakers(func() httpapi.BreakerStatus {
			st := policyIssuer.Status()
			return httpapi.BreakerStatus{Name: st.Name, State: st.State, ConsecutiveFailures: st.ConsecutiveFailures}
		}),
		// Listo = la base responde y la conexión con RabbitMQ sigue abierta. Si no, el balanceador deja de
		// enviarle tráfico a esta réplica sin reiniciarla.
		Ready: httpapi.Ready(
			httpapi.Check{Name: "postgres", Probe: pool.Ping},
			httpapi.Check{Name: "rabbitmq", Probe: func(context.Context) error {
				if conn.IsClosed() {
					return errors.New("connection closed")
				}
				return nil
			}},
		),
		Require: requireOperator,
	})

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.WithCorrelation(mux), // toda solicitud tiene correlation-id
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second, // corta clientes que envían el cuerpo a goteo (slowloris)
		WriteTimeout:      20 * time.Second, // cubre la cotización con sus reintentos; el flujo SSE lo desactiva para sí
	}
	srv.RegisterOnShutdown(ops.CloseStreams) // los flujos SSE abiertos no deben retrasar el apagado

	// API, despachador de la outbox, workers de pago y emisión y reconciliador corren juntos en este proceso. Si cualquiera falla
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
	g.Go(func() error { return paymentWorker.Run(gctx) })
	g.Go(func() error { return issueWorker.Run(gctx) })
	g.Go(func() error { return reconciler.Run(gctx, reconcileEvery) })
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

// bootstrapOperator crea el primer operador de la consola si OPS_BOOTSTRAP_USER y OPS_BOOTSTRAP_PASSWORD
// están definidas. Si ya existe, no lo modifica.
func bootstrapOperator(ctx context.Context, svc *auth.Service) error {
	user, password := os.Getenv("OPS_BOOTSTRAP_USER"), os.Getenv("OPS_BOOTSTRAP_PASSWORD")
	if user == "" && password == "" {
		return nil
	}
	if err := svc.Bootstrap(ctx, user, envOr("OPS_BOOTSTRAP_NAME", user), password); err != nil {
		return fmt.Errorf("bootstrap operator: %w", err)
	}
	return nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration like 5m: %q", key, v)
	}
	return d, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
