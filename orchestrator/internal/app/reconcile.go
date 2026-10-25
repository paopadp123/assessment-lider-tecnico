package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// Reconciler revisa periódicamente las órdenes atascadas y las destraba según su estado.
//
// Toma las órdenes con una transacción corta que solo las marca como revisadas (SKIP LOCKED, sin esperar a
// otras réplicas), y después decide cada una fuera de esa transacción: las llamadas a terceros no retienen
// locks. Los cambios de estado usan el mismo control optimista que el resto del flujo, así que si la orden
// avanzó entre la toma y la decisión, el cambio se descarta.
type Reconciler struct {
	repo           OrderRepository
	store          ReconcileStore
	quoter         Quoter
	issuer         PolicyIssuer
	log            *slog.Logger
	now            func() time.Time
	sleep          func(context.Context, time.Duration) error
	stuckAfter     time.Duration
	batch          int
	maxIssueMisses int
	attemptTimeout time.Duration
}

// NewReconciler arma el reconciliador. stuckAfter es el N del enunciado: cuánto tiempo sin avanzar
// convierte una orden en atascada.
func NewReconciler(repo OrderRepository, store ReconcileStore, quoter Quoter, issuer PolicyIssuer, log *slog.Logger, stuckAfter time.Duration) *Reconciler {
	return &Reconciler{repo: repo, store: store, quoter: quoter, issuer: issuer, log: log, now: utcMicros, sleep: sleepCtx,
		stuckAfter: stuckAfter, batch: 20, maxIssueMisses: 3, attemptTimeout: 10 * time.Second}
}

// Run reconcilia cada every hasta que se cancele el contexto. Un error de una ronda no detiene el job.
func (r *Reconciler) Run(ctx context.Context, every time.Duration) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if _, err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.log.Error("reconciliation round failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// RunOnce reconcilia un lote y devuelve cuántas órdenes revisó.
func (r *Reconciler) RunOnce(ctx context.Context) (int, error) {
	claimed, claimErr := r.store.ClaimStuck(ctx, r.now().Add(-r.stuckAfter), r.batch)
	if claimErr != nil {
		claimErr = fmt.Errorf("claim stuck orders: %w", claimErr)
	}
	for _, s := range claimed {
		// El job corre fuera de cualquier solicitud: cada orden usa el correlation-id de su compra.
		orderCtx := trace.WithID(ctx, s.Order.CorrelationID)
		log := trace.Logger(orderCtx, r.log).With("order_id", s.Order.ID, "status", s.Order.Status, "stuck_since", s.Order.UpdatedAt)
		// Una orden que falla no frena a las demás; se vuelve a revisar en el próximo intervalo.
		if err := r.reconcile(orderCtx, log, s); err != nil && !errors.Is(err, order.ErrInvalidTransition) {
			log.Error("could not reconcile order", "error", err)
		}
	}
	return len(claimed), claimErr
}

func (r *Reconciler) reconcile(ctx context.Context, log *slog.Logger, s StuckOrder) error {
	o := s.Order
	switch o.Status {
	case order.Created:
		// El proceso murió antes de guardar la cotización. No hay dinero en juego: se vuelve a cotizar.
		log.Warn("stuck order quoted again")
		ev, cmds, err := quoteOrder(ctx, r.quoter, r.sleep, log, r.now, &o)
		if err != nil {
			return err
		}
		return r.repo.SaveTransition(ctx, o, ev, cmds...)
	case order.Quoted:
		// Si el PSP ya había aceptado, la misma Idempotency-Key devuelve el mismo paymentId: no hay doble cobro.
		log.Warn("stuck order: payment request enqueued again")
		return r.store.Enqueue(ctx, Command{Type: CommandRequestPayment, OrderID: o.ID})
	case order.PaymentPending:
		// El PSP no ofrece consulta: solo se puede hacer visible. El webhook la destrabará cuando llegue.
		log.Warn("stuck order waiting for payment webhook; PSP has no lookup to verify it", "payment_id", o.PaymentID)
		return nil
	case order.Paid:
		log.Warn("stuck order: policy issue enqueued again")
		return r.store.Enqueue(ctx, Command{Type: CommandIssuePolicy, OrderID: o.ID})
	case order.Issuing:
		return r.reconcileIssuing(ctx, log, s)
	default:
		return nil
	}
}

// reconcileIssuing consulta al emisor antes de decidir: un timeout nunca autoriza a reemitir.
func (r *Reconciler) reconcileIssuing(ctx context.Context, log *slog.Logger, s StuckOrder) error {
	o := s.Order
	attemptCtx, cancel := context.WithTimeout(ctx, r.attemptTimeout)
	number, err := r.issuer.FindByExternalRef(attemptCtx, o.ID)
	cancel()

	switch {
	case err == nil:
		log.Warn("stuck order: policy found by external reference")
		return recordPolicy(ctx, r.repo, r.now, log, o, number)
	case !errors.Is(err, ErrPolicyNotFound):
		// Breaker abierto o emisor caído: sin respuesta confiable no se cuenta la ronda ni se decide nada.
		log.Warn("stuck order: issuer unavailable, will check again", "error", err)
		return nil
	}

	misses, err := r.store.RecordIssueMiss(ctx, o.ID)
	if err != nil {
		return err
	}
	if misses < r.maxIssueMisses {
		log.Warn("stuck order: policy not issued, issue enqueued again", "round", misses)
		return r.store.Enqueue(ctx, Command{Type: CommandIssuePolicy, OrderID: o.ID})
	}

	// Falla definitiva: pago aprobado sin póliza. Operaciones decide el reverso; no se reembolsa automáticamente.
	log.Error("policy issuance failed definitively; refund required", "rounds", misses, "reason", ReasonIssueFailedDefinitively)
	o.FailureReason = ReasonIssueFailedDefinitively
	_, err = transition(ctx, r.repo, r.now, o, order.RefundRequired, ReasonIssueFailedDefinitively)
	return err
}
