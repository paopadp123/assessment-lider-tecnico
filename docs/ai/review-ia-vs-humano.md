# Code review: IA vs. revisión humana

Comparación de lo que encontró la IA contra lo que encontró la revisión humana sobre el mismo SHA. La IA recibió las áreas de foco y el alcance, **sin** los hallazgos ya encontrados (prompt en [`prompt-log.md`](prompt-log.md), entrada del 2026-10-25). Todo lo que reportó se verificó contra el código y, cuando era serio, corriendo la aplicación.

## golang-gin-realworld-example-app @ `626c372` — Antigravity

**Revisión humana antes de la IA:** REV-GO-001 a REV-GO-006 (con apoyo de Claude Code para precisar líneas y probar en vivo).

### Lo que la IA reportó

| # | Reporte de la IA (severidad) | Veredicto | Motivo |
|---|------------------------------|-----------|--------|
| 1 | Secreto del JWT y `RandomPassword` en el código (Crítica) | **Coincide** con REV-GO-001 | Bien identificado. Errores menores: el fragmento "copiado" de `RandomPassword` está mal transcrito (`!@##$!#4` en vez de `!@##$!@#4`) aunque lo marcó "verificado", y dice que `RandomPassword` permite "predecir contraseñas", cuando en realidad es un valor centinela de `users/validators.go:35` para no cambiar la contraseña. No vio el `#nosec` como problema |
| 2 | Error de `bcrypt` ignorado → "cuentas sin contraseña, accesos no autorizados" (Crítica) | **Corregido** → REV-GO-007 (Media) | Lugar correcto, mecanismo e impacto equivocados. La causa real es el límite de 72 bytes de bcrypt contra el `max=255` del validador; el efecto es una cuenta **bloqueada**, no abierta. Probado en vivo: registro 201, login 401 para siempre |
| 3 | `ensureDir` ignora el error (Alta) | **Coincide y se amplía** → REV-GO-008 (Baja) | Válido, pero lo más grave en las líneas siguientes (`gorm.Open`, `db.DB()`) no lo mencionó. Severidad sobrestimada: el servicio no arranca de todos modos |
| 4 | Falta `SetMaxOpenConns` y `SetConnMaxLifetime` (Media) | **Descartado** | La base es SQLite en un archivo local: el problema de producción es usar SQLite, no el tamaño del pool. Además marcó esfuerzo "L" para "2–3 líneas", inconsistente con su propia escala |
| 5 | Sin apagado ordenado (Media) | **Corregido** → REV-GO-009 (Baja) | Válido, pero el impacto es el contrario: Go no queda "colgado" con `SIGTERM`, termina de inmediato y corta las peticiones en curso |
| 6 | Error de `bcrypt` ignorado, "repetido para claridad" (Media) | **Duplicado** | Es el mismo hallazgo que el #2 contado dos veces; infla el total |
| 7 | `AutoMigrate` depende de `common.GetDB()` (Baja) | **Válido, Baja** | Coincide con la pista humana del estado global (`var DB` en `common/database.go`). Se agrupa con esa observación; no se registra aparte |

### Lo que la IA no encontró

| Hallazgo humano | Por qué se le escapó (según su propio reporte) |
|-----------------|-----------------------------------------------|
| REV-GO-002 — JSON roto → panic y 500 | No relacionó `common/utils.go:71` con los handlers |
| REV-GO-003 — claim `id` → panic | No abrió `users/middlewares.go` |
| REV-GO-004 — usuario inexistente autenticado | No abrió `users/middlewares.go` |
| REV-GO-005 — contrato implícito del contexto | No siguió el flujo entre middleware y serializer |
| REV-GO-006 — suite verde sin casos de error | No abrió ni ejecutó las pruebas |
| Modo debug de Gin y "trusted all proxies" | Solo se ve al ejecutar la aplicación |

### Lectura

- **Coincidencias:** 2 de 6 hallazgos humanos (REV-GO-001 y, en parte, la pista del estado global).
- **Aportes reales de la IA:** 3 hallazgos nuevos válidos (REV-GO-007, 008 y 009), **los tres con la severidad o el impacto corregidos** tras verificarlos.
- **Errores de la IA:** un fragmento mal copiado marcado como "verificado", un hallazgo duplicado, uno descartado por falta de contexto (SQLite) y dos impactos técnicamente invertidos (bcrypt, apagado).
- **Patrón:** la IA es buena encontrando *patrones locales* (un `_` que descarta un error, una constante con un secreto) y débil siguiendo *flujos entre archivos* (middleware → contexto → serializer), que es donde estuvieron los hallazgos de autenticación. También tiende a **sobrestimar la severidad**: dos "Críticas" de las que solo una lo era.
- **Lección para la política de uso:** la IA sirve como segunda pasada para ampliar la cobertura, pero ningún hallazgo suyo entra al informe sin leer la línea en el SHA, y los serios sin reproducirlos. Su "verificado" no es una verificación.
