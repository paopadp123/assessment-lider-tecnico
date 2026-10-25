# Escenarios de punta a punta (guion de la demo)

Cada escenario es un script en `scripts/e2e/`. Guarda el caos del simulador, lo deja en calma (sin fallas al azar), fuerza solo la falla que quiere mostrar con `POST /chaos` y lo restaura al salir, aunque falle. Verifica el resultado y termina con la línea de tiempo de la orden. Si algo no cumple, sale con un código distinto de 0: por eso el CI los usa como prueba.

```bash
bash scripts/dev-env.sh && docker compose up --build --wait
bash scripts/e2e/run-all.sh          # o uno solo: bash scripts/e2e/03-caida-emisor.sh
```

**"Una sola póliza"** se verifica de dos formas. Primero, la póliza de la orden es la que el emisor tiene para ese `externalRef`. Segundo, en los logs del emisor simulado, `/issue` emitió una sola vez y nadie lo volvió a llamar para una orden ya emitida. Contar pólizas en el emisor no prueba nada, porque el simulador guarda una por referencia.

| # | Script | Falla forzada | Qué demuestra | Resultado esperado |
|---|---|---|---|---|
| 1 | `01-compra-feliz.sh` | Cada webhook llega 2 veces; la mitad, antes del 202 del PSP | Idempotencia del `POST /orders` (misma clave → 200 y la misma orden) y del webhook (duplicados y fuera de orden) | ISSUED, una póliza |
| 2 | `02-timeout-pero-emitio.sh` | El emisor emite y responde a los 8 s; la ACL corta a los 5 s | Un timeout no prueba que no se emitió: la ACL consulta por `externalRef` antes de responder | ISSUED, una póliza, sin reemitir |
| 3 | `03-caida-emisor.sh` | Emisor caído (503) con 3 compras en curso | 5 fallas seguidas abren el breaker; las órdenes esperan en ISSUING sin gastar intentos. Al volver el emisor: half-open → closed, y la reconciliación recoge las que quedaron atascadas | Las 3 en ISSUED, una póliza cada una |
| 4 | `04-reembolso.sh` | El emisor rechaza con un error de negocio (E-102/E-104 dentro de un 200) | Pago aprobado + emisión rechazada → REFUND_REQUIRED (no hay reembolso automático); luego un operador reintenta y queda auditado | REFUND_REQUIRED con `ISSUE_REJECTED`; tras el reintento, ISSUED con `actor` = operador |

La corrida completa tarda ~1 min. El escenario 3 tarda ~40 s, porque el breaker espera 30 s antes de volver a probar.

## Guion para el video (~4 min de demo)

1. **Arranque (20 s).** `docker compose ps`: los 5 servicios healthy. Decir: un comando, secretos fuera del YAML y healthchecks con `depends_on`.
2. **Compra feliz (45 s).** Correr el escenario 1. Señalar el 200 de la repetición y la línea de tiempo. Con `docker compose logs orchestrator | grep <correlation-id>` se siguen todos los logs de esa compra.
3. **Timeout pero emitió (45 s).** Correr el escenario 2. Mostrar los logs de la ACL: *"unconfirmed (no answer after 5 s); checking whether the policy exists"* → *"finished as Issued"*. Decir: el riesgo era cobrar una vez y emitir dos.
4. **Caída del emisor (75 s).** Correr `OUTAGE_HOLD_S=40 bash scripts/e2e/03-caida-emisor.sh` (deja el emisor caído 40 s en vez de 10, para alcanzar a mostrar las colas). Con el breaker abierto, abrir http://localhost:15672 → *Queues*: se ve `orders.policy.issue.deferred` con los mensajes en espera (y `orders.policy.issue.retry.N` con los reintentos). Al levantar el emisor, mostrar las órdenes en ISSUED y `breaker: closed`.
5. **Reembolso (45 s).** Correr el escenario 4: la orden pagada sin póliza queda en REFUND_REQUIRED y es visible. Después, el reintento del operador aparece en la línea de tiempo con su usuario.
6. **Cierre (10 s).** El job `e2e` del CI corre estos 4 escenarios en cada push.

Para mostrar una falla a mano, sin script: `curl -X POST localhost:8090/chaos -H 'Content-Type: application/json' -d '{"issuer":{"down":true}}'` (ver `simulator/README.md`).

## Problemas conocidos al correrlo

- **`login fallido`**: el volumen de PostgreSQL es anterior al `.env` actual. El operador ya existía con otra contraseña y el arranque no la cambia. Solución: `docker compose down -v`.
- **RabbitMQ sale con `.erlang.cookie: eacces` la primera vez** (visto una vez en Docker Desktop para Windows): `docker compose up --wait` otra vez. El orquestador se reinicia solo (`restart: on-failure`).
