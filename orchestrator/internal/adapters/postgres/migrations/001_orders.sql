CREATE TABLE orders (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status            text        NOT NULL CHECK (status IN ('CREATED', 'QUOTED', 'PAYMENT_PENDING', 'PAID',
                                                             'ISSUING', 'ISSUED', 'FAILED', 'REFUND_REQUIRED')),
    plate             text        NOT NULL,
    document_id       text        NOT NULL,
    vehicle_year      integer     NOT NULL,
    city_code         text        NOT NULL,
    quote_id          text,
    premium           numeric(14, 2),
    currency          char(3),
    quote_valid_until timestamptz,
    failure_reason    text,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);

CREATE INDEX orders_status_updated_at_idx ON orders (status, updated_at);

CREATE TABLE order_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES orders (id),
    from_status text,
    to_status   text        NOT NULL CHECK (to_status IN ('CREATED', 'QUOTED', 'PAYMENT_PENDING', 'PAID',
                                                          'ISSUING', 'ISSUED', 'FAILED', 'REFUND_REQUIRED')),
    reason      text        NOT NULL,
    occurred_at timestamptz NOT NULL
);

CREATE INDEX order_events_order_id_idx ON order_events (order_id, id);

-- La clave primaria es lo que garantiza una sola orden por clave, incluso con solicitudes concurrentes.
CREATE TABLE idempotency_keys (
    key          text PRIMARY KEY CHECK (length(key) BETWEEN 1 AND 255),
    request_hash text        NOT NULL,
    order_id     uuid        NOT NULL REFERENCES orders (id),
    created_at   timestamptz NOT NULL DEFAULT now()
);
