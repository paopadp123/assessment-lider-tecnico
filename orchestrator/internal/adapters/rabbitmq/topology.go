// Package rabbitmq implementa la publicación y el consumo de comandos sobre RabbitMQ.
package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

// queueNames da a cada comando su cola de trabajo. Cada comando tiene sus propias colas de reintento y
// su DLQ: una caída del emisor no frena los pagos, y operaciones ve por separado qué se atascó.
var queueNames = map[app.CommandType]string{
	app.CommandRequestPayment: "orders.payment.request",
	app.CommandIssuePolicy:    "orders.policy.issue",
}

// Topology nombra y declara los exchanges y colas. Prefix permite aislar entornos y pruebas.
// Para cada comando (la routing key es el nombre del comando, p. ej. request_payment):
//
//	orders.commands (direct) ──<cmd>──▶ <cola> ──(nack sin requeue)──▶ orders.dlx
//	orders.retry    (direct) ──<cmd>.N──▶ <cola>.retry.N (TTL por mensaje) ──(expira)──▶ orders.commands
//	orders.retry    (direct) ──<cmd>.deferred──▶ <cola>.deferred (TTL por mensaje) ──(expira)──▶ orders.commands
//	orders.dlx      (direct) ──<cmd>──▶ <cola>.dlq
//
// RabbitMQ solo vence el mensaje que está al frente de una cola. Por eso cada cola de espera agrupa mensajes
// con esperas parecidas: un aplazamiento largo (breaker abierto) en la cola de un reintento corto dejaría
// esperando a los reintentos que llegan detrás.
type Topology struct {
	Prefix      string
	RetryLevels int
}

func (t Topology) CommandsExchange() string   { return t.Prefix + "orders.commands" }
func (t Topology) RetryExchange() string      { return t.Prefix + "orders.retry" }
func (t Topology) DeadLetterExchange() string { return t.Prefix + "orders.dlx" }

// Commands son los comandos que la topología enruta.
func (t Topology) Commands() []app.CommandType {
	return []app.CommandType{app.CommandRequestPayment, app.CommandIssuePolicy}
}

// Queue es la cola de trabajo del comando.
func (t Topology) Queue(cmd app.CommandType) string { return t.Prefix + queueNames[cmd] }

// DeadLetterQueue es la DLQ del comando.
func (t Topology) DeadLetterQueue(cmd app.CommandType) string { return t.Queue(cmd) + ".dlq" }

// RetryQueue es la cola de espera del nivel de reintento del comando.
func (t Topology) RetryQueue(cmd app.CommandType, level int) string {
	return fmt.Sprintf("%s.retry.%d", t.Queue(cmd), level)
}

// DeferredQueue es la cola de espera de los comandos aplazados sin consumir intento.
func (t Topology) DeferredQueue(cmd app.CommandType) string { return t.Queue(cmd) + ".deferred" }

// Queues son todas las colas declaradas (útil para limpiar en pruebas).
func (t Topology) Queues() []string {
	var qs []string
	for _, cmd := range t.Commands() {
		qs = append(qs, t.Queue(cmd), t.DeadLetterQueue(cmd), t.DeferredQueue(cmd))
		for level := 1; level <= t.RetryLevels; level++ {
			qs = append(qs, t.RetryQueue(cmd, level))
		}
	}
	return qs
}

func retryRoutingKey(cmd app.CommandType, level int) string { return fmt.Sprintf("%s.%d", cmd, level) }

func deferredRoutingKey(cmd app.CommandType) string { return string(cmd) + ".deferred" }

// Declare crea (o confirma, si ya existen) exchanges, colas y enlaces. Es idempotente.
func (t Topology) Declare(ch *amqp.Channel) error {
	for _, ex := range []string{t.CommandsExchange(), t.RetryExchange(), t.DeadLetterExchange()} {
		if err := ch.ExchangeDeclare(ex, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", ex, err)
		}
	}
	for _, cmd := range t.Commands() {
		if err := t.declareCommand(ch, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (t Topology) declareCommand(ch *amqp.Channel, cmd app.CommandType) error {
	key := string(cmd)
	// Un mensaje que el worker rechaza sin reencolar (mensaje venenoso) termina en la DLQ.
	if err := declareAndBind(ch, t.Queue(cmd), key, t.CommandsExchange(), amqp.Table{
		"x-dead-letter-exchange":    t.DeadLetterExchange(),
		"x-dead-letter-routing-key": key,
	}); err != nil {
		return err
	}

	// Cada cola de espera retiene el mensaje durante su TTL y, al expirar, lo devuelve a la cola de trabajo.
	backToWork := amqp.Table{"x-dead-letter-exchange": t.CommandsExchange(), "x-dead-letter-routing-key": key}
	for level := 1; level <= t.RetryLevels; level++ {
		if err := declareAndBind(ch, t.RetryQueue(cmd, level), retryRoutingKey(cmd, level), t.RetryExchange(), backToWork); err != nil {
			return err
		}
	}
	if err := declareAndBind(ch, t.DeferredQueue(cmd), deferredRoutingKey(cmd), t.RetryExchange(), backToWork); err != nil {
		return err
	}

	return declareAndBind(ch, t.DeadLetterQueue(cmd), key, t.DeadLetterExchange(), nil)
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
