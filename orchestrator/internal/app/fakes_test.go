package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

type storedPaymentEvent struct {
	ev      PaymentEvent
	applied bool
	outcome PaymentOutcome
}

// fakeRepo es un repositorio en memoria que respeta el contrato del puerto,
// incluido abortar con un contexto cancelado, como hace pgx.
type fakeRepo struct {
	mu        sync.Mutex
	orders    map[order.OrderID]order.Order
	keys      map[string]IdempotencyKey
	byKey     map[string]order.OrderID
	byPayment map[order.PaymentID]order.OrderID
	events    []order.Event
	payEvents []*storedPaymentEvent
	nextID    int
	onCreate  func() // se ejecuta tras crear la orden; permite simular eventos durante la cotización
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		orders:    map[order.OrderID]order.Order{},
		keys:      map[string]IdempotencyKey{},
		byKey:     map[string]order.OrderID{},
		byPayment: map[order.PaymentID]order.OrderID{},
	}
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

func (r *fakeRepo) MarkPaymentPending(ctx context.Context, o order.Order, ev order.Event, apply ApplyPaymentFunc) (order.Order, error) {
	if err := ctx.Err(); err != nil {
		return order.Order{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.ID] = o
	r.byPayment[o.PaymentID] = o.ID
	r.events = append(r.events, ev)

	var pending []*storedPaymentEvent
	for _, s := range r.payEvents {
		if s.ev.PaymentID == o.PaymentID && !s.applied {
			pending = append(pending, s)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool { return pending[i].ev.OccurredAt.Before(pending[j].ev.OccurredAt) })
	for _, s := range pending {
		if _, err := r.applyLocked(o.ID, s, apply); err != nil {
			return order.Order{}, err
		}
	}
	return r.orders[o.ID], nil
}

func (r *fakeRepo) RecordPaymentEvent(ctx context.Context, ev PaymentEvent, apply ApplyPaymentFunc) (PaymentEventResult, error) {
	if err := ctx.Err(); err != nil {
		return PaymentEventResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.payEvents {
		if s.ev.PaymentID == ev.PaymentID && s.ev.Status == ev.Status {
			return PaymentEventResult{Outcome: PaymentDuplicate}, nil
		}
	}
	s := &storedPaymentEvent{ev: ev}
	r.payEvents = append(r.payEvents, s)
	id, ok := r.byPayment[ev.PaymentID]
	if !ok {
		return PaymentEventResult{Outcome: PaymentPendingOrder}, nil
	}
	outcome, err := r.applyLocked(id, s, apply)
	if err != nil {
		return PaymentEventResult{}, err
	}
	return PaymentEventResult{Outcome: outcome, OrderID: id}, nil
}

// applyLocked replica al repositorio real: un conflicto de dominio es anomalía; cualquier otro error
// se propaga sin cambiar la orden (en Postgres, la transacción hace rollback).
func (r *fakeRepo) applyLocked(id order.OrderID, s *storedPaymentEvent, apply ApplyPaymentFunc) (PaymentOutcome, error) {
	cur := r.orders[id]
	e, err := apply(&cur, s.ev)
	switch {
	case errors.Is(err, order.ErrPaymentConflict):
		s.applied, s.outcome = true, PaymentAnomaly
		return PaymentAnomaly, nil
	case err != nil:
		return "", err
	}
	r.orders[id] = cur
	r.events = append(r.events, e)
	s.applied, s.outcome = true, PaymentApplied
	return PaymentApplied, nil
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

// fakePSP devuelve, en orden, los errores configurados y después el paymentId "pay-1".
type fakePSP struct {
	errs  []error
	calls int
	got   []PaymentRequest
}

func (p *fakePSP) CreatePayment(_ context.Context, req PaymentRequest) (order.PaymentID, error) {
	p.calls++
	p.got = append(p.got, req)
	if p.calls <= len(p.errs) {
		return "", p.errs[p.calls-1]
	}
	return "pay-1", nil
}
