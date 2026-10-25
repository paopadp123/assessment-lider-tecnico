package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

var validInput = order.NewOrderInput{Plate: "ABC123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"}

func newUseCase(repo OrderRepository, q Quoter, logs *bytes.Buffer) *CreateOrder {
	uc := NewCreateOrder(repo, q, slog.New(slog.NewJSONHandler(logs, nil)))
	uc.sleep = func(context.Context, time.Duration) error { return nil } // sin esperas reales en pruebas
	return uc
}

// Regla crítica: QUOTED y el comando de pago se guardan juntos (outbox); el pago no se pide en la petición.
func TestCreateOrder_QuotesAndEnqueuesPaymentRequest(t *testing.T) {
	repo := newFakeRepo()

	res, err := newUseCase(repo, &fakeQuoter{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || !res.Created || res.Order.Status != order.Quoted || res.Order.Quote == nil {
		t.Fatalf("res=%+v err=%v", res.Order, err)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->QUOTED" {
		t.Fatalf("eventos = %s", got)
	}
	want := Command{Type: CommandRequestPayment, OrderID: res.Order.ID}
	if len(repo.commands) != 1 || repo.commands[0] != want {
		t.Fatalf("comandos = %+v, want %+v", repo.commands, want)
	}
}

func TestCreateOrder_RetriesTransientQuoteFailures(t *testing.T) {
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, context.DeadlineExceeded}}

	res, err := newUseCase(newFakeRepo(), q, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || res.Order.Status != order.Quoted || q.calls != 3 {
		t.Fatalf("status=%s err=%v llamadas=%d", res.Order.Status, err, q.calls)
	}
}

func TestCreateOrder_ExhaustedQuoteRetriesEndsFailedWithoutPaymentCommand(t *testing.T) {
	repo := newFakeRepo()
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, ErrQuoteUnavailable, ErrQuoteUnavailable}}
	var logs bytes.Buffer

	res, err := newUseCase(repo, q, &logs).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("una falla del cotizador no es un error de la solicitud: %v", err)
	}
	if res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteUnavailable || q.calls != 3 || len(repo.commands) != 0 {
		t.Fatalf("status=%s reason=%q cotizador=%d comandos=%d", res.Order.Status, res.Order.FailureReason, q.calls, len(repo.commands))
	}
	assertErrorLog(t, logs.String(), res.Order.ID, 3)
}

func TestCreateOrder_RejectedQuoteIsNotRetried(t *testing.T) {
	q := &fakeQuoter{errs: []error{ErrQuoteRejected}}

	res, _ := newUseCase(newFakeRepo(), q, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if q.calls != 1 || res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteRejected {
		t.Fatalf("calls=%d status=%s reason=%q", q.calls, res.Order.Status, res.Order.FailureReason)
	}
}

func TestCreateOrder_SameKeySameRequestReturnsExistingWithoutQuotingAgain(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	uc := newUseCase(repo, q, &bytes.Buffer{})
	first, _ := uc.Execute(context.Background(), "key-1", validInput)

	second, err := uc.Execute(context.Background(), "key-1", validInput)

	if err != nil || second.Created || second.Order.ID != first.Order.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if q.calls != 1 || len(repo.commands) != 1 {
		t.Fatalf("el reintento del cliente repitió trabajo: cotizador=%d comandos=%d", q.calls, len(repo.commands))
	}
}

func TestCreateOrder_SameKeyDifferentRequestIsConflict(t *testing.T) {
	uc := newUseCase(newFakeRepo(), &fakeQuoter{}, &bytes.Buffer{})
	if _, err := uc.Execute(context.Background(), "key-1", validInput); err != nil {
		t.Fatal(err)
	}
	other := validInput
	other.VehicleYear = 2021

	_, err := uc.Execute(context.Background(), "key-1", other)

	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateOrder_InvalidInputIsNotPersisted(t *testing.T) {
	repo := newFakeRepo()

	_, err := newUseCase(repo, &fakeQuoter{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", order.NewOrderInput{})

	var verr order.ValidationErrors
	if !errors.As(err, &verr) || len(repo.orders) != 0 {
		t.Fatalf("err=%v órdenes=%d", err, len(repo.orders))
	}
}

// Si el cliente se desconecta después de crear la orden, la cotización y su guardado terminan igual.
func TestCreateOrder_ClientCancellationAfterCreateDoesNotAbortQuote(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	ctx, cancel := context.WithCancel(context.Background())
	repo.onCreate = cancel // el cliente se va justo después de guardar la orden

	res, err := newUseCase(repo, q, &bytes.Buffer{}).Execute(ctx, "key-1", validInput)

	if err != nil || res.Order.Status != order.Quoted {
		t.Fatalf("status=%s err=%v", res.Order.Status, err)
	}
	if q.ctxErrs[0] != nil {
		t.Fatalf("el cotizador recibió un contexto cancelado: %v", q.ctxErrs[0])
	}
}

// Si el cliente cancela antes de que la orden exista, no se crea nada (así se comporta Postgres real).
func TestCreateOrder_CancelledBeforeCreateCreatesNothing(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newUseCase(repo, q, &bytes.Buffer{}).Execute(ctx, "key-1", validInput)

	if !errors.Is(err, context.Canceled) || len(repo.orders) != 0 || q.calls != 0 {
		t.Fatalf("err=%v órdenes=%d llamadas=%d", err, len(repo.orders), q.calls)
	}
}

func assertErrorLog(t *testing.T, out string, id order.OrderID, attempts int) {
	t.Helper()
	for _, want := range []string{`"level":"ERROR"`, `"order_id":"` + string(id) + `"`, `"attempts":` + string(rune('0'+attempts))} {
		if !strings.Contains(out, want) {
			t.Errorf("el log no contiene %s: %s", want, out)
		}
	}
	if strings.Contains(out, "ABC123") || strings.Contains(out, "1020304050") {
		t.Fatalf("dato sensible en el log: %s", out)
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)) }

func statuses(evs []order.Event) string {
	parts := make([]string, len(evs))
	for i, e := range evs {
		parts[i] = string(e.From) + "->" + string(e.To)
	}
	return strings.Join(parts, ",")
}
