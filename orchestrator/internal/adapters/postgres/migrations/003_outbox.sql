-- Comandos a publicar en el broker. Se insertan en la misma transacción que el cambio de estado
-- que los origina: no puede existir el estado "guardé en base pero no publiqué".
CREATE TABLE outbox (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_id   uuid        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    type         text        NOT NULL CHECK (type IN ('request_payment')),
    order_id     uuid        NOT NULL REFERENCES orders (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text
);

-- El despachador solo recorre lo pendiente; el índice parcial se mantiene pequeño.
CREATE INDEX outbox_pending_idx ON outbox (id) WHERE published_at IS NULL;

-- Comandos que agotaron sus reintentos, visibles para operaciones (también quedan en la DLQ del broker).
CREATE TABLE dead_letters (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_id text        NOT NULL UNIQUE,
    type       text        NOT NULL,
    order_id   text        NOT NULL,
    attempts   integer     NOT NULL,
    reason     text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
