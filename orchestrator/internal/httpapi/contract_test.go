package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/api"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

func loadContract(t *testing.T) (*openapi3.T, routers.Router) {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(api.Spec)
	if err != nil {
		t.Fatalf("contrato ilegible: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("contrato inválido: %v", err)
	}
	// El router de kin-openapi exige que el host coincida con "servers"; las solicitudes de prueba usan
	// example.com. Sin servers, enruta solo por método y ruta.
	doc.Servers = nil
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return doc, router
}

// routeRecorder guarda los patrones registrados en lugar de atenderlos.
type routeRecorder struct{ patterns []string }

func (r *routeRecorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

func testHandlers(creator OrderCreator, reader OrderReader, actions *fakeActions, events *fakeEvents, ready http.HandlerFunc) Handlers {
	v, _ := NewSignatureVerifier(testSecret)
	ops := NewOps(actions, events, discard())
	return Handlers{
		Orders:   NewOrders(creator, reader, discard()),
		Webhooks: NewWebhooks(&fakePaymentHandler{res: app.PaymentEventResult{Outcome: app.PaymentApplied}}, v, discard()),
		Login:    NewLogin(operators, NewIPRateLimiter(5), discard()),
		Ops:      ops,
		Console:  NewConsole(&fakeLister{page: app.OrderPage{Orders: []order.Order{contractOrder()}, Next: &app.OrderCursor{CreatedAt: time.Now(), ID: orderID}}}, fakeCalls{calls: contractCalls}, discard()),
		Driver:   NewDriver(&fakeDriverReporter{created: true}, testDeviceToken, discard()),
		Breakers: Breakers(func() BreakerStatus { return BreakerStatus{Name: "policy-issuer", State: "closed"} }),
		Ready:    ready,
		Require:  RequireOperator(operators),
	}
}

func allReady() http.HandlerFunc { return Ready() }

// contractOrder es una orden emitida completa para validar el listado contra el contrato.
func contractOrder() order.Order {
	o, _ := order.NewOrder(order.NewOrderInput{Plate: "ABC123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"})
	o.ID, o.Status, o.PaymentID, o.PolicyNumber = orderID, order.Issued, "pay-1", "POL-1"
	o.Quote = &order.Quote{ID: "q-1", Premium: "850000.50", Currency: "COP", ValidUntil: time.Now()}
	o.CreatedAt, o.UpdatedAt = time.Now(), time.Now()
	return o
}

var contractCalls = []app.StoredCall{
	{ID: 1, ThirdPartyCall: app.ThirdPartyCall{OrderID: orderID, Party: app.PartyIssuer, Operation: app.OpIssue,
		Outcome: app.CallTimeout, Detail: "deadline exceeded", Duration: 5 * time.Second, StartedAt: time.Now()}},
	{ID: 2, ThirdPartyCall: app.ThirdPartyCall{OrderID: orderID, Party: app.PartyIssuer, Operation: app.OpFindPolicy,
		Outcome: app.CallOK, Duration: 300 * time.Millisecond, StartedAt: time.Now()}},
}

// Si una ruta existe en el código pero no en el contrato, la consola no tendría su tipo: la prueba falla.
func TestContract_EveryRegisteredRouteIsDocumented(t *testing.T) {
	doc, _ := loadContract(t)
	rec := &routeRecorder{}

	Routes(rec, testHandlers(&fakeCreator{}, fakeReader{}, &fakeActions{}, &fakeEvents{}, allReady()))

	for _, want := range []string{"GET /health/live", "GET /health/ready", "GET /openapi.yaml"} {
		if !contains(rec.patterns, want) {
			t.Errorf("falta registrar %s", want)
		}
	}
	for _, p := range rec.patterns {
		method, path, _ := strings.Cut(p, " ")
		item := doc.Paths.Find(path)
		if item == nil || item.GetOperation(method) == nil {
			t.Errorf("%s está registrada pero no está en el contrato", p)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// validateAgainstContract falla si la respuesta real no cumple el contrato: código no documentado,
// campo que falta o de otro tipo, o content-type distinto.
func validateAgainstContract(t *testing.T, router routers.Router, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s no está en el contrato: %v", req.Method, req.URL.Path, err)
	}
	opts := &openapi3filter.Options{IncludeResponseStatus: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: opts},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options:                opts,
	}
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		t.Errorf("%s %s → %d no cumple el contrato: %v\ncuerpo: %s", req.Method, req.URL.Path, rec.Code, err, rec.Body.String())
	}
}

// Las respuestas reales (éxito y error) de cada ruta cumplen el contrato publicado.
func TestContract_ResponsesMatchSchemas(t *testing.T) {
	_, router := loadContract(t)
	issued := quotedOrder(t)
	issued.Status, issued.PolicyNumber, issued.CorrelationID = order.Issued, "POL-1", "corr-1"
	events := &fakeEvents{}
	events.add(1, order.Created, "")
	events.add(2, order.RefundRequired, "ana.perez")
	v, _ := NewSignatureVerifier(testSecret)
	webhook := webhookBody(t, v, "pay-1", "APPROVED", "2026-10-24T10:00:00.123Z")
	notReady := Ready(Check{Name: "postgres", Probe: func(context.Context) error { return errors.New("down") }})
	orderPath := "/orders/" + string(issued.ID)

	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		token   string
		creator *fakeCreator
		reader  fakeReader
		actions *fakeActions
		ready   http.HandlerFunc
	}{
		{name: "crear orden", method: http.MethodPost, path: "/orders", body: validBody, creator: &fakeCreator{res: app.CreateOrderResult{Order: quotedOrder(t), Created: true}}},
		{name: "repetir orden", method: http.MethodPost, path: "/orders", body: validBody, creator: &fakeCreator{res: app.CreateOrderResult{Order: quotedOrder(t)}}},
		{name: "orden inválida", method: http.MethodPost, path: "/orders", body: `{"plate":"x"}`, creator: &fakeCreator{err: order.ValidationErrors{{Field: "plate", Message: "invalid"}}}},
		{name: "clave reutilizada", method: http.MethodPost, path: "/orders", body: validBody, creator: &fakeCreator{err: app.ErrIdempotencyConflict}},
		{name: "consultar orden emitida", method: http.MethodGet, path: orderPath, reader: fakeReader{o: issued}},
		{name: "orden inexistente", method: http.MethodGet, path: orderPath, reader: fakeReader{err: app.ErrOrderNotFound}},
		{name: "webhook", method: http.MethodPost, path: "/webhooks/payments", body: webhook},
		{name: "webhook sin firma válida", method: http.MethodPost, path: "/webhooks/payments", body: strings.Replace(webhook, "APPROVED", "REJECTED", 1)},
		{name: "login", method: http.MethodPost, path: "/auth/login", body: `{"username":"ana.perez","password":"clave"}`},
		{name: "login fallido", method: http.MethodPost, path: "/auth/login", body: `{"username":"ana.perez","password":"x"}`},
		{name: "línea de tiempo", method: http.MethodGet, path: orderPath + "/events", token: "token-ana.perez"},
		{name: "línea de tiempo sin token", method: http.MethodGet, path: orderPath + "/events"},
		{name: "reintentar emisión", method: http.MethodPost, path: orderPath + "/actions/retry-issuance", token: "token-ana.perez", actions: &fakeActions{}},
		{name: "reverso no permitido", method: http.MethodPost, path: orderPath + "/actions/mark-for-reversal", token: "token-ana.perez", actions: &fakeActions{err: app.ErrActionNotAllowed}},
		{name: "listado", method: http.MethodGet, path: "/orders?status=ISSUING&q=pay-1&limit=10", token: "token-ana.perez"},
		{name: "listado inválido", method: http.MethodGet, path: "/orders?status=PAGADA", token: "token-ana.perez"},
		{name: "listado sin token", method: http.MethodGet, path: "/orders"},
		{name: "llamadas a terceros", method: http.MethodGet, path: "/orders/" + string(orderID) + "/calls", token: "token-ana.perez"},
		{name: "llamadas de orden inexistente", method: http.MethodGet, path: "/orders/4f1c2a7e-0000-4000-8000-0000000000ff/calls", token: "token-ana.perez"},
		{name: "breakers", method: http.MethodGet, path: "/ops/breakers", token: "token-ana.perez"},
		{name: "vivo", method: http.MethodGet, path: "/health/live"},
		{name: "listo", method: http.MethodGet, path: "/health/ready"},
		{name: "no listo", method: http.MethodGet, path: "/health/ready", ready: notReady},
		{name: "contrato", method: http.MethodGet, path: "/openapi.yaml"},
		{name: "alerta de pánico", method: http.MethodPost, path: "/driver/alerts", body: alertBody(time.Now()), token: testDeviceToken},
		{name: "alerta sin token de dispositivo", method: http.MethodPost, path: "/driver/alerts", body: alertBody(time.Now())},
		{name: "alerta inválida", method: http.MethodPost, path: "/driver/alerts", body: `{"alertId":"x"}`, token: testDeviceToken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			creator, actions, ready := c.creator, c.actions, c.ready
			if creator == nil {
				creator = &fakeCreator{}
			}
			if actions == nil {
				actions = &fakeActions{}
			}
			if ready == nil {
				ready = allReady()
			}
			mux := http.NewServeMux()
			Routes(mux, testHandlers(creator, c.reader, actions, events, ready))
			req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "key-1")
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			validateAgainstContract(t, router, req, rec)
		})
	}
}
