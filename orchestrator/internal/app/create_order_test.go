package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// fakeRepo es un repositorio en memoria que respeta el contrato del puerto,
// incluido abortar con un contexto cancelado, como hace pgx.
type fakeRepo struct {
	mu       sync.Mutex
	orders   map[order.OrderID]order.Order
	keys     map[string]IdempotencyKey
	byKey    map[string]order.OrderID
	events   []order.Event
	nextID   int
	onCreate func() // se ejecuta tras crear la orden; permite simular eventos durante la cotización
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{orders: map[order.OrderID]order.Order{}, keys: map[string]IdempotencyKey{}, byKey: map[string]order.OrderID{}}
}

func (r *fakeRepo) Create(ctx context.Context, key IdempotencyKey, o order.Order, ev order.Event) (order.Order, bool, error) {
	if err := ctx.Err(); err != nil {
		return order.Order{}, false, err
	}
	if r.onCreate != nil {
		defer r.onCreate()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.keys[key.Key]; ok {
		if existing.RequestHash != key.RequestHash {
			return order.Order{}, false, ErrIdempotencyConflict
		}
		return r.orders[r.byKey[key.Key]], false, nil
	}
	r.nextID++
	o.ID = order.OrderID(fmt.Sprintf("order-%d", r.nextID))
	r.orders[o.ID] = o
	r.keys[key.Key] = key
	r.byKey[key.Key] = o.ID
	r.events = append(r.events, ev)
	return o, true, nil
}

func (r *fakeRepo) SaveTransition(ctx context.Context, o order.Order, ev order.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.ID] = o
	r.events = append(r.events, ev)
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id order.OrderID) (order.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return order.Order{}, ErrOrderNotFound
	}
	return o, nil
}

// fakeQuoter devuelve, en orden, los errores configurados y después una cotización válida.
type fakeQuoter struct {
	errs    []error
	calls   int
	ctxErrs []error // estado del contexto en el momento de cada llamada
}

func (q *fakeQuoter) Quote(ctx context.Context, _ QuoteRequest) (order.Quote, error) {
	q.calls++
	q.ctxErrs = append(q.ctxErrs, ctx.Err())
	if q.calls <= len(q.errs) {
		return order.Quote{}, q.errs[q.calls-1]
	}
	return order.Quote{ID: "q-1", Premium: "850000.00", Currency: "COP", ValidUntil: time.Now().Add(time.Hour)}, nil
}

var validInput = order.NewOrderInput{Plate: "ABC123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"}

func newUseCase(repo OrderRepository, q Quoter, logs *bytes.Buffer) *CreateOrder {
	uc := NewCreateOrder(repo, q, slog.New(slog.NewJSONHandler(logs, nil)))
	uc.sleep = func(context.Context, time.Duration) error { return nil } // sin esperas reales en pruebas
	return uc
}

func TestCreateOrder_QuotesAndEndsQuoted(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}

	res, err := newUseCase(repo, q, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !res.Created || res.Order.Status != order.Quoted || res.Order.Quote == nil || res.Order.Quote.ID != "q-1" {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->QUOTED" {
		t.Fatalf("eventos = %s", got)
	}
}

func TestCreateOrder_RetriesTransientFailures(t *testing.T) {
	repo := newFakeRepo()
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, context.DeadlineExceeded}}

	res, err := newUseCase(repo, q, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || res.Order.Status != order.Quoted {
		t.Fatalf("status=%s err=%v, want QUOTED", res.Order.Status, err)
	}
	if q.calls != 3 {
		t.Fatalf("llamadas al cotizador = %d, want 3", q.calls)
	}
}

func TestCreateOrder_ExhaustedRetriesEndsFailedAndLogsError(t *testing.T) {
	repo := newFakeRepo()
	q := &fakeQuoter{errs: []error{ErrQuoteUnavailable, ErrQuoteUnavailable, ErrQuoteUnavailable}}
	var logs bytes.Buffer

	res, err := newUseCase(repo, q, &logs).Execute(context.Background(), "key-1", validInput)

	if err != nil {
		t.Fatalf("una falla del cotizador no es un error de la solicitud: %v", err)
	}
	if res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteUnavailable {
		t.Fatalf("status=%s reason=%q", res.Order.Status, res.Order.FailureReason)
	}
	if q.calls != 3 {
		t.Fatalf("llamadas = %d, want 3", q.calls)
	}
	if got := statuses(repo.events); got != "->CREATED,CREATED->FAILED" {
		t.Fatalf("eventos = %s", got)
	}
	out := logs.String()
	for _, want := range []string{`"level":"ERROR"`, `"order_id":"` + string(res.Order.ID) + `"`, `"attempts":3`} {
		if !strings.Contains(out, want) {
			t.Errorf("el log no contiene %s: %s", want, out)
		}
	}
	if strings.Contains(out, "ABC123") || strings.Contains(out, "1020304050") {
		t.Fatalf("dato sensible en el log: %s", out)
	}
}

func TestCreateOrder_RejectedQuoteIsNotRetried(t *testing.T) {
	repo := newFakeRepo()
	q := &fakeQuoter{errs: []error{ErrQuoteRejected}}

	res, _ := newUseCase(repo, q, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if q.calls != 1 || res.Order.Status != order.Failed || res.Order.FailureReason != ReasonQuoteRejected {
		t.Fatalf("calls=%d status=%s reason=%q", q.calls, res.Order.Status, res.Order.FailureReason)
	}
}

func TestCreateOrder_SameKeySameRequestReturnsExistingWithoutQuoting(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	uc := newUseCase(repo, q, &bytes.Buffer{})
	first, _ := uc.Execute(context.Background(), "key-1", validInput)

	second, err := uc.Execute(context.Background(), "key-1", validInput)

	if err != nil || second.Created || second.Order.ID != first.Order.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if q.calls != 1 {
		t.Fatalf("el reintento del cliente volvió a cotizar: llamadas = %d", q.calls)
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

// Si el cliente se desconecta después de crear la orden, la cotización y su guardado terminan igual:
// la orden no queda atascada en CREATED.
func TestCreateOrder_ClientCancellationAfterCreateDoesNotAbortQuote(t *testing.T) {
	repo, q := newFakeRepo(), &fakeQuoter{}
	ctx, cancel := context.WithCancel(context.Background())
	repo.onCreate = cancel // el cliente se va justo después de guardar la orden

	res, err := newUseCase(repo, q, &bytes.Buffer{}).Execute(ctx, "key-1", validInput)

	if err != nil || res.Order.Status != order.Quoted {
		t.Fatalf("status=%s err=%v", res.Order.Status, err)
	}
	if stored, _ := repo.Get(context.Background(), res.Order.ID); stored.Status != order.Quoted {
		t.Fatalf("estado guardado = %s, want QUOTED", stored.Status)
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

func statuses(evs []order.Event) string {
	parts := make([]string, len(evs))
	for i, e := range evs {
		parts[i] = string(e.From) + "->" + string(e.To)
	}
	return strings.Join(parts, ",")
}
