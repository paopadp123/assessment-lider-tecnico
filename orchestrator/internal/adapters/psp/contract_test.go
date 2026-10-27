package psp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/contract"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// TestContractWithSimulator es el lado consumidor del PSP en contracts/orchestrator-simulator.json.
func TestContractWithSimulator(t *testing.T) {
	f := contract.Load(t, "orchestrator-simulator.json")
	interactions := f.ForPath("/v1/payments")
	if len(interactions) == 0 {
		t.Fatal("el contrato no tiene interacciones del PSP")
	}
	for _, it := range interactions {
		t.Run(it.Name, func(t *testing.T) {
			var req struct {
				QuoteID     string      `json:"quoteId"`
				Amount      json.Number `json:"amount"`
				Currency    string      `json:"currency"`
				CustomerRef string      `json:"customerRef"`
			}
			if err := json.Unmarshal(it.Request.Body, &req); err != nil {
				t.Fatal(err)
			}
			var want struct {
				PaymentID string `json:"paymentId"`
			}
			if err := json.Unmarshal(it.Response.Body, &want); err != nil {
				t.Fatal(err)
			}
			client := NewClient(contract.Provider(t, it).URL, &http.Client{Timeout: 5 * time.Second})

			// customerRef y la Idempotency-Key salen del id de la orden.
			payment, err := client.CreatePayment(context.Background(), app.PaymentRequest{
				OrderID: order.OrderID(req.CustomerRef), QuoteID: req.QuoteID, Amount: req.Amount.String(), Currency: req.Currency,
			})

			if it.ConsumerExpects != "accepted" {
				t.Fatalf("consumerExpects sin cubrir: %q", it.ConsumerExpects)
			}
			if err != nil || string(payment) != want.PaymentID {
				t.Fatalf("se esperaba el pago %q, llegó %q (error %v)", want.PaymentID, payment, err)
			}
		})
	}
}
