package quoter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// El cotizador recibe el correlation-id de la compra: sus logs se pueden cruzar con los nuestros.
func TestQuote_SendsCorrelationID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(trace.Header)
		_, _ = w.Write([]byte(`{"quoteId":"q-1","premium":850000,"currency":"COP","validUntil":"2030-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	_, _ = NewClient(srv.URL, srv.Client()).Quote(trace.WithID(context.Background(), "corr-1"),
		app.QuoteRequest{Plate: "ABC123", VehicleYear: 2020, CityCode: "11001", DocumentID: "1020304050"})

	if got != "corr-1" {
		t.Fatalf("%s = %q", trace.Header, got)
	}
}
