package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

type fakeCreator struct {
	res    app.CreateOrderResult
	err    error
	gotKey string
	gotIn  order.NewOrderInput
	called bool
}

func (f *fakeCreator) Execute(_ context.Context, key string, in order.NewOrderInput) (app.CreateOrderResult, error) {
	f.called, f.gotKey, f.gotIn = true, key, in
	return f.res, f.err
}

type fakeReader struct {
	o   order.Order
	err error
}

func (f fakeReader) Get(context.Context, order.OrderID) (order.Order, error) { return f.o, f.err }

const validBody = `{"plate":"ABC123","vehicleYear":2020,"cityCode":"11001","documentId":"1020304050"}`

func quotedOrder(t *testing.T) order.Order {
	t.Helper()
	o, err := order.NewOrder(order.NewOrderInput{Plate: "ABC123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"})
	if err != nil {
		t.Fatal(err)
	}
	o.ID, o.Status = "4f1c2a7e-0000-4000-8000-000000000001", order.Quoted
	o.Quote = &order.Quote{ID: "q-1", Premium: "850000.50", Currency: "COP"}
	return o
}

func do(t *testing.T, h *Orders, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func newHandler(c OrderCreator, r OrderReader) *Orders {
	return NewOrders(c, r, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func TestCreate_StatusCodesAndBodies(t *testing.T) {
	created := app.CreateOrderResult{Order: quotedOrder(t), Created: true}
	replayed := app.CreateOrderResult{Order: quotedOrder(t), Created: false}
	tests := []struct {
		name        string
		key, body   string
		creator     *fakeCreator
		wantStatus  int
		wantInBody  string
		wantInvoked bool
	}{
		{"sin Idempotency-Key", "", validBody, &fakeCreator{}, http.StatusBadRequest, "Idempotency-Key", false},
		{"clave demasiado larga", strings.Repeat("k", 256), validBody, &fakeCreator{}, http.StatusBadRequest, "Idempotency-Key", false},
		{"clave con espacios", "   ", validBody, &fakeCreator{}, http.StatusBadRequest, "Idempotency-Key", false},
		{"clave con control", "k\x01", validBody, &fakeCreator{}, http.StatusBadRequest, "Idempotency-Key", false},
		{"JSON inválido", "k", `{`, &fakeCreator{}, http.StatusBadRequest, "invalid JSON", false},
		{"campo desconocido", "k", `{"plate":"ABC123","extra":1}`, &fakeCreator{}, http.StatusBadRequest, "invalid JSON", false},
		{"validación de dominio", "k", validBody, &fakeCreator{err: order.ValidationErrors{{Field: "plate", Message: "bad"}}}, http.StatusBadRequest, `"field":"plate"`, true},
		{"clave con otra solicitud", "k", validBody, &fakeCreator{err: app.ErrIdempotencyConflict}, http.StatusUnprocessableEntity, "idempotency key reused", true},
		{"error interno", "k", validBody, &fakeCreator{err: errors.New("db down")}, http.StatusInternalServerError, `"internal error"`, true},
		{"creada", "k", validBody, &fakeCreator{res: created}, http.StatusCreated, `"status":"QUOTED"`, true},
		{"repetida", "k", validBody, &fakeCreator{res: replayed}, http.StatusOK, `"id":"4f1c2a7e-0000-4000-8000-000000000001"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newHandler(tt.creator, fakeReader{}), http.MethodPost, "/orders", tt.key, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Fatalf("el cuerpo no contiene %s: %s", tt.wantInBody, rec.Body)
			}
			if tt.creator.called != tt.wantInvoked {
				t.Fatalf("caso de uso invocado = %v, want %v", tt.creator.called, tt.wantInvoked)
			}
		})
	}
}

func TestCreate_PassesKeyAndInputAndHidesSensitiveData(t *testing.T) {
	c := &fakeCreator{res: app.CreateOrderResult{Order: quotedOrder(t), Created: true}}

	rec := do(t, newHandler(c, fakeReader{}), http.MethodPost, "/orders", "key-1", validBody)

	if c.gotKey != "key-1" || c.gotIn.Plate != "ABC123" || c.gotIn.VehicleYear != 2020 {
		t.Fatalf("entrada recibida: key=%q in=%+v", c.gotKey, c.gotIn)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["plate"] != "ABC***" || body["documentId"] != nil {
		t.Fatalf("la respuesta expone datos sensibles: %s", rec.Body)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"premium":850000.50`)) {
		t.Fatalf("la prima debe salir como número exacto: %s", rec.Body)
	}
}

// Una falla del cotizador no es un error de la solicitud: 201 con la orden en FAILED y su motivo.
func TestCreate_FailedQuoteReturns201WithReason(t *testing.T) {
	o := quotedOrder(t)
	o.Status, o.Quote, o.FailureReason = order.Failed, nil, app.ReasonQuoteUnavailable
	c := &fakeCreator{res: app.CreateOrderResult{Order: o, Created: true}}

	rec := do(t, newHandler(c, fakeReader{}), http.MethodPost, "/orders", "k", validBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var body struct {
		Status        string `json:"status"`
		FailureReason string `json:"failureReason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "FAILED" || body.FailureReason != "QUOTE_UNAVAILABLE" {
		t.Fatalf("cuerpo = %s", rec.Body)
	}
}

func TestCreate_InternalErrorDoesNotLeakDetails(t *testing.T) {
	c := &fakeCreator{err: errors.New("pq: password authentication failed")}

	rec := do(t, newHandler(c, fakeReader{}), http.MethodPost, "/orders", "k", validBody)

	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("detalle interno filtrado: %s", rec.Body)
	}
}

func TestGet(t *testing.T) {
	o := quotedOrder(t)
	tests := []struct {
		name       string
		path       string
		reader     fakeReader
		wantStatus int
	}{
		{"existe", "/orders/" + string(o.ID), fakeReader{o: o}, http.StatusOK},
		{"no existe", "/orders/" + string(o.ID), fakeReader{err: app.ErrOrderNotFound}, http.StatusNotFound},
		{"id inválido no llega a la base", "/orders/not-a-uuid", fakeReader{err: errors.New("no debería llamarse")}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newHandler(&fakeCreator{}, tt.reader), http.MethodGet, tt.path, "", "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestGet_ReturnsContractWithoutSensitiveData(t *testing.T) {
	o := quotedOrder(t)

	rec := do(t, newHandler(&fakeCreator{}, fakeReader{o: o}), http.MethodGet, "/orders/"+string(o.ID), "", "")

	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Plate  string `json:"plate"`
		Quote  struct {
			ID      string      `json:"id"`
			Premium json.Number `json:"premium"`
		} `json:"quote"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != string(o.ID) || body.Status != "QUOTED" || body.Plate != "ABC***" || body.Quote.ID != "q-1" || body.Quote.Premium != "850000.50" {
		t.Fatalf("contrato inesperado: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "1020304050") || strings.Contains(rec.Body.String(), "ABC123") {
		t.Fatalf("datos sensibles en la respuesta: %s", rec.Body)
	}
}

// Una orden emitida muestra su número de póliza.
func TestGet_IssuedOrderIncludesPolicyNumber(t *testing.T) {
	o := quotedOrder(t)
	o.Status, o.PolicyNumber = order.Issued, "POL-77"

	rec := do(t, newHandler(&fakeCreator{}, fakeReader{o: o}), http.MethodGet, "/orders/"+string(o.ID), "", "")

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"policyNumber":"POL-77"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
