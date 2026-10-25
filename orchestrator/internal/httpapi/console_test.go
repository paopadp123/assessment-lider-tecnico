package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// fakeLister guarda el filtro recibido y devuelve la página configurada.
type fakeLister struct {
	got  app.OrderFilter
	page app.OrderPage
	err  error
}

func (f *fakeLister) ListOrders(_ context.Context, filter app.OrderFilter) (app.OrderPage, error) {
	f.got = filter
	return f.page, f.err
}

type fakeCalls struct {
	calls []app.StoredCall
	err   error
}

func (f fakeCalls) ListCalls(_ context.Context, id order.OrderID) ([]app.StoredCall, error) {
	if f.err != nil {
		return nil, f.err
	}
	if id != orderID {
		return nil, app.ErrOrderNotFound
	}
	return f.calls, nil
}

func consoleMux(lister *fakeLister, calls fakeCalls) *http.ServeMux {
	mux := http.NewServeMux()
	NewConsole(lister, calls, discard()).Register(mux, RequireOperator(operators))
	return mux
}

func TestListOrders_PassesFilterAndMasksOrders(t *testing.T) {
	o := quotedOrder(t)
	lister := &fakeLister{page: app.OrderPage{Orders: []order.Order{o}}}
	mux := consoleMux(lister, fakeCalls{})

	rec := call(mux, http.MethodGet, "/orders?status=ISSUING&status=REFUND_REQUIRED&q=+pay-77+&limit=5", "token-ana.perez", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	f := lister.got
	if len(f.Statuses) != 2 || f.Statuses[0] != order.Issuing || f.Statuses[1] != order.RefundRequired {
		t.Errorf("estados = %v", f.Statuses)
	}
	if f.Query != "pay-77" || f.Limit != 5 || f.Cursor != nil {
		t.Errorf("filtro = %+v (la búsqueda se recorta, el límite pasa y sin cursor es nil)", f)
	}
	body := rec.Body.String()
	if strings.Contains(body, "1020304050") || strings.Contains(body, o.Plate.Raw()) {
		t.Errorf("el listado expone el documento o la placa completa: %s", body)
	}
	var out struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0]["id"] != string(o.ID) || out.NextCursor != nil {
		t.Errorf("respuesta = %s", body)
	}
}

func TestListOrders_DefaultLimit(t *testing.T) {
	lister := &fakeLister{}
	rec := call(consoleMux(lister, fakeCalls{}), http.MethodGet, "/orders", "token-ana.perez", "")

	if rec.Code != http.StatusOK || lister.got.Limit != defaultPageSize {
		t.Fatalf("status %d, límite %d (se esperaba %d)", rec.Code, lister.got.Limit, defaultPageSize)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("una página vacía debe ser [] y no null: %s", rec.Body)
	}
}

// El cursor que devuelve una página sirve, tal cual, para pedir la siguiente.
func TestListOrders_CursorRoundTrip(t *testing.T) {
	next := &app.OrderCursor{CreatedAt: time.Date(2026, 10, 24, 10, 0, 0, 123456000, time.UTC), ID: orderID}
	lister := &fakeLister{page: app.OrderPage{Orders: []order.Order{quotedOrder(t)}, Next: next}}
	mux := consoleMux(lister, fakeCalls{})

	rec := call(mux, http.MethodGet, "/orders?limit=1", "token-ana.perez", "")
	var out struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.NextCursor == "" {
		t.Fatalf("sin nextCursor: %s", rec.Body)
	}

	rec = call(mux, http.MethodGet, "/orders?limit=1&cursor="+out.NextCursor, "token-ana.perez", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("segunda página: %d %s", rec.Code, rec.Body)
	}
	if c := lister.got.Cursor; c == nil || c.ID != next.ID || !c.CreatedAt.Equal(next.CreatedAt) {
		t.Errorf("cursor recibido = %+v, se esperaba %+v", c, next)
	}
}

func TestListOrders_RejectsInvalidParameters(t *testing.T) {
	mux := consoleMux(&fakeLister{}, fakeCalls{})
	for _, q := range []string{
		"status=PAGADA",
		"limit=0",
		"limit=101",
		"limit=abc",
		"cursor=no-es-un-cursor",
		"q=" + strings.Repeat("x", maxQueryLength+1),
	} {
		rec := call(mux, http.MethodGet, "/orders?"+q, "token-ana.perez", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, se esperaba 400", q, rec.Code)
		}
	}
}

func TestConsole_RequiresToken(t *testing.T) {
	mux := consoleMux(&fakeLister{}, fakeCalls{})
	for _, path := range []string{"/orders", "/orders/" + string(orderID) + "/calls"} {
		if rec := call(mux, http.MethodGet, path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sin token: %d, se esperaba 401", path, rec.Code)
		}
	}
}

func TestListOrders_InternalErrorHidesDetail(t *testing.T) {
	rec := call(consoleMux(&fakeLister{err: errors.New("pq: secret detail")}, fakeCalls{}), http.MethodGet, "/orders", "token-ana.perez", "")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("status %d, cuerpo %s", rec.Code, rec.Body)
	}
}

func TestListCalls(t *testing.T) {
	started := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	calls := fakeCalls{calls: []app.StoredCall{{ID: 7, ThirdPartyCall: app.ThirdPartyCall{OrderID: orderID,
		Party: app.PartyIssuer, Operation: app.OpIssue, Outcome: app.CallTimeout, Detail: "deadline exceeded",
		Duration: 1500 * time.Millisecond, StartedAt: started}}}}
	mux := consoleMux(&fakeLister{}, calls)

	rec := call(mux, http.MethodGet, "/orders/"+string(orderID)+"/calls", "token-ana.perez", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 {
		t.Fatalf("respuesta = %s", rec.Body)
	}
	c := out[0]
	if c["party"] != "issuer" || c["operation"] != "issue" || c["outcome"] != "TIMEOUT" || c["durationMs"] != float64(1500) ||
		c["detail"] != "deadline exceeded" || c["startedAt"] != "2026-10-24T10:00:00Z" {
		t.Errorf("llamada = %v", c)
	}

	if rec := call(mux, http.MethodGet, "/orders/4f1c2a7e-0000-4000-8000-0000000000ff/calls", "token-ana.perez", ""); rec.Code != http.StatusNotFound {
		t.Errorf("orden inexistente: %d, se esperaba 404", rec.Code)
	}
	// Igual que la línea de tiempo: un id que no es UUID no puede existir, así que es 404.
	if rec := call(mux, http.MethodGet, "/orders/no-uuid/calls", "token-ana.perez", ""); rec.Code != http.StatusNotFound {
		t.Errorf("id inválido: %d, se esperaba 404", rec.Code)
	}
}
