-- HU-05: reconciliación y auditoría de acciones operativas.
-- actor: operador que ejecutó la acción (NULL = la hizo el sistema).
ALTER TABLE order_events ADD COLUMN actor text;

-- reconciled_at: última revisión del reconciliador; evita revisar la misma orden en cada pasada.
-- reconcile_misses: rondas seguidas en que el emisor confirmó que la póliza no existe. Se reinicia con
-- cualquier cambio de estado.
ALTER TABLE orders ADD COLUMN reconciled_at timestamptz;
ALTER TABLE orders ADD COLUMN reconcile_misses integer NOT NULL DEFAULT 0;

-- El reconciliador solo recorre órdenes que pueden quedar atascadas; el índice parcial se mantiene pequeño.
CREATE INDEX orders_stuck_idx ON orders (updated_at)
    WHERE status IN ('CREATED', 'QUOTED', 'PAYMENT_PENDING', 'PAID', 'ISSUING');
