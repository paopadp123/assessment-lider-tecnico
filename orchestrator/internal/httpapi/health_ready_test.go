package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

func TestReady_AllDependenciesUp(t *testing.T) {
	ok := func(context.Context) error { return nil }
	rec := httptest.NewRecorder()

	Ready(Check{Name: "postgres", Probe: ok}, Check{Name: "rabbitmq", Probe: ok})(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Si una dependencia falla, el balanceador deja de enviar tráfico (503) y se ve cuál falló, sin exponer el
// detalle del error.
func TestReady_ReportsTheFailingDependency(t *testing.T) {
	rec := httptest.NewRecorder()

	Ready(
		Check{Name: "postgres", Probe: func(context.Context) error { return nil }},
		Check{Name: "rabbitmq", Probe: func(context.Context) error { return errors.New("dial tcp 10.0.0.5:5672: connection refused") }},
	)(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if rec.Code != http.StatusServiceUnavailable || json.NewDecoder(rec.Body).Decode(&body) != nil {
		t.Fatalf("status = %d", rec.Code)
	}
	if body.Status != "not_ready" || body.Checks["postgres"] != "up" || body.Checks["rabbitmq"] != "down" {
		t.Fatalf("cuerpo = %+v", body)
	}
}

func TestOpenAPI_ServesTheContract(t *testing.T) {
	rec := httptest.NewRecorder()

	OpenAPI(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/yaml" || rec.Body.Len() < 100 {
		t.Fatalf("status=%d content-type=%q largo=%d", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
}

func correlated(t *testing.T, header string) (seen string, rec *httptest.ResponseRecorder) {
	t.Helper()
	h := WithCorrelation(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = trace.ID(r.Context()) }))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if header != "" {
		req.Header.Set(trace.Header, header)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return seen, rec
}

func TestWithCorrelation(t *testing.T) {
	seen, rec := correlated(t, "corr-cliente-1")
	if seen != "corr-cliente-1" || rec.Header().Get(trace.Header) != "corr-cliente-1" {
		t.Fatalf("recibido: contexto=%q respuesta=%q", seen, rec.Header().Get(trace.Header))
	}

	seen, rec = correlated(t, "")
	if !trace.Valid(seen) || rec.Header().Get(trace.Header) != seen {
		t.Fatalf("generado: contexto=%q respuesta=%q", seen, rec.Header().Get(trace.Header))
	}

	// Un valor inválido (p. ej. con salto de línea para falsificar logs) se reemplaza, no se propaga.
	seen, _ = correlated(t, "abc\nlevel=ERROR")
	if !trace.Valid(seen) || seen == "abc\nlevel=ERROR" {
		t.Fatalf("inválido: contexto=%q", seen)
	}
}

// Al repetir una compra (misma Idempotency-Key), la respuesta trae el correlation-id de la compra original.
func TestCreateOrder_ResponseCarriesTheOrderCorrelationID(t *testing.T) {
	o := quotedOrder(t)
	o.CorrelationID = "corr-original"

	rec := do(t, newHandler(&fakeCreator{res: appResult(o)}, fakeReader{}), http.MethodPost, "/orders", "key-1", validBody)

	if rec.Header().Get(trace.Header) != "corr-original" {
		t.Fatalf("%s = %q", trace.Header, rec.Header().Get(trace.Header))
	}
}

func appResult(o order.Order) app.CreateOrderResult { return app.CreateOrderResult{Order: o} }

// Los logs de error de los handlers llevan el correlation-id de la solicitud: sin él no se puede unir un
// "request failed" con el resto de la compra.
func TestHandlerErrorLogsCarryCorrelationID(t *testing.T) {
	var logs bytes.Buffer
	h := NewOrders(&fakeCreator{err: errors.New("db down")}, fakeReader{}, slog.New(slog.NewJSONHandler(&logs, nil)))
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(validBody))
	req.Header.Set("Idempotency-Key", "key-1")
	req.Header.Set(trace.Header, "corr-error-1")

	WithCorrelation(mux).ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(logs.String(), `"correlation_id":"corr-error-1"`) {
		t.Fatalf("log sin correlation_id: %s", logs.String())
	}
}
