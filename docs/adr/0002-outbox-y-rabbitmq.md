# ADR 0002 — Outbox en PostgreSQL + RabbitMQ para las tareas asíncronas

- **Estado:** Aceptado
- **Fecha:** 2026-10-24
- **Responsable:** Líder técnico

## Contexto
El orquestador llama a terceros que fallan de forma independiente. En HU-02 la intención de pago se pedía dentro de `POST /orders`: si el proceso moría entre el OK del PSP y el guardado de PAYMENT_PENDING, quedaba un cobro sin orden que avanzara (hallazgo de dos revisores). El enunciado exige reintentos con backoff y jitter, una DLQ, que no exista "guardé en base pero no publiqué el evento" y que `docker compose up` levante "cache o cola". Escala de referencia: 10.000 órdenes/día, picos de 50 solicitudes/s.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **Outbox en PostgreSQL + RabbitMQ** | Atomicidad local (estado + comando en una transacción); DLQ y reintentos con TTL nativos del broker; consumidores competidores; interfaz para operar y demostrar; equivale a SQS en AWS | Un componente más que operar; entrega "al menos una vez" exige consumidores idempotentes |
| Solo PostgreSQL como cola (outbox + `SKIP LOCKED`) | Una pieza menos | DLQ y reintentos hay que construirlos; no cumple de forma explícita el "cola" del compose; techo de rendimiento menor |
| Solo RabbitMQ (publicar después de guardar) | Simple | **No resuelve la doble escritura**: si el proceso muere entre guardar y publicar, el mensaje se pierde |
| SQS directo | Administrado | No corre en local sin emuladores; acopla el código a AWS |

## Decisión
Outbox en PostgreSQL + RabbitMQ 4:
- Cada cambio de estado que dispara trabajo asíncrono escribe el comando en `outbox` en la misma transacción.
- Un despachador publica con *publisher confirms* y marca `published_at` solo tras la confirmación.
- Un worker consume con ack manual y `prefetch` acotado. Reintentos con colas por nivel con TTL por mensaje (backoff base·2ⁿ con jitter completo) que regresan a la cola de trabajo. Agotados los intentos → DLQ con motivo + tabla `dead_letters` + orden en FAILED.
- Idempotencia **por estado**: el worker solo actúa si la orden sigue en el estado esperado, y el PSP recibe `Idempotency-Key = orderId`.
- La intención de pago pasa a asíncrona: `POST /orders` responde en QUOTED (decisión aprobada por producto).

## Consecuencias
- **Buenas:** ningún comando se pierde por una caída; el cobro huérfano se recupera con una reentrega; `POST /orders` responde en el tiempo de la cotización (antes, hasta ~25 s); la DLQ hace visible lo que falló.
- **Malas:** entrega "al menos una vez" (los consumidores deben ser idempotentes); el cliente ve el pago avanzar de forma asíncrona (consulta o SSE); hoy API, despachador y worker corren en el mismo proceso (en producción, despliegues separados que escalan por separado); una cola de reintento por nivel puede retener detrás de un mensaje con TTL largo a otro más corto del mismo nivel (acotado por el jitter del nivel).
- **En AWS:** RabbitMQ → SQS (cola + DLQ con `maxReceiveCount`) o Amazon MQ; la outbox no cambia.

## Criterios de reversión
- Si la operación de RabbitMQ cuesta más que su valor (equipo pequeño, sin experiencia), pasar a SQS administrado manteniendo la outbox.
- Si la latencia p95 entre `created_at` y `published_at` de la outbox supera 5 s sostenida, escalar despachadores (`SKIP LOCKED` lo permite) antes de cambiar de tecnología.
- Si el volumen de reintentos hace visible el bloqueo por TTL dentro de un nivel, pasar al plugin de mensajes retrasados o a SQS con `DelaySeconds`.
