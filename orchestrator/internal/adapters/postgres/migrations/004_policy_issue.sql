-- HU-04: emisión de la póliza. Al quedar PAID se encola issue_policy por la misma outbox,
-- y la orden emitida guarda el número de póliza que devuelve la capa de emisión.
ALTER TABLE outbox DROP CONSTRAINT outbox_type_check;
ALTER TABLE outbox ADD CONSTRAINT outbox_type_check CHECK (type IN ('request_payment', 'issue_policy'));

ALTER TABLE orders ADD COLUMN policy_number text;
