// Package rabbitmq implementa la publicación y el consumo de comandos sobre RabbitMQ.
package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

const paymentRoutingKey = string(app.CommandRequestPayment)

// Topology nombra y declara los exchanges y colas. Prefix permite aislar entornos y pruebas.
//
//	orders.commands (direct) ──request_payment──▶ orders.payment.request ──(nack sin requeue)──▶ orders.dlx
//	orders.retry    (direct) ──request_payment.N──▶ ...retry.N (TTL por mensaje) ──(expira)──▶ orders.commands
//	orders.dlx      (direct) ──request_payment──▶ orders.payment.request.dlq
type Topology struct {
	Prefix      string
	RetryLevels int
}

func (t Topology) CommandsExchange() string   { return t.Prefix + "orders.commands" }
func (t Topology) RetryExchange() string      { return t.Prefix + "orders.retry" }
func (t Topology) DeadLetterExchange() string { return t.Prefix + "orders.dlx" }
func (t Topology) PaymentQueue() string       { return t.Prefix + "orders.payment.request" }
func (t Topology) DeadLetterQueue() string    { return t.PaymentQueue() + ".dlq" }
func (t Topology) RetryQueue(level int) string {
	return fmt.Sprintf("%s.retry.%d", t.PaymentQueue(), level)
}
func retryRoutingKey(level int) string { return fmt.Sprintf("%s.%d", paymentRoutingKey, level) }

// Declare crea (o confirma, si ya existen) exchanges, colas y enlaces. Es idempotente.
func (t Topology) Declare(ch *amqp.Channel) error {
	for _, ex := range []string{t.CommandsExchange(), t.RetryExchange(), t.DeadLetterExchange()} {
		if err := ch.ExchangeDeclare(ex, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", ex, err)
		}
	}

	// Un mensaje que el worker rechaza sin reencolar (mensaje venenoso) termina en la DLQ.
	if err := declareAndBind(ch, t.PaymentQueue(), paymentRoutingKey, t.CommandsExchange(), amqp.Table{
		"x-dead-letter-exchange":    t.DeadLetterExchange(),
		"x-dead-letter-routing-key": paymentRoutingKey,
	}); err != nil {
		return err
	}

	// Cada nivel de reintento retiene el mensaje durante su TTL y, al expirar, lo devuelve a la cola de trabajo.
	for level := 1; level <= t.RetryLevels; level++ {
		if err := declareAndBind(ch, t.RetryQueue(level), retryRoutingKey(level), t.RetryExchange(), amqp.Table{
			"x-dead-letter-exchange":    t.CommandsExchange(),
			"x-dead-letter-routing-key": paymentRoutingKey,
		}); err != nil {
			return err
		}
	}

	return declareAndBind(ch, t.DeadLetterQueue(), paymentRoutingKey, t.DeadLetterExchange(), nil)
}

func declareAndBind(ch *amqp.Channel, queue, key, exchange string, args amqp.Table) error {
	if _, err := ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare queue %s: %w", queue, err)
	}
	if err := ch.QueueBind(queue, key, exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", queue, err)
	}
	return nil
}
