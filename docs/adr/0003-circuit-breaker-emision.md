# ADR 0003 — Circuit breaker hacia la capa de emisión, sin consumir intentos con el circuito abierto

- **Estado:** Aceptado
- **Fecha:** 2026-10-24
- **Responsable:** Líder técnico

## Contexto
Cuando una orden queda PAID, el orquestador emite la póliza a través de la ACL en C#, que envuelve al emisor legado. El emisor tiene ventanas de caída total y timeouts de 8 s **habiendo emitido**. El enunciado exige un circuit breaker hacia el emisor, que la compra no se pierda si está caído ("encolada y observable") y que un timeout no autorice a reemitir: antes de reintentar se consulta por `externalRef`. Además, el dinero ya se cobró: emitir dos veces o perder la orden son los dos errores caros.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **Breaker en el orquestador; con el circuito abierto el comando se aplaza sin consumir intentos** | Una caída larga no manda órdenes pagadas a la DLQ; no se martilla al emisor caído; la orden queda visible en ISSUING | Un mensaje puede aplazarse indefinidamente si el emisor no vuelve (lo acota la reconciliación de HU-05) |
| Breaker que cuenta cada rechazo como un intento | Más simple: reutiliza el reintento normal | Con 4 intentos, una caída de pocos minutos manda a la DLQ todas las órdenes pagadas en ese lapso |
| Breaker dentro de la ACL (C#) | El orquestador queda más simple | El enunciado lo pide en el orquestador; la ACL caída no puede protegerse a sí misma |
| Sin breaker, solo reintentos con backoff | Una pieza menos | Cada orden insiste por su cuenta contra un emisor caído; carga inútil y recuperación más lenta |

## Decisión
- **`sony/gobreaker/v2`** como decorador del puerto `PolicyIssuer`. Emisión y consulta comparten el circuito (mismo servicio).
- **Contrato con la ACL:** solo un **422** es rechazo de negocio definitivo. Cualquier otro status (400, 401, 404, 5xx, 429) se trata como transitorio: un error de configuración o del request nunca deja una orden pagada en REFUND_REQUIRED.
- **Qué cuenta como falla:** todo lo transitorio (status distinto de 2xx y 422, errores de red, timeouts y respuestas inválidas). **No cuentan** un rechazo de negocio (422) ni un 404 de la consulta: la ACL respondió. Tampoco una cancelación propia (apagado del proceso).
- **Umbrales:** abre tras **5 fallas seguidas**, queda abierto **30 s** y deja pasar **1 llamada de prueba** (half-open). A 50 rps pico, 5 fallas seguidas son ~0,1 s de evidencia; 30 s cubre una recuperación corta sin dejar órdenes esperando demasiado.
- **Breaker abierto → `DEFERRED`:** el comando vuelve a la cola tras 30 s **con el mismo número de intento**. Va a una cola de espera propia (`<cola>.deferred`) para no bloquear a los reintentos cortos (RabbitMQ solo vence el mensaje al frente de la cola). Cada aplazamiento queda en un log WARN con `ISSUER_UNAVAILABLE`, y cada cambio de estado del breaker también.
- **Estado visible para operación:** `GET /ops/breakers` devuelve el estado de cada breaker (`closed`, `open`, `half-open`) y las fallas seguidas mientras está cerrado.
- **Nunca emitir a ciegas:** una orden que ya estaba en ISSUING (reentrega o reintento tras un timeout) primero se consulta por `externalRef`. Si la consulta falla, se reintenta más tarde; no se emite.
- **Resultado definitivo:** 422 → REFUND_REQUIRED (sin reembolso automático). Reintentos transitorios agotados → DLQ con la orden **en ISSUING**, no REFUND_REQUIRED: un timeout no prueba que la póliza no exista. La reconciliación (HU-05) consulta y decide.

## Consecuencias
- **Buenas:** con el emisor caído, ninguna orden pagada se pierde ni termina en la DLQ por la caída; el emisor no recibe carga mientras se recupera; "timeout pero sí emitió" termina en ISSUED con una sola emisión (prueba dedicada, validada quitando la consulta a propósito).
- **Malas:** el breaker vive en memoria de cada réplica (con N réplicas, cada una aprende la caída por su cuenta: hasta 5·N llamadas fallidas); una orden puede aplazarse sin límite si el emisor no vuelve, por eso HU-05 vigila las órdenes atascadas en ISSUING; queda una ventana en la que el emisor aún procesa y la consulta responde 404. Esa ventana la cierra la idempotencia por `externalRef` de la ACL (HU-07); el orquestador solo no puede garantizarlo.
- **En AWS:** el aplazamiento equivale a devolver el mensaje a SQS con `ChangeMessageVisibility` de 30 s sin contar el recibo, o a un `DelaySeconds` al reencolar. La alerta se dispara sobre la métrica de cambios de estado del breaker, no sobre cada aplazamiento.
