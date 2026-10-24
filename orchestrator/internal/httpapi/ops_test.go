package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const orderID = order.OrderID("4f1c2a7e-0000-4000-8000-000000000001")

type fakeActions struct {
	err      error
	operator string
	action   string
}

func (f *fakeActions) RetryIssuance(_ context.Context, id order.OrderID, op string) (order.Order, error) {
	f.action, f.operator = "retry", op
	return order.Order{ID: id, Status: order.Issuing}, f.err
}

func (f *fakeActions) MarkForReversal(_ context.Context, id order.OrderID, op string) (order.Order, error) {
	f.action, f.operator = "reversal", op
	return order.Order{ID: id, Status: order.RefundRequired}, f.err
}

// fakeEvents guarda eventos en memoria; los IDs son crecientes como en order_events.
type fakeEvents struct {
	mu     sync.Mutex
	events []app.StoredEvent
}

func (f *fakeEvents) add(id int64, to order.Status, actor string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, app.StoredEvent{ID: id, OrderID: orderID,
		Event: order.Event{To: to, Reason: "test", Actor: actor, OccurredAt: time.Now()}})
}

func (f *fakeEvents) ListEvents(_ context.Context, id order.OrderID) ([]app.StoredEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != orderID {
		return nil, app.ErrOrderNotFound
	}
	return append([]app.StoredEvent(nil), f.events...), nil
}

func (f *fakeEvents) EventsAfter(_ context.Context, after int64, limit int) ([]app.StoredEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []app.StoredEvent
	for _, e := range f.events {
		if e.ID > after && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeEvents) LatestEventID(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) == 0 {
		return 0, nil
	}
	return f.events[len(f.events)-1].ID, nil
}

func opsMux(actions *fakeActions, events *fakeEvents) *http.ServeMux {
	mux := http.NewServeMux()
	h := NewOps(actions, events, discard())
	h.poll, h.heartbeat = 10*time.Millisecond, 50*time.Millisecond
	h.Register(mux, RequireOperator(operators))
	return mux
}

func call(mux *http.ServeMux, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// El operador sale del token, nunca del cuerpo: no se puede actuar en nombre de otro.
func TestActions_UseOperatorFromToken(t *testing.T) {
	actions := &fakeActions{}
	mux := opsMux(actions, &fakeEvents{})

	rec := call(mux, http.MethodPost, "/orders/"+string(orderID)+"/actions/retry-issuance", "token-ana.perez", `{"operator":"otro"}`)

	if rec.Code != http.StatusOK || actions.action != "retry" || actions.operator != "ana.perez" {
		t.Fatalf("status=%d acción=%s operador=%q", rec.Code, actions.action, actions.operator)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ISSUING"`) {
		t.Fatalf("cuerpo = %s", rec.Body.String())
	}
}

func TestActions_StatusMapping(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		token string
		err   error
		want  int
	}{
		{"reverso ok", "/orders/" + string(orderID) + "/actions/mark-for-reversal", "token-ana.perez", nil, http.StatusOK},
		{"sin token", "/orders/" + string(orderID) + "/actions/mark-for-reversal", "", nil, http.StatusUnauthorized},
		{"estado no permitido", "/orders/" + string(orderID) + "/actions/mark-for-reversal", "token-ana.perez", app.ErrActionNotAllowed, http.StatusConflict},
		{"cambio concurrente", "/orders/" + string(orderID) + "/actions/retry-issuance", "token-ana.perez", order.ErrInvalidTransition, http.StatusConflict},
		{"no existe", "/orders/" + string(orderID) + "/actions/retry-issuance", "token-ana.perez", app.ErrOrderNotFound, http.StatusNotFound},
		{"id inválido", "/orders/no-es-uuid/actions/retry-issuance", "token-ana.perez", nil, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := call(opsMux(&fakeActions{err: c.err}, &fakeEvents{}), http.MethodPost, c.path, c.token, "")

			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestEvents_ListsTimelineWithActor(t *testing.T) {
	events := &fakeEvents{}
	events.add(1, order.Created, "")
	events.add(2, order.RefundRequired, "ana.perez")

	rec := call(opsMux(&fakeActions{}, events), http.MethodGet, "/orders/"+string(orderID)+"/events", "token-ana.perez", "")

	var got []map[string]any
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&got) != nil || len(got) != 2 {
		t.Fatalf("status=%d cuerpo=%v", rec.Code, got)
	}
	if got[0]["actor"] != "system" || got[1]["actor"] != "ana.perez" || got[1]["to"] != "REFUND_REQUIRED" || got[1]["id"] != 2.0 {
		t.Fatalf("eventos = %v", got)
	}
}

func TestEvents_RequireTokenAndExistingOrder(t *testing.T) {
	mux := opsMux(&fakeActions{}, &fakeEvents{})

	if rec := call(mux, http.MethodGet, "/orders/"+string(orderID)+"/events", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: status = %d", rec.Code)
	}
	if rec := call(mux, http.MethodGet, "/orders/4f1c2a7e-0000-4000-8000-000000000999/events", "token-ana.perez", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("orden inexistente: status = %d", rec.Code)
	}
}

// sseEvent es un evento leído del flujo.
type sseEvent struct {
	id   string
	data string
}

// readSSE lee eventos del flujo hasta juntar n o agotar el tiempo.
func readSSE(t *testing.T, srv *httptest.Server, lastEventID string, n int) []sseEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events/stream", nil)
	req.Header.Set("Authorization", "Bearer token-ana.perez")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	var out []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(resp.Body)
	for len(out) < n && sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = strings.TrimPrefix(line, "data: ")
		case line == "" && cur.id != "":
			out = append(out, cur)
			cur = sseEvent{}
		}
	}
	return out
}

// Reanudación: con Last-Event-ID el flujo entrega lo que pasó después de ese evento, sin perder nada,
// y sigue entregando los cambios nuevos.
func TestStream_ResumesFromLastEventIDAndFollowsNewEvents(t *testing.T) {
	events := &fakeEvents{}
	for i, to := range []order.Status{order.Created, order.Quoted, order.PaymentPending} {
		events.add(int64(i+1), to, "")
	}
	srv := httptest.NewServer(opsMux(&fakeActions{}, events))
	t.Cleanup(srv.Close)
	go func() {
		time.Sleep(100 * time.Millisecond)
		events.add(4, order.Paid, "")
	}()

	got := readSSE(t, srv, "1", 3)

	if len(got) != 3 || got[0].id != "2" || got[1].id != "3" || got[2].id != "4" {
		t.Fatalf("eventos = %+v", got)
	}
	if !strings.Contains(got[2].data, `"to":"PAID"`) || !strings.Contains(got[2].data, string(orderID)) {
		t.Fatalf("data = %s", got[2].data)
	}
}

// Sin Last-Event-ID el flujo arranca en el presente: no reenvía todo el historial.
func TestStream_WithoutLastEventIDStartsAtTheEnd(t *testing.T) {
	events := &fakeEvents{}
	events.add(1, order.Created, "")
	events.add(2, order.Quoted, "")
	srv := httptest.NewServer(opsMux(&fakeActions{}, events))
	t.Cleanup(srv.Close)
	go func() {
		time.Sleep(100 * time.Millisecond)
		events.add(3, order.PaymentPending, "")
	}()

	got := readSSE(t, srv, "", 1)

	if len(got) != 1 || got[0].id != "3" {
		t.Fatalf("eventos = %+v", got)
	}
}

func TestStream_RequiresToken(t *testing.T) {
	rec := call(opsMux(&fakeActions{}, &fakeEvents{}), http.MethodGet, "/events/stream", "", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Al apagar el servidor, los flujos abiertos terminan: si no, Shutdown esperaría su timeout completo.
func TestStream_EndsWhenStreamsAreClosed(t *testing.T) {
	mux := http.NewServeMux()
	h := NewOps(&fakeActions{}, &fakeEvents{}, discard())
	h.poll, h.heartbeat = 10*time.Millisecond, time.Hour
	h.Register(mux, RequireOperator(operators))
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil)
	req.Header.Set("Authorization", "Bearer token-ana.perez")
	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	h.CloseStreams()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("el flujo siguió abierto después de CloseStreams")
	}
}

// El latido mantiene viva la conexión a través de balanceadores con timeout por inactividad.
func TestStream_SendsHeartbeat(t *testing.T) {
	mux := http.NewServeMux()
	h := NewOps(&fakeActions{}, &fakeEvents{}, discard())
	h.poll, h.heartbeat = time.Hour, 20*time.Millisecond
	h.Register(mux, RequireOperator(operators))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events/stream", nil)
	req.Header.Set("Authorization", "Bearer token-ana.perez")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)

	if !sc.Scan() || sc.Text() != ": ping" {
		t.Fatalf("primera línea = %q, want el latido", sc.Text())
	}
}

// El servidor real corta las respuestas a los 20 s (WriteTimeout); el flujo debe seguir vivo después.
func TestStream_OutlivesServerWriteTimeout(t *testing.T) {
	events := &fakeEvents{}
	srv := httptest.NewUnstartedServer(opsMux(&fakeActions{}, events))
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	go func() {
		time.Sleep(300 * time.Millisecond) // después del WriteTimeout
		events.add(1, order.Created, "")
	}()

	got := readSSE(t, srv, "0", 1)

	if len(got) != 1 || got[0].id != "1" {
		t.Fatalf("eventos = %+v; el flujo murió con el WriteTimeout", got)
	}
}

// Si por un error de cableado la ruta quedara sin el middleware, la acción responde 401, no 500.
func TestActions_WithoutOperatorInContextIs401(t *testing.T) {
	mux := http.NewServeMux()
	h := NewOps(&fakeActions{}, &fakeEvents{}, discard())
	h.Register(mux, func(next http.HandlerFunc) http.HandlerFunc { return next }) // sin autenticación

	rec := call(mux, http.MethodPost, "/orders/"+string(orderID)+"/actions/retry-issuance", "", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
