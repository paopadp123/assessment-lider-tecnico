# Servicio de emisión (capa anticorrupción)

API mínima en .NET 10. Envuelve el contrato del emisor legado y le ofrece al orquestador un contrato limpio. Es idempotente por la referencia externa (el id de la orden) y no guarda estado.

```bash
dotnet test IssuingService.slnx
LegacyIssuer__BaseUrl=http://localhost:8090 dotnet run --project src/Issuing.Api
```

## Contrato hacia el orquestador

| Solicitud | Respuesta |
|---|---|
| `POST /policies` `{externalRef, quoteId, premium, currency, plate, documentId, vehicleYear, cityCode}` | `201 {policyNumber}` emitida · `200 {policyNumber}` ya existía · `422 {code, message}` rechazo de negocio · `400` solicitud inválida (detalle por campo) · `503` no disponible · `502` falla de la integración |
| `GET /policies?externalRef={orderId}` | `200 {policyNumber}` · `404` · `503` |

**Solo el 422 es definitivo**: es el único que lleva una orden pagada a `REFUND_REQUIRED`. Todo lo demás lo reintenta el orquestador, que también tiene el circuit breaker y la reconciliación. Por eso la ACL hace un solo intento por solicitud y no lleva reintentos propios.

Cada solicitud acepta `X-Correlation-ID` (o se genera uno). El id se reenvía al emisor y aparece en todos los logs. Los logs nunca incluyen la placa ni el documento.

## Cómo traduce al emisor legado

1. **Valida** con las mismas reglas que el orquestador (placa, documento, año, ciudad, prima con 2 decimales como máximo, moneda). Si algo no cumple, responde 400 y no llama al emisor.
2. **Consulta** `GET /policies?externalRef=`. Si la póliza ya existe, responde 200 sin emitir. Si la consulta falla, responde 503 y no emite a ciegas.
3. **Emite** con `POST /issue`, armando el contrato verboso (campos redundantes coherentes, `transactionId` único, `requestDate` en formato `yyyyMMddHHmmss` y UTC).
4. **Interpreta** la respuesta:

| El emisor responde | La ACL |
|---|---|
| `200 {result: "OK", data.policy}` | `201` |
| `200 {result: "ERROR", code: "E-102"}` | Consulta primero: si la póliza vigente es la de esta orden (un intento anterior emitió sin confirmarse), `200`; si no, `422 VEHICLE_HAS_ACTIVE_POLICY` |
| `200 {result: "ERROR", code: "E-104"}` | `422 INVALID_PLATE` |
| `200 {result: "ERROR", code: "E-901"}` · `503` | `503` |
| `200 {result: "ERROR", code: "E-100"}` · un código desconocido · una póliza de otra referencia · un `4xx` | `502` (defecto de la integración, no del cliente) |
| Sin respuesta en `IssueTimeout`, JSON ilegible, `OK` sin número de póliza | Consulta otra vez: `201` si la encuentra, `503` si no |

El mensaje del emisor nunca se reenvía: puede traer datos personales y no es parte del contrato limpio.

## Configuración

| Variable | Por defecto | |
|---|---|---|
| `LegacyIssuer__BaseUrl` | — (obligatoria) | URL del emisor legado |
| `LegacyIssuer__IssueTimeout` | `00:00:05` | Espera de `POST /issue` |
| `LegacyIssuer__LookupTimeout` | `00:00:02` | Espera de cada consulta |

Consulta previa, emisión y consulta posterior suman como máximo 9 s, dentro de los 10 s que el orquestador espera por intento. Con valores que no quepan, el servicio no arranca.

**Supuestos:**
- El producto es `SOAT`.
- El documento es una cédula de ciudadanía (`CC`): el contrato del orquestador no trae el tipo.
- La idempotencia depende de la consulta previa y de que el emisor no emita dos veces la misma `externalRef`. Si dos solicitudes iguales llegan al mismo tiempo, las dos pueden pasar la consulta, y entonces la protección es la idempotencia del emisor por `externalRef` (el simulador la tiene). Con un emisor que no la garantice, haría falta un candado por `externalRef` compartido entre réplicas (Redis o la base del orquestador). El orquestador lo hace poco probable: cada comando de emisión lo procesa un solo worker.
