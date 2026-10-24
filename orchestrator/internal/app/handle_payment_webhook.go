package app

import (
	"context"
	"errors"
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

// Execute registra el evento y lo aplica si la orden lo espera.
func (uc *HandlePaymentWebhook) Execute(ctx context.Context, ev PaymentEvent) (PaymentEventResult, error) {
	return PaymentEventResult{}, errors.New("not implemented")
}
