package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

const corr = "corr-compra-1"

func withCorr() context.Context { return trace.WithID(context.Background(), corr) }

func hasCorrelation(logs string) bool {
	return strings.Contains(logs, `"correlation_id":"`+corr+`"`)
}

// La compra guarda su correlation-id al crearse y el cotizador lo recibe.
func TestCreateOrder_StoresCorrelationIDAndPropagatesIt(t *testing.T) {
	repo, quoter := newFakeRepo(), &fakeQuoter{}
	var logs bytes.Buffer

	res, err := newUseCase(repo, quoter, &logs).Execute(withCorr(), "key-1", validInput)

	if err != nil || res.Order.CorrelationID != corr {
		t.Fatalf("correlation-id = %q, err=%v", res.Order.CorrelationID, err)
	}
	if len(quoter.correlations) != 1 || quoter.correlations[0] != corr {
		t.Fatalf("el cotizador recibió %v", quoter.correlations)
	}
	if !hasCorrelation(logs.String()) {
		t.Fatalf("logs sin correlation_id: %s", logs.String())
	}
}

// Si el cliente no envía uno, la compra igual queda con un correlation-id propio.
func TestCreateOrder_GeneratesCorrelationIDWhenMissing(t *testing.T) {
	res, err := newUseCase(newFakeRepo(), &fakeQuoter{}, &bytes.Buffer{}).Execute(context.Background(), "key-1", validInput)

	if err != nil || !trace.Valid(res.Order.CorrelationID) {
		t.Fatalf("correlation-id = %q, err=%v", res.Order.CorrelationID, err)
	}
}

// Los workers reciben el correlation-id en el contexto (lo pone el adaptador de RabbitMQ) y lo pasan al
// tercero y a sus logs.
func TestWorkers_PropagateCorrelationIDToThirdPartiesAndLogs(t *testing.T) {
	repo, id := quotedInRepo(t)
	psp := &fakePSP{}
	var payLogs bytes.Buffer
	newRequestPayment(repo, psp, &payLogs).Execute(withCorr(), delivery(id, 1))
	if _, err := NewHandlePaymentWebhook(repo, discardLogger()).Execute(context.Background(), approved()); err != nil {
		t.Fatal(err)
	}
	issuer := newFakeIssuer()
	var issueLogs bytes.Buffer

	newIssuePolicy(repo, issuer, &issueLogs).Execute(withCorr(), delivery(id, 1))

	if len(psp.correlations) != 1 || psp.correlations[0] != corr || len(issuer.correlations) != 1 || issuer.correlations[0] != corr {
		t.Fatalf("PSP recibió %v, ACL recibió %v", psp.correlations, issuer.correlations)
	}
	if !hasCorrelation(payLogs.String()) || !hasCorrelation(issueLogs.String()) {
		t.Fatalf("logs sin correlation_id:\n%s\n%s", payLogs.String(), issueLogs.String())
	}
}

// El PSP no conoce nuestro correlation-id: el webhook lo recupera de la orden para sus logs.
func TestWebhook_LogsTheOrderCorrelationID(t *testing.T) {
	repo, id := pendingOrder(t)
	o := orderIn(t, repo, id)
	o.CorrelationID = corr
	repo.orders[id] = o
	var logs bytes.Buffer

	res, err := NewHandlePaymentWebhook(repo, slog.New(slog.NewJSONHandler(&logs, nil))).Execute(context.Background(), approved())

	if err != nil || res.CorrelationID != corr || !hasCorrelation(logs.String()) {
		t.Fatalf("res=%+v err=%v logs=%s", res, err, logs.String())
	}
}

// El reconciliador corre fuera de cualquier solicitud: usa el correlation-id de cada orden.
func TestReconcile_UsesTheOrderCorrelationID(t *testing.T) {
	repo, id := createdInRepo(t)
	o := orderIn(t, repo, id)
	o.CorrelationID = corr
	repo.orders[id] = o
	stuck(repo, id)
	quoter := &fakeQuoter{}
	var logs bytes.Buffer

	reconcileOnce(t, newTestReconciler(repo, quoter, newFakeIssuer(), &logs))

	if len(quoter.correlations) != 1 || quoter.correlations[0] != corr || !hasCorrelation(logs.String()) {
		t.Fatalf("cotizador recibió %v; logs=%s", quoter.correlations, logs.String())
	}
}

// Barrido: el flujo completo, con todos los casos de uso escribiendo en el mismo log, nunca deja la placa
// ni el documento en claro, ni siquiera en errores o anomalías.
func TestSensitiveDataNeverReachesLogs(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := withCorr()
	repo := newFakeRepo()
	create := NewCreateOrder(repo, &fakeQuoter{}, log)
	create.sleep = func(context.Context, time.Duration) error { return nil }

	// Compra feliz hasta la emisión rechazada → REFUND_REQUIRED, con un webhook contradictorio en el medio.
	res, err := create.Execute(ctx, "k-1", validInput)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Order.ID
	NewRequestPayment(repo, repo, &fakePSP{}, log).Execute(ctx, delivery(id, 1))
	webhook := NewHandlePaymentWebhook(repo, log)
	_, _ = webhook.Execute(ctx, approved())
	rejected := approved()
	rejected.Status = order.PaymentRejected
	_, _ = webhook.Execute(ctx, rejected)
	issuer := newFakeIssuer()
	issuer.issueErrs = []error{ErrIssueRejected}
	NewIssuePolicy(repo, repo, issuer, log, time.Second).Execute(ctx, delivery(id, 1))
	_, _ = NewOperatorActions(repo, log).RetryIssuance(ctx, id, "ana.perez")

	// Una segunda compra que se atasca y la recotiza el reconciliador, con el cotizador caído.
	res2, _ := create.Execute(ctx, "k-2", order.NewOrderInput{Plate: "XYZ987", DocumentID: "9988776655", VehicleYear: 2021, CityCode: "05001"})
	o2 := orderIn(t, repo, res2.Order.ID)
	o2.Status, o2.Quote = order.Created, nil
	repo.orders[o2.ID] = o2
	stuck(repo, o2.ID)
	r := NewReconciler(repo, repo, &fakeQuoter{errs: []error{ErrQuoteUnavailable, ErrQuoteUnavailable, ErrQuoteUnavailable}}, issuer, log, stuckAfter)
	r.sleep = func(context.Context, time.Duration) error { return nil }
	reconcileOnce(t, r)

	for _, secret := range []string{"ABC123", "1020304050", "XYZ987", "9988776655"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("el dato %q aparece en claro en los logs", secret)
		}
	}
	if logs.Len() == 0 || strings.Count(logs.String(), "\n") < 8 {
		t.Fatalf("el barrido no generó suficientes logs para ser significativo: %s", logs.String())
	}
}
