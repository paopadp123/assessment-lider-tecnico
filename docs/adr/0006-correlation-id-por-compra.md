# ADR 0006 — Un correlation-id por compra, guardado con la orden

- **Estado:** Aceptado
- **Fecha:** 2026-10-24
- **Responsable:** Líder técnico

## Contexto
El enunciado exige un correlation-id propagado en todos los saltos y en los logs. La compra no es una sola solicitud: pasa por `POST /orders`, la outbox, RabbitMQ, un worker que llama al PSP, un webhook que llega horas o segundos después, otro worker que llama a la ACL, el reconciliador y las acciones de operadores. Una cabecera HTTP se pierde en cuanto el trabajo pasa por la base de datos y por la cola.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **Uno por compra, guardado en la orden** | Una búsqueda por ese id muestra toda la vida de la compra, del pago a la póliza, aunque pase por la cola, el webhook o el reconciliador; operaciones puede buscar la orden por el id que le da el cliente | Una columna más; las solicitudes de operadores necesitan su propio id |
| Uno por solicitud HTTP | Es lo estándar en un servicio sin colas | El hilo se corta en cada salto asíncrono: el pago y la emisión quedan con otro id o sin ninguno |
| Trazas distribuidas (OpenTelemetry) | Tiempos por salto, visualización de cada traza | Infraestructura (colector, backend de trazas) que no aporta al alcance; se puede agregar encima, porque el id por compra es compatible |

## Decisión
- `POST /orders` usa el `X-Correlation-ID` del cliente si es válido (hasta 64 caracteres `[A-Za-z0-9._:-]`, para que no se pueda inyectar una línea de log falsa) o genera un UUID. Se guarda en `orders.correlation_id` y se devuelve en la respuesta. Al repetir la solicitud (misma `Idempotency-Key`) se devuelve el de la compra original.
- Cada comando de la outbox **copia el id de su orden en el mismo `INSERT`**: quien lo encole (creación, webhook, reconciliador, operador) no necesita conocerlo.
- En RabbitMQ viaja en la propiedad estándar `CorrelationId`; los reintentos y la DLQ la conservan. El worker la pone en el contexto, y los clientes del cotizador, el PSP y la ACL la envían como `X-Correlation-ID`.
- El webhook del PSP no lo trae: se toma de la orden. El reconciliador usa el de cada orden.
- Los logs lo agregan explícitamente (`trace.Logger(ctx, log)`) al entrar a cada caso de uso, en lugar de un handler de slog que lo lea "por magia": es más fácil de seguir y de probar.

## Consecuencias
- **Buenas:** verificado de punta a punta con el binario real: el cotizador recibió el id en la solicitud HTTP y el PSP lo recibió desde el worker, después de pasar por la outbox y RabbitMQ.
- **Malas:** los logs de las solicitudes de operadores y del login tienen un id propio por solicitud, no el de la compra (la acción se une a la compra por `order_id`). Un log escrito sin pasar por `trace.Logger` queda sin id: lo mitigan las pruebas por caso de uso y la revisión.
- **En AWS:** el mismo id sirve como clave de búsqueda en CloudWatch Logs Insights y como atributo del mensaje en SQS (`MessageAttributes`).
