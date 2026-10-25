# ADR 0004 — Reconciliación de órdenes atascadas y eventos por SSE

- **Estado:** Aceptado
- **Fecha:** 2026-10-24
- **Responsable:** Líder técnico

## Contexto
La outbox y los reintentos (ADR 0002 y 0003) cubren las fallas transitorias, pero una orden todavía puede quedar sin avanzar: el proceso muere después de cotizar (CREATED), el PSP aceptó el pago sin que se guardara PAYMENT_PENDING (QUOTED), el webhook nunca llega (PAYMENT_PENDING), o la emisión agotó reintentos o espera con el breaker abierto (ISSUING). El enunciado pide un job que detecte órdenes atascadas más de N minutos, que consulte al emisor antes de reintentar (un timeout no autoriza a reemitir) y que deje en REFUND_REQUIRED la orden pagada cuya emisión falla en definitiva, sin reembolso automático. La consola necesita la línea de tiempo y ver los cambios sin recargar.

## Decisión
**Reconciliador**
- Corre cada `RECONCILE_EVERY` (1 min) dentro del orquestador. Una orden está atascada si no cambió en `STUCK_AFTER` (N = 5 min) y no se revisó en ese mismo lapso.
- **Toma con lock corto:** una sola sentencia `UPDATE … WHERE id IN (SELECT … FOR UPDATE SKIP LOCKED) RETURNING` marca `reconciled_at` y devuelve el lote. Varias réplicas no se pisan ni se esperan, y las llamadas a terceros ocurren **después**, sin locks tomados. Si la orden avanzó entre la toma y la decisión, el control optimista (`WHERE status = <origen>`) descarta el cambio.
- **Por estado:** CREATED → se vuelve a cotizar; QUOTED → se reencola `request_payment` (misma `Idempotency-Key`: sin doble cobro); PAYMENT_PENDING → solo WARN, porque el PSP no ofrece consulta; PAID → se reencola `issue_policy`; ISSUING → **se consulta por `externalRef`**: si existe la póliza → ISSUED; si no existe → se reencola y se cuenta la ronda; sin respuesta confiable (breaker abierto o emisor caído) → no se cuenta ni se decide.
- **Falla definitiva = 3.ª ronda** en que el emisor confirma que la póliza no existe (decisión de producto). Con N = 5 min son ≥ 15 min de un emisor que responde y no tiene la póliza. Cualquier cambio de estado reinicia la cuenta.
- **La póliza manda:** si aparece una póliza para una orden en REFUND_REQUIRED (el emisor terminó tarde, o un operador marcó el reverso mientras el worker emitía), la orden pasa a ISSUED con un WARN. Es seguro porque el reverso nunca es automático: todavía no se devolvió nada.

**Eventos para la consola**
- `GET /orders/{id}/events`: línea de tiempo con estado, motivo, actor (`system` u operador) y fecha.
- `GET /events/stream` por **SSE**. Cada evento lleva como `id` el ID de `order_events`; el navegador lo reenvía en `Last-Event-ID` al reconectar y recibe lo que se perdió. El servidor lee la tabla cada segundo (no de memoria), así que funciona igual con varias réplicas detrás de un balanceador. Envía un latido cada 15 s para que el balanceador no corte la conexión por inactividad, y se desactiva el `WriteTimeout` solo para el flujo.
- **Sin saltarse eventos:** los IDs de `order_events` se asignan al insertar, no al confirmar; la transacción del evento 5 puede confirmar después que la del 6. Si el flujo entregara el 6 y avanzara, el 5 se perdería para siempre (hallazgo del QA). Por eso, ante un hueco en los IDs, el flujo **espera** a que se llene; si no se llena en 10 s, fue un rollback y sigue. Se descartó filtrar por `xmin` de Postgres: compara IDs de transacción de 32 bits y se rompe al dar la vuelta el contador.

## Opciones consideradas (actualización en vivo)
| Opción | A favor | En contra |
|---|---|---|
| **SSE** | El flujo es solo servidor → navegador; HTTP simple, pasa por ALB y proxies; reconexión y `Last-Event-ID` nativos del navegador | `EventSource` no envía cabeceras: la consola usa `fetch` con streaming o un proxy del servidor de Next.js (HU-09) |
| WebSocket | Bidireccional | La consola no envía nada por ese canal; más infraestructura (upgrade, sesiones pegajosas o pub/sub entre réplicas) |
| Polling del listado | Lo más simple | Latencia o carga: a 50 rps de cambios, cada consola pediría la lista completa una y otra vez |

## Consecuencias
- **Buenas:** ninguna orden queda atascada en silencio; la recuperación del "cobro huérfano" de HU-02 y de la orden en CREATED de HU-01 queda automática; la consola se actualiza sola y reanuda sin perder eventos.
- **Malas:** después de un rollback, el flujo se demora hasta 10 s en entregar lo que sigue; cada flujo SSE abierto consulta la base una vez por segundo (con 20 operadores, 20 consultas/s por índice de clave primaria; en AWS se reemplazaría por `LISTEN/NOTIFY` o un tópico SNS→SQS por réplica si crece). Una orden en PAYMENT_PENDING solo se puede reportar: si el PSP nunca envía el webhook, la resuelve operaciones. Una orden reconciliada puede recibir dos `issue_policy` (el reencolado y uno aplazado); lo absorbe la idempotencia por estado y por `externalRef`.
- **En AWS:** el reconciliador sería una tarea programada (EventBridge Scheduler → ECS task) o se quedaría en el servicio; la toma con `SKIP LOCKED` ya soporta varias réplicas.
