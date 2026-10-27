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
| Observación sin ID (no se registró como hallazgo): modo debug de Gin y "trusted all proxies" en el log de arranque | Solo se ve al ejecutar la aplicación |

### Lectura

- **Coincidencias:** 1 de 6 hallazgos humanos (REV-GO-001). Además coincidió con una observación humana que no se registró como hallazgo (el estado global de `common/database.go`).
- **Aportes reales de la IA:** 3 hallazgos nuevos válidos (REV-GO-007, 008 y 009), **los tres con la severidad o el impacto corregidos** tras verificarlos.
- **Errores de la IA:** un fragmento mal copiado marcado como "verificado", un hallazgo duplicado, uno descartado por falta de contexto (SQLite) y dos impactos técnicamente invertidos (bcrypt, apagado).
- **Patrón:** la IA es buena encontrando *patrones locales* (un `_` que descarta un error, una constante con un secreto) y débil siguiendo *flujos entre archivos* (middleware → contexto → serializer), que es donde estuvieron los hallazgos de autenticación. También tiende a **sobrestimar la severidad**: dos "Críticas" de las que solo una lo era.
- **Lección para la política de uso:** la IA sirve como segunda pasada para ampliar la cobertura, pero ningún hallazgo suyo entra al informe sin leer la línea en el SHA, y los serios sin reproducirlos. Su "verificado" no es una verificación.

## aspnetcore-realworld-example-app @ `8aa617a` — Antigravity

**Revisión antes de la IA:** REV-NET-001 a REV-NET-011, hechos por Claude Code (lectura del código y pruebas en vivo) y pendientes de la confirmación humana. Aquí la comparación es **IA contra IA**: Antigravity contra Claude Code con verificación en vivo.

**Cobertura:** Antigravity abrió **2 archivos** (`Program.cs` y `ServicesExtensions.cs`) de los más de 60 del repositorio. El prompt le indicaba empezar por ahí y seguir con `Infrastructure/`, `Features/`, las pruebas y Docker, y recorrer un flujo completo. No lo hizo, aunque lo reconoce en "lo no revisado".

### Lo que la IA reportó

| # | Reporte de la IA (severidad) | Veredicto | Motivo |
|---|------------------------------|-----------|--------|
| 1 | CORS abierto, "facilita CSRF" (Crítica) | **Coincide, corregido** → REV-NET-008 (Baja) | El hallazgo es real, pero el impacto está mal: el token viaja en un encabezado y no en una cookie, así que no hay CSRF. Citó la línea 78; la línea correcta es la 79 |
| 2 | Issuer y audience escritos en el código (Crítica) | **Descartado** (observación menor) | No son secretos. Su propio argumento dice "si logra obtener la clave", y con la clave ya se puede firmar cualquier token. Como mucho es configuración por entorno, de severidad Baja |
| 3 | "Solo acepta el prefijo `Token`; los clientes con `Bearer` son rechazados; la especificación de RealWorld usa `Bearer`" (Alta) | **Falso, probado en vivo** | `Authorization: Bearer <jwt>` → **HTTP 200**. `OnMessageReceived` solo toma el token cuando ve `Token`; en los demás casos, el manejador de JwtBearer extrae `Bearer` por su cuenta. Además, la especificación de RealWorld usa `Authorization: Token jwt.token.here`. Marcado como "verificado leyendo el código" |
| 4 | Sin health checks ni métricas (Alta, sospecha) | **Coincide en parte** → REV-NET-007 (Media) | Es real, pero no vio el problema de fondo de los logs (eventos duplicados, texto plano). Severidad sobrestimada |
| 5 | La validación de la clave JWT es insuficiente; quitar la comprobación de la "cadena mágica" (Media) | **Descartado** | Es una defensa adicional que el repositorio de Go no tiene (ver REV-GO-001). Quitar esa comprobación permitiría volver a usar la clave de ejemplo histórica. El fragmento que citó como 108–115 mezcla líneas de 99 a 120 con `...`: no está copiado del archivo |
| 6 | Sin token de renovación ni revocación, "afecta la seguridad de las sesiones largas" (Media, sospecha) | **Coincide en parte, impacto invertido** → REV-NET-003 | No vio que el token vence a los **5 minutos**: el problema real es que las sesiones son demasiado **cortas** y el usuario pierde su trabajo |
| 7 | `IOptions<JwtIssuerOptions>` "no se usa en otro lugar" (Baja, sospecha) | **Falso** | `JwtTokenGenerator.cs:9` lo inyecta. Le bastaba abrir un archivo más |
| 8 | Lista de pipeline behaviors escrita a mano (Baja) | **Descartado** | Es estilo. Dos elementos explícitos son más legibles que el escaneo de ensamblados que propone |

**Fortalezas que reportó y que en realidad son hallazgos:** "migraciones automáticas que facilitan despliegues" (REV-NET-006: con varias réplicas es un riesgo) y "Serilog listo para producción" (REV-NET-007: el propio código dice `//just for local debug` y duplica cada evento). La fortaleza "validación robusta del JWT" sí es correcta.

### Lo que la IA no encontró

| Hallazgo previo | Por qué se le escapó |
|-----------------|----------------------|
| REV-NET-001 — `limit=-1` devuelve todo (Alta) | No abrió `Features/` |
| REV-NET-002 — login sin límite de intentos y con enumeración por tiempo (Alta) | No abrió `Features/Users/Login.cs`; la ausencia del limitador estaba en `Program.cs`, que sí leyó |
| REV-NET-004 — slug sin tildes o vacío | No abrió `Infrastructure/Slug.cs` |
| REV-NET-005 — campos sin longitud máxima | No abrió los validadores |
| REV-NET-006 — migraciones al arrancar con varias réplicas | Lo vio y lo presentó como fortaleza |
| REV-NET-009, 010, 011 | No abrió esos archivos |

### Lectura

- **Coincidencias:** 3 de 11, todas parciales y con la severidad o el impacto corregidos (008, 007 y 003).
- **Aportes nuevos válidos:** ninguno. El único rescatable (issuer y audience por configuración) es una observación Baja.
- **Errores:** un "Alta" falso marcado como "verificado", que se desmiente con un solo `curl`; una afirmación falsa sobre la especificación; un fragmento armado con `...` y presentado como copia; dos sospechas que se resolvían abriendo un archivo más; dos riesgos presentados como fortalezas. Las severidades están infladas: 2 "Críticas" y 2 "Altas", de las que ninguna lo es tal como las describe.
- **Patrón, confirmado con la pasada de Go:** la IA revisa lo que tiene delante y no recorre el repositorio, aunque el prompt se lo pida de forma explícita. Su "verificado" no implica que lo haya comprobado. En este repositorio, que ya está endurecido en lo obvio (secretos, validación del token), eso la deja casi sin hallazgos útiles: lo valioso estaba en los handlers y en el comportamiento en ejecución.
- **Lección para la política de uso:** una pasada de IA sin herramientas de ejecución, o sin la obligación de listar los archivos que abrió, da una falsa sensación de cobertura. Al prompt de la próxima pasada (Next) conviene agregarle: "lista primero los archivos que abriste; si son menos de N, sigue antes de reportar".

## next-realworld-example-app @ `be9ef56` — Antigravity

**Revisión antes de la IA:** REV-NEXT-001 a REV-NEXT-009, hechos por Claude Code (lectura del código, `npm audit`, prueba de `marked` en dos versiones y prueba en el navegador contra la API de C# local) y pendientes de la confirmación humana.

**Cobertura:** el prompt le exigía listar los archivos abiertos y no reportar con menos de 20. Escribió "13 ≺ 20" y siguió. De esos 13, 3 dicen "no se necesitó abrir": abrió **10**. En "lo no revisado" justificó que se acercaba al "número **máximo** de archivos a abrir (20)", cuando era un mínimo. No abrió `package.json`, `tsconfig.json` ni ningún componente de artículo, comentario o editor.

### Lo que la IA reportó

| # | Reporte de la IA (severidad) | Veredicto | Motivo |
|---|------------------------------|-----------|--------|
| 1 | `user.ts:7-9`: el token se lee sin `JSON.parse`, se envía `Token undefined` y hay riesgo de XSS (Crítica) | **Coincide en parte, corregido** → REV-NEXT-005 (Media) | El error existe, pero `UserAPI.current` **no se usa en ninguna parte**: hoy no tiene impacto. Mezcla un error de lectura con el riesgo de XSS, que es otro tema (REV-NEXT-001). Marcó "verificado (leyó código completo y su cadena de llamadas)", pero esa cadena no existe. El fragmento junta las líneas 9 a 14 en una sola |
| 2 | `marked` con `sanitize` obsoleto; "un autor podría subir `<script>`" (Alta) | **Coincide en parte, impacto incorrecto** → REV-NEXT-001 | Con la versión fijada (1.1.1), el HTML sale escapado: hoy no pasa un `<script>`. No vio el riesgo real: la versión actual de `marked` ignora la opción en silencio, y actualizar es obligatorio por los CVE |
| 3 | `favorite/unfavorite` sin `Authorization`; "permite abuso de rate limiting" (Alta) | **Coincide en parte, corregido** → REV-NEXT-005 | Las funciones no se usan: `ArticlePreview.tsx:43-57` llama a axios por su cuenta con el token. El "abuso de rate limiting" no tiene sustento. **Alucinación:** el fragmento "copiado" incluye `33: // sin encabezado Authorization`, una línea que no existe en el archivo |
| 4 | URL del backend fija en el código (Media) | **Coincide en parte** → REV-NEXT-002 (Alta) | No comprobó que el dominio ya no funciona (530) ni vio las hojas de estilo externas sin SRI. Severidad subestimada |
| 5 | `login` sin tipos (Media) | **Coincide en parte** → síntoma de REV-NEXT-006 | Válido como ejemplo; la causa es `strict: false` en `tsconfig.json`, que no abrió. Propone un `LoginResponse` con `token` aparte, pero en la API el token viene dentro de `user` |
| 6 | El login no tiene `<label>` (Media) | **Aporte nuevo, confirmado y ampliado** → REV-NEXT-010 (Baja) | Real y no estaba en la revisión previa. Se amplió: **ningún** formulario de la aplicación tiene etiquetas. Severidad bajada porque el placeholder funciona como nombre accesible de respaldo |

**Fortalezas y afirmaciones que se contradicen con el código:** "API centralizada en `lib/api/*`" (falso: los componentes llaman a axios directamente y parte de `lib/api` es código muerto, REV-NEXT-005); "no hay scripts de build" (falso: `package.json:7`).

### Lo que la IA no encontró

| Hallazgo previo | Por qué se le escapó |
|-----------------|----------------------|
| REV-NEXT-003 — 106 vulnerabilidades en las dependencias (Alta) | No abrió `package.json` ni el lockfile, aunque el prompt los nombraba |
| REV-NEXT-004 — publicar un artículo rompe el editor | No abrió `pages/editor/` ni `ListErrors.tsx`; solo se ve siguiendo el flujo o ejecutando |
| REV-NEXT-007 — sin licencia | No revisó la raíz del repositorio |
| REV-NEXT-008 — URL sin codificar | Abrió `article.ts` y `user.ts`, pero no comparó las llamadas entre sí |
| REV-NEXT-009 — zoom bloqueado | No abrió `_app.tsx` |

### Lectura

- **Coincidencias:** 4 de 9 hallazgos previos (REV-NEXT-001, 002, 005 y 006), con 5 reportes (dos caen en REV-NEXT-005), todas parciales y con la severidad o el impacto corregidos.
- **Aportes nuevos válidos:** 1 (REV-NEXT-010), con la severidad corregida y ampliado tras verificarlo.
- **Errores:** una línea de código inventada dentro de un fragmento presentado como copia; "verificado" en funciones que nadie llama; una afirmación falsa sobre `package.json`; una regla explícita del prompt (mínimo de 20 archivos) que reconoció incumplir y reinterpretó como "máximo".
- **Patrón en las tres pasadas:** las instrucciones de cobertura no cambian su comportamiento. Con el mínimo de 20 archivos lo hizo un poco mejor que en C# (10 archivos frente a 2), pero igual se detuvo en cuanto tuvo 6 hallazgos. La severidad sigue inflada (de sus cinco "Críticas" en las tres pasadas, solo una lo era: el secreto del JWT en Go) y el "verificado" no es confiable.
- **Lección para la política de uso:** pedirle más rigor en el prompt no basta. Lo que funciona es tratar su salida como una lista de **pistas** y verificar cada una contra el SHA, y medir su cobertura con lo que ella misma declara haber abierto. En las tres pasadas su aporte neto fue de 4 hallazgos válidos (3 en Go y 1 en Next), todos con correcciones.
