-- HU-09: consola de operaciones.

-- Cada intento con un tercero (cotizador, PSP, emisor), para que la consola muestre qué respondió cada uno.
-- Nunca guarda los cuerpos de la solicitud ni de la respuesta: llevan placa y documento.
CREATE TABLE third_party_calls (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES orders (id),
    party       text        NOT NULL CHECK (party IN ('quoter', 'psp', 'issuer')),
    operation   text        NOT NULL CHECK (operation IN ('quote', 'create_payment', 'issue', 'find_policy')),
    outcome     text        NOT NULL CHECK (outcome IN ('OK', 'REJECTED', 'UNAVAILABLE', 'TIMEOUT', 'CIRCUIT_OPEN', 'NOT_FOUND')),
    detail      text,
    duration_ms integer     NOT NULL CHECK (duration_ms >= 0),
    started_at  timestamptz NOT NULL
);

CREATE INDEX third_party_calls_order_id_idx ON third_party_calls (order_id, id);

-- Listado de la consola: de la más reciente a la más antigua, y búsqueda por los identificadores que un
-- operador recibe en un reclamo (paymentId ya es UNIQUE y el correlation-id ya tiene índice).
CREATE INDEX orders_created_at_id_idx ON orders (created_at DESC, id DESC);
CREATE INDEX orders_policy_number_idx ON orders (policy_number);
CREATE INDEX orders_plate_idx ON orders (plate);
