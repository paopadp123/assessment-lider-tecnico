package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const reasonHeader = "x-reason"

// CommandHandler es el caso de uso que procesa un comando recibido.
type CommandHandler interface {
	Execute(ctx context.Context, d app.Delivery) app.CommandResult
}

// Worker consume la cola de trabajo y traduce el resultado del caso de uso a operaciones del broker:
// DONE → ack; RETRY → copia a la cola de reintento del nivel con TTL = backoff con jitter, y ack;
// DEAD_LETTER → copia a la DLQ con el motivo, y ack. El ack siempre va después de que el broker confirmó
// la copia: si el proceso muere en medio, el mensaje se reentrega (al menos una vez) y el caso de uso,
// idempotente por estado, no repite el efecto.
type Worker struct {
	conn      *amqp.Connection
	topo      Topology
	handler   CommandHandler
	log       *slog.Logger
	prefetch  int
	retryBase time.Duration
	backoff   func(base time.Duration, attempt int) time.Duration // reemplazable en pruebas para medir el TTL
}

// NewWorker arma el worker. retryBase es la base del backoff exponencial entre reintentos.
func NewWorker(conn *amqp.Connection, topo Topology, handler CommandHandler, log *slog.Logger, retryBase time.Duration) *Worker {
	return &Worker{conn: conn, topo: topo, handler: handler, log: log, prefetch: 10, retryBase: retryBase, backoff: app.Backoff}
}

// Run consume hasta que se cancele el contexto. Si el canal se cierra (caída del broker), devuelve error:
// el proceso termina y el orquestador de contenedores lo reinicia.
func (w *Worker) Run(ctx context.Context) error {
	ch, err := w.conn.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}
	defer ch.Close()
	// prefetch acota cuántos mensajes sin confirmar recibe el worker: es la contrapresión (backpressure)
	// que evita que un pico en la cola sature al worker o a los terceros.
	if err := ch.Qos(w.prefetch, 0, false); err != nil {
		return fmt.Errorf("set prefetch: %w", err)
	}
	pub, err := NewPublisher(w.conn, w.topo)
	if err != nil {
		return err
	}
	defer pub.Close()

	deliveries, err := ch.ConsumeWithContext(ctx, w.topo.PaymentQueue(), "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", w.topo.PaymentQueue(), err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("consumer channel closed")
			}
			w.handle(ctx, pub, d)
		}
	}
}

func (w *Worker) handle(ctx context.Context, pub *Publisher, d amqp.Delivery) {
	attempt := attemptOf(d)
	log := w.log.With("message_id", d.MessageId, "attempt", attempt)

	var body commandBody
	if err := json.Unmarshal(d.Body, &body); err != nil || body.OrderID == "" {
		// Mensaje venenoso: reintentarlo no lo arregla. Sin reencolar, la cola lo envía a la DLQ.
		log.Error("poison message sent to dead letter queue", "error", err)
		_ = d.Nack(false, false)
		return
	}

	res := w.handler.Execute(ctx, app.Delivery{MessageID: d.MessageId, OrderID: order.OrderID(body.OrderID), Attempt: attempt})

	var err error
	switch res.Outcome {
	case app.CommandDone:
		err = d.Ack(false)
	case app.CommandRetry:
		err = w.forward(ctx, pub, d, w.topo.RetryExchange(), retryRoutingKey(min(attempt, w.topo.RetryLevels)), attempt+1, res.Reason,
			w.backoff(w.retryBase, attempt))
	case app.CommandDeadLetter:
		err = w.forward(ctx, pub, d, w.topo.DeadLetterExchange(), paymentRoutingKey, attempt, res.Reason, 0)
	default:
		err = fmt.Errorf("unknown outcome %q", res.Outcome)
	}
	if err != nil {
		// No se pudo reprogramar: se devuelve a la cola para no perderlo.
		log.Error("could not settle message, requeueing", "outcome", res.Outcome, "error", err)
		_ = d.Nack(false, true)
	}
}

// forward publica una copia (con confirmación) y solo entonces confirma el original.
func (w *Worker) forward(ctx context.Context, pub *Publisher, d amqp.Delivery, exchange, key string, attempt int, reason string, delay time.Duration) error {
	m := amqp.Publishing{
		MessageId: d.MessageId,
		Type:      d.Type,
		Headers:   amqp.Table{attemptHeader: int32(attempt), reasonHeader: reason},
		Body:      d.Body,
	}
	if delay > 0 {
		m.Expiration = strconv.FormatInt(max(delay.Milliseconds(), 1), 10) // TTL por mensaje en milisegundos
	}
	if err := pub.publish(ctx, exchange, key, m); err != nil {
		return err
	}
	return d.Ack(false)
}

func attemptOf(d amqp.Delivery) int {
	switch v := d.Headers[attemptHeader].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 1
	}
}
