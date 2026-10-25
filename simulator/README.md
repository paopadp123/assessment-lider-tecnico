# Simulador de terceros

Un servicio Node + TypeScript (Fastify) que simula a los tres terceros con el caos del enunciado:

- el cotizador,
- la pasarela de pagos (PSP),
- el emisor legado.

Las fallas tienen probabilidades configurables, y se pueden cambiar en vivo con `/chaos` para la demo.

```bash
npm ci
PSP_WEBHOOK_SECRET=<el mismo del orquestador> PSP_WEBHOOK_URL=http://localhost:8080/webhooks/payments npm run build && npm start
npm test        # Vitest, con semilla fija: los porcentajes no son intermitentes
```

El estado (pagos y pólizas) vive en memoria: reiniciar el simulador lo borra. No tiene límite porque es un stub para demos y pruebas; en una corrida de carga larga crece con cada compra.

## Rutas

| Tercero | Ruta | Respuesta normal |
|---|---|---|
| Cotizador | `POST /v1/quotes` `{plate, vehicleYear, cityCode, documentId}` | `200 {quoteId, premium, currency, validUntil}` (400 si falta un campo) |
| PSP | `POST /v1/payments` `{quoteId, amount, currency, customerRef}` + `Idempotency-Key` | `202 {paymentId, status: "PENDING"}` y luego un webhook firmado |
| Emisor | `POST /issue` (contrato verboso, ver abajo) | `200 {result: "OK", code: "00", data: {policy}}` |
| Emisor | `GET /policies?externalRef={orderId}` | `200 {result: "OK", code: "00", policies: [...]}` (lista vacía si no existe) |
| Control | `GET /chaos`, `POST /chaos` | configuración del caos |
| Salud | `GET /health` | `200 {status: "ok"}` |

**Webhook del PSP.** Se envía `POST {PSP_WEBHOOK_URL}` con `{paymentId, status: "APPROVED"|"REJECTED", occurredAt, signature}`. La firma es HMAC-SHA256 en hexadecimal sobre `paymentId|status|occurredAt`. Si el orquestador no responde 2xx, el webhook se reintenta a 1 s, 2 s y 4 s. La misma `Idempotency-Key` devuelve el mismo pago y no genera otro webhook; con otro monto responde 409.

**Correlation-id.** Cada log de una solicitud lleva el `X-Correlation-ID` recibido, o uno generado si no viene o es inválido. En los logs, la placa aparece como `ABC***` y el documento como `****4050`.

## Contrato del emisor legado (lo envuelve la ACL, HU-07)

```jsonc
POST /issue
{
  "header": { "transactionId": "tx-1", "channel": "DIGITAL", "requestDate": "20261024100000" },
  "policyRequest": {
    "externalRef": "<orderId>",
    "externalReference": "<orderId>",          // redundante: debe coincidir con externalRef
    "product": { "code": "SOAT", "productCode": "SOAT" },
    "vehicle": { "plate": "ABC123", "plateNumber": "ABC123", "modelYear": "2020", "cityCode": "11001" },
    "holder": { "document": { "type": "CC", "number": "1020304050" }, "documentNumber": "1020304050" },
    "premium": { "amount": "412000.00", "value": 412000, "currency": "COP" }
  }
}
```

| Respuesta | Significado | ¿Emitió? |
|---|---|---|
| `200 {result: "OK", code: "00", message: "EMITIDA", data: {policy: {policyNumber, number, statusCode, status, externalRef, issueDate, premium}}}` | Emitida. `number` repite `policyNumber`; `issueDate` usa el formato `dd/MM/yyyy` | Sí |
| `200 {result: "ERROR", code: "E-100"}` | Solicitud incompleta, o campos redundantes que no coinciden | No |
| `200 {result: "ERROR", code: "E-102"}` | Rechazo de negocio: el vehículo ya tiene póliza vigente | No |
| `200 {result: "ERROR", code: "E-104"}` | Rechazo de negocio: placa con formato inválido | No |
| `200 {result: "ERROR", code: "E-901"}` | Error técnico: reintentar | No |
| `200` con JSON cortado | Formato inválido | **Sí**: hay que consultarla |
| Respuesta a los 8 s | Timeout pero sí emitió | **Sí**: hay que consultarla |
| `503 text/html` | Caída total | No |

- Reemitir el mismo `externalRef` devuelve la misma póliza.
- `E-1xx` son rechazos de negocio definitivos; `E-9xx`, el JSON cortado y el 503 son transitorios.
- Antes de reintentar, hay que consultar `GET /policies?externalRef=`.

## Caos

Cada solicitud tira el azar **una vez**. Por eso, las probabilidades de las fallas de un mismo tercero no pueden sumar más de 1.

| Variable | Por defecto | Qué hace |
|---|---|---|
| `QUOTER_ERROR_RATE` | 0.05 | 500 del cotizador |
| `QUOTER_INVALID_RATE` | 0.05 | 200 con JSON cortado |
| `QUOTER_MIN_LATENCY_MS` / `QUOTER_MAX_LATENCY_MS` | 100 / 3000 | Latencia |
| `PSP_APPROVE_RATE` | 0.9 | Proporción de pagos `APPROVED`; el resto son `REJECTED` |
| `PSP_DUPLICATE_RATE` | 0.1 | Mismo webhook entregado dos veces |
| `PSP_EARLY_WEBHOOK_RATE` | 0.1 | Webhook que llega antes que el 202 (fuera de orden); el 202 espera la entrega como máximo 1 s |
| `PSP_INVALID_RATE` | 0.05 | 202 con JSON cortado (el pago sí se crea) |
| `PSP_MIN_WEBHOOK_DELAY_MS` / `PSP_MAX_WEBHOOK_DELAY_MS` | 500 / 3000 | Retardo del webhook |
| `PSP_WEBHOOK_RETRIES` | 3 | Reintentos del webhook |
| `ISSUER_MIN_LATENCY_MS` / `ISSUER_MAX_LATENCY_MS` | 100 / 1500 | Latencia |
| `ISSUER_INVALID_RATE` | 0.05 | Emite y responde JSON cortado |
| `ISSUER_TECHNICAL_ERROR_RATE` | 0.025 | `E-901`, sin emitir |
| `ISSUER_BUSINESS_ERROR_RATE` | 0.025 | `E-102`, sin emitir |
| `ISSUER_TIMEOUT_BUT_ISSUED_RATE` | 0.1 | Emite y responde después de `ISSUER_TIMEOUT_MS` (8000) |
| `ISSUER_DOWN` | false | Caída total desde el arranque |
| `ISSUER_OUTAGE_START_S` + `ISSUER_OUTAGE_DURATION_S` | — | Ventana de caída, contada desde el arranque |
| `SIM_SEED` | aleatoria (queda en el log) | Repite exactamente una corrida si las solicitudes llegan en el mismo orden (con solicitudes concurrentes el orden cambia) |
| `PSP_WEBHOOK_SECRET` | obligatorio, ≥ 32 bytes | El mismo secreto del orquestador |
| `PSP_WEBHOOK_URL` | `http://localhost:8080/webhooks/payments` | Dónde se envía el webhook |
| `PORT` | 8090 | Puerto |

### En vivo, para la demo

```bash
curl localhost:8090/chaos                                                            # ver
curl -X POST localhost:8090/chaos -H 'Content-Type: application/json' -d '{"issuer":{"down":true}}'   # tumbar el emisor
curl -X POST localhost:8090/chaos -H 'Content-Type: application/json' -d '{"issuer":{"down":false}}'  # levantarlo
curl -X POST localhost:8090/chaos -H 'Content-Type: application/json' -d '{"issuer":{"timeoutButIssuedRate":1}}'  # forzar "timeout pero emitió"
curl -X POST localhost:8090/chaos -H 'Content-Type: application/json' -d '{"seed":42}'                # reiniciar el azar
```

`POST /chaos` cambia solo lo que se envía. Un valor fuera de rango o un campo desconocido responde 400 y no cambia nada. La ruta no tiene autenticación porque el simulador nunca va a producción.
