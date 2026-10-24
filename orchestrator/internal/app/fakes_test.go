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
	mu          sync.Mutex
	orders      map[order.OrderID]order.Order
	keys        map[string]IdempotencyKey
	byKey       map[string]order.OrderID
	byPayment   map[order.PaymentID]order.OrderID
	events      []order.Event
	payEvents   []*storedPaymentEvent
	commands    []Command
	deadLetters []DeadLetter
	reconciled  map[order.OrderID]time.Time
	issueMisses map[order.OrderID]int
	nextID      int
	onCreate    func() // se ejecuta tras crear la orden; permite simular eventos durante la cotización
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		orders:      map[order.OrderID]order.Order{},
		keys:        map[string]IdempotencyKey{},
		byKey:       map[string]order.OrderID{},
		byPayment:   map[order.PaymentID]order.OrderID{},
		reconciled:  map[order.OrderID]time.Time{},
		issueMisses: map[order.OrderID]int{},
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

// SaveTransition replica el control optimista del repositorio real: si la orden ya no está en el
// estado de origen del evento, devuelve ErrInvalidTransition sin cambiar nada.
func (r *fakeRepo) SaveTransition(ctx context.Context, o order.Order, ev order.Event, cmds ...Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkFromLocked(o.ID, ev.From); err != nil {
		return err
	}
	r.orders[o.ID] = o
	r.events = append(r.events, ev)
	r.commands = append(r.commands, cmds...)
	delete(r.issueMisses, o.ID) // como el UPDATE real: un cambio de estado reinicia las rondas fallidas
	return nil
}

// checkFromLocked replica el UPDATE ... WHERE status = <origen>: una orden inexistente o que ya cambió
// de estado no se actualiza.
func (r *fakeRepo) checkFromLocked(id order.OrderID, from order.Status) error {
	if cur, ok := r.orders[id]; !ok || cur.Status != from {
		return fmt.Errorf("%w: order %s is no longer in %s", order.ErrInvalidTransition, id, from)
	}
	return nil
}

// fakeSnapshot permite simular el rollback de una transacción.
type fakeSnapshot struct {
	orders    map[order.OrderID]order.Order
	byPayment map[order.PaymentID]order.OrderID
	events    int
	commands  int
	payEvents []storedPaymentEvent
}

func (r *fakeRepo) snapshotLocked() fakeSnapshot {
	s := fakeSnapshot{orders: map[order.OrderID]order.Order{}, byPayment: map[order.PaymentID]order.OrderID{}, events: len(r.events), commands: len(r.commands)}
	for k, v := range r.orders {
		s.orders[k] = v
	}
	for k, v := range r.byPayment {
		s.byPayment[k] = v
	}
	for _, p := range r.payEvents {
		s.payEvents = append(s.payEvents, *p)
	}
	return s
}

func (r *fakeRepo) rollbackLocked(s fakeSnapshot) {
	r.orders, r.byPayment, r.events, r.commands = s.orders, s.byPayment, r.events[:s.events], r.commands[:s.commands]
	r.payEvents = r.payEvents[:0]
	for i := range s.payEvents {
		p := s.payEvents[i]
		r.payEvents = append(r.payEvents, &p)
	}
}

func (r *fakeRepo) RecordDeadLetter(_ context.Context, dl DeadLetter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.deadLetters {
		if existing.MessageID == dl.MessageID {
			return nil
		}
	}
	r.deadLetters = append(r.deadLetters, dl)
	return nil
}

func (r *fakeRepo) MarkPaymentPending(ctx context.Context, o order.Order, ev order.Event, apply ApplyPaymentFunc) (order.Order, error) {
	if err := ctx.Err(); err != nil {
		return order.Order{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkFromLocked(o.ID, ev.From); err != nil {
		return order.Order{}, err
	}
	snap := r.snapshotLocked()
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
			r.rollbackLocked(snap) // como Postgres: si algo falla, no queda nada de esta transacción
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
	snap := r.snapshotLocked()
	s := &storedPaymentEvent{ev: ev}
	r.payEvents = append(r.payEvents, s)
	id, ok := r.byPayment[ev.PaymentID]
	if !ok {
		return PaymentEventResult{Outcome: PaymentPendingOrder}, nil
	}
	outcome, err := r.applyLocked(id, s, apply)
	if err != nil {
		r.rollbackLocked(snap) // ni el evento ni el cambio de estado quedan, igual que en Postgres
		return PaymentEventResult{}, err
	}
	return PaymentEventResult{Outcome: outcome, OrderID: id}, nil
}

// applyLocked replica al repositorio real: un conflicto de dominio es anomalía; cualquier otro error
// se propaga sin cambiar la orden (en Postgres, la transacción hace rollback).
func (r *fakeRepo) applyLocked(id order.OrderID, s *storedPaymentEvent, apply ApplyPaymentFunc) (PaymentOutcome, error) {
	cur := r.orders[id]
	e, cmds, err := apply(&cur, s.ev)
	switch {
	case errors.Is(err, order.ErrPaymentConflict):
		s.applied, s.outcome = true, PaymentAnomaly
		return PaymentAnomaly, nil
	case err != nil:
		return "", err
	}
	r.orders[id] = cur
	r.events = append(r.events, e)
	r.commands = append(r.commands, cmds...)
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
// Como un PSP real con Idempotency-Key, la misma orden recibe siempre el mismo paymentId.
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

// fakeOutbox simula la outbox: los mensajes que el publicador rechaza siguen pendientes.
type fakeOutbox struct {
	pending   []OutboxMessage
	published []OutboxMessage
}

func (f *fakeOutbox) PublishPending(ctx context.Context, limit int, publish func(context.Context, OutboxMessage) error) (int, error) {
	var still []OutboxMessage
	n := 0
	for i, m := range f.pending {
		if i >= limit {
			still = append(still, m)
			continue
		}
		if err := publish(ctx, m); err != nil {
			m.Attempts++
			still = append(still, m)
			continue
		}
		f.published = append(f.published, m)
		n++
	}
	f.pending = still
	return n, nil
}

// fakePublisher falla en los mensajes indicados y acepta el resto.
type fakePublisher struct {
	failFor map[string]bool
	sent    []string
}

func (p *fakePublisher) Publish(_ context.Context, m OutboxMessage) error {
	if p.failFor[m.MessageID] {
		return errors.New("broker did not confirm")
	}
	p.sent = append(p.sent, m.MessageID)
	return nil
}

// fakeIssuer se comporta como la ACL: idempotente por externalRef (emitir dos veces la misma orden
// devuelve la misma póliza) y consultable. issueErrs se devuelven en orden antes de emitir; si
// issuedOnTimeout está activo, un timeout deja la póliza emitida igual ("timeout pero sí emitió").
type fakeIssuer struct {
	mu              sync.Mutex
	issueErrs       []error
	findErrs        []error
	issuedOnTimeout bool
	policies        map[order.OrderID]order.PolicyNumber
	issueCalls      int
	findCalls       int
	emissions       int // pólizas distintas creadas: más de una por orden sería una doble emisión
	got             []IssueRequest
	onIssue         func() // se ejecuta al recibir la emisión; permite simular cambios concurrentes
}

func newFakeIssuer() *fakeIssuer {
	return &fakeIssuer{policies: map[order.OrderID]order.PolicyNumber{}}
}

func (f *fakeIssuer) Issue(_ context.Context, req IssueRequest) (order.PolicyNumber, error) {
	if f.onIssue != nil {
		f.onIssue()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issueCalls++
	f.got = append(f.got, req)
	if f.issueCalls <= len(f.issueErrs) {
		err := f.issueErrs[f.issueCalls-1]
		if f.issuedOnTimeout && errors.Is(err, context.DeadlineExceeded) {
			f.emitLocked(req.ExternalRef)
		}
		return "", err
	}
	return f.emitLocked(req.ExternalRef), nil
}

func (f *fakeIssuer) emitLocked(ref order.OrderID) order.PolicyNumber {
	if n, ok := f.policies[ref]; ok {
		return n
	}
	f.emissions++
	n := order.PolicyNumber(fmt.Sprintf("POL-%d", f.emissions))
	f.policies[ref] = n
	return n
}

func (f *fakeIssuer) FindByExternalRef(_ context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findCalls++
	if f.findCalls <= len(f.findErrs) {
		return "", f.findErrs[f.findCalls-1]
	}
	if n, ok := f.policies[ref]; ok {
		return n, nil
	}
	return "", ErrPolicyNotFound
}

// ClaimStuck replica la consulta real: estados no terminales, sin cambios ni reconciliación desde before.
func (r *fakeRepo) ClaimStuck(ctx context.Context, before time.Time, limit int) ([]StuckOrder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []StuckOrder
	ids := make([]order.OrderID, 0, len(r.orders))
	for id := range r.orders {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		o := r.orders[id]
		last, seen := r.reconciled[id]
		if o.Status.IsTerminal() || o.Status == order.RefundRequired || !o.UpdatedAt.Before(before) || (seen && !last.Before(before)) {
			continue
		}
		if len(out) == limit {
			break
		}
		r.reconciled[id] = before.Add(time.Nanosecond) // "ahora" es posterior al umbral
		out = append(out, StuckOrder{Order: o, IssueMisses: r.issueMisses[id]})
	}
	return out, nil
}

func (r *fakeRepo) Enqueue(ctx context.Context, cmds ...Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmds...)
	return nil
}

func (r *fakeRepo) RecordIssueMiss(_ context.Context, id order.OrderID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.orders[id]; !ok {
		return 0, ErrOrderNotFound
	}
	r.issueMisses[id]++
	return r.issueMisses[id], nil
}

// age deja la orden sin cambios desde hace d, como si estuviera atascada.
func (r *fakeRepo) age(id order.OrderID, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.orders[id]
	o.UpdatedAt = time.Now().Add(-d)
	r.orders[id] = o
}
