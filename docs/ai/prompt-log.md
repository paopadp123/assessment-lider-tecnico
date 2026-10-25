# Bitácora de prompts refinados

Cada entrada guarda el prompt original tal como se escribió y la versión refinada que se ejecutó.

## 2026-10-23 22:02 — [HU-01] Orquestador: creación de órdenes idempotente y máquina de estados
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> vamos con la h1

**Refinado (ejecutado):**
```text
[HU-01] Orquestador: creación de órdenes idempotente y máquina de estados (issue #2)

Contexto:
- Repo paopadp123/assessment-lider-tecnico, rama feature/hu-01 desde dev.
- orchestrator/ ya tiene: go.mod (Go 1.27), cmd/orchestrator/main.go con net/http + slog JSON,
  internal/httpapi/health.go con su prueba. docker-compose con postgres:17. CI con gofmt, vet y test -race.
- ADR 0001: PostgreSQL, driver pgx.
- Contrato del cotizador (Anexo B): POST /v1/quotes {plate, vehicleYear, cityCode, documentId}
  → 200 {quoteId, premium, currency, validUntil}; falla con 500 en ~5% de los casos y tarda entre 100 ms y 3 s.
  El simulador real llega en HU-08: aquí se prueba contra un servidor falso con httptest.

Objetivo: POST /orders crea una orden de forma idempotente, la cotiza y la deja en QUOTED,
con una máquina de estados explícita y persistida que ya contemple todo el flujo.

Restricciones:
- Ruteo con net/http de la librería estándar (patrones "POST /orders" de Go 1.22+): sin chi ni frameworks.
- Persistencia con pgx/v5; migraciones SQL embebidas (embed) que se aplican al arrancar.
- Arquitectura hexagonal liviana (puertos y adaptadores): internal/order es el dominio sin E/S
  (estados, transiciones, validación, tipos Plate/DocumentID); internal/app son los casos de uso y define
  los puertos; internal/adapters/postgres, internal/adapters/quoter e internal/httpapi son adaptadores.
- Pruebas guiadas por riesgo: prueba primero (commit test → feat) en las reglas críticas; pruebas de tabla
  para la máquina de estados; integración contra Postgres para idempotencia y transacciones; sin pruebas
  para el cableado de main.
- Errores explícitos con contexto (fmt.Errorf con %w), sin panic fuera de main, context.Context en toda la E/S.
- Logs: slog JSON a la salida estándar con order_id; Plate y DocumentID se enmascaran solos al loguearse
  (con su prueba).

Criterios de aceptación (cada uno con su prueba, escrita primero en rojo):
1. Máquina de estados: tabla de transiciones válidas CREATED → QUOTED → PAYMENT_PENDING → PAID → ISSUING → ISSUED,
   más FAILED y REFUND_REQUIRED. Una prueba recorre TODAS las combinaciones y rechaza las inválidas.
2. Cada cambio de estado guarda una fila en order_events (de, a, motivo, fecha) en la misma transacción.
3. Idempotency-Key obligatoria (si falta → 400). Misma clave y mismo cuerpo → misma respuesta, sin crear otra orden.
   Misma clave con otro cuerpo → 422. Dos peticiones concurrentes con la misma clave → una sola orden
   (garantizado con un índice único, no con un if).
4. Validación de la entrada (placa, año, ciudad, documento) → 400 con un error claro.
5. Cotizador: timeout por intento y hasta 3 intentos ante 500 o timeout. Si se agotan:
   la orden pasa a FAILED con motivo QUOTE_UNAVAILABLE, se registra un log ERROR (order_id, intentos),
   se guarda el evento en order_events y se responde 201 con status FAILED y el motivo.
6. Pruebas de integración contra Postgres real (variable DATABASE_URL); el CI agrega un servicio postgres:17 al job de Go.

Fuera de alcance: intención de pago y webhook (HU-02), outbox y DLQ (HU-03), circuit breaker (HU-04),
consola, autenticación, cifrado en reposo (se argumenta en el Bloque 4).

Formato de salida:
1) Plan breve de archivos y tablas; espera mi OK antes de escribir código.
2) Código + pruebas, mostrando el rojo → verde.
3) Explicación en 3–4 líneas por paquete y los conceptos de Go usados, para GUIA_ESTUDIO.md.
4) Cierre con /cerrar-hu: QA, resumen y commits propuestos. Sin commit ni push.
```

**Qué se agregó y por qué:** el original no tenía contexto ni criterios verificables. Se agregaron el contrato del cotizador, los casos borde de idempotencia (otro cuerpo, concurrencia), la decisión ante la falla del cotizador (FAILED + log + evento), la arquitectura (hexagonal liviana) y la estrategia de pruebas (guiada por riesgo), que se decidieron con el usuario antes de ejecutar.
**Resultado:** HU-01 terminada con 5 paquetes de pruebas en verde (rojo real primero en dominio y casos de uso). Errores de la IA detectados: (1) una prueba revisaba el contexto del cotizador después de cancelarlo; la detectó la propia prueba al fallar contra código correcto. (2) Las pruebas de integración hacían TRUNCATE sobre la base indicada en DATABASE_URL, que en local apuntaba a la base de la aplicación; lo detectó el agente QA (Haiku), no el modelo que escribió el código. Se corrigió con TEST_DATABASE_URL y una protección que exige el sufijo _test. (3) La prueba de humo de punta a punta mostró que la respuesta repetida por idempotencia no era idéntica (prima y fechas); las pruebas unitarias no lo veían.

## 2026-10-24 00:10 — [HU-02] Orquestador: intención de pago y webhook del PSP
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> ya hice el merge, vamos con la hu-02

**Refinado (ejecutado):**
```text
[HU-02] Orquestador: intención de pago y webhook del PSP (issue #3)

Contexto:
- Rama feature/hu-02 desde dev (HU-01 mergeada). Arquitectura hexagonal liviana:
  internal/order (dominio), internal/app (casos de uso + puertos), adapters/postgres, adapters/quoter, httpapi.
- Hoy POST /orders termina en QUOTED o FAILED. Máquina de estados ya permite QUOTED → PAYMENT_PENDING → PAID | FAILED.
- Contrato PSP (Anexo B): POST /v1/payments {quoteId, amount, currency, customerRef} → 202 {paymentId, status:"PENDING"}.
  Webhook hacia nosotros: POST /webhooks/payments {paymentId, status:"APPROVED"|"REJECTED", occurredAt, signature}.
  El PSP reenvía duplicados y puede entregar fuera de orden; el simulador (HU-08) enviará 10% de duplicados.
- Caso exigido: APPROVED puede llegar antes de que hayamos guardado PAYMENT_PENDING.

Objetivo: después de cotizar, crear la intención de pago y dejar la orden en PAYMENT_PENDING; recibir el webhook
firmado y llevar la orden a PAID o FAILED exactamente una vez, sin importar duplicados ni el orden de llegada.

Restricciones:
- Mismo estilo de HU-01: errores con %w, context en toda la E/S, slog sin datos sensibles (nunca la firma ni el secreto),
  OrderID y tipos propios (PaymentID), pruebas guiadas por riesgo con rojo → verde en las reglas críticas.
- Firma: HMAC-SHA256 con secreto compartido (env PSP_WEBHOOK_SECRET) sobre "paymentId|status|occurredAt"
  (supuesto documentado). Comparación en tiempo constante (hmac.Equal).
- Nuevo adaptador internal/adapters/psp con la misma clasificación de errores que el cotizador. Se envía
  Idempotency-Key = orderId al PSP para que un reintento no cree dos pagos (ampliación permitida del contrato).
- Decisión del usuario: la intención de pago se crea de forma síncrona en POST /orders, justo después de cotizar.

Criterios de aceptación (cada uno con su prueba):
1. QUOTED → intención de pago → PAYMENT_PENDING con paymentId guardado; la respuesta incluye el pago.
   PSP caído tras reintentos → FAILED con PAYMENT_UNAVAILABLE (+ log ERROR + evento).
2. Firma inválida o ausente → 401 sin efecto; formato inválido → 400 sin efecto.
3. Duplicado (paymentId + status) → 200 sin segundo efecto (índice único en payment_events).
4. Fuera de orden: el webhook se guarda siempre; si la orden aún no está en PAYMENT_PENDING con ese paymentId,
   se aplica en la misma transacción que guarda PAYMENT_PENDING. Prueba de integración con ambas órdenes de
   llegada y otra concurrente → una sola transición.
5. APPROVED → PAID; REJECTED → FAILED con PAYMENT_REJECTED.
6. Evento contradictorio (REJECTED después de PAID): se guarda, log WARN, 200; la orden no cambia.
7. Error de base de datos → 500 para que el PSP reintente.

Fuera de alcance: outbox y DLQ (HU-03), replay por antigüedad de occurredAt, emisión (HU-04), reembolsos, consola.

Formato de salida: 1) plan breve de archivos y tablas y espera OK; 2) código + pruebas rojo → verde;
3) sección en GUIA_ESTUDIO.md; 4) QA + /cerrar-hu, commits propuestos, sin commit.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregaron el contrato del PSP, el caso exigido de APPROVED antes de PAYMENT_PENDING con su solución, la definición de qué se firma (el contrato no lo dice), la idempotencia hacia el PSP, el manejo de eventos contradictorios y la decisión del usuario (pago síncrono).
**Resultado:** HU-02 terminada, 7 paquetes de pruebas en verde con rojo real primero (dominio y casos de uso). La prueba de la carrera se validó a propósito: sin el advisory lock falla 3 de 3 veces. Errores de la IA: (1) el fake del repositorio volvió a ser complaciente (guardaba ante un error que el repositorio real revierte); lo detectó el QA. (2) Firma ausente respondía 400 en vez del 401 que pedía el propio prompt. (3) El QA aplicó la regla "un timeout no autoriza a reemitir" al PSP en vez de al emisor; se descartó con argumento. (4) Antigravity propuso cambios que contradicen el contrato (firma en header) y reportó como bug un caso que el diseño cubre. Ver docs/ai/revisiones-cruzadas.md.

## 2026-10-24 01:07 — [HU-03] Orquestador: outbox + RabbitMQ, reintentos y DLQ
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> si, vamos con outbox y rabbitmq pero enteonces toca cambiar el issue, ademas toca agregar conceptos de colas para estudas

**Refinado (ejecutado):**
```text
[HU-03] Orquestador: outbox + RabbitMQ, reintentos con backoff y jitter, y DLQ (issue #4)

Contexto:
- dev tiene HU-01 y HU-02. Hoy POST /orders llama al cotizador y al PSP dentro de la petición (síncrono).
- Hallazgo pendiente de HU-02 (QA y Antigravity): si el proceso muere entre el OK del PSP y el guardado de
  PAYMENT_PENDING, el cobro queda huérfano.
- Enunciado 4.2.A: reintentos con backoff exponencial y jitter, DLQ, outbox. 4.2.E: docker compose debe levantar "cache o cola".
- Decisión del usuario: outbox en PostgreSQL + RabbitMQ (ADR 0002).

Objetivo: toda llamada a un tercero que hoy depende de que el proceso no se caiga pasa a una tarea durable: se escribe
en la outbox en la misma transacción del cambio de estado, un despachador la publica en RabbitMQ y un worker la ejecuta
con reintentos; lo que agota los reintentos queda en una DLQ visible.

Alcance concreto:
- La intención de pago pasa a asíncrona (decisión del usuario): al quedar QUOTED se escribe "request_payment" en la
  outbox y POST /orders responde 201 con la orden en QUOTED. La cotización sigue síncrona.
  El worker pide el pago con Idempotency-Key = orderId; una reentrega tras una caída obtiene el mismo paymentId.
- La infraestructura queda lista para que HU-04 publique "issue_policy" al quedar PAID.

Restricciones:
- github.com/rabbitmq/amqp091-go; RabbitMQ 4 con interfaz de administración en el compose.
- Outbox con FOR UPDATE SKIP LOCKED en lotes, publisher confirms, published_at solo tras confirmación.
- Colas de reintento por intento con TTL de vuelta a la cola de trabajo (backoff base·2ⁿ con jitter); agotados → DLQ con motivo.
- Consumidor idempotente, ack manual tras guardar, prefetch acotado. Hexagonal; errores con %w; slog sin datos sensibles.

Criterios de aceptación (con prueba):
1. Cambio de estado y fila de outbox en la misma transacción.
2. Sin confirmación del broker la fila sigue pendiente y se reintenta.
3. Falla transitoria del PSP → reintento con backoff y jitter; éxito posterior → PAYMENT_PENDING.
4. Agotados los reintentos → DLQ con motivo e intentos; orden en FAILED PAYMENT_UNAVAILABLE.
5. Mensaje entregado dos veces → un solo efecto.
6. Recuperación del cobro huérfano por reentrega con la misma Idempotency-Key.
7. CI con servicio rabbitmq; integración obligatoria en CI.

Además: actualizar el issue #4; ADR 0002; conceptos de colas en GUIA_ESTUDIO.md con su equivalente en AWS.

Fuera de alcance: circuit breaker y emisión (HU-04), reconciliación (HU-05), correlation-id (HU-06), consola de la DLQ (HU-09).

Formato de salida: 1) plan y espera OK; 2) código + pruebas rojo → verde; 3) issue, ADR y guía; 4) QA + /cerrar-hu, sin commit.
```

**Qué se agregó y por qué:** el original solo confirmaba la decisión de RabbitMQ. Se agregaron el alcance concreto (solo el pago pasa a asíncrono, decisión aprobada por el usuario tras explicarle el impacto en el comprador), cómo se cierra el hallazgo del cobro huérfano de HU-02, la topología de reintentos y DLQ, las pruebas de integración en CI y la lista de conceptos de colas pedida para estudiar.
**Resultado:** HU-03 terminada: 7 paquetes en verde, integración real contra PostgreSQL y RabbitMQ, prueba de humo con recuperación (PAID tras 2 fallas del PSP) y DLQ (FAILED tras 4 intentos). Ajuste aprobado durante la ejecución: idempotencia por estado en lugar de tabla de mensajes procesados. Errores de la IA detectados por la propia IA antes de cualquier revisor: (1) una prueba de atomicidad de la outbox era un falso positivo (la falla ocurría antes del insert que debía probar); (2) la prueba de reintento no medía la espera (aserción `> 0` con jitter); (3) a esta misma entrada le había puesto una hora inventada (01:40) en vez de la real (01:07), corregida con el registro automático del hook; (4) el hook registraba los informes del agente QA como prompts del usuario. Las pruebas de concurrencia se validaron quitando el lock a propósito.

## 2026-10-24 09:23 — [HU-04] Emisión de la póliza con circuit breaker
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> arranca el 04 /prompt

**Ajuste del usuario al revisar el refinado:**
> toca estudiar que es el acl si el breaker abierto no cuenta como intento si que queden encoladas

**Refinado (ejecutado):**
```text
[HU-04] Emisión de la póliza con circuit breaker (issue #5)

Contexto:
- Rama feature/hu-04 desde dev (HU-01..03 mergeadas).
- Hoy la orden llega a PAID dentro de RecordPaymentEvent / MarkPaymentPending (repo, bajo lock por
  pago), con applyPayment. La outbox solo acepta 'request_payment' (CHECK en 003_outbox.sql), y el
  worker de RabbitMQ solo procesa ese comando.
- Enunciado: circuit breaker hacia el emisor; si está caído, la compra no se pierde (queda encolada y
  observable). El emisor real tiene errores en HTTP 200 y "timeout tras 8 s habiendo emitido";
  consulta obligatoria por externalRef = orderId antes de reintentar. Pago aprobado + emisión fallida
  en definitiva → REFUND_REQUIRED, sin reembolso automático.
- El orquestador habla con la ACL en C# (HU-07, aún no existe), no con el emisor legado. El contrato
  limpio que se define aquí es el que HU-07 debe implementar.

Objetivo: al quedar PAID se publica issue_policy por la outbox en la misma transacción, y un worker
emite a través de la ACL, protegido por un circuit breaker, hasta ISSUED o REFUND_REQUIRED, sin
emitir dos veces.

Restricciones:
- Go, misma arquitectura (order / app / adapters / httpapi). Breaker con sony/gobreaker (v2).
- Migración NUEVA 004: 'issue_policy' en el CHECK de la outbox + columna policy_number.
- Contrato hacia la ACL (puerto PolicyIssuer + adaptador HTTP):
    POST /policies {externalRef, ...} → 201/200 {policyNumber}; 422 = rechazo de negocio definitivo;
    5xx/timeout = transitorio. GET /policies?externalRef={orderId} → 200 {policyNumber} | 404.
- El breaker cuenta solo fallas transitorias, no rechazos de negocio. Placa y documento nunca en claro.
- Rojo → verde real. Explicar Go en 3–4 líneas y actualizar GUIA_ESTUDIO.md (incluida la ACL).

Criterios de aceptación:
1. PAID (por webhook o por eventos pendientes) ⇒ issue_policy en la outbox en la misma transacción.
2. Worker idempotente por estado: PAID → ISSUING y llama; ISSUING (reentrega) → GET por externalRef
   primero; ISSUED / REFUND_REQUIRED → sin efecto.
3. Éxito → ISSUED con policyNumber y evento en la línea de tiempo.
4. 422 → REFUND_REQUIRED con motivo, sin reintento.
5. Timeout / 5xx → reintento con backoff; el siguiente intento consulta antes de reemitir
   ("timeout pero sí emitió" ⇒ ISSUED con una sola emisión).
6. Breaker abierto → no llama a la ACL; la orden sigue en ISSUING, el mensaje se reprograma SIN
   consumir intentos (decisión del usuario: las órdenes quedan encoladas) y queda observable.
7. Reintentos transitorios agotados → DLQ y la orden queda en ISSUING (un timeout no prueba que no se
   emitió); la decide la reconciliación de HU-05.
8. Pruebas de dominio, caso de uso con fakes fieles al repo real, breaker, adaptador HTTP con
   httptest e integración con Postgres/RabbitMQ.

Fuera de alcance: ACL C# (HU-07), simulador (HU-08), reconciliación y acciones operativas (HU-05),
correlation-id y OpenAPI (HU-06), compose punta a punta (HU-10).

Formato de salida: plan corto; código + pruebas en verde; explicación de Go; ADR 0003; QA con
qa-reviewer. Sin Antigravity (revisión A al cerrar HU-05). Sin commit ni push hasta el OK.
```

**Qué se agregó y por qué:** el original era solo "arranca el 04". Se agregaron los requisitos del enunciado (consulta por externalRef antes de reintentar, errores en 200, REFUND_REQUIRED), el contrato limpio hacia la ACL (HU-07 aún no existe), y la regla de que los reintentos agotados no prueban una falla definitiva. El usuario resolvió la pregunta abierta: el breaker abierto no consume intentos.
**Resultado:** HU-04 terminada: 8 paquetes en verde con integración real (PostgreSQL y RabbitMQ) y el orquestador arrancando con las colas nuevas. La regla "un timeout no autoriza a reemitir" se validó quitando la consulta a propósito (3 pruebas fallaron). Errores de la IA: (1) mapeó cualquier 4xx de la ACL a rechazo de negocio, copiando el criterio del PSP: habría dejado órdenes pagadas en REFUND_REQUIRED por un error de configuración; lo detectó el agente QA (Haiku). (2) Omitió el criterio "estado del breaker expuesto" del issue #5; lo detectó la propia IA al releer el issue antes de cerrar. (3) Dos pruebas propias mal escritas (un servidor de prueba que se colgaba y una suposición falsa sobre gobreaker), detectadas al ejecutarlas.

## 2026-10-24 10:37 — [HU-05] Reconciliación, compensación, acciones operativas con login JWT y eventos
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> si h5 /prompt

**Ajustes del usuario al revisar el refinado:**
> a que te refieres con operador cual es en este caso?
> no quiero autenticacion, podemos utilizar en ccl?
> si con autenticacin, y si que falle definitiva a la 3 ronda
> (al confirmar con una pregunta cerrada eligió: "Login con JWT, como en CCL")

**Refinado (ejecutado):**
```text
[HU-05] Reconciliación, compensación, acciones operativas y eventos (issue #6)

Contexto:
- HU-01..04 mergeadas. Outbox + RabbitMQ con request_payment e issue_policy (idempotentes por estado);
  IssuePolicy consulta por externalRef antes de reemitir. HU-04 deja órdenes en ISSUING a propósito
  (DLQ o breaker abierto): esta HU las resuelve.
- Pendientes heredados: CREATED si falló el guardado tras cotizar (QA HU-01); QUOTED con el pago
  aceptado sin PAYMENT_PENDING (QA y Antigravity HU-02): se recupera reenviando request_payment con
  Idempotency-Key = orderId.
- El PSP no tiene endpoint de consulta: PAYMENT_PENDING no se puede verificar.
- Enunciado: job de órdenes atascadas > N min que consulta al emisor antes de reintentar; pago aprobado
  + emisión fallida en definitiva → REFUND_REQUIRED sin reembolso automático; acciones "reintentar
  emisión" y "marcar para reverso" con registro de quién; línea de tiempo y actualización sin recargar.
- Decisión del usuario: el operador se identifica con login JWT, como en la prueba CCL (ADR 0006 de CCL).

Objetivo: ninguna orden queda atascada en silencio; operaciones actúa autenticado y auditado; la API
expone la línea de tiempo y un flujo SSE.

Restricciones:
- Go, misma arquitectura. Migraciones nuevas (actor en order_events, operadores).
- Reconciliador seguro con varias réplicas: FOR UPDATE SKIP LOCKED en lotes. N configurable (5 min).
- Nunca emitir sin consultar por externalRef. Placa y documento nunca en claro en logs.
- Login: POST /auth/login con bcrypt, respuesta y tiempo iguales si el usuario no existe; JWT HS256
  corto (60 min) con sub y nombre; secreto por variable de entorno (≥ 32 bytes, validado al iniciar);
  límite de intentos por IP. Sin roles (un solo perfil: operador) y sin registro de usuarios: el
  primer operador se crea al iniciar desde variables de entorno.
- Rojo → verde real; Go explicado en 3–4 líneas; ADR 0004 (reconciliación y SSE) y ADR 0005 (JWT).

Criterios de aceptación (estado atascado > N min):
1. CREATED → se vuelve a cotizar: QUOTED + request_payment, o FAILED si el cotizador rechaza.
2. QUOTED → se reencola request_payment (mismo paymentId, sin doble cobro).
3. PAYMENT_PENDING → solo visible como atascada (WARN), sin cambiar estado.
4. PAID → se reencola issue_policy.
5. ISSUING → consulta por externalRef: existe → ISSUED; no existe → se reencola issue_policy; breaker
   abierto → nada en esta ronda. A la 3.ª ronda en que la consulta confirma que no existe →
   REFUND_REQUIRED con motivo ISSUE_FAILED_DEFINITIVELY (decisión del usuario).
6. Acciones (JWT obligatorio; 401 sin token o con token inválido/vencido):
   - POST /orders/{id}/actions/retry-issuance desde ISSUING o REFUND_REQUIRED (nueva transición
     REFUND_REQUIRED → ISSUING); reencola issue_policy, que consulta antes de emitir.
   - POST /orders/{id}/actions/mark-for-reversal desde PAID / ISSUING → REFUND_REQUIRED.
   - Estado inválido → 409. El operador (del token, no del cuerpo) y la acción quedan en la línea de tiempo.
7. GET /orders/{id}/events → línea de tiempo (estado, motivo, actor, fecha). Requiere JWT.
8. GET /events/stream (SSE, requiere JWT): cada cambio de estado; Last-Event-ID para reanudar sin
   perder eventos; funciona con varias réplicas (lee de order_events).
9. Pruebas: ramas del reconciliador con fakes fieles, SKIP LOCKED con dos reconciliadores (validado
   quitando el lock), login (credenciales malas, usuario inexistente, límite), middleware (401),
   acciones (409, auditoría) y SSE (reanudación).

Fuera de alcance: la consola y su pantalla de login (HU-09), listado/filtro para la consola (HU-09),
correlation-id y OpenAPI (HU-06), ACL y simulador (HU-07/08), roles, refresh tokens y gestión de usuarios.

Formato de salida: plan corto y OK; código + pruebas en verde; ADR 0004 y 0005; QA; al cerrar, el
prompt de la revisión A para Antigravity (HU-03 a HU-05). Sin commit ni push hasta el OK.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregó qué hacer con cada estado atascado (incluidos los pendientes de QA y Antigravity de HU-01 y HU-02), que PAYMENT_PENDING solo se puede hacer visible porque el PSP no tiene consulta, la transición REFUND_REQUIRED → ISSUING para "reintentar emisión", y el login JWT. El usuario resolvió las dos dudas: falla definitiva en la 3.ª ronda e identidad con JWT. Primero dijo "no quiero autenticación" y luego eligió JWT al confirmar: la IA preguntó con opciones cerradas en lugar de suponer.
**Resultado:** HU-05 terminada: 9 paquetes en verde con integración real y prueba de humo con el binario (login, 401 sin token, SSE en vivo, línea de tiempo con actor, 409 y el reconciliador recuperando una orden QUOTED vieja al arrancar). Validaciones por mutación: sin `SKIP LOCKED` dos reconciliadores tomaron la misma orden (3/3); sin el hash ficticio, el login reveló qué usuarios existen. Errores de la IA: (1) diseñó el SSE suponiendo que el orden de los IDs es el orden de los commits; un evento que confirma tarde se habría perdido. Lo detectó el agente QA (Haiku), cuya solución propuesta (`xmin`) también tenía un defecto, y se resolvió con espera ante huecos. (2) Dejó valores de ejemplo usables para el secreto JWT y la contraseña en `.env.example` (QA). (3) Primero propuso una cabecera sin autenticación; el usuario eligió JWT al confirmar con una pregunta cerrada.

## 2026-10-24 15:18 — [HU-06] Trazabilidad, salud y contrato OpenAPI
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> HU-06 con el /prompt

**Ajuste del usuario al revisar el refinado:**
> que es opon api? si correlation id unico por compra

**Refinado (ejecutado):**
```text
[HU-06] Trazabilidad, salud y contrato OpenAPI (issue #7)

Contexto:
- HU-01..05 mergeadas. Saltos asíncronos: POST /orders → outbox → RabbitMQ → worker → PSP / ACL;
  webhook del PSP; reconciliador y acciones de operadores.
- Placa y documento ya se enmascaran por tipo (order/identity.go). No hay correlation-id.
- /health solo indica que el proceso vive. No hay OpenAPI; la consola (HU-09) generará sus tipos
  TypeScript desde él.
- Enunciado: correlation-id propagado en todos los saltos y en los logs; ningún dato sensible en claro.

Objetivo: seguir cualquier compra de punta a punta con un solo id, informar si el orquestador
está listo para recibir tráfico y publicar su contrato HTTP verificado contra las respuestas reales.

Restricciones:
- Go, misma arquitectura; migración nueva si hace falta.
- Handler de slog que agrega correlation_id desde el contexto.
- OpenAPI 3.0 escrito a mano (orchestrator/api/openapi.yaml), servido en GET /openapi.yaml y
  validado en pruebas con kin-openapi contra las respuestas reales.
- Rojo → verde; Go explicado en 3–4 líneas; guía de estudio.

Criterios de aceptación:
1. Correlation-id ÚNICO POR COMPRA (decisión del usuario): POST /orders acepta X-Correlation-ID
   validado o genera uno y lo devuelve; se guarda en la orden y lo heredan outbox → cabecera AMQP →
   worker → X-Correlation-ID hacia cotizador, PSP y ACL, el webhook y el reconciliador. Acciones de
   operadores y login llevan uno propio. Pruebas en httptest, integración y logs.
2. Barrido de datos sensibles sobre el flujo completo con logger en memoria: falla si aparece
   placa, documento, token o contraseña en claro.
3. GET /health/live y GET /health/ready (PostgreSQL y RabbitMQ; 503 con el componente que falla);
   /health como alias de live.
4. OpenAPI con todas las rutas, esquemas, errores y seguridad bearer; prueba que falla si una ruta
   registrada no está en el contrato.

Fuera de alcance: OpenTelemetry, métricas, consola y generación de tipos (HU-09), ACL (HU-07),
simulador (HU-08).

Formato de salida: plan corto y OK; código + pruebas en verde (también Linux con -race); QA;
commits propuestos. Sin commit ni push hasta el OK.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregó cómo sobrevive el correlation-id a los saltos asíncronos (una cabecera HTTP se pierde en la base y en la cola), una prueba de barrido de datos sensibles sobre el flujo completo y un OpenAPI verificado contra las respuestas reales para que la consola no reciba tipos falsos. El usuario eligió un id único por compra.
**Resultado:** HU-06 terminada: 10 paquetes en verde (también en Linux con -race) y prueba de humo de punta a punta: el PSP recibió el correlation-id del cliente desde el worker, después de la outbox y RabbitMQ. El contrato OpenAPI se valida contra las respuestas reales; una mutación (renombrar `policyNumber`) hizo fallar la prueba. Errores de la IA: (1) dejó los logs de error de los handlers sin el correlation-id (QA); (2) escribió en el contrato un ejemplo de placa enmascarada que no coincide con el código; (3) el primer esquema aceptaba campos extra, así que un campo renombrado no se habría detectado: lo vio la propia IA al diseñar la mutación.

## 2026-10-24 18:46 — [HU-08] Simulador de terceros con caos configurable
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> 08 /prompt

**Refinado (ejecutado):**
```text
[HU-08] Simulador de terceros con caos configurable (issue #9)

Contexto:
- El orquestador Go (HU-01..06, en dev) ya consume a los terceros: cotizador POST /v1/quotes
  {plate, vehicleYear, cityCode, documentId} → 200 {quoteId, premium, currency, validUntil}; PSP
  POST /v1/payments {quoteId, amount, currency, customerRef} con Idempotency-Key = orderId → 202
  {paymentId, status:"PENDING"}; webhook hacia el orquestador POST /webhooks/payments {paymentId,
  status, occurredAt, signature}, HMAC-SHA256 hex sobre "paymentId|status|occurredAt" con
  PSP_WEBHOOK_SECRET (≥32 bytes). Todos reciben X-Correlation-ID.
- El emisor legado lo consume la ACL C# (HU-07, siguiente): este simulador DEFINE el contrato feo que
  la ACL envolverá (POST /issue y GET /policies?externalRef={orderId}, Anexo B).
- Plan: Node + TypeScript (Fastify), probabilidades por variables de entorno. Carpeta /simulator vacía.

Objetivo: un servicio que simule a los 3 terceros con el caos del enunciado, reproducible en pruebas y
controlable durante la demo del video.

Restricciones: Node 24, TS strict sin any, Fastify, Vitest, ESLint, pocas dependencias; azar inyectable
con semilla; un puerto con rutas por tercero; logs JSON sin placa ni documento completos; configuración
por variables de entorno con los valores por defecto del enunciado.

Criterios de aceptación:
1. Cotizador: 200 con el contrato del Anexo B; 500 en ~5%; latencia 100 ms–3 s; 400 si falta un campo.
2. PSP: 202 PENDING; misma Idempotency-Key → mismo paymentId sin segundo webhook; webhook firmado tras
   un retardo, APPROVED/REJECTED en proporción configurable; 10% duplicados; entrega fuera de orden;
   reintento del webhook si el orquestador no responde 2xx (con límite).
3. Emisor legado: POST /issue verboso, redundante y anidado; 200 con póliza o 200 con {result:"ERROR",
   code, message} (negocio y técnico); latencia variable; "timeout pero sí emitió" (emite y responde a
   los 8 s); idempotente por externalRef; GET /policies?externalRef= documentado.
4. 5% de respuestas con formato inválido, configurable por tercero (decisión del usuario: los 3).
5. Ventana de caída total del emisor por variables de entorno y endpoint de control POST /chaos para
   ver y cambiar porcentajes y caída en vivo (decisión del usuario).
6. Pruebas con semilla fija: porcentajes en N solicitudes, idempotencia de PSP y emisor, firma con
   vector compartido con el Go, duplicado y desorden, timeout-pero-emitió + GET, ventana de caída.
7. Dockerfile multi-stage y job de CI (lint + test).
8. README del simulador: variables, contrato del emisor, cómo activar cada falla.

Fuera de alcance: docker-compose y escenarios E2E (HU-10), ACL C# (HU-07), persistencia (estado en
memoria, documentado).

Formato de salida: rojo→verde real (stubs en test:), QA con qa-reviewer, guía de estudio, commits con
Refs #9 y PR abierto por Claude.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregaron los contratos que el orquestador ya consume (para que el simulador no los contradiga), la firma exacta del webhook, el orden 08 antes de 07 (el simulador define el contrato del emisor que la ACL envuelve), azar con semilla para que las pruebas de porcentajes no sean intermitentes y el endpoint /chaos para que la demo del video no dependa del reloj.
**Resultado:** HU-08 terminada: 47 pruebas con semilla fija (rojo real primero: 36 de 44 en rojo con stubs) y prueba de humo con el orquestador real: 6 compras pagadas que absorbieron un webhook duplicado y una cotización inválida. Errores de la IA: (1) no previó que Fastify borra en silencio los campos desconocidos (`/chaos` habría aceptado un error de tipeo); lo detectó su propia prueba. (2) Dejó el build (`dist/`) sin ignorar (QA, bloqueante). (3) El webhook temprano podía retener el 202 hasta 5 s (QA).

## 2026-10-24 20:45 — [HU-07] Capa anticorrupción de emisión en C#
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> ya hice el merge, HU-07 /prompt

**Refinado (ejecutado):**
```text
[HU-07] Capa anticorrupción de emisión en C# (issue #8)

Contexto:
- El orquestador Go ya consume el contrato limpio (orchestrator/internal/adapters/issuer/client.go):
  POST /policies {externalRef, quoteId, premium, currency, plate, documentId, vehicleYear, cityCode}
  → 201 {policyNumber} si la emite, 200 si ya existía, 422 {code, message} si es un rechazo de negocio
  definitivo (lleva la orden a REFUND_REQUIRED). GET /policies?externalRef={orderId} → 200 {policyNumber}
  | 404. Cualquier otro status, error de red o cuerpo inválido es transitorio: el reintento, el circuit
  breaker y la reconciliación ya están en Go, así que la ACL hace un solo intento (sin Polly). El
  orquestador envía X-Correlation-ID y espera hasta 10 s por intento.
- El emisor legado lo define el simulador (HU-08, simulator/README.md): POST /issue verboso con campos
  redundantes; 200 {result:"OK", data:{policy}}; 200 {result:"ERROR", code} (E-100 inconsistente, E-102
  póliza vigente, E-104 placa inválida, E-901 técnico); 200 con JSON cortado (sí emitió); respuesta a los
  8 s (sí emitió); 503 HTML caído. GET /policies?externalRef= → {result:"OK", policies:[…]}.
- issuing-service/ es un esqueleto .NET 10 minimal API con /health, xUnit + WebApplicationFactory,
  analizadores con warnings como errores y job de CI.
- Enunciado: traduce, valida y normaliza errores; los errores de negocio ocultos en 200 se vuelven
  errores explícitos del dominio; idempotente por referencia externa; pruebas unitarias del mapeo y la
  validación; "300 líneas impecables".

Objetivo: un servicio pequeño y pulido que traduzca el contrato feo del emisor al contrato limpio que el
Go ya usa, sin que un timeout o una respuesta ilegible produzcan una póliza duplicada ni un rechazo falso.

Restricciones: .NET 10 minimal API, C# sin var, records, nullable estricto; resultado explícito del
dominio (éxito / rechazo / falla transitoria) en vez de excepciones para el flujo normal; HttpClient
tipado con timeout configurable; sin base de datos; logs estructurados con correlation-id y sin placa ni
documento completos; configuración por variables de entorno.

Criterios de aceptación:
1. Validación de entrada: 400 con detalle por campo (externalRef, prima > 0, moneda ISO de 3 letras,
   placa, documento, año, ciudad); el emisor no se llama.
2. Mapeo limpio → feo con campos redundantes coherentes, transactionId único y requestDate
   yyyyMMddHHmmss. Supuestos documentados: documento "CC", producto "SOAT".
3. Idempotencia por externalRef: consulta GET /policies antes de emitir; si existe → 200 con la misma
   póliza sin llamar a /issue; si no → POST /issue → 201.
4. E-102 y E-104 → 422 con códigos limpios del dominio (sin repetir el mensaje del legado); E-901 → 503;
   E-100 → 502 (decisión del usuario: es un error de mapeo de la ACL, no un rechazo del cliente, y no debe
   llevar una orden pagada a REFUND_REQUIRED); código desconocido → 502.
5. Timeout del emisor (~6 s) o JSON ilegible: la ACL consulta GET /policies antes de responder (decisión
   del usuario); encontrada → 201; si no → 503. Todo dentro de los 10 s del intento del orquestador.
6. Emisor caído (503 o sin conexión) → 503. GET /policies?externalRef= → 200 | 404 | 503.
7. xUnit: unitarias del mapeo y de cada regla de validación; endpoints con WebApplicationFactory y un
   HttpMessageHandler falso (idempotencia, timeout con consulta, JSON roto, caída); logs sin datos
   sensibles.
8. Dockerfile multi-stage; prueba de humo real orquestador + simulador + ACL hasta ISSUED.

Fuera de alcance: reintentos o breaker en la ACL, docker-compose y escenarios E2E (HU-10), OpenAPI de la
ACL.

Formato de salida: rojo→verde real (stubs en test:), QA con qa-reviewer, guía de estudio, commits con
Refs #8 y PR abierto por Claude; al cerrar, el prompt de la revisión B de Antigravity.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregaron los dos contratos que la ACL une (el limpio que el Go ya consume y el feo que definió el simulador), qué status tienen consecuencias de negocio (solo el 422 lleva a REFUND_REQUIRED) y el presupuesto de tiempo del intento. El usuario decidió que E-100 sea 502 y que la ACL consulte la póliza antes de responder tras un timeout o un JSON ilegible.
**Resultado:** HU-07 terminada: 71 pruebas xUnit (rojo real primero: 53 de 60 en rojo con stubs) y prueba de humo con los 3 servicios. Con "timeout pero sí emitió" forzado al 100%, la ACL consultó y la compra quedó en ISSUED con una sola póliza. Errores de la IA, encontrados por el QA: (1) devolvía 422 ante `E-102` sin consultar, cuando la póliza vigente podía ser la propia y la orden pagada habría ido a reembolso; (2) usó `$` en las expresiones regulares de .NET, que acepta un salto de línea final. También se corrigieron, por el QA, la prima redundante con más de 2 decimales y el presupuesto de tiempos sin validar.

## 2026-10-24 23:08 — [HU-10] Escenarios end to end y docker compose completo
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> hu-10 /prompt

**Refinado (ejecutado):**
```text
[HU-10] Escenarios end to end y docker compose completo (Refs #11)

Contexto:
- docker-compose.yml solo levanta postgres (5433) y rabbitmq (5672/15672). Faltan los servicios.
- Orquestador Go: exige DATABASE_URL, QUOTER_URL, PSP_URL, PSP_WEBHOOK_SECRET, RABBITMQ_URL, ISSUER_URL,
  JWT_SECRET; opcionales STUCK_AFTER, RECONCILE_EVERY, PORT, OPS_BOOTSTRAP_*. No tiene Dockerfile.
- ACL C# (issuing-service): ya tiene Dockerfile; necesita LegacyIssuer__BaseUrl. PR #30 ya está en dev.
- Simulador: ya tiene Dockerfile, puerto 8090, caos por variables de entorno y en vivo con GET/POST /chaos.
  PSP_WEBHOOK_SECRET compartido con el orquestador y PSP_WEBHOOK_URL apuntando a él.
- Reglas ya implementadas: consulta por externalRef antes de reintentar; breaker abierto → ISSUING
  encolada; 422 del emisor o 3.ª ronda sin póliza → REFUND_REQUIRED.
- La consola (HU-09) no existe todavía; decisión del usuario: HU-10 primero y HU-09 después.

Objetivo: que `docker compose up` levante todo el backend y haya 4 escenarios reproducibles que sirvan de
guion para la demo del video.

Restricciones: Dockerfile multi-stage del orquestador (imagen mínima, usuario no root); compose con
healthchecks y depends_on service_healthy; secretos por .env (.env.example versionado), nunca en el YAML;
un solo PSP_WEBHOOK_SECRET; STUCK_AFTER/RECONCILE_EVERY cortos y SIM_SEED fijo; scripts en bash con curl
+ jq (decisión del usuario) y el control /chaos; solo endpoints que existen; cambios en Go explicados en
3–4 líneas.

Criterios de aceptación:
1. `docker compose up --build` desde cero deja todo healthy; /health y /ready del orquestador en 200.
2. Compra feliz hasta ISSUED con póliza; repetir el POST con la misma Idempotency-Key devuelve la misma orden.
3. Timeout pero emitió (timeoutButIssuedRate=1): ISSUED con una sola póliza para su externalRef.
4. Caída del emisor: breaker abierto y orden en ISSUING visible; al levantarlo, la reconciliación la lleva
   a ISSUED sin duplicar.
5. Pago aprobado + emisión fallida (businessErrorRate=1): REFUND_REQUIRED con su motivo en los eventos.
6. Cada escenario restaura el caos y muestra el estado final y la línea de tiempo de la orden.
7. Guion de la demo en docs/ (incluida la UI de RabbitMQ: cola, reintentos, DLQ).
8. CI en verde (build de la imagen del orquestador si encaja con el pipeline).

Fuera de alcance: consola y su servicio en el compose (HU-09), Terraform/AWS (HU-15), cambios de reglas
de negocio (si un escenario revela un bug, se reporta y se arregla aparte, con su prueba).

Formato de salida: Dockerfile, compose, .env.example, scripts, guion, QA con qa-reviewer, guía de
estudio, commits con Refs #11 y, al cerrar, PR dev → main del hito 1 y guía práctica de estudio manual.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se agregaron las variables que exige cada servicio, el control /chaos para forzar cada escenario y qué reglas ya existen para no reimplementarlas. El usuario decidió hacer HU-10 antes que HU-09 (la consola queda fuera del compose por ahora) y escribir los escenarios en bash con jq.
**Resultado:** compose completo (5 servicios healthy desde un volumen limpio) y 4 escenarios en verde, dos corridas seguidas (~1 min cada una), más el job `e2e` en el CI. Errores de la IA: (1) asignó una variable dentro de `$(...)` esperando leerla afuera (subshell; corregido antes de correr); (2) su primera verificación de "una sola póliza" contaba las pólizas del simulador, que guarda una por referencia: habría pasado aun con doble emisión. Se cambió a los logs de `/issue`. El QA encontró 2 bloqueantes: `.env.example` con CRLF en un clon de Windows rompía `dev-env.sh`, y la auditoría del operador se anunciaba sin verificarse. También encontró conteos de logs que daban 0 si `docker compose logs` fallaba (falso positivo), el límite de 5 logins por IP con varias corridas y la orden completa impresa en los errores. Todo quedó corregido.

## 2026-10-24 23:55 — [HU-09] Consola de operaciones en Next.js + TypeScript
**Herramienta:** Claude Code (skill /prompt)

**Original (tal como se escribió):**
> hu-09 /prompt

**Refinado (ejecutado):**
```text
[HU-09] Consola de operaciones en Next.js + TypeScript (Refs #10, cierra #11)

Contexto:
- console/ está vacía. El OpenAPI del orquestador es la fuente de los tipos: login JWT (60 min), GET de
  la orden, línea de tiempo, acciones retry-issuance / mark-for-reversal (auditadas, 409 si no aplican),
  SSE /events/stream con Last-Event-ID, GET /ops/breakers.
- Auth solo por "Authorization: Bearer"; no hay CORS; EventSource no envía cabeceras (ADR 0004).
- No existe un endpoint de listado; el orquestador no guarda las llamadas a terceros, solo el resultado.
- Los escenarios de scripts/e2e/ producen órdenes en ISSUED, ISSUING y REFUND_REQUIRED.
- Plan: sin dark mode ni dashboard.

Objetivo: un operador inicia sesión, encuentra órdenes, ve su línea de tiempo y las llamadas a terceros,
ejecuta las acciones con confirmación y ve los cambios en vivo sin recargar.

Restricciones:
- Next.js (App Router) + TS strict sin any; tipos con openapi-typescript desde openapi.yaml, con un script
  para regenerarlos y una verificación en CI.
- BFF (decisión del usuario): el navegador solo habla con Next. El JWT va en una cookie httpOnly, Secure
  y SameSite=Strict. Los route handlers reenvían al orquestador con el Bearer y el X-Correlation-ID; el
  SSE también, con Last-Event-ID. Sin CORS en Go.
- Go: GET /orders (filtro por estado repetible, búsqueda q, paginación por cursor, JWT) y registro de las
  llamadas a terceros (decisión del usuario: tabla por intento con tercero, operación, resultado, duración
  y detalle sin datos sensibles) con GET /orders/{id}/calls. Todo en el OpenAPI y con pruebas; explicar
  el Go nuevo en 3–4 líneas.
- Estilos simples, accesible; español y formato es-CO (sin i18n).

Criterios de aceptación:
1. Login: credenciales malas → mensaje claro; 429 → "espera"; 401 en cualquier llamada → al login;
   logout borra la cookie.
2. Lista: filtro por estado, búsqueda y paginación; estado vacío y errores visibles; nunca el documento.
3. Detalle: cotización, pago, póliza, motivo de falla, línea de tiempo (from → to, motivo, actor, hora)
   y llamadas a terceros.
4. Acciones solo cuando aplican, con diálogo de confirmación; 409 → mensaje y recarga; actor en la línea.
5. SSE: lista y detalle en vivo, reanudación con Last-Event-ID e indicador de conexión; justificación en
   el README.
6. Estado del breaker del emisor visible.
7. Vitest para la lógica (reducer SSE, reglas de acciones, BFF); ESLint + typecheck + tests + build en CI.
8. Servicio console en el compose (Dockerfile multi-stage, sin root, healthcheck) y en el guion de la
   demo; verificado en el navegador con los escenarios E2E, con capturas.

Fuera de alcance: dark mode, dashboards, gestión de operadores, i18n, E2E de navegador en CI.

Formato de salida: rojo→verde (Go y lógica TS), QA con qa-reviewer, guía de estudio, commits con Refs #10
/ Closes #11 sin ejecutar. Al cerrar: PR dev → main (hito 1) y guía práctica de estudio manual.
```

**Qué se agregó y por qué:** el original solo nombraba la HU. Se detectó que faltaban dos cosas en el backend para cumplir el issue: un endpoint de listado y el registro de las respuestas de terceros. También se planteó cómo autenticar el SSE, porque el navegador no puede mandar la cabecera. El usuario eligió guardar cada intento con un tercero (opción B) y el patrón BFF con cookie httpOnly (opción A). i18n quedó fuera de forma explícita.
**Resultado:** backend con `GET /orders` (keyset) y registro de llamadas a terceros (decoradores, por fuera del breaker), consola Next.js con BFF y cookie httpOnly, 6 servicios en compose y los 4 escenarios en verde. Rojo real primero en Go y en la lógica de TS. Verificado en Chrome: la orden pasó sola de "Emitiendo" a "Emitida" con su historia "breaker abierto → no disponible → no existe → OK". Errores de la IA: (1) el refresco de la lista podía mostrar resultados del filtro anterior; (2) la conexión en vivo no se recuperaba si el orquestador se caía (los dos los encontró el QA); (3) el BFF tardaba 15 s en abrir el SSE porque Next no envía cabeceras hasta el primer byte. Lo encontré midiendo, porque las pruebas pasaban. También se corrigieron: logout sin protección CSRF, `JSON.parse` sin validar y una prueba que cambió de 400 a 404 por consistencia con la API existente.
