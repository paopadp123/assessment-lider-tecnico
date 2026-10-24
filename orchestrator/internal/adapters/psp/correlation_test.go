package psp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

func TestCreatePayment_SendsCorrelationID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(trace.Header)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"paymentId":"pay-1","status":"PENDING"}`))
	}))
	t.Cleanup(srv.Close)

	_, _ = NewClient(srv.URL, srv.Client()).CreatePayment(trace.WithID(context.Background(), "corr-1"), req)

	if got != "corr-1" {
		t.Fatalf("%s = %q", trace.Header, got)
	}
}
