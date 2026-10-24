package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/auth"
)

func TestOperators_CreateIfMissingAndFind(t *testing.T) {
	pool := testPool(t)
	repo := NewOperatorRepository(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE operators"); err != nil {
		t.Fatal(err)
	}
	op := auth.StoredOperator{Username: "ana.perez", DisplayName: "Ana Pérez", PasswordHash: "hash-1", Active: true}

	if err := repo.CreateOperatorIfMissing(ctx, op); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateOperatorIfMissing(ctx, auth.StoredOperator{Username: "ana.perez", DisplayName: "Otra", PasswordHash: "hash-2", Active: true}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindOperator(ctx, "ana.perez")

	if err != nil || got != op {
		t.Fatalf("operador = %+v, err=%v; el segundo alta no debía pisar al primero", got, err)
	}
	if _, err := repo.FindOperator(ctx, "no-existe"); !errors.Is(err, auth.ErrOperatorNotFound) {
		t.Fatalf("inexistente: err = %v", err)
	}
}
