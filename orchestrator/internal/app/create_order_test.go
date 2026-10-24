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

func newUseCase(repo OrderRepository, q Quoter, psp PaymentGateway, logs *bytes.Buffer) *CreateOrder {
	uc := NewCreateOrder(repo, q, psp, slog.New(slog.NewJSONHandler(logs, nil)))
	uc.sleep = func(context.Context, time.Duration) error { return nil } // sin esperas reales en pruebas
	return uc
}

func TestCreateOrder_QuotesRequestsPaymentAndEndsPaymentPending(t *testing.T) {
	repo, q, psp := newFakeRepo(), &fakeQuoter{}, &fakePSP{}

	res, err := newUseCase(repo, q, psp, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !res.Created || res.Order.Status != order.PaymentPending || res.Order.PaymentID != "pay-1" {
		t.Fatalf("resultado inesperado: %+v", res.Order)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->QUOTED,QUOTED->PAYMENT_PENDING" {
		t.Fatalf("eventos = %s", got)
	}
	want := PaymentRequest{OrderID: res.Order.ID, QuoteID: "q-1", Amount: "850000.00", Currency: "COP"}
	if len(psp.got) != 1 || psp.got[0] != want {
		t.Fatalf("intención de pago = %+v, want %+v", psp.got, want)
	}
}

func TestCreateOrder_RetriesTransientQuoteFailures(t *testing.T) {
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, context.DeadlineExceeded}}

	res, err := newUseCase(newFakeRepo(), q, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || res.Order.Status != order.PaymentPending || q.calls != 3 {
		t.Fatalf("status=%s err=%v llamadas=%d", res.Order.Status, err, q.calls)
	}
}

func TestCreateOrder_ExhaustedQuoteRetriesEndsFailedAndLogsError(t *testing.T) {
	repo, psp := newFakeRepo(), &fakePSP{}
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, ErrQuoteUnavailable, ErrQuoteUnavailable}}
	var logs bytes.Buffer

	res, err := newUseCase(repo, q, psp, &logs).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("una falla del cotizador no es un error de la solicitud: %v", err)
	}
	if res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteUnavailable || q.calls != 3 || psp.calls != 0 {
		t.Fatalf("status=%s reason=%q cotizador=%d psp=%d", res.Order.Status, res.Order.FailureReason, q.calls, psp.calls)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->FAILED" {
		t.Fatalf("eventos = %s", got)
	}
	assertErrorLog(t, logs.String(), res.Order.ID, 3)
}

func TestCreateOrder_RejectedQuoteIsNotRetried(t *testing.T) {
	q := &fakeQuoter{errs: []error{ErrQuoteRejected}}

	res, _ := newUseCase(newFakeRepo(), q, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if q.calls != 1 || res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteRejected {
		t.Fatalf("calls=%d status=%s reason=%q", q.calls, res.Order.Status, res.Order.FailureReason)
	}
}

func TestCreateOrder_PaymentUnavailableAfterRetriesEndsFailed(t *testing.T) {
	repo := newFakeRepo()
	psp := &fakePSP{errs: []error{ErrPaymentUnavailable, context.DeadlineExceeded, ErrPaymentUnavailable}}
	var logs bytes.Buffer

	res, err := newUseCase(repo, &fakeQuoter{}, psp, &logs).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("una falla del PSP no es un error de la solicitud: %v", err)
	}
	if res.Order.Status != order.Failed || res.Order.FailureReason != ReasonPaymentUnavailable || psp.calls != 3 {
		t.Fatalf("status=%s reason=%q llamadas=%d", res.Order.Status, res.Order.FailureReason, psp.calls)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->QUOTED,QUOTED->FAILED" {
		t.Fatalf("eventos = %s", got)
	}
	assertErrorLog(t, logs.String(), res.Order.ID, 3)
}

func TestCreateOrder_PaymentRequestRejectedIsNotRetried(t *testing.T) {
	psp := &fakePSP{errs: []error{ErrPaymentRequestRejected}}

	res, _ := newUseCase(newFakeRepo(), &fakeQuoter{}, psp, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if psp.calls != 1 || res.Order.Status != order.Failed || res.Order.FailureReason != ReasonPaymentRequestRejected {
		t.Fatalf("calls=%d status=%s reason=%q", psp.calls, res.Order.Status, res.Order.FailureReason)
	}
}

// Caso exigido: el PSP confirma el pago antes de que la orden quede guardada en PAYMENT_PENDING.
func TestCreateOrder_ApprovalArrivedBeforePendingEndsPaid(t *testing.T) {
	repo := newFakeRepo()
	early := PaymentEvent{PaymentID: "pay-1", Status: order.PaymentApproved, OccurredAt: time.Now()}
	if res, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), early); err != nil || res.Outcome != PaymentPendingOrder {
		t.Fatalf("webhook temprano: %+v %v", res, err)
	}

	res, err := newUseCase(repo, &fakeQuoter{}, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || res.Order.Status != order.Paid {
		t.Fatalf("status=%s err=%v, want PAID", res.Order.Status, err)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->QUOTED,QUOTED->PAYMENT_PENDING,PAYMENT_PENDING->PAID" {
		t.Fatalf("eventos = %s", got)
	}
}

func TestCreateOrder_SameKeySameRequestReturnsExistingWithoutCallingThirdParties(t *testing.T) {
	q, psp := &fakeQuoter{}, &fakePSP{}
	uc := newUseCase(newFakeRepo(), q, psp, &bytes.Buffer{})
	first, _ := uc.Execute(context.Background(), "key-1", validInput)

	second, err := uc.Execute(context.Background(), "key-1", validInput)

	if err != nil || second.Created || second.Order.ID != first.Order.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if q.calls != 1 || psp.calls != 1 {
		t.Fatalf("el reintento del cliente volvió a llamar terceros: cotizador=%d psp=%d", q.calls, psp.calls)
	}
}

func TestCreateOrder_SameKeyDifferentRequestIsConflict(t *testing.T) {
	uc := newUseCase(newFakeRepo(), &fakeQuoter{}, &fakePSP{}, &bytes.Buffer{})
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

	_, err := newUseCase(repo, &fakeQuoter{}, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", order.NewOrderInput{})

	var verr order.ValidationErrors
	if !errors.As(err, &verr) || len(repo.orders) != 0 {
		t.Fatalf("err=%v órdenes=%d", err, len(repo.orders))
	}
}

// Si el cliente se desconecta después de crear la orden, cotizar, pedir el pago y guardar terminan igual.
func TestCreateOrder_ClientCancellationAfterCreateDoesNotAbortProcessing(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	ctx, cancel := context.WithCancel(context.Background())
	repo.onCreate = cancel // el cliente se va justo después de guardar la orden

	res, err := newUseCase(repo, q, &fakePSP{}, &bytes.Buffer{}).Execute(ctx, "key-1", validInput)

	if err != nil || res.Order.Status != order.PaymentPending {
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

	_, err := newUseCase(repo, q, &fakePSP{}, &bytes.Buffer{}).Execute(ctx, "key-1", validInput)

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
