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
	cmd       app.CommandType
	handler   CommandHandler
	log       *slog.Logger
	prefetch  int
	retryBase time.Duration
	backoff   func(base time.Duration, attempt int) time.Duration // reemplazable en pruebas para medir el TTL
}

// NewWorker arma el worker de un comando. retryBase es la base del backoff exponencial entre reintentos.
func NewWorker(conn *amqp.Connection, topo Topology, cmd app.CommandType, handler CommandHandler, log *slog.Logger, retryBase time.Duration) *Worker {
	return &Worker{conn: conn, topo: topo, cmd: cmd, handler: handler, log: log.With("command", cmd),
		prefetch: 10, retryBase: retryBase, backoff: app.Backoff}
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

	// Consume sin contexto a propósito: ConsumeWithContext cancela el consumidor desde otra goroutine al
	// cancelarse el contexto, y esa cancelación compite con el Close diferido sobre el mismo canal; en CI
	// (Linux con -race) dejó Close esperando para siempre. El apagado se hace aquí, en secuencia.
	consumerTag := string(w.cmd) + "-worker"
	queue := w.topo.Queue(w.cmd)
	deliveries, err := ch.Consume(queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}
	for {
		select {
		case <-ctx.Done():
			// Primero se deja de recibir; los mensajes sin ack vuelven a la cola al cerrar el canal.
			if err := ch.Cancel(consumerTag, false); err != nil {
				w.log.Warn("could not cancel consumer", "error", err)
			}
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
		if nackErr := d.Nack(false, false); nackErr != nil {
			log.Error("could not reject poison message", "error", nackErr)
		}
		return
	}

	res := w.handler.Execute(ctx, app.Delivery{MessageID: d.MessageId, OrderID: order.OrderID(body.OrderID), Attempt: attempt})

	var err error
	switch res.Outcome {
	case app.CommandDone:
		err = d.Ack(false)
	case app.CommandRetry:
		// El jitter puede dar 0; un TTL mínimo de 1 ms garantiza que el mensaje no se quede sin vencer en la cola de reintento.
		delay := max(w.backoff(w.retryBase, attempt), time.Millisecond)
		err = w.forward(ctx, pub, d, w.topo.RetryExchange(), retryRoutingKey(w.cmd, min(attempt, w.topo.RetryLevels)), attempt+1, res.Reason, delay)
	case app.CommandDeadLetter:
		// Si esta copia falla, el mensaje se reencola; la reentrega encuentra la orden en FAILED y termina
		// sin copia en la DLQ del broker. La fila de dead_letters (fuente para operaciones) sí quedó guardada.
		err = w.forward(ctx, pub, d, w.topo.DeadLetterExchange(), string(w.cmd), attempt, res.Reason, 0)
	default:
		err = fmt.Errorf("unknown outcome %q", res.Outcome)
	}
	if err != nil {
		// No se pudo reprogramar: se devuelve a la cola para no perderlo.
		log.Error("could not settle message, requeueing", "outcome", res.Outcome, "error", err)
		if nackErr := d.Nack(false, true); nackErr != nil {
			// Si tampoco se puede devolver, el broker lo reentrega al cerrarse el canal (nunca se pierde).
			log.Error("could not requeue message", "error", nackErr)
		}
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
		m.Expiration = strconv.FormatInt(max(delay.Milliseconds(), 1), 10) // TTL por mensaje en milisegundos; la DLQ no lleva TTL
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
