-- HU-06: correlation-id por compra. Se guarda con la orden y cada comando de la outbox lo copia de su
-- orden al insertarse, para que viaje en el mensaje de RabbitMQ sin depender de quién lo encoló.
ALTER TABLE orders ADD COLUMN correlation_id text;
ALTER TABLE outbox ADD COLUMN correlation_id text;

-- Operaciones busca una compra por su correlation-id (p. ej. desde un log o un reclamo).
CREATE INDEX orders_correlation_id_idx ON orders (correlation_id);
