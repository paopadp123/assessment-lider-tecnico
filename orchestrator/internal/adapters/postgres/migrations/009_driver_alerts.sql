-- HU-20: alertas de la app del conductor (botón de pánico).
-- El id lo genera el teléfono al crear la alerta: es la clave de idempotencia de los reintentos de la cola
-- offline. La placa se guarda como en orders, para cruzarla con la póliza del vehículo.
CREATE TABLE driver_alerts (
    id          uuid        PRIMARY KEY,
    plate       text        NOT NULL,
    trip_id     text        NOT NULL,
    type        text        NOT NULL CHECK (type IN ('PANIC')),
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX driver_alerts_received_at_idx ON driver_alerts (received_at DESC);
