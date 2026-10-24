package rabbitmq

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

// testBroker conecta al RabbitMQ de pruebas y declara una topología con prefijo único,
// que se borra al terminar. Sin TEST_RABBITMQ_URL se omite en local; en CI falla.
func testBroker(t *testing.T) (*amqp.Connection, Topology) {
	t.Helper()
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_RABBITMQ_URL es obligatoria en CI")
		}
		t.Skip("TEST_RABBITMQ_URL no definida: se omiten las pruebas de integración")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	topo := Topology{Prefix: fmt.Sprintf("test.%s.%d.", strings.ToLower(t.Name()), time.Now().UnixNano()), RetryLevels: 3}
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := topo.Declare(ch); err != nil {
		t.Fatalf("declare: %v", err)
	}
	t.Cleanup(func() {
		queues := []string{topo.PaymentQueue(), topo.DeadLetterQueue()}
		for l := 1; l <= topo.RetryLevels; l++ {
			queues = append(queues, topo.RetryQueue(l))
		}
		for _, q := range queues {
			_, _ = ch.QueueDelete(q, false, false, false)
		}
		for _, ex := range []string{topo.CommandsExchange(), topo.RetryExchange(), topo.DeadLetterExchange()} {
			_ = ch.ExchangeDelete(ex, false, false)
		}
		_ = conn.Close()
	})
	return conn, topo
}

// scriptedHandler devuelve los resultados configurados en orden y registra cada entrega con su hora.
type scriptedHandler struct {
	mu       sync.Mutex
	outcomes []app.CommandOutcome
	got      []app.Delivery
	at       []time.Time
	done     chan struct{}
}

func newScripted(outcomes ...app.CommandOutcome) *scriptedHandler {
	return &scriptedHandler{outcomes: outcomes, done: make(chan struct{}, 10)}
}

func (h *scriptedHandler) Execute(_ context.Context, d app.Delivery) app.CommandResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, d)
	h.at = append(h.at, time.Now())
	out := h.outcomes[min(len(h.got), len(h.outcomes))-1]
	h.done <- struct{}{}
	return app.CommandResult{Outcome: out, Reason: "PAYMENT_UNAVAILABLE"}
}

func (h *scriptedHandler) wait(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-h.done:
		case <-time.After(10 * time.Second):
			t.Fatalf("esperaba %d entregas, llegaron %d", n, len(h.got))
		}
	}
}

func startWorker(t *testing.T, conn *amqp.Connection, topo Topology, h CommandHandler, retryBase time.Duration) {
	t.Helper()
	startConfiguredWorker(t, NewWorker(conn, topo, h, slog.New(slog.NewJSONHandler(io.Discard, nil)), retryBase))
}

func startConfiguredWorker(t *testing.T, w *Worker) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func publish(t *testing.T, conn *amqp.Connection, topo Topology, id string) {
	t.Helper()
	pub, err := NewPublisher(conn, topo)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	msg := app.OutboxMessage{MessageID: id, Type: app.CommandRequestPayment, OrderID: "order-1"}
	if err := pub.Publish(context.Background(), msg); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// getDeadLetter espera el mensaje en la DLQ.
func getDeadLetter(t *testing.T, conn *amqp.Connection, topo Topology) amqp.Delivery {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d, ok, err := ch.Get(topo.DeadLetterQueue(), true)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no llegó nada a la DLQ")
	return amqp.Delivery{}
}

func TestPublishAndConsume_DeliversCommandWithFirstAttempt(t *testing.T) {
	conn, topo := testBroker(t)
	h := newScripted(app.CommandDone)
	startWorker(t, conn, topo, h, 10*time.Millisecond)

	publish(t, conn, topo, "msg-1")
	h.wait(t, 1)

	if d := h.got[0]; d.MessageID != "msg-1" || d.OrderID != "order-1" || d.Attempt != 1 {
		t.Fatalf("entrega = %+v", d)
	}
}

// Regla crítica: una falla transitoria vuelve a la cola después de un backoff, con el intento incrementado.
func TestRetry_RedeliversAfterBackoffWithIncrementedAttempt(t *testing.T) {
	conn, topo := testBroker(t)
	h := newScripted(app.CommandRetry, app.CommandRetry, app.CommandDone)
	const delay = 300 * time.Millisecond
	w := NewWorker(conn, topo, h, slog.New(slog.NewJSONHandler(io.Discard, nil)), delay)
	w.backoff = func(time.Duration, int) time.Duration { return delay } // espera fija para poder medirla
	startConfiguredWorker(t, w)

	publish(t, conn, topo, "msg-1")
	h.wait(t, 3)

	for i, d := range h.got {
		if d.Attempt != i+1 || d.MessageID != "msg-1" {
			t.Fatalf("entrega %d = %+v", i, d)
		}
	}
	// Cada reintento esperó en su cola de reintento al menos el TTL antes de volver a la cola de trabajo.
	for i := 1; i < len(h.at); i++ {
		if gap := h.at[i].Sub(h.at[i-1]); gap < delay {
			t.Fatalf("entrega %d llegó a los %v; debía esperar al menos %v en la cola de reintento", i+1, gap, delay)
		}
	}
}

// Regla crítica: lo que agota reintentos queda en la DLQ con motivo e intentos, visible para operaciones.
func TestDeadLetter_GoesToDLQWithReasonAndAttempts(t *testing.T) {
	conn, topo := testBroker(t)
	h := newScripted(app.CommandDeadLetter)
	startWorker(t, conn, topo, h, 10*time.Millisecond)

	publish(t, conn, topo, "msg-1")
	h.wait(t, 1)
	d := getDeadLetter(t, conn, topo)

	if d.MessageId != "msg-1" || d.Headers[reasonHeader] != "PAYMENT_UNAVAILABLE" || attemptOf(d) != 1 {
		t.Fatalf("mensaje en DLQ: id=%s headers=%v", d.MessageId, d.Headers)
	}
}

// Un mensaje que no se puede leer no se reintenta: va directo a la DLQ sin llegar al caso de uso.
func TestPoisonMessage_GoesToDLQWithoutReachingHandler(t *testing.T) {
	conn, topo := testBroker(t)
	h := newScripted(app.CommandDone)
	startWorker(t, conn, topo, h, 10*time.Millisecond)
	ch, _ := conn.Channel()
	defer ch.Close()
	if err := ch.PublishWithContext(context.Background(), topo.CommandsExchange(), paymentRoutingKey, false, false,
		amqp.Publishing{MessageId: "poison", Body: []byte("{no es json")}); err != nil {
		t.Fatal(err)
	}

	d := getDeadLetter(t, conn, topo)

	if d.MessageId != "poison" || len(h.got) != 0 {
		t.Fatalf("id=%s entregas al caso de uso=%d", d.MessageId, len(h.got))
	}
}

// Si el broker no está disponible, Publish devuelve error: el despachador deja el mensaje pendiente.
func TestPublish_FailsWhenConnectionIsClosed(t *testing.T) {
	conn, topo := testBroker(t)
	pub, err := NewPublisher(conn, topo)
	if err != nil {
		t.Fatal(err)
	}
	_ = pub.Close()

	err = pub.Publish(context.Background(), app.OutboxMessage{MessageID: "m", Type: app.CommandRequestPayment, OrderID: "o"})

	if err == nil {
		t.Fatal("se esperaba error al publicar sin canal")
	}
}
