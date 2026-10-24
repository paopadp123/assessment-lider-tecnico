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

## HU-03 — Outbox + RabbitMQ, reintentos y DLQ

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| La espera de confirmación del broker no tenía tope: un broker colgado bloquearía el despachador con los locks de la outbox tomados | QA (Haiku) | Válido, importante | Timeout de 5 s por publicación; sin confirmación, el mensaje queda pendiente |
| Errores de `Nack` ignorados | QA (Haiku) | Válido | Se loguean |
| El fake no hacía rollback si fallaba la decisión de dominio (el repositorio real sí) | QA (Haiku) | Válido (tercer fake complaciente del proyecto) | El fake simula rollback, con prueba nueva |
| El fake permitía transicionar una orden inexistente | QA (Haiku) | Válido | Devuelve `ErrInvalidTransition`, como el UPDATE real |
| Con jitter = 0 el mensaje de reintento quedaba sin TTL | QA (Haiku) | Válido, severidad baja | TTL mínimo de 1 ms |
| Si el canal del publicador se cierra, el despachador seguía intentando sin terminar | QA (Haiku) | Válido | `main` vigila el cierre y termina el proceso para que se reinicie |
| Si falla la copia a la DLQ, la reentrega hace ack y la copia del broker se pierde | QA (Haiku) | Válido, impacto bajo: la fila en `dead_letters` (fuente para operaciones) sí queda | Documentado en el código |
| Timeout del PSP "reemite sin consultar" | QA (Haiku) | **Descartado (repetido de HU-02):** la regla es del emisor; el PSP se protege con `Idempotency-Key` | Sin cambio |
| Errores de infraestructura reintentan sin tope | QA (Haiku) | Decisión deliberada: un error de base de datos no debe mandar el mensaje a la DLQ | Sin cambio; se observa por el nivel de reintento y los logs WARN |
| Bloqueo de cabeza de línea por TTL dentro de un nivel de reintento | QA (Haiku) | Conocido y documentado en el ADR 0002 | Sin cambio |

**Verificaciones propias antes de cualquier revisor:**
- Se quitó `FOR UPDATE SKIP LOCKED` a propósito: la prueba de despachadores concurrentes detectó un mensaje publicado dos veces.
- Se detectó y corrigió un falso positivo propio (la prueba de atomicidad de la outbox fallaba antes del insert que debía probar) y una aserción débil (reintento sin medir la espera).

**Lectura.** El QA repitió por segunda vez la confusión entre la regla del emisor y el PSP: una regla correcta aplicada al tercero equivocado. Y por tercera HU seguida apareció un fake más permisivo que el sistema real. Lección para la política de IA del equipo: los dobles de prueba escritos por la IA deben revisarse contra el comportamiento del sistema real, no solo contra la interfaz.

_Antigravity: diferida a la revisión A (diff acumulado de HU-03 a HU-05). Desde HU-03, la IA externa se usa por riesgo y por bloque, no en cada HU, porque sus tokens son limitados._

## HU-04 — Emisión de la póliza con circuit breaker

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| El cliente de la ACL trataba cualquier 4xx como rechazo de negocio: un 400/401/404 por configuración o por un bug propio dejaba órdenes **pagadas** en REFUND_REQUIRED | QA (Haiku) | **Válido, bloqueante.** Error de Claude: copió el criterio del cliente del PSP sin pensar en la consecuencia distinta (en el PSP no hay dinero cobrado todavía) | Solo 422 es rechazo; el resto de 4xx es transitorio (reintento → DLQ, orden en ISSUING). La prueba que fijaba el comportamiento equivocado se corrigió primero (rojo) |
| Un comando de emisión para una orden no pagada terminaba en silencio (log INFO) | QA (Haiku) | Válido, no bloqueante | WARN, con prueba |
| Dos entregas simultáneas en ISSUING pueden llamar dos veces a la ACL | QA (Haiku) | Conocido y documentado en el ADR 0003 | Lo cierra la idempotencia por `externalRef` de la ACL (HU-07) |
| Breaker en memoria por réplica | QA (Haiku) | Conocido y documentado en el ADR 0003 | Sin cambio |

**Verificaciones propias antes de cualquier revisor:**
- Se quitó a propósito la consulta por `externalRef` antes de reemitir: fallaron 3 pruebas (2 emisiones en lugar de 1). Con la consulta, verde.
- Claude omitió un criterio del issue #5 ("estado del breaker expuesto para operación") y lo detectó al releer el issue antes de proponer commits: se agregó `GET /ops/breakers`.
- Dos pruebas propias mal escritas, detectadas al ejecutarlas: un servidor de prueba que dejaba colgado `srv.Close()` y una aserción que suponía que gobreaker conserva los contadores al abrir el circuito (los reinicia).
- Riesgo de diseño detectado antes de implementar: los aplazamientos de 30 s en la cola de reintento corto bloquearían a los reintentos de ~1 s (RabbitMQ solo vence el mensaje al frente). Se creó una cola `.deferred` por comando.

**Lectura.** El QA vuelve a aportar el hallazgo más caro, y otra vez por un patrón copiado de un tercero a otro (en HU-02 y HU-03 fue al revés: el QA aplicó al PSP una regla del emisor). La lección para la política de IA: una regla de mapeo de errores se decide por la consecuencia de negocio de cada tercero, no se reutiliza por parecido.

_Antigravity: entra en la revisión A al cerrar HU-05._

## HU-05 — Reconciliación, acciones operativas con login JWT y eventos

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| El flujo SSE avanzaba por ID sin esperar commits: un evento con ID menor que confirma tarde se saltaba para siempre, y se rompía la promesa de "reanudar sin perder nada" | QA (Haiku) | **Válido, bloqueante.** Error de diseño de Claude: asumió que el orden de los IDs es el orden de confirmación | El flujo espera ante un hueco en los IDs y lo salta solo pasado un margen (rollback). Se descartó la propuesta del QA de filtrar por `xmin` (32 bits, se rompe al dar la vuelta el contador). Pruebas nuevas del cursor |
| Si faltaba el operador en el contexto, la acción respondía 500 en vez de 401 | QA (Haiku) | Válido, defensa en profundidad | 401, con prueba |
| `ClaimStuck` descartaba el lote entero si fallaba la lectura de una orden ya marcada | QA (Haiku) | Válido | Devuelve las leídas y el error; el reconciliador procesa las demás |
| El limitador de login se reiniciaba entero al llegar a 10 000 IP: rotar IP devolvía el cupo a todas | QA (Haiku) | Válido | Se podan solo las IP sin intentos recientes, con prueba |
| Prueba de tiempo del login frágil y sin el caso del usuario inactivo | QA (Haiku) | Válido | Mediana de 5 intentos e inactivo incluido |
| `.env.example` traía un secreto JWT y una contraseña que cumplían el largo mínimo | QA (Haiku) | Válido | Vacíos y documentados como obligatorios |
| Latido, `WriteTimeout` del flujo y cierre al apagar sin prueba | QA (Haiku) | Válido | Pruebas de latido y de un flujo que sobrevive al `WriteTimeout`; el cierre al apagar se verificó en la prueba de humo |

**Verificaciones propias antes de cualquier revisor:**
- Se quitó a propósito el `FOR UPDATE SKIP LOCKED` de la toma de órdenes: dos reconciliadores tomaron la misma orden (3 de 3 veces).
- Se quitó a propósito el hash ficticio del login: la prueba detectó la diferencia de tiempo (0 s vs. 59 ms), que revelaría qué usuarios existen.
- Riesgo de diseño detectado antes de implementar: en la 3.ª ronda, el reconciliador podía marcar REFUND_REQUIRED mientras un worker emitía. Regla: si aparece la póliza, la orden pasa a ISSUED (el reverso nunca es automático).
- Un `WriteTimeout` de 20 s habría cortado el SSE, y `Shutdown` habría esperado 25 s por los flujos abiertos: ambos se resolvieron antes de la prueba de humo.
- La prueba de humo mostró fechas de eventos con zona local y de órdenes en UTC; se unificaron.

**Lectura.** El hallazgo bloqueante es del mismo tipo que el de HU-04: una suposición que parece obvia (los IDs ordenan los commits) y que solo falla con concurrencia real. El QA acertó el problema y propuso una solución con un defecto propio; la decisión final requirió criterio humano o del modelo principal. Lección para la política de IA: el revisor detecta, pero su arreglo también se revisa.

## Revisión A de Antigravity (HU-03 a HU-05, diff acumulado de Go)

**Cómo se hizo.** Una sola pasada sobre unas 2.400 líneas de código de producción (sin pruebas ni documentos), con 5 preguntas de riesgo concretas, la lista de decisiones ya aceptadas para no repetirlas y un máximo de 5 hallazgos. Reemplazó a tres revisiones por HU para ahorrar tokens (estrategia en la sección 9.2 del plan).

| Pregunta / hallazgo | Veredicto | Acción |
|---|---|---|
| Doble emisión, orden pagada perdida, concurrencia SQL | Sin hallazgo, con argumento correcto en las tres | — |
| Comandos duplicados si la base falla después de que el broker confirmó (y en el `ack` del worker) | Real pero **ya aceptado** (entrega "al menos una vez", ADR 0002; lo absorben la idempotencia por estado y el control optimista). Una parte del escenario es inexacta: una falla al publicar un mensaje no deshace el lote; solo un error de la base lo hace | Sin cambio |
| El limitador de login crece sin tope si todas las IP están activas (ataque distribuido): la poda no libera nada | **Válido.** Lo introdujo Claude al corregir el hallazgo del QA sobre el mismo limitador: arregló un problema y abrió otro | Si la poda no libera espacio, se descarta la IP más antigua. Prueba nueva en rojo primero, validada con una mutación (descartar la más reciente la hace fallar) |

**Efecto lateral.** Al corregirlo apareció una prueba intermitente que Claude escribió en HU-04: el breaker de prueba quedaba abierto solo 100 ms, y una pausa de la máquina durante la suite completa lo pasaba a half-open. Las pruebas que no son de half-open ahora lo dejan abierto una hora; 4 corridas completas seguidas en verde.

**Lo encontró el CI, no un revisor.** Después de la revisión A, el CI de Linux falló en una prueba del reconciliador. El fake marcaba las órdenes revisadas con "umbral + 1 ns" en vez de la hora actual, como hace la base (`now()`): en Windows, con un reloj de poca resolución, las dos pasadas leían la misma hora y la prueba pasaba por casualidad. Es el **cuarto fake del proyecto que no se comporta como el sistema real**. Se reprodujo la falla en un contenedor Linux con `-race` antes de corregirla, y la corrección se confirmó ahí mismo (3 corridas). Ni el QA ni Antigravity lo vieron: los dobles de prueba escritos por la IA necesitan pruebas en el mismo entorno que el CI.

**Lectura.** Con preguntas acotadas y las decisiones aceptadas listadas, Antigravity no repitió hallazgos conocidos (en HU-02 aportó 1 cambio de 12) y su único hallazgo nuevo fue válido. Que el defecto viniera de la corrección de otro revisor es el punto para la política de IA: cada arreglo sugerido por una IA vuelve a pasar por revisión, y una revisión por bloque al final atrapa lo que dejaron las revisiones por HU.

## HU-06 — Trazabilidad, salud y contrato OpenAPI

| Hallazgo | Lo encontró | Veredicto | Acción |
|---|---|---|---|
| Los logs de error de los handlers HTTP y del webhook no llevaban el correlation-id, aunque estaba en el contexto (p. ej. "request failed" de `POST /orders`) | QA (Haiku) | **Válido.** Incumplía "en todos los logs" | Todos pasan por `trace.Logger`; prueba nueva en rojo primero |
| El criterio "también en la DLQ" no tenía prueba | QA (Haiku) | Válido | Aserción del `CorrelationId` en el mensaje de la DLQ |
| El ejemplo del contrato mostraba la placa como `***123`; el código la enmascara como `ABC***` | Claude, al revisar el hallazgo del QA sobre el formato de la placa | Válido (documentación incorrecta) | Ejemplo corregido |
| La placa enmascarada deja ver los 3 primeros caracteres | QA (Haiku) | Decisión aceptada: el enunciado prohíbe la placa **completa**; la serie sola no identifica al vehículo y ayuda a operaciones | Sin cambio |
| Sin backfill del correlation-id para órdenes previas | QA (Haiku) | No aplica: no hay datos de producción | Sin cambio |
| El despachador registra la falla del lote sin correlation-id | QA (Haiku) | Aceptable: es un error del lote, no de una compra | Sin cambio |

**Verificaciones propias antes de cualquier revisor:**
- Prueba de humo con el binario y un cotizador y un PSP falsos: el PSP recibió el mismo `X-Correlation-ID` que envió el cliente, **desde el worker**, después de pasar por la outbox y RabbitMQ.
- Mutación del contrato: al renombrar `policyNumber` en el código, la prueba de contrato falló (para eso se agregó `additionalProperties: false` a las respuestas; sin eso, un campo renombrado pasaba inadvertido).
- Al cambiar la inserción de la outbox a `INSERT … SELECT`, Claude notó que un comando para una orden inexistente dejaría de fallar y se perdería en silencio; ahora devuelve `ErrOrderNotFound`, con prueba.
- Todo se corrió también en Linux con `-race` antes de proponer los commits (lección de la falla del CI en HU-05).
