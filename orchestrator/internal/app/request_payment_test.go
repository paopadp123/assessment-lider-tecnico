package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// quotedInRepo deja una orden cotizada (con su comando request_payment) como la deja CreateOrder.
func quotedInRepo(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo := newFakeRepo()
	res, err := newUseCase(repo, &fakeQuoter{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)
	if err != nil || res.Order.Status != order.Quoted {
		t.Fatalf("preparación: status=%s err=%v", res.Order.Status, err)
	}
	return repo, res.Order.ID
}

func newRequestPayment(repo *fakeRepo, psp PaymentGateway, logs *bytes.Buffer) *RequestPayment {
	return NewRequestPayment(repo, repo, psp, slog.New(slog.NewJSONHandler(logs, nil)))
}

func delivery(id order.OrderID, attempt int) Delivery {
	return Delivery{MessageID: "msg-1", OrderID: id, Attempt: attempt}
}

func TestRequestPayment_SuccessMovesToPaymentPending(t *testing.T) {
	repo, id := quotedInRepo(t)
	psp := &fakePSP{}

	res := newRequestPayment(repo, psp, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Reason)
	}
	o, _ := repo.Get(context.Background(), id)
	if o.Status != order.PaymentPending || o.PaymentID != "pay-1" {
		t.Fatalf("orden = %s / %q", o.Status, o.PaymentID)
	}
	want := PaymentRequest{OrderID: id, QuoteID: "q-1", Amount: "850000.00", Currency: "COP"}
	if len(psp.got) != 1 || psp.got[0] != want {
		t.Fatalf("intención de pago = %+v", psp.got)
	}
}

func TestRequestPayment_TransientFailureBeforeLastAttemptIsRetried(t *testing.T) {
	repo, id := quotedInRepo(t)
	psp := &fakePSP{errs: []error{ErrPaymentUnavailable}}

	res := newRequestPayment(repo, psp, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandRetry {
		t.Fatalf("outcome = %s, want RETRY", res.Outcome)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Quoted {
		t.Fatalf("status = %s; un reintento no debe cambiar la orden", o.Status)
	}
}

func TestRequestPayment_TimeoutIsTransient(t *testing.T) {
	repo, id := quotedInRepo(t)

	res := newRequestPayment(repo, &fakePSP{errs: []error{context.DeadlineExceeded}}, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 2))

	if res.Outcome != CommandRetry {
		t.Fatalf("outcome = %s, want RETRY", res.Outcome)
	}
}

// Regla crítica: agotados los intentos, el comando va a la DLQ y la orden queda en FAILED, visible para operaciones.
func TestRequestPayment_LastAttemptExhaustedGoesToDeadLetterAndFailsOrder(t *testing.T) {
	repo, id := quotedInRepo(t)
	var logs bytes.Buffer
	uc := newRequestPayment(repo, &fakePSP{errs: []error{ErrPaymentUnavailable}}, &logs)

	res := uc.Execute(context.Background(), delivery(id, uc.MaxAttempts()))

	if res.Outcome != CommandDeadLetter || res.Reason != ReasonPaymentUnavailable {
		t.Fatalf("res = %+v", res)
	}
	o, _ := repo.Get(context.Background(), id)
	if o.Status != order.Failed || o.FailureReason != ReasonPaymentUnavailable {
		t.Fatalf("orden = %s / %q", o.Status, o.FailureReason)
	}
	if len(repo.deadLetters) != 1 || repo.deadLetters[0].Attempts != uc.MaxAttempts() || repo.deadLetters[0].OrderID != id {
		t.Fatalf("dead letters = %+v", repo.deadLetters)
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("falta el log ERROR: %s", logs.String())
	}
}

func TestRequestPayment_RejectedRequestFailsOrderWithoutRetry(t *testing.T) {
	repo, id := quotedInRepo(t)

	res := newRequestPayment(repo, &fakePSP{errs: []error{ErrPaymentRequestRejected}}, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone {
		t.Fatalf("outcome = %s, want DONE", res.Outcome)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Failed || o.FailureReason != ReasonPaymentRequestRejected {
		t.Fatalf("orden = %s / %q", o.Status, o.FailureReason)
	}
}

// Regla crítica: el broker entrega al menos una vez. Una segunda entrega no vuelve a cobrar.
func TestRequestPayment_DuplicateDeliveryHasSingleEffect(t *testing.T) {
	repo, id := quotedInRepo(t)
	psp := &fakePSP{}
	uc := newRequestPayment(repo, psp, &bytes.Buffer{})
	uc.Execute(context.Background(), delivery(id, 1))
	events := len(repo.events)

	res := uc.Execute(context.Background(), delivery(id, 1))

	if res.Outcome != CommandDone || psp.calls != 1 || len(repo.events) != events {
		t.Fatalf("outcome=%s llamadas al PSP=%d eventos %d→%d", res.Outcome, psp.calls, events, len(repo.events))
	}
}

// Recuperación del cobro huérfano (hallazgo de HU-02): el PSP aceptó y el webhook llegó, pero el proceso
// murió antes de guardar PAYMENT_PENDING. La reentrega usa la misma Idempotency-Key, el PSP devuelve el
// mismo paymentId y el webhook guardado se aplica: la orden termina pagada.
func TestRequestPayment_RedeliveryRecoversOrphanPayment(t *testing.T) {
	repo, id := quotedInRepo(t)
	early := PaymentEvent{PaymentID: "pay-1", Status: order.PaymentApproved, OccurredAt: time.Now()}
	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), early); err != nil {
		t.Fatal(err)
	}

	res := newRequestPayment(repo, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), delivery(id, 2))

	if res.Outcome != CommandDone {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if o, _ := repo.Get(context.Background(), id); o.Status != order.Paid {
		t.Fatalf("status = %s, want PAID", o.Status)
	}
}

func TestRequestPayment_UnknownOrderGoesToDeadLetter(t *testing.T) {
	repo := newFakeRepo()

	res := newRequestPayment(repo, &fakePSP{}, &bytes.Buffer{}).Execute(context.Background(), delivery("no-existe", 1))

	if res.Outcome != CommandDeadLetter || len(repo.deadLetters) != 1 {
		t.Fatalf("res=%+v dead letters=%d", res, len(repo.deadLetters))
	}
}
