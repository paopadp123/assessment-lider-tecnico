package order

import (
	"errors"
	"testing"
	"time"
)

func TestParsePaymentStatus(t *testing.T) {
	tests := []struct {
		in      string
		want    PaymentStatus
		wantErr bool
	}{
		{in: "APPROVED", want: PaymentApproved},
		{in: "REJECTED", want: PaymentRejected},
		{in: "approved", wantErr: true},
		{in: "PENDING", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePaymentStatus(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q err %v", got, err)
			}
		})
	}
}

func TestApplyPayment(t *testing.T) {
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		from       Status
		payment    PaymentStatus
		wantStatus Status
		wantReason string
		wantErr    error
	}{
		{"aprobado mientras espera el pago", PaymentPending, PaymentApproved, Paid, "", nil},
		{"rechazado mientras espera el pago", PaymentPending, PaymentRejected, Failed, ReasonPaymentRejected, nil},
		{"rechazo después de pagada es anomalía", Paid, PaymentRejected, Paid, "", ErrPaymentConflict},
		{"aprobación después de fallida es anomalía", Failed, PaymentApproved, Failed, "", ErrPaymentConflict},
		{"rechazo durante la emisión es anomalía", Issuing, PaymentRejected, Issuing, "", ErrPaymentConflict},
		{"orden aún sin pago pedido", Quoted, PaymentApproved, Quoted, "", ErrPaymentConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := Order{Status: tt.from}

			ev, err := o.ApplyPayment(tt.payment, now)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if o.Status != tt.wantStatus || o.FailureReason != tt.wantReason {
				t.Fatalf("status=%s reason=%q, want %s %q", o.Status, o.FailureReason, tt.wantStatus, tt.wantReason)
			}
			if tt.wantErr == nil && (ev.From != tt.from || ev.To != tt.wantStatus || !ev.OccurredAt.Equal(now)) {
				t.Fatalf("evento incorrecto: %+v", ev)
			}
		})
	}
}
