package httpapi

import (
	"strings"
	"testing"
)

const testSecret = "test-secret-with-at-least-32-bytes!!"

func TestSignatureVerifier(t *testing.T) {
	v, err := NewSignatureVerifier(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewSignatureVerifier(strings.Repeat("x", 32))
	valid := v.Sign("pay-1", "APPROVED", "2026-10-24T10:00:00Z")

	tests := []struct {
		name                          string
		paymentID, status, occurredAt string
		signature                     string
		want                          bool
	}{
		{"firma válida", "pay-1", "APPROVED", "2026-10-24T10:00:00Z", valid, true},
		{"estado alterado", "pay-1", "REJECTED", "2026-10-24T10:00:00Z", valid, false},
		{"pago alterado", "pay-2", "APPROVED", "2026-10-24T10:00:00Z", valid, false},
		{"fecha alterada", "pay-1", "APPROVED", "2026-10-24T10:00:01Z", valid, false},
		{"otro secreto", "pay-1", "APPROVED", "2026-10-24T10:00:00Z", other.Sign("pay-1", "APPROVED", "2026-10-24T10:00:00Z"), false},
		{"no hexadecimal", "pay-1", "APPROVED", "2026-10-24T10:00:00Z", "zz" + valid[2:], false},
		{"vacía", "pay-1", "APPROVED", "2026-10-24T10:00:00Z", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Verify(tt.paymentID, tt.status, tt.occurredAt, tt.signature); got != tt.want {
				t.Fatalf("Verify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSignatureVerifier_RejectsShortSecret(t *testing.T) {
	if _, err := NewSignatureVerifier("short"); err == nil {
		t.Fatal("se esperaba error con un secreto corto")
	}
}
