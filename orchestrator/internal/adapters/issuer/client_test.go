package issuer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

const ref = order.OrderID("4f1c2a7e-0000-4000-8000-000000000001")

func sampleRequest(t *testing.T) app.IssueRequest {
	t.Helper()
	plate, err := order.NewPlate("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := order.NewDocumentID("1020304050")
	if err != nil {
		t.Fatal(err)
	}
	return app.IssueRequest{ExternalRef: ref, QuoteID: "q-1", Premium: "850000.00", Currency: "COP",
		Plate: plate, DocumentID: doc, VehicleYear: 2020, CityCode: "11001"}
}

// issueServer responde a POST /policies y verifica el contrato que se le envía a la ACL.
func issueServer(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if r.Method != http.MethodPost || r.URL.Path != "/policies" || json.NewDecoder(r.Body).Decode(&got) != nil {
			t.Errorf("solicitud inesperada: %s %s", r.Method, r.URL.Path)
		}
		want := map[string]any{"externalRef": string(ref), "quoteId": "q-1", "premium": 850000.0, "currency": "COP",
			"plate": "ABC123", "documentId": "1020304050", "vehicleYear": 2020.0, "cityCode": "11001"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("campo %s = %v, want %v", k, got[k], v)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, srv.Client())
}

func TestIssue_CreatedReturnsPolicyNumber(t *testing.T) {
	n, err := issueServer(t, http.StatusCreated, `{"policyNumber":"POL-77","externalRef":"x"}`).Issue(context.Background(), sampleRequest(t))

	if err != nil || n != "POL-77" {
		t.Fatalf("n=%q err=%v", n, err)
	}
}

// La ACL es idempotente por externalRef: si ya estaba emitida responde 200 con la misma póliza.
func TestIssue_AlreadyIssuedReturnsSamePolicy(t *testing.T) {
	n, err := issueServer(t, http.StatusOK, `{"policyNumber":"POL-77"}`).Issue(context.Background(), sampleRequest(t))

	if err != nil || n != "POL-77" {
		t.Fatalf("n=%q err=%v", n, err)
	}
}

func TestIssue_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"rechazo de negocio", http.StatusUnprocessableEntity, `{"code":"VEHICLE_NOT_INSURABLE","message":"no asegurable"}`, app.ErrIssueRejected},
		// Solo un 422 es rechazo de negocio. Otro 4xx es un error nuestro o de configuración: marcar
		// REFUND_REQUIRED una orden pagada por eso sería un error caro; se reintenta y termina en la DLQ.
		{"solicitud inválida", http.StatusBadRequest, `{}`, app.ErrIssuerUnavailable},
		{"sin autorización", http.StatusUnauthorized, ``, app.ErrIssuerUnavailable},
		{"ruta mal configurada", http.StatusNotFound, ``, app.ErrIssuerUnavailable},
		{"falla del servidor", http.StatusBadGateway, ``, app.ErrIssuerUnavailable},
		{"saturado", http.StatusTooManyRequests, ``, app.ErrIssuerUnavailable},
		{"cuerpo inválido", http.StatusCreated, `{no json`, app.ErrIssuerUnavailable},
		{"sin número de póliza", http.StatusCreated, `{}`, app.ErrIssuerUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := issueServer(t, tc.status, tc.body).Issue(context.Background(), sampleRequest(t))

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// El código del rechazo se conserva para operaciones; el mensaje de la ACL no (puede traer datos personales).
func TestIssue_RejectionKeepsCodeButNotMessage(t *testing.T) {
	_, err := issueServer(t, http.StatusUnprocessableEntity, `{"code":"VEHICLE_NOT_INSURABLE","message":"placa ABC123 no asegurable"}`).
		Issue(context.Background(), sampleRequest(t))

	if err == nil || !strings.Contains(err.Error(), "VEHICLE_NOT_INSURABLE") || strings.Contains(err.Error(), "ABC123") {
		t.Fatalf("err = %v", err)
	}
}

// Un timeout del contexto es transitorio: el caso de uso consulta antes de reemitir.
func TestIssue_TimeoutIsDeadlineExceeded(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // simula un emisor que no responde; se libera al terminar para que srv.Close no espere
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // las limpiezas corren en orden inverso: primero se libera el handler
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := NewClient(srv.URL, srv.Client()).Issue(ctx, sampleRequest(t))

	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, app.ErrIssuerUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func findServer(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/policies" || r.URL.Query().Get("externalRef") != string(ref) {
			t.Errorf("consulta inesperada: %s %s", r.Method, r.URL)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, srv.Client())
}

func TestFindByExternalRef(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    order.PolicyNumber
		wantErr error
	}{
		{"emitida", http.StatusOK, `{"policyNumber":"POL-77"}`, "POL-77", nil},
		{"no existe", http.StatusNotFound, ``, "", app.ErrPolicyNotFound},
		{"falla del servidor", http.StatusServiceUnavailable, ``, "", app.ErrIssuerUnavailable},
		{"respuesta sin póliza", http.StatusOK, `{}`, "", app.ErrIssuerUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := findServer(t, tc.status, tc.body).FindByExternalRef(context.Background(), ref)

			if n != tc.want || !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("n=%q err=%v", n, err)
			}
		})
	}
}
