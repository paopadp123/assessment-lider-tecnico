package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

func newActions(repo *fakeRepo) *OperatorActions { return NewOperatorActions(repo, discardLogger()) }

func refundRequiredInRepo(t *testing.T) (*fakeRepo, order.OrderID) {
	t.Helper()
	repo, id := issuingInRepo(t)
	o := orderIn(t, repo, id)
	o.FailureReason = ReasonIssueRejected
	if _, err := transition(context.Background(), repo, time.Now, o, order.RefundRequired, ReasonIssueRejected); err != nil {
		t.Fatal(err)
	}
	return repo, id
}

func lastEvent(repo *fakeRepo) order.Event { return repo.events[len(repo.events)-1] }

func TestRetryIssuance_FromRefundRequiredGoesBackToIssuing(t *testing.T) {
	repo, id := refundRequiredInRepo(t)
	issued := commandsFor(repo, id, CommandIssuePolicy)

	o, err := newActions(repo).RetryIssuance(context.Background(), id, "ana.perez")

	if err != nil || o.Status != order.Issuing || o.FailureReason != "" {
		t.Fatalf("orden = %s / %q, err=%v", o.Status, o.FailureReason, err)
	}
	if commandsFor(repo, id, CommandIssuePolicy) != issued+1 {
		t.Fatal("no se encoló la emisión")
	}
	if ev := lastEvent(repo); ev.From != order.RefundRequired || ev.To != order.Issuing || ev.Actor != "ana.perez" {
		t.Fatalf("evento = %+v; debe registrar quién ejecutó la acción", ev)
	}
}

// Desde ISSUING no hay cambio de estado, pero la acción igual queda registrada en la línea de tiempo.
func TestRetryIssuance_FromIssuingReenqueuesAndIsAudited(t *testing.T) {
	repo, id := issuingInRepo(t)
	issued := commandsFor(repo, id, CommandIssuePolicy)

	o, err := newActions(repo).RetryIssuance(context.Background(), id, "ana.perez")

	if err != nil || o.Status != order.Issuing || commandsFor(repo, id, CommandIssuePolicy) != issued+1 {
		t.Fatalf("status=%s err=%v", o.Status, err)
	}
	if ev := lastEvent(repo); ev.From != order.Issuing || ev.To != order.Issuing || ev.Actor != "ana.perez" {
		t.Fatalf("evento = %+v", ev)
	}
}

func TestMarkForReversal_FromPaidOrIssuingRequiresRefund(t *testing.T) {
	for name, setup := range map[string]func(*testing.T) (*fakeRepo, order.OrderID){"PAID": paidInRepo, "ISSUING": issuingInRepo} {
		t.Run(name, func(t *testing.T) {
			repo, id := setup(t)

			o, err := newActions(repo).MarkForReversal(context.Background(), id, "ana.perez")

			if err != nil || o.Status != order.RefundRequired || o.FailureReason != ReasonMarkedForReversal {
				t.Fatalf("orden = %s / %q, err=%v", o.Status, o.FailureReason, err)
			}
			if ev := lastEvent(repo); ev.To != order.RefundRequired || ev.Actor != "ana.perez" {
				t.Fatalf("evento = %+v", ev)
			}
		})
	}
}

// Una acción que no aplica al estado se rechaza sin tocar nada (la API responde 409).
func TestActions_NotAllowedInCurrentStatus(t *testing.T) {
	repo, id := pendingOrder(t)
	events := len(repo.events)
	a := newActions(repo)

	_, retryErr := a.RetryIssuance(context.Background(), id, "ana.perez")
	_, reversalErr := a.MarkForReversal(context.Background(), id, "ana.perez")

	if !errors.Is(retryErr, ErrActionNotAllowed) || !errors.Is(reversalErr, ErrActionNotAllowed) {
		t.Fatalf("errores = %v / %v", retryErr, reversalErr)
	}
	if len(repo.events) != events {
		t.Fatal("una acción rechazada no debe quedar en la línea de tiempo")
	}
}

func TestActions_IssuedOrderCannotBeReversedOrReissued(t *testing.T) {
	repo, id := paidInRepo(t)
	newIssuePolicy(repo, newFakeIssuer(), &bytes.Buffer{}).Execute(context.Background(), delivery(id, 1))
	a := newActions(repo)

	_, retryErr := a.RetryIssuance(context.Background(), id, "ana.perez")
	_, reversalErr := a.MarkForReversal(context.Background(), id, "ana.perez")

	if !errors.Is(retryErr, ErrActionNotAllowed) || !errors.Is(reversalErr, ErrActionNotAllowed) {
		t.Fatalf("errores = %v / %v", retryErr, reversalErr)
	}
}

func TestActions_UnknownOrderAndMissingOperator(t *testing.T) {
	repo, id := paidInRepo(t)
	a := newActions(repo)

	if _, err := a.MarkForReversal(context.Background(), "missing", "ana.perez"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("orden inexistente: err = %v", err)
	}
	if _, err := a.MarkForReversal(context.Background(), id, ""); err == nil {
		t.Fatal("una acción sin operador no debe ejecutarse: el registro de quién es obligatorio")
	}
	if o := orderIn(t, repo, id); o.Status != order.Paid {
		t.Fatalf("status = %s", o.Status)
	}
}

// Si la póliza aparece después de marcar REFUND_REQUIRED (el emisor terminó tarde, o un operador marcó el
// reverso mientras el worker emitía), la verdad es ISSUED: como no hay reembolso automático, aún no se
// devolvió nada. Queda en WARN para operaciones.
func TestIssuePolicy_PolicyIssuedAfterRefundRequiredWins(t *testing.T) {
	repo, id := paidInRepo(t)
	issuer := newFakeIssuer()
	issuer.onIssue = func() {
		if _, err := newActions(repo).MarkForReversal(context.Background(), id, "ana.perez"); err != nil {
			t.Errorf("marcar reverso: %v", err)
		}
	}
	logs := &bytes.Buffer{}

	res := newIssuePolicy(repo, issuer, logs).Execute(context.Background(), delivery(id, 1))

	if o := orderIn(t, repo, id); res.Outcome != CommandDone || o.Status != order.Issued || o.PolicyNumber != "POL-1" {
		t.Fatalf("outcome=%s orden=%s/%q", res.Outcome, o.Status, o.PolicyNumber)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Fatalf("falta el WARN para operaciones: %s", logs.String())
	}
}
