package app

import (
	"context"
	"log/slog"
	"time"
)

// Reconciler revisa periódicamente las órdenes atascadas y las destraba según su estado.
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

// RunOnce reconcilia un lote y devuelve cuántas órdenes revisó.
func (r *Reconciler) RunOnce(ctx context.Context) (int, error) {
	return 0, nil
}
