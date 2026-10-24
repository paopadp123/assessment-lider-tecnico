package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

const attemptHeader = "x-attempt"

// confirmTimeout acota la espera de la confirmación del broker. Sin tope, un broker que ni confirma ni
// cierra la conexión dejaría al despachador bloqueado con los locks de la outbox tomados.
const confirmTimeout = 5 * time.Second

type commandBody struct {
	OrderID string `json:"orderId"`
}

// Publisher publica los comandos de la outbox con publisher confirms: Publish devuelve nil solo cuando
// el broker confirmó que guardó el mensaje. Así el despachador nunca marca como publicado algo perdido.
type Publisher struct {
	mu   sync.Mutex // un canal AMQP no es seguro para uso concurrente
	ch   *amqp.Channel
	topo Topology
}

var _ app.MessagePublisher = (*Publisher)(nil)

// NewPublisher abre un canal en modo confirmación.
func NewPublisher(conn *amqp.Connection, topo Topology) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open publisher channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return &Publisher{ch: ch, topo: topo}, nil
}

// Publish envía el comando a la cola de trabajo con el intento 1.
func (p *Publisher) Publish(ctx context.Context, msg app.OutboxMessage) error {
	body, err := json.Marshal(commandBody{OrderID: string(msg.OrderID)})
	if err != nil {
		return fmt.Errorf("encode command: %w", err)
	}
	return p.publish(ctx, p.topo.CommandsExchange(), string(msg.Type), amqp.Publishing{
		MessageId: msg.MessageID,
		Type:      string(msg.Type),
		Headers:   amqp.Table{attemptHeader: int32(1)},
		Body:      body,
	})
}

func (p *Publisher) publish(ctx context.Context, exchange, key string, m amqp.Publishing) error {
	m.ContentType = "application/json"
	m.DeliveryMode = amqp.Persistent // sobrevive a un reinicio del broker

	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	p.mu.Lock()
	defer p.mu.Unlock()
	confirm, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, m)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", exchange, err)
	}
	ok, err := confirm.WaitContext(ctx)
	if err != nil {
		// Sin confirmación a tiempo se trata como no publicado: la outbox lo deja pendiente.
		return fmt.Errorf("wait broker confirm: %w", err)
	}
	if !ok {
		return fmt.Errorf("broker rejected message %s", m.MessageId)
	}
	return nil
}

// Closed avisa cuando el canal se cierra (p. ej. se cae el broker). El proceso debe terminar para que
// el orquestador de contenedores lo reinicie, en vez de seguir intentando publicar sobre un canal muerto.
func (p *Publisher) Closed() <-chan *amqp.Error {
	return p.ch.NotifyClose(make(chan *amqp.Error, 1))
}

// Close cierra el canal.
func (p *Publisher) Close() error { return p.ch.Close() }
