package app

import (
	"context"
	"log/slog"
	"time"
)

// Dispatcher publica en el broker los comandos que quedaron en la outbox.
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
func (d *Dispatcher) DispatchOnce(ctx context.Context) (int, error) { return 0, nil }

// Run despacha en ciclo hasta que se cancele el contexto.
func (d *Dispatcher) Run(ctx context.Context) error { return nil }
