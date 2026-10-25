# ADR 0007 — Consola con BFF y cookie httpOnly; registro de llamadas a terceros

- **Estado:** Aceptado
- **Fecha:** 2026-10-25
- **Responsable:** Líder técnico

## Contexto
La consola necesita tres cosas que el orquestador no resolvía:
1. **Autenticar el navegador.** El orquestador exige `Authorization: Bearer` y no tiene CORS, y `EventSource` (SSE) no puede enviar cabeceras.
2. **Mostrar "respuestas de terceros".** El orquestador solo guardaba el resultado final (cotización, `paymentId`, póliza, motivo de falla), no cada intento.
3. **Listar y buscar órdenes.** No había listado.

## Opciones consideradas
**Autenticación del navegador**

| Opción | A favor | En contra |
|---|---|---|
| **BFF: Next.js reenvía al orquestador; JWT en cookie httpOnly** | Un XSS no puede robar el token; sin CORS en Go; el SSE funciona con `EventSource` nativo (reconexión y `Last-Event-ID` incluidos) | Un salto más; el servidor de Next mantiene abierta una conexión SSE por pestaña |
| Navegador → Go directo, token en memoria | Una pieza menos | CORS en Go; SSE con `fetch` y lectura manual del flujo; al recargar se pierde la sesión (o se guarda en `localStorage`, expuesto a XSS) |

**Respuestas de terceros**

| Opción | A favor | En contra |
|---|---|---|
| **Tabla `third_party_calls`: un registro por intento** | El operador ve por qué se atascó una orden (503, timeout, breaker abierto, póliza no encontrada) sin abrir logs | Una escritura más por intento; la tabla crece con el tráfico |
| Mostrar solo el resultado final | Sin trabajo extra | No explica el camino: una orden que se recuperó se ve igual que una que salió bien a la primera |
| Guardar los cuerpos crudos | Máximo detalle | Llevan placa y documento: datos personales en otra tabla (Ley 1581) |

## Decisión
- **BFF.** El navegador solo habla con `/api/*` de Next. El JWT va en una cookie `httpOnly`, `Secure` y `SameSite=Strict`, que vence con el token. El BFF reenvía con `Authorization: Bearer` y `X-Correlation-ID`. Si recibe un 401, borra la cookie. Rechaza los POST con `Origin` de otro sitio y los ids que no son UUID (para que no se puedan armar otras rutas del orquestador). El SSE se reenvía tal cual, con `Last-Event-ID`.
- **Llamadas a terceros.** Decoradores en `app/call_recording.go` envuelven los puertos del cotizador, el PSP y la emisión, y registran por cada intento el tercero, la operación, el resultado (`OK`, `REJECTED`, `UNAVAILABLE`, `TIMEOUT`, `CIRCUIT_OPEN`, `NOT_FOUND`), la duración y el error del adaptador (recortado a 300 caracteres). El decorador de emisión va **por fuera del breaker**, así que también queda el intento que no se hizo porque el circuito estaba abierto. Registrar es *best effort*: si falla, queda un WARN y la compra sigue. Nunca se guardan cuerpos.
- **Listado.** `GET /orders` con filtro por estado (repetible), búsqueda exacta (id, `paymentId`, póliza, correlation-id, placa) y paginación por cursor (`created_at DESC, id DESC`, keyset). Se pide una fila de más para saber si hay otra página sin un `COUNT`.

## Consecuencias
- **Buenas:** verificado en Chrome con el escenario de caída del emisor. La orden pasó sola de "Emitiendo" a "Emitida", y su detalle mostró "No se llamó (breaker abierto)" → "No disponible" → "No existe" → "Emitir póliza: OK". La cookie no es visible desde `document.cookie`.
- **Malas:** el registro es síncrono: cada intento espera la escritura, hasta 2 s si la base está lenta, y con el breaker abierto se escribe una fila por reintento aplazado. Aceptable a 50 rps; si pesara, se escribiría por lotes desde un canal en memoria, aceptando perder registros en un reinicio. `third_party_calls` crece con cada intento. En producción necesita retención (p. ej. 90 días, partición por mes) y entra en la política de borrado del Bloque 4. La búsqueda es exacta, no por prefijo: es lo que llega en un reclamo, y no exige índices de texto.
- **En AWS:** el BFF corre como un servicio ECS más detrás del ALB. El tiempo de inactividad del ALB debe superar el latido SSE de 15 s.
