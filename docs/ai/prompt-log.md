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
