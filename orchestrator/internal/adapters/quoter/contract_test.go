package quoter

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/contract"
)

// TestContractWithSimulator es el lado consumidor del cotizador en contracts/orchestrator-simulator.json.
func TestContractWithSimulator(t *testing.T) {
	f := contract.Load(t, "orchestrator-simulator.json")
	interactions := f.ForPath("/v1/quotes")
	if len(interactions) == 0 {
		t.Fatal("el contrato no tiene interacciones del cotizador")
	}
	for _, it := range interactions {
		t.Run(it.Name, func(t *testing.T) {
			var req struct {
				Plate       string `json:"plate"`
				VehicleYear int    `json:"vehicleYear"`
				CityCode    string `json:"cityCode"`
				DocumentID  string `json:"documentId"`
			}
			if err := json.Unmarshal(it.Request.Body, &req); err != nil {
				t.Fatal(err)
			}
			var want struct {
				QuoteID  string `json:"quoteId"`
				Currency string `json:"currency"`
			}
			if err := json.Unmarshal(it.Response.Body, &want); err != nil {
				t.Fatal(err)
			}
			client := NewClient(contract.Provider(t, it).URL, &http.Client{Timeout: 5 * time.Second})

			quote, err := client.Quote(context.Background(), app.QuoteRequest{
				OrderID: "order-1", Plate: req.Plate, VehicleYear: req.VehicleYear, CityCode: req.CityCode, DocumentID: req.DocumentID,
			})

			if it.ConsumerExpects != "quoted" {
				t.Fatalf("consumerExpects sin cubrir: %q", it.ConsumerExpects)
			}
			if err != nil || quote.ID != want.QuoteID {
				t.Fatalf("se esperaba la cotización %q, llegó %+v (error %v)", want.QuoteID, quote, err)
			}
			// La prima numérica del contrato se guarda con dos decimales, como numeric(14,2).
			if quote.Premium != "412000.00" || quote.Currency != want.Currency {
				t.Fatalf("prima o moneda mal interpretadas: %+v", quote)
			}
		})
	}
}
