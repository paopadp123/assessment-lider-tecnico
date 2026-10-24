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

## HU-02 — Intención de pago y webhook del PSP

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| Reintentar el PSP tras un timeout "viola la regla de no reemitir sin consultar" | QA (Haiku) | **Descartado: regla aplicada al tercero equivocado.** La regla del enunciado es sobre el emisor de pólizas. El doble cobro ya se evita con `Idempotency-Key = orderId` | Se documenta el supuesto (el PSP respeta la clave) y pasa como criterio del simulador (HU-08) |
| Firma ausente respondía 400; el criterio pide 401 | QA (Haiku) | Válido | Firma ausente o inválida → 401 |
| El fake guardaba el estado aunque la decisión de dominio fallara; el repositorio real hace rollback | QA (Haiku) | Válido (mismo tipo de falso positivo que en HU-01) | El fake propaga el error sin cambiar la orden |
| Orden en QUOTED si el PSP aceptó pero falló el guardado de PAYMENT_PENDING | QA (Haiku) | Válido | Diferido a HU-05 (reconciliación) |
| Webhook con error de base de datos probado solo con fake | QA (Haiku) | Aceptado como está | Sin cambio |

**Verificación propia (no la pidió ningún revisor):** se desactivó el advisory lock a propósito y la prueba de integración de la carrera falló 3 de 3 veces; con el lock pasa. Así se confirma que la prueba detecta el bug y no es un falso positivo.

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| `paymentId` admitía `\|`, el separador del texto firmado | Antigravity | Válido como defensa en profundidad (no se encontró forma de explotarlo hoy) | `paymentId` restringido a `[A-Za-z0-9._:-]`, con prueba |
| Cobro huérfano si el proceso muere entre el OK del PSP y el guardado | Antigravity y QA | Válido | HU-05: recuperar volviendo a pedir el pago con la misma `Idempotency-Key` |
| Repetir con la misma clave devuelve QUOTED sin pedir el pago | Antigravity | Válido (mismo caso) | HU-05 |
| Sin ventana de tiempo contra replay | Antigravity | Diferido a propósito: el replay exacto ya se descarta por índice único y uno distinto termina en ANOMALY; una ventana corta rechazaría reintentos legítimos del PSP | Sin cambio; documentado |
| Firma en el cuerpo JSON en vez de un header, y sobre 3 campos en vez del payload crudo | Antigravity | **Descartado: lo impone el contrato del Anexo B.** Cuerpo limitado a 16 KB | Recomendación para el README: en producción, header con firma sobre el cuerpo crudo |
| Colisión de hash en el advisory lock | Antigravity | Descartado: no afecta la corrección, solo serializa de más | Sin cambio |
| Duplicado de un evento sin aplicar "se descarta" | Antigravity | **Descartado: incorrecto.** Todo evento sin aplicar se aplica al guardar PAYMENT_PENDING bajo el mismo lock; la prueba de carrera verifica 0 sin aplicar | Sin cambio |
| Pruebas HTTP de duplicado y anomalía "tautológicas" | Antigravity | Descartado: prueban el contrato HTTP (200 para que el PSP no reenvíe); las reglas se prueban en el repositorio contra Postgres | Sin cambio |
| `t.Skip` local y `TestParsePaymentStatus` "trivial" | Antigravity | Descartado (repetido de HU-01; la segunda prueba cubre la frontera con un sistema externo) | Sin cambio |

**Lectura.** En HU-02 Antigravity aportó 1 cambio de 12 y repitió patrones de HU-01. Propuso endurecimientos genéricos de webhooks sin considerar lo que impone el contrato, y un "bug" que el diseño ya cubre. Los dos hallazgos válidos más importantes (el cobro huérfano) coinciden con el QA: cuando dos revisores independientes coinciden, la señal es fuerte.
