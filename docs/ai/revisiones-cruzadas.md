# Revisiones cruzadas de IA

Cada HU la escribe Claude Code (Opus) y la revisan dos IA distintas antes del merge: el agente QA del repositorio (Claude Haiku, solo lectura) y Antigravity (Google). La decisión final sobre cada hallazgo es humana.

## HU-01 — Orquestador: creación idempotente y máquina de estados

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| Las pruebas de integración hacían `TRUNCATE` sobre la base indicada; en local apuntaba a la base de la aplicación | QA (Haiku) | Válido, grave | `TEST_DATABASE_URL` + la prueba se niega si la base no termina en `_test` |
| Falta prueba HTTP del 201 con `status: FAILED` | QA (Haiku) | Válido | Prueba agregada |
| 429 tratado como rechazo definitivo | QA (Haiku) | Válido | 429 se reintenta |
| `t.Fatal` llamado desde una goroutine | QA (Haiku) | Válido | Datos preparados fuera de la goroutine |
| Orden atascada en CREATED si falla el guardado después de cotizar | QA y Antigravity | Válido | Diferido a HU-05 (reconciliación) |
| `%#v` imprime placa y documento en claro (ignora `String()`) | Antigravity | Válido, grave | `GoString()` y `MarshalJSON()` enmascarados, con prueba |
| Prueba de cancelación del cliente: falso positivo, el fake ignoraba el contexto y Postgres real no | Antigravity | Válido | El fake respeta el contexto; la cancelación se simula después de crear la orden |
| Se aceptaban cotizaciones vencidas | Antigravity | Válido (regla de negocio) | `validUntil` en el pasado = respuesta inválida |
| `context.WithoutCancel` sin límite total | Antigravity | Válido | Presupuesto de 30 s para cotizar y guardar |
| Advisory lock de sesión en una conexión que vuelve al pool | Antigravity | Válido, severidad baja | La conexión de migración se saca del pool y se cierra |
| Falta `ReadTimeout` (slowloris) | Antigravity | Válido | `ReadTimeout` de 10 s |
| Cuerpo de respuesta no drenado | Antigravity | Válido, **severidad exagerada** ("agotamiento de sockets") | Se drena antes de cerrar |
| `Idempotency-Key` acepta espacios y caracteres de control | Antigravity | Válido, severidad baja | Solo ASCII visible |
| Pruebas HTTP que solo revisan el código de estado | Antigravity | Parcial (los cuerpos se probaban en otras pruebas) | Se agregó la revisión del cuerpo y del contrato |
| `t.Skip` local de las pruebas de integración da "falsa sensación" | Antigravity | **Descartado**: decisión deliberada; en CI fallan si falta la base | Sin cambio |
| `TestTerminalStatuses` es redundante | Antigravity | **Descartado**: prueba comportamiento, no implementación | Sin cambio |

**Lectura.** Ninguna IA encontró todo. El autor (Opus) no vio sus propios errores; el revisor pequeño (Haiku) encontró el más peligroso para el entorno (borrar datos); Antigravity encontró los más sutiles (fuga de PII por `%#v`, falso positivo de un fake). Antigravity también sobredimensionó severidades y propuso dos cambios que no aportan. Por eso la política del equipo exige revisión humana de cada hallazgo, no aplicarlos en bloque.
