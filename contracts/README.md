# Contratos entre servicios

Pruebas de contrato **guiadas por el consumidor**: cada archivo describe lo que un consumidor envía y lo que necesita recibir de un proveedor. Los dos lados prueban contra el **mismo archivo**, así que si uno cambia un campo, el contrato se rompe en el CI de ese lado antes de llegar a producción.

| Archivo | Consumidor | Proveedor |
|---|---|---|
| [`orchestrator-issuing-service.json`](orchestrator-issuing-service.json) | Orquestador (Go): `internal/adapters/issuer` | Capa de emisión (C#): `POST/GET /policies` |
| [`orchestrator-simulator.json`](orchestrator-simulator.json) | Orquestador (Go): `internal/adapters/quoter` y `psp` | Simulador (Node): cotizador y PSP |
| [`issuing-service-simulator.json`](issuing-service-simulator.json) | Capa de emisión (C#): `LegacyIssuerClient` | Simulador (Node): emisor legado |

## Formato

Cada interacción tiene:

- `providerState`: el estado en que debe estar el proveedor (por ejemplo, "el emisor legado está caído"). La prueba del proveedor lo prepara con sus propios fakes o con el caos del simulador.
- `request`: método, ruta, query, cabeceras y cuerpo que envía el consumidor. Se compara **por valor**, salvo los campos de `request.dynamic` (los que el consumidor genera en cada llamada, como un id de transacción o la fecha), que se comparan por forma.
- `response`: status y cuerpo de ejemplo. **Solo lleva los campos que el consumidor usa**: el proveedor puede devolver más.
- `exact`: rutas (`data.policy.policyNumber`) cuyo valor tiene que coincidir exactamente. El resto se compara por forma: los mismos campos con el mismo tipo JSON.
- `consumerExpects`: cómo debe interpretar el consumidor esa respuesta (`issued`, `rejected`, `transient`, `found`, `not-found`, `quoted`, `accepted`).

## Qué verifica cada lado

- **Consumidor:** su cliente HTTP real habla con un servidor que responde lo que dice el contrato. La prueba verifica que la solicitud coincida con la del contrato (forma y valores) y que el cliente interprete la respuesta como dice `consumerExpects`.
- **Proveedor:** recibe la solicitud del contrato en el estado indicado y su respuesta tiene que tener la forma del contrato y los valores de `exact`.

Las pruebas corren dentro de los jobs que ya existen (`go test`, `dotnet test`, `npm test`). Para cambiar un contrato hay que cambiar el archivo y los dos lados en el mismo PR.

## Cómo se validó que detectan una ruptura

Se rompió una cosa a la vez en el código real, se corrió la prueba y se restauró el archivo. Todas fallaron señalando el campo:

| Ruptura | Lado | Falla |
|---|---|---|
| El orquestador envía `external_ref` en vez de `externalRef` | Consumidor Go (`issuer`) | `solicitud fuera del contrato: $.externalRef: falta` |
| El orquestador envía `documentID` en vez de `documentId` | Consumidor Go (`quoter`) | `$.documentId: falta` |
| La ACL responde `number` en vez de `policyNumber` | Proveedor C# | `$.policyNumber: falta` |
| La ACL envía `plateNo` en vez de `plateNumber` al legado | Consumidor C# | `$.policyRequest.vehicle.plateNumber: falta` |
| El emisor simulado responde `items` en vez de `policies` | Proveedor Node | `$.policies: falta` |
| El PSP simulado responde `status: "CREATED"` | Proveedor Node | `status: se esperaba "PENDING"` |
| El orquestador intercambia placa y documento (los dos son texto) | Consumidor Go (`issuer`) | `plate: se esperaba ABC123, llegó 1020304050` |
| La ACL envía el documento como placa al legado | Consumidor C# | `policyRequest.vehicle.plate: se esperaba "ABC123"` |
| La ACL consulta con `?externalReference=` en vez de `?externalRef=` | Consumidor C# | la query enviada no coincide con la del contrato |

**Fuera de alcance:** el webhook del PSP (el simulador llama al orquestador). Es la interacción de los duplicados y del webhook que llega antes del 202; hoy la cubren las pruebas de cada lado y el E2E, no un contrato compartido.

**Por qué no Pact:** sus librerías para Go y .NET dependen de una librería nativa que complica el CI y el desarrollo en Windows, y sin un broker de contratos su principal ventaja (versionar contratos entre repositorios) no aplica en un monorepo. La idea es la misma, con un formato pequeño y sin dependencias.
