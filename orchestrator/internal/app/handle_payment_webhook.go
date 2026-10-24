package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// HandlePaymentWebhook es el caso de uso que procesa un resultado de pago ya verificado.
type HandlePaymentWebhook struct {
	repo OrderRepository
	log  *slog.Logger
	now  func() time.Time
}

// NewHandlePaymentWebhook arma el caso de uso.
func NewHandlePaymentWebhook(repo OrderRepository, log *slog.Logger) *HandlePaymentWebhook {
	return &HandlePaymentWebhook{repo: repo, log: log, now: utcMicros}
}

// Execute registra el evento y lo aplica si la orden lo espera. Los duplicados no tienen efecto,
// los eventos tempranos quedan guardados para aplicarse al registrar PAYMENT_PENDING,
// y los contradictorios se registran como anomalía sin tocar la orden.
func (uc *HandlePaymentWebhook) Execute(ctx context.Context, ev PaymentEvent) (PaymentEventResult, error) {
	res, err := uc.repo.RecordPaymentEvent(ctx, ev, applyPayment(uc.now))
	if err != nil {
		return PaymentEventResult{}, fmt.Errorf("record payment event: %w", err)
	}

	attrs := []any{"payment_id", ev.PaymentID, "status", ev.Status, "outcome", res.Outcome}
	if res.OrderID != "" {
		attrs = append(attrs, "order_id", res.OrderID)
	}
	if res.Outcome == PaymentAnomaly {
		uc.log.Warn("payment event conflicts with order status", attrs...)
	} else {
		uc.log.Info("payment event received", attrs...)
	}
	return res, nil
}
