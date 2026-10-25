-- El webhook del PSP solo trae el paymentId: con él se encuentra la orden.
ALTER TABLE orders ADD COLUMN payment_id text UNIQUE;

-- Todo webhook se guarda. El índice único descarta los reenvíos del PSP;
-- applied_at nulo significa que el evento llegó antes de que la orden esperara ese pago.
CREATE TABLE payment_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_id  text        NOT NULL,
    status      text        NOT NULL CHECK (status IN ('APPROVED', 'REJECTED')),
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    applied_at  timestamptz,
    outcome     text CHECK (outcome IN ('APPLIED', 'ANOMALY')),
    UNIQUE (payment_id, status)
);

CREATE INDEX payment_events_unapplied_idx ON payment_events (payment_id) WHERE applied_at IS NULL;
