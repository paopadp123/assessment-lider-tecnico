# ADR 0005 — Login JWT para los operadores de la consola

- **Estado:** Aceptado
- **Fecha:** 2026-10-24
- **Responsable:** Líder técnico

## Contexto
Las acciones operativas ("reintentar emisión" y "marcar para reverso") mueven dinero y pólizas, y el enunciado exige registrar quién las ejecutó. Sin autenticación, cualquiera podría escribir el nombre de otro. La línea de tiempo y el flujo SSE también exponen datos de clientes, así que no deben ser públicos. El patrón ya se aplicó en otra prueba del autor (login JWT con roles y auditoría por trigger).

## Decisión
- **`POST /auth/login`** con usuario y contraseña. La contraseña se guarda con **bcrypt**; nunca en claro ni en logs.
- La respuesta **y el tiempo** son iguales si el usuario no existe, está inactivo o la contraseña es otra: contra un usuario inexistente se compara un hash ficticio. Así no se puede averiguar qué operadores existen.
- **JWT HS256 de 60 minutos** con `sub` (usuario), `name`, `iss`, `aud`, `iat`, `exp` y `jti`. Se valida firma (solo HS256: un token con `alg: none` o con otro algoritmo se rechaza), emisor, audiencia y vencimiento, con 30 s de tolerancia de reloj. El secreto viene de `JWT_SECRET` y se exige de al menos 32 bytes al iniciar.
- **El actor sale del token**, nunca del cuerpo de la solicitud: no se puede actuar en nombre de otro. Queda en `order_events.actor` en la misma transacción que el cambio.
- **Protegido:** acciones, línea de tiempo, flujo SSE y `GET /ops/breakers`. **Público:** `POST /orders` (es del cliente; usa `Idempotency-Key`), el webhook del PSP (firma HMAC) y `/health`.
- **Límite de intentos:** 5 logins por minuto por IP (`429` con `Retry-After`). La IP es la de la conexión: no se confía en `X-Forwarded-For`, que cualquiera puede inventar. El registro de IP nunca supera 10 000 entradas: primero se descartan las que no tienen intentos recientes (rotar IP no le devuelve el cupo a una que está siendo frenada) y, si todas están activas (ataque distribuido), la que lleva más tiempo sin intentar. La memoria queda acotada aunque esa IP recupere su cupo.
- **Primer operador** desde `OPS_BOOTSTRAP_USER` / `OPS_BOOTSTRAP_PASSWORD` al iniciar (contraseña ≥ 12 caracteres). Si ya existe, no se toca. No hay registro ni gestión de usuarios.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **Login propio con JWT** | Identidad verificada; sin estado en el servidor (escala con réplicas); probado de punta a punta | Gestionar contraseñas; un token no se puede revocar antes de vencer |
| Cabecera `X-Operator` sin login | Cero costo | La identidad no se verifica: la auditoría no prueba nada |
| SSO corporativo (Cognito / Entra ID) | Lo correcto en producción: MFA, bajas centralizadas, sin contraseñas propias | Infraestructura externa que no corre en local con `docker compose up` |

## Consecuencias
- **Buenas:** la auditoría de acciones tiene una identidad verificada; las rutas de operación dejan de ser públicas.
- **Malas:** sin roles (todos los operadores pueden todo) ni revocación (se mitiga con 60 min); el límite de intentos vive en memoria de cada réplica, así que con N réplicas el límite efectivo es N veces mayor (en AWS: AWS WAF con una regla de tasa por IP delante del ALB); detrás de un balanceador habría que leer la IP real solo de la cabecera que pone el balanceador.
- **En producción:** se reemplaza el login propio por el SSO de la compañía. El middleware solo cambia la verificación del token (JWKS del proveedor en lugar del secreto compartido); acciones y auditoría no cambian.
