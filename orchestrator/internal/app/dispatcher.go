package app

import (
	"context"
	"log/slog"
	"time"
)

// Dispatcher publica en el broker los comandos que quedaron en la outbox.
// Garantiza "al menos una vez": un mensaje se marca como publicado solo cuando el broker confirmó;
// si no confirma, queda pendiente para el siguiente ciclo. Los consumidores son idempotentes.
type Dispatcher struct {
	store    OutboxStore
	pub      MessagePublisher
	log      *slog.Logger
	batch    int
	interval time.Duration
}

// NewDispatcher arma el despachador.
func NewDispatcher(store OutboxStore, pub MessagePublisher, log *slog.Logger) *Dispatcher {
	return &Dispatcher{store: store, pub: pub, log: log, batch: 100, interval: 500 * time.Millisecond}
}

// DispatchOnce publica un lote de mensajes pendientes y devuelve cuántos se publicaron.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (int, error) {
	return d.store.PublishPending(ctx, d.batch, d.pub.Publish)
}

// Run despacha en ciclo hasta que se cancele el contexto. Si un lote salió lleno, sigue de inmediato
// (hay más trabajo); si no, espera el intervalo.
func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := d.DispatchOnce(ctx)
		if err != nil && ctx.Err() == nil {
			d.log.Error("outbox dispatch failed", "error", err)
		}
		if n == d.batch {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d.interval):
		}
	}
}
