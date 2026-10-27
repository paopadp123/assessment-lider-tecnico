package issuer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/contract"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// TestContractWithIssuingService es el lado consumidor de contracts/orchestrator-issuing-service.json:
// el cliente real envía lo que dice el contrato e interpreta cada respuesta como el contrato espera.
func TestContractWithIssuingService(t *testing.T) {
	f := contract.Load(t, "orchestrator-issuing-service.json")
	interactions := f.ForPath("/policies")
	if len(interactions) == 0 {
		t.Fatal("el contrato no tiene interacciones de /policies")
	}
	for _, it := range interactions {
		t.Run(it.Name, func(t *testing.T) {
			client := NewClient(contract.Provider(t, it).URL, &http.Client{Timeout: 5 * time.Second})
			ctx := context.Background()

			var policy order.PolicyNumber
			var err error
			switch it.Request.Method {
			case http.MethodPost:
				policy, err = client.Issue(ctx, issueRequestFrom(t, it.Request.Body))
			case http.MethodGet:
				policy, err = client.FindByExternalRef(ctx, order.OrderID(it.Request.Query["externalRef"]))
			default:
				t.Fatalf("método sin cubrir: %s", it.Request.Method)
			}

			switch it.ConsumerExpects {
			case "issued", "found":
				var body struct {
					PolicyNumber string `json:"policyNumber"`
				}
				if jsonErr := json.Unmarshal(it.Response.Body, &body); jsonErr != nil {
					t.Fatal(jsonErr)
				}
				if err != nil || string(policy) != body.PolicyNumber {
					t.Fatalf("se esperaba la póliza %q, llegó %q (error %v)", body.PolicyNumber, policy, err)
				}
			case "rejected":
				if !errors.Is(err, app.ErrIssueRejected) {
					t.Fatalf("se esperaba un rechazo definitivo, llegó %v", err)
				}
				var body struct {
					Code string `json:"code"`
				}
				if jsonErr := json.Unmarshal(it.Response.Body, &body); jsonErr != nil {
					t.Fatal(jsonErr)
				}
				if !strings.Contains(err.Error(), body.Code) {
					t.Fatalf("el rechazo no conserva el código %q: %v", body.Code, err)
				}
			case "transient":
				if !errors.Is(err, app.ErrIssuerUnavailable) || errors.Is(err, app.ErrIssueRejected) {
					t.Fatalf("se esperaba un error transitorio, llegó %v", err)
				}
			case "not-found":
				if !errors.Is(err, app.ErrPolicyNotFound) {
					t.Fatalf("se esperaba ErrPolicyNotFound, llegó %v", err)
				}
			default:
				t.Fatalf("consumerExpects sin cubrir: %q", it.ConsumerExpects)
			}
		})
	}
}

// issueRequestFrom arma la solicitud del puerto con los valores del ejemplo del contrato.
func issueRequestFrom(t *testing.T, raw json.RawMessage) app.IssueRequest {
	t.Helper()
	var body struct {
		ExternalRef string      `json:"externalRef"`
		QuoteID     string      `json:"quoteId"`
		Premium     json.Number `json:"premium"`
		Currency    string      `json:"currency"`
		Plate       string      `json:"plate"`
		DocumentID  string      `json:"documentId"`
		VehicleYear int         `json:"vehicleYear"`
		CityCode    string      `json:"cityCode"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		t.Fatal(err)
	}
	plate, err := order.NewPlate(body.Plate)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := order.NewDocumentID(body.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	return app.IssueRequest{ExternalRef: order.OrderID(body.ExternalRef), QuoteID: body.QuoteID, Premium: body.Premium.String(),
		Currency: body.Currency, Plate: plate, DocumentID: doc, VehicleYear: body.VehicleYear, CityCode: body.CityCode}
}
