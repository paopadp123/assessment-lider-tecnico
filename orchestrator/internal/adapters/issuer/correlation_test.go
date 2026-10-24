package issuer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

// La ACL recibe el correlation-id en la emisión y en la consulta.
func TestClient_SendsCorrelationID(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(trace.Header))
		_, _ = w.Write([]byte(`{"policyNumber":"POL-1"}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client())
	ctx := trace.WithID(context.Background(), "corr-1")

	_, _ = c.Issue(ctx, sampleRequest(t))
	_, _ = c.FindByExternalRef(ctx, ref)

	if len(got) != 2 || got[0] != "corr-1" || got[1] != "corr-1" {
		t.Fatalf("%s recibidos = %v", trace.Header, got)
	}
}
