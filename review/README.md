# Bloque 2 — Code review

## Repositorios revisados

Cada repositorio se revisó en un fork propio, en el SHA indicado. Todos los enlaces a líneas apuntan a ese SHA.

| # | Repositorio original | Fork | Rama | SHA revisado | Último commit | Licencia |
|---|----------------------|------|------|--------------|---------------|----------|
| 1 | [gothinkster/golang-gin-realworld-example-app](https://github.com/gothinkster/golang-gin-realworld-example-app) | [paopadp123/golang-gin-realworld-example-app](https://github.com/paopadp123/golang-gin-realworld-example-app) | `main` | `626c372d259472148d93303f74aa9b9a1cdcef24` | 2026-01-04 | MIT |
| 2 | [realworld-apps/aspnetcore-realworld-example-app](https://github.com/realworld-apps/aspnetcore-realworld-example-app) | [paopadp123/aspnetcore-realworld-example-app](https://github.com/paopadp123/aspnetcore-realworld-example-app) | `master` | `8aa617abed5dd645f8f9d99578a3e07b4e2e52e7` | 2026-10-15 | MIT |
| 3 | [reck1ess/next-realworld-example-app](https://github.com/reck1ess/next-realworld-example-app) | [paopadp123/next-realworld-example-app](https://github.com/paopadp123/next-realworld-example-app) | `master` | `be9ef569bc395975d98c1973fac23ba9277797e7` | 2020-08-06 | Sin licencia declarada |

## Diferencias con el Anexo A (verificado el 2026-10-25)

| Repositorio | Lo que dice el Anexo A (2026-09-02) | Lo que encontré | Por qué importa |
|-------------|-------------------------------------|-----------------|-----------------|
| aspnetcore | `gothinkster/aspnetcore-realworld-example-app` | El repositorio se movió a la organización `realworld-apps`; la URL anterior redirige | Un enlace o un fork automatizado que dependa del nombre viejo puede romperse |
| aspnetcore | `Conduit.sln` en la raíz | No existe: la solución es `Conduit.slnx` (formato XML nuevo de .NET) | Herramientas de CI o de análisis que buscan `*.sln` no la encuentran |
| aspnetcore | Estado al 2 de septiembre | 7 commits posteriores, el último el 2026-10-15 (actualizaciones de Dependabot) | El SHA revisado no es el que verificó el Anexo A; los hallazgos aplican a este SHA |
| aspnetcore | Workflow "Build and Test" | Existe, y además "RealWorld API Tests" | — |
| next | `lib/utils/constant.js`, referenciado en el README | No existe: es `lib/utils/constant.ts`; el README (línea 27) sigue diciendo `.js` | Documentación desactualizada: un desarrollador nuevo sigue una instrucción que no corresponde al código |
| next | Licencia no mencionada | No hay archivo `LICENSE` ni campo `license` en `package.json` | Sin licencia, el código queda con todos los derechos reservados por defecto: reutilizarlo en producción no está permitido sin autorización del autor |
| golang-gin | `common/utils.go` (99 líneas), `hello.go`, `users/routers.go`, `AGENTS.md` | Coinciden; `utils.go` tiene 99 líneas | — |
| golang-gin | ~2.7k estrellas, 545 forks | 2 718 estrellas, 557 forks | Diferencia menor, esperable |
| aspnetcore | ~2.1k estrellas, ~605 forks | 2 094 estrellas, 646 forks | Diferencia menor, esperable |
| next | 825 estrellas, 250 forks, 153 commits | 824 estrellas, 250 forks, 153 commits | Coincide |

## Archivos de instrucciones para agentes de IA (revisados a mano antes de usar IA)

Antes de pasarle cualquier repositorio a una IA, se leyeron a mano sus archivos de instrucciones para agentes, por si traían instrucciones escondidas (inyección de prompts). Además se escanearon en busca de caracteres invisibles (ancho cero, bidi, etiquetas Unicode), comentarios HTML y bloques codificados.

| Repositorio | Archivos | Resultado |
|-------------|----------|-----------|
| golang-gin | `AGENTS.md`, `BACKEND_INSTRUCTIONS.md`, `FRONTEND_INSTRUCTIONS.md`, `MOBILE_INSTRUCTIONS.md` | Sin inyección. Los únicos caracteres no ASCII son los del dibujo del árbol de carpetas; los comentarios HTML son de la plantilla del frontend. `AGENTS.md` se titula "Copilot Coding Agent Instructions" y llegó con la modernización hecha por el agente Copilot (commit `804a013`, 2026-01-04) |
| aspnetcore | `AGENTS.md`, `.github/copilot-csharp-*.md` (6 archivos: buenas prácticas, C#, Docker, políglota, seguridad, pruebas), `.rules4rc` | Sin inyección. El único carácter no ASCII es un apóstrofo tipográfico en `copilot-csharp-docker.md:32`; sin comentarios HTML ni bloques codificados. Describe las puertas de calidad del repositorio (restauración con lock, analizadores, advertencias como errores, formateador) |

Lo que sí aportan como **pistas para verificar en el código** (no son hallazgos hasta comprobarlos):
- golang-gin `AGENTS.md:112` reconoce que el secreto del JWT está definido en `common/utils.go`.
- golang-gin `AGENTS.md:70` exige no ignorar errores con `_`: se contrasta contra el código.
- golang-gin `BACKEND_INSTRUCTIONS.md:1` y `MOBILE_INSTRUCTIONS.md:1` dicen "Delete this file before publishing your app!" y siguen en el repositorio.
- aspnetcore `AGENTS.md:7`: parte de las pruebas de autenticación usa EF Core InMemory, que no se comporta como una base real.

## Hallazgos

Cada hallazgo usa la plantilla del Anexo C. Los marcados como **probado en vivo** se reprodujeron con la aplicación corriendo; los scripts están en [`pruebas-en-vivo/`](pruebas-en-vivo/) (uno por repositorio).

### 1. golang-gin-realworld-example-app (Go + Gin + GORM)

#### REV-GO-001 — Secreto del JWT escrito en el código, publicado y con la alarma del linter silenciada

| Campo | Contenido |
|---|---|
| ID | REV-GO-001 |
| Severidad | **Crítica** (seguridad) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `common/utils.go:40-41` (usado en `users/middlewares.go:60`) |
| Evidencia | [`common/utils.go#L40-L41`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/common/utils.go#L40-L41): `// Keep this two config private, it should not expose to open source` · `const JWTSecret = "A String Very Very Very Strong!!@##$!@#$" // #nosec G101`. El comentario pide no exponerlo y está publicado; `#nosec G101` apaga el aviso de gosec que lo detectaba. El propio `AGENTS.md:112` lo reconoce. **Probado en vivo:** con ese secreto se firmó un token para otro usuario sin conocer su contraseña y `GET /api/user` devolvió sus datos (HTTP 200) y un token nuevo válido |
| Impacto en producción | Cualquiera que lea el repositorio puede **entrar como cualquier usuario**: leer y modificar su perfil, publicar y borrar a su nombre. Es una toma de cuentas masiva sin rastro de contraseña robada, con exposición legal por datos personales (Ley 1581 de 2012) y daño reputacional. Cambiar el secreto exige además invalidar todas las sesiones |
| Recomendación | Leer el secreto de una variable de entorno o de un gestor de secretos y **negarse a arrancar** si falta (sin valor por defecto). Quitar el `#nosec`. Rotar el secreto (todos los tokens emitidos con el actual quedan comprometidos). Agregar una prueba que falle si la configuración no trae secreto |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí**: en producción es una puerta abierta. *Aceptable en un proyecto de ejemplo* para que corra sin configurar nada, **inaceptable en producción**; aun en el ejemplo, silenciar el linter en vez de documentar la excepción es una mala señal |

Origen: humano (pista de `AGENTS.md:112`, confirmada leyendo el código); prueba en vivo preparada con Claude Code.

#### REV-GO-002 — Un JSON mal formado produce un panic y responde 500 en vez de 400

| Campo | Contenido |
|---|---|
| ID | REV-GO-002 |
| Severidad | **Media** (errores) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `common/utils.go:71`, llamado desde `users/routers.go:97` (login) y otros cinco handlers (`users/routers.go:81,125`, `articles/routers.go:41,116,190`) |
| Evidencia | [`common/utils.go#L68-L72`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/common/utils.go#L68-L72): `errs := err.(validator.ValidationErrors)` convierte el error sin verificar el tipo (sin `, ok`). [`users/routers.go#L96-L97`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/routers.go#L96-L97) le pasa cualquier error de `Bind`, incluido un JSON roto. **Probado en vivo:** `POST /api/users/login` con JSON incompleto → **HTTP 500**; log: `interface conversion: error is *errors.errorString, not validator.ValidationErrors`. No tumba el proceso solo porque `gin.Default()` incluye el middleware Recovery |
| Impacto en producción | Un error del cliente aparece como **falla del servidor**: infla la tasa de 5xx, dispara alertas falsas y consume el presupuesto de errores del SLO. Cada request mal formado (o malicioso) escribe un stack trace de ~30 líneas en el log. El cliente recibe un 500 sin explicación en vez de un 400 que le diga qué corregir. Si alguien cambiara Recovery o reutilizara la función fuera de Gin, el panic tumbaría el servicio |
| Recomendación | Usar `errors.As(err, &validationErrs)` y, si no es un error de validación, responder **400** con un mensaje genérico. Prueba de tabla con un JSON roto, un cuerpo vacío y un error de validación |
| Esfuerzo | S |
| ¿Bloquea el merge? | No por sí solo (Recovery evita la caída), pero se corrige en el mismo sprint: es la puerta de entrada pública (login y registro) |

Origen: humano (detectó la falta de manejo de errores en `utils.go`) con apoyo de Claude Code, que precisó la línea y el mecanismo del panic.

#### REV-GO-003 — El middleware de autenticación hace panic si el claim `id` no es numérico

| Campo | Contenido |
|---|---|
| ID | REV-GO-003 |
| Severidad | **Baja**, sube a **Media** mientras exista REV-GO-001 (errores, seguridad) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `users/middlewares.go:71` |
| Evidencia | [`users/middlewares.go#L70-L72`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/middlewares.go#L70-L72): `my_user_id := uint(claims["id"].(float64))` confía en que el claim existe y es número. **Probado en vivo:** un token firmado con `"id":"abc"` → **HTTP 500**; log: `interface conversion: interface {} is string, not float64` en `/src/users/middlewares.go:71` |
| Impacto en producción | Hoy cualquiera puede firmar tokens (REV-GO-001), así que cualquiera puede provocar 500 y stack traces en **todas** las rutas autenticadas. Corregido REV-GO-001, solo el servidor firma tokens y el riesgo baja, pero el middleware de autenticación es la pieza que menos debe confiar en su entrada. Un `id` negativo o enorme además se convierte a `uint` sin control |
| Recomendación | Leer el claim con verificación (`v, ok := claims["id"].(float64)`), validar que sea un entero positivo y responder **401** si no lo es. Mejor aún: claims tipados (`jwt.RegisteredClaims` + struct propio) en vez de `MapClaims` |
| Esfuerzo | S |
| ¿Bloquea el merge? | No, salvo que se corrija junto con REV-GO-001 (mismo archivo de autenticación, misma prueba) |

Origen: Claude Code, confirmado en vivo.

#### REV-GO-004 — El middleware ignora el error de la base y autentica a un usuario que no existe

| Campo | Contenido |
|---|---|
| ID | REV-GO-004 |
| Severidad | **Media** (seguridad, errores) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `users/middlewares.go:34` |
| Evidencia | [`users/middlewares.go#L30-L38`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/middlewares.go#L30-L38): `db.First(&myUserModel, my_user_id)` descarta el error; si el usuario no existe, el contexto queda con un `UserModel` vacío y el request sigue como autenticado. Contradice la regla del propio repositorio (`AGENTS.md:70`: "Always handle errors explicitly"). **Probado en vivo:** un token válido para el id 999999 (inexistente) → `GET /api/user` responde **HTTP 200** con un usuario vacío y **un token nuevo para el id 0**, en vez de 401 |
| Impacto en producción | Dar de baja o bloquear a un usuario **no corta su acceso**: su token sigue aceptado hasta 24 h y el sistema le emite tokens nuevos. Lo mismo pasa si la base falla momentáneamente: en vez de un error, el usuario ve su perfil vacío, lo que genera tickets de soporte y confusión. Las rutas que escriben dependen de que la validación posterior rechace el usuario vacío (hoy `PUT /api/user` responde 422 por suerte, no por diseño) |
| Recomendación | Revisar el error de `First`: si es `gorm.ErrRecordNotFound`, responder **401**; si es otro error, **503** y log. No emitir tokens para el id 0. Prueba de tabla: usuario existente, borrado y base caída |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí** en producción: es el punto donde se decide quién está autenticado. *Aceptable en un ejemplo* que nunca borra usuarios, **inaceptable en producción** |

Origen: humano (siguió el flujo desde `serializers.go` hasta el middleware) con apoyo de Claude Code, que encontró la línea y lo probó en vivo.

#### REV-GO-005 — Contrato implícito en el contexto: claves de texto, `MustGet` y orden de las rutas

| Campo | Contenido |
|---|---|
| ID | REV-GO-005 |
| Severidad | **Baja** (arquitectura) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `users/serializers.go:53`, `users/routers.go:89,111,122`, `hello.go:42-49` |
| Evidencia | [`users/serializers.go#L52-L53`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/serializers.go#L52-L53): `self.c.MustGet("my_user_model").(UserModel)` hace panic si la clave no está o no es de ese tipo. Que esté depende de quién la puso: el registro la pone a mano ([`routers.go#L89`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/routers.go#L89)), el login con `UpdateContextUserModel` ([`routers.go#L111`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/routers.go#L111)) y el resto el middleware, **solo porque** esas rutas se registran después de `v1.Use(AuthMiddleware(true))` ([`hello.go#L42-L49`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/hello.go#L42-L49)). Hoy todas las rutas cumplen; no hay panic en el código actual |
| Impacto en producción | Un cambio inocente (mover una línea de `hello.go`, agregar un handler que olvide el `c.Set`, o escribir mal la clave) compila sin avisos y **solo falla en ejecución**, con un 500 en una ruta de usuarios. El costo es de mantenimiento: cada desarrollador nuevo tiene que conocer una regla que no está escrita en ningún lado |
| Recomendación | Encapsular el acceso en funciones tipadas (`CurrentUser(c) (UserModel, bool)`) con una clave privada del paquete, y que el serializer reciba el usuario como parámetro en vez de leerlo del contexto. Una prueba que recorra las rutas protegidas sin token y verifique 401, no 500 |
| Esfuerzo | M |
| ¿Bloquea el merge? | No: es deuda de diseño, no una falla actual. Razonable en un ejemplo pequeño; en un equipo grande es una fuente segura de regresiones |

Origen: humano (detectó el posible panic de `MustGet`); la verificación de por qué hoy no ocurre se hizo con Claude Code.

#### REV-GO-006 — Suite verde con 87–99 % de cobertura que no prueba ninguno de los caminos de error anteriores

| Campo | Contenido |
|---|---|
| ID | REV-GO-006 |
| Severidad | **Media** (pruebas) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `common/unit_test.go:188-225`, `users/unit_test.go:111-112`, `users/unit_test.go:511-545` |
| Evidencia | [`common/unit_test.go#L188-L225`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/common/unit_test.go#L188-L225): `TestNewValidatorError` solo usa cuerpos JSON bien formados; ninguno roto ni vacío (REV-GO-002). [`users/unit_test.go#L511-L545`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/unit_test.go#L511-L545): el middleware se prueba con token inválido y sin token, pero no con un token válido de un usuario inexistente (REV-GO-004) ni con un claim de tipo inesperado (REV-GO-003). [`users/unit_test.go#L111`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/unit_test.go#L111): "Testing will run one by one, so you can combine it to a user story": los casos dependen del orden y comparten la base del paquete (`TestMain`, `users/unit_test.go:548-553`). **Ejecutado:** `go test -cover ./...` en el SHA → todo en verde, con cobertura de 92,7 % (`articles`), 87,1 % (`common`) y 99,5 % (`users`) |
| Impacto en producción | La cobertura alta da una **falsa sensación de seguridad**: los defectos REV-GO-002 a REV-GO-005 (tres probados en vivo con 500 o acceso indebido) pasan el CI sin que nadie se entere. Un equipo que use la cobertura como meta de calidad aprobaría estos cambios. Además, al depender del orden, un caso que falla arrastra a los siguientes y cuesta encontrar la causa |
| Recomendación | Agregar casos de error a las tablas: JSON roto, cuerpo vacío, token de usuario inexistente, claim con tipo incorrecto, base caída; cada uno con su código esperado (400, 401, 503). Que cada caso prepare su propio estado (sin "historias" encadenadas). Medir la calidad con estos casos de riesgo, no solo con el porcentaje de cobertura |
| Esfuerzo | M |
| ¿Bloquea el merge? | No por sí solo, pero las correcciones de REV-GO-001 a 004 no se aceptan sin su prueba que falle antes y pase después |

Origen: humano (notó que no había pruebas del JSON roto); Claude Code corrió la suite, midió la cobertura y verificó la dependencia del orden.

#### REV-GO-007 — Una contraseña de más de 72 bytes crea la cuenta y la deja bloqueada para siempre

| Campo | Contenido |
|---|---|
| ID | REV-GO-007 |
| Severidad | **Media** (errores, seguridad) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `users/models.go:63`, con el límite de `users/validators.go:16` (`max=255`) |
| Evidencia | [`users/models.go#L57-L66`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/users/models.go#L57-L66): `passwordHash, _ := bcrypt.GenerateFromPassword(bytePassword, bcrypt.DefaultCost)` descarta el error y `setPassword` devuelve `nil`. Con `golang.org/x/crypto v0.32.0`, bcrypt **rechaza las contraseñas de más de 72 bytes** (`ErrPasswordTooLong`), y el validador acepta hasta 255. **Probado en vivo:** registro con 100 caracteres → **HTTP 201** (se guarda un hash vacío); login con la misma contraseña → **HTTP 401** para siempre |
| Impacto en producción | Los usuarios que usan gestores de contraseñas o frases largas **se registran "con éxito" y nunca pueden volver a entrar**: tickets de soporte, abandono en el registro y una cuenta inservible ocupando el correo (no pueden registrarse de nuevo con él). No da acceso indebido: `bcrypt` rechaza el hash vacío |
| Recomendación | Propagar el error de `GenerateFromPassword` y responder 422 con un mensaje claro; alinear el validador con el límite real (`max=72` bytes, o pre-hashear con SHA-256 si se quieren frases largas). Prueba de tabla con 72, 73 y 255 bytes |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí** en producción: rompe el registro para un grupo de usuarios sin avisar. *Aceptable en un ejemplo*, que nadie usa con contraseñas largas |

Origen: IA (Antigravity) **corregido**. Señaló la línea correcta, pero con un mecanismo ("falta de entropía o coste no soportado") y un impacto ("cuentas sin contraseña, accesos no autorizados") equivocados, y como Crítica. La causa real (el límite de 72 bytes de bcrypt contra el `max=255` del validador) y el impacto (cuenta bloqueada) se encontraron al probarlo en vivo con Claude Code.

#### REV-GO-008 — Si la base no abre, la aplicación sigue y se cae con un `nil pointer`

| Campo | Contenido |
|---|---|
| ID | REV-GO-008 |
| Severidad | **Baja** (producción, errores) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `common/database.go:53-63` |
| Evidencia | [`common/database.go#L53-L63`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/common/database.go#L53-L63): los tres errores de inicialización (crear la carpeta, `gorm.Open`, `db.DB()`) solo se imprimen con `fmt.Println` y la función devuelve igual. **Probado en vivo:** con una ruta de base inválida, el proceso imprime `db err: (Init) unable to open database file` y luego muere con `panic: runtime error: invalid memory address or nil pointer dereference` al migrar |
| Impacto en producción | El servicio no arranca, que es lo correcto, pero **el error que ve operaciones es un `nil pointer`** en el migrador y no "no se pudo abrir la base". En una caída a las 3 a. m. eso alarga el diagnóstico. El mismo patrón (`fmt.Println` en vez de un logger) deja estos errores fuera de cualquier sistema de logs estructurados |
| Recomendación | Que `Init` devuelva `(*gorm.DB, error)` y que `main` termine con un mensaje claro (`log.Fatalf`). Reemplazar `fmt.Println` por un logger estructurado |
| Esfuerzo | S |
| ¿Bloquea el merge? | No: falla de todos modos al arrancar, solo con un mensaje confuso |

Origen: IA (Antigravity) confirmado (señaló `ensureDir`); Claude Code amplió a `gorm.Open` y `db.DB()`, que son más graves, y lo probó en vivo.

#### REV-GO-009 — Sin apagado ordenado: un despliegue corta las peticiones en curso

| Campo | Contenido |
|---|---|
| ID | REV-GO-009 |
| Severidad | **Baja** (producción) |
| Repositorio y SHA | `paopadp123/golang-gin-realworld-example-app@626c372` |
| Archivo y línea | `hello.go:67` |
| Evidencia | [`hello.go#L67-L69`](https://github.com/paopadp123/golang-gin-realworld-example-app/blob/626c372d259472148d93303f74aa9b9a1cdcef24/hello.go#L67-L69): `r.Run(":" + port)` no escucha `SIGTERM` ni usa `http.Server.Shutdown`. Al recibir `SIGTERM`, Go termina el proceso de inmediato |
| Impacto en producción | En cada despliegue o escalado hacia abajo (ECS, Kubernetes), las peticiones en curso se cortan: el usuario ve un error al guardar un artículo o al registrarse justo en ese momento. Con despliegues frecuentes se vuelve un goteo constante de errores |
| Recomendación | `http.Server` con `signal.NotifyContext` y `Shutdown(ctx)` con un tiempo de gracia menor al del orquestador de contenedores |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: IA (Antigravity) **corregido**: el hallazgo es válido, pero el impacto que describió ("el proceso puede quedar colgado, pods que no terminan") es el contrario de lo que pasa en Go, que termina de inmediato y corta las peticiones.

**Nota de verificación:** en un primer intento la suite falló en paralelo, pero la causa fue la configuración de la prueba (una variable `TEST_DB_PATH` absoluta que hizo compartir la base entre paquetes) y correr como root (una prueba quita permisos al archivo y root los ignora). Con la configuración por defecto y un usuario normal, todo pasa. No se reporta como hallazgo.

**Pista descartada:** `common.GenToken` (`common/utils.go:51-55`) traga el error de firma y devuelve un token vacío que `users/serializers.go:63` entregaría sin revisar. Se descarta como hallazgo porque firmar con HS256 y una clave en memoria no falla en la práctica; queda como observación de estilo (usar el logger y devolver el error).

**Fortaleza observada:** `users/middlewares.go:57` valida que el algoritmo de firma sea HMAC antes de aceptar el token; eso evita el ataque clásico de `alg: none`.

### 2. aspnetcore-realworld-example-app (C# + ASP.NET Core + EF Core)

Este repositorio está mucho más trabajado en seguridad que el de Go (ver las fortalezas al final). Los hallazgos están en lo que queda: límites de entrada, configuración de producción y operación. Las pruebas en vivo están en [`pruebas-en-vivo/aspnetcore.sh`](pruebas-en-vivo/aspnetcore.sh).

#### REV-NET-001 — `limit` y `offset` sin validar: `limit=-1` devuelve todos los artículos

| Campo | Contenido |
|---|---|
| ID | REV-NET-001 |
| Severidad | **Alta** (seguridad / producción) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Features/Articles/List.cs:74-75` (entrada en `ArticlesController.cs:19-20`) |
| Evidencia | [`List.cs#L74-L75`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Articles/List.cs#L74-L75): `.Skip(message.Offset ?? 0)` · `.Take(message.Limit ?? 20)`. `List.Query` no tiene validador (las demás consultas sí), así que el valor llega tal cual a la base. **Probado en vivo:** `limit=1` → 1 artículo; `limit=-1` → **todos** (SQLite interpreta `LIMIT -1` como "sin límite"); `limit=1000000` → HTTP 200 |
| Impacto en producción | Una sola petición anónima obliga a leer y serializar la tabla completa con sus etiquetas, favoritos y autores. Con miles de artículos, unas cuantas peticiones repetidas agotan la memoria y la CPU del servicio y lo tumban para todos (denegación de servicio sin autenticarse). También sirve para extraer el catálogo entero de un tirón |
| Recomendación | Validador para `List.Query`: `limit` entre 1 y 100 y `offset >= 0`, con 422 si no se cumple. Prueba con `-1`, `0`, `101` y un valor enorme |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí**: es un endpoint público sin autenticación. *Aceptable en un proyecto de ejemplo* con pocos datos, **inaceptable en producción** |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana.

#### REV-NET-002 — Login sin límite de intentos y con una diferencia de tiempo que revela qué correos existen

| Campo | Contenido |
|---|---|
| ID | REV-NET-002 |
| Severidad | **Alta** (seguridad) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Features/Users/Login.cs:55-67`; `src/Conduit/Program.cs:76-83` (pipeline sin limitador) |
| Evidencia | [`Login.cs#L55-L67`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Users/Login.cs#L55-L67): si el correo no existe responde 401 de inmediato; si existe, calcula PBKDF2 con 210 000 iteraciones antes de responder. En [`Program.cs#L76-L83`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Program.cs#L76-L83) no hay `AddRateLimiter` ni bloqueo por intentos. **Probado en vivo:** 30 contraseñas equivocadas seguidas → 30 × HTTP 401, ningún 429; correo existente **0,128 s** vs. inexistente **0,004 s** |
| Impacto en producción | Permite probar contraseñas sin freno contra cualquier cuenta (relleno de credenciales con listas filtradas) y armar la lista de clientes registrados midiendo tiempos. Además, cada intento cuesta ~120 ms de CPU: un atacante convierte el login en una forma barata de saturar el servidor. Toma de cuentas, exposición de datos personales (Ley 1581 de 2012) y caída del servicio |
| Recomendación | `AddRateLimiter` por IP y por cuenta en `/users/login` (p. ej. 5 intentos por minuto, luego 429 con `Retry-After`). Verificar contra un hash ficticio cuando el correo no existe para igualar el tiempo. Alertar sobre picos de 401 |
| Esfuerzo | M |
| ¿Bloquea el merge? | **Sí** para producción. *Aceptable en un proyecto de ejemplo*; **inaceptable en producción**, donde el login es la superficie más atacada |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana.

#### REV-NET-003 — El token vence a los 5 minutos y no hay forma de renovarlo

| Campo | Contenido |
|---|---|
| ID | REV-NET-003 |
| Severidad | **Media** (producción / experiencia del usuario) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Infrastructure/Security/JwtIssuerOptions.cs:71`; `src/Conduit/ServicesExtensions.cs:150` |
| Evidencia | [`JwtIssuerOptions.cs#L71`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Infrastructure/Security/JwtIssuerOptions.cs#L71): `ValidFor { get; set; } = TimeSpan.FromMinutes(5)`; nada en el repositorio lo configura. [`ServicesExtensions.cs#L150`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/ServicesExtensions.cs#L150): `ClockSkew = TimeSpan.Zero`. La API no tiene endpoint de renovación. **Probado en vivo:** `exp - nbf = 300` segundos |
| Impacto en producción | Un usuario que tarda más de 5 minutos escribiendo un artículo recibe 401 al publicarlo y **pierde lo que escribió**; los frontends de RealWorld guardan el token y no lo renuevan. Con `ClockSkew` en cero, un desfase de reloj de pocos segundos entre servidores rechaza tokens recién emitidos. Más tickets de soporte y abandono |
| Recomendación | Decidir la política de sesión: token de acceso corto **más** token de renovación (rotado y revocable), o una vigencia mayor configurable por entorno. Dar un margen de reloj de 30–60 s. Documentarlo en el contrato de la API |
| Esfuerzo | M |
| ¿Bloquea el merge? | No como defecto de seguridad (un token corto es más seguro), pero sí exige una decisión de producto antes de salir a producción |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana. Antigravity lo tocó en parte ("sin token de renovación") pero con el impacto invertido: habló de sesiones largas, cuando el problema es que son demasiado cortas.

#### REV-NET-004 — El slug borra las tildes y la ñ, y un título sin letras ASCII deja el artículo sin URL

| Campo | Contenido |
|---|---|
| ID | REV-NET-004 |
| Severidad | **Media** (errores) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Infrastructure/Slug.cs:16` y `:25` |
| Evidencia | [`Slug.cs#L16`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Infrastructure/Slug.cs#L16) aplica [`[^a-z0-9\s-]`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Infrastructure/Slug.cs#L25) y **elimina** cualquier letra no ASCII en vez de transliterarla. **Probado en vivo:** "Póliza de vehículo" → `pliza-de-vehculo`; "¿¡!?" → slug **vacío** (`""`), y el segundo con ese título → `-1`. `GET /api/articles/` cae en el listado y `DELETE /api/articles/` responde 405: el artículo con slug vacío no se puede abrir, editar ni borrar por la API |
| Impacto en producción | En un producto en español las URL quedan mal escritas ("pliza", "vehculo"), lo que afecta SEO y confianza. Un título en otro alfabeto, o solo con signos, crea un artículo **huérfano** que su autor no puede gestionar y que soporte tiene que borrar a mano en la base |
| Recomendación | Normalizar a forma D y quitar los diacríticos (`ó` → `o`, `ñ` → `n`) antes de filtrar; si el resultado queda vacío, usar un respaldo (p. ej. un id corto). Pruebas con tildes, ñ, emoji y solo signos |
| Esfuerzo | S |
| ¿Bloquea el merge? | No, pero se corrige antes de abrir el producto a usuarios hispanohablantes |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana.

#### REV-NET-005 — Sin longitud máxima en artículos, etiquetas y comentarios

| Campo | Contenido |
|---|---|
| ID | REV-NET-005 |
| Severidad | **Media** (seguridad / producción) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Features/Articles/Create.cs:32-34`; `src/Conduit/Features/Comments/Create.cs:25`; `src/Conduit/Features/Articles/Edit.cs:84-85` |
| Evidencia | [`Articles/Create.cs#L32-L34`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Articles/Create.cs#L32-L34): título, descripción y cuerpo solo exigen `NotEmpty()`; las etiquetas no tienen regla. [`Comments/Create.cs#L25`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Comments/Create.cs#L25) igual. Además, [`Articles/Edit.cs#L84-L85`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Articles/Edit.cs#L84-L85) acepta descripción y cuerpo vacíos, que la creación rechaza. El modelo sí limita `Username` y `Email` (`ConduitContext.cs:33-34`): el criterio existe pero no se aplicó al contenido. **Probado en vivo:** artículo con cuerpo de 5 MB → HTTP 201 |
| Impacto en producción | Cualquier usuario registrado puede llenar la base con contenido gigante (el límite real es el de Kestrel, ~30 MB por petición), encarecer el almacenamiento y el ancho de banda, y volver lentas las páginas que lo muestran. La regla inconsistente entre crear y editar deja datos que el resto del sistema considera inválidos |
| Recomendación | `MaximumLength` en título, descripción, cuerpo, cada etiqueta y el número de etiquetas, y en el comentario; aplicar las mismas reglas al editar (reutilizar `ArticleDataValidator`). Límite de tamaño de petición acorde al caso de uso |
| Esfuerzo | S |
| ¿Bloquea el merge? | No, pero se corrige antes de producción |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana.

#### REV-NET-006 — Migraciones automáticas al arrancar y SQLite por defecto: el diseño no escala a varias réplicas

| Campo | Contenido |
|---|---|
| ID | REV-NET-006 |
| Severidad | **Media** (arquitectura) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Program.cs:91-96`; `Dockerfile:15-16`; `docker-compose.yml:6-7` |
| Evidencia | [`Program.cs#L91-L96`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Program.cs#L91-L96): `GetValue("Database:ApplyMigrations", true)` → cada instancia que arranca ejecuta `MigrateAsync`. [`Dockerfile#L15-L16`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/Dockerfile#L15-L16) fija SQLite en un archivo local (`/data/realworld.db`) como opción por defecto de la imagen |
| Impacto en producción | Con dos o más réplicas, todas intentan migrar a la vez al desplegar: bloqueos, despliegues que fallan a medias o una migración destructiva que se aplica sin que nadie la apruebe. Con SQLite en un volumen local, cada réplica tendría **su propia base**: los usuarios verían datos distintos según la instancia que los atienda. No se puede escalar horizontalmente sin cambiar el diseño |
| Recomendación | Migraciones como paso separado del despliegue (job o tarea previa, con `ApplyMigrations=false` en las réplicas). Base gestionada (SQL Server o PostgreSQL) como opción por defecto fuera de desarrollo, y que la aplicación se niegue a arrancar con SQLite en producción |
| Esfuerzo | M |
| ¿Bloquea el merge? | No en el ejemplo; **sí** antes de desplegar con más de una réplica. *Aceptable en un proyecto de ejemplo*, **inaceptable en producción** |

Origen: Claude Code (lectura del código); pendiente de confirmación humana.

#### REV-NET-007 — Logs duplicados, en texto plano y con un "solo para depurar en local" que llega a producción

| Campo | Contenido |
|---|---|
| ID | REV-NET-007 |
| Severidad | **Media** (producción / observabilidad) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Program.cs:74`; `src/Conduit/ServicesExtensions.cs:245-259` |
| Evidencia | [`ServicesExtensions.cs#L245-L259`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/ServicesExtensions.cs#L245-L259): Serilog con `MinimumLevel.Verbose()`, salida a consola con colores ANSI y el comentario `//just for local debug`; [`Program.cs#L74`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Program.cs#L74) lo agrega **además** del proveedor de consola por defecto, en todos los entornos. **Observado en el log de la prueba en vivo** (no lo reproduce el script): 799 eventos `info:` y 799 eventos `[Information]` idénticos: cada evento se escribe **dos veces**. No hay JSON, id de correlación ni endpoint de salud |
| Impacto en producción | El doble de volumen en el agregador de logs (CloudWatch cobra por GB ingerido), consultas más difíciles y ningún campo estructurado para filtrar por usuario o petición. Sin `/health`, el balanceador no puede saber si una instancia está sana. Investigar un incidente toma más tiempo |
| Recomendación | Un solo proveedor (`builder.Host.UseSerilog` leyendo la configuración), formato JSON fuera de desarrollo, nivel por entorno, id de correlación por petición y `AddHealthChecks` con verificación de la base |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (lectura del código y conteo en el log de la prueba en vivo); pendiente de confirmación humana. Antigravity vio la falta de health checks, pero presentó la configuración de Serilog como una fortaleza.

#### REV-NET-008 — CORS abierto a cualquier origen y Swagger publicado en todos los entornos

| Campo | Contenido |
|---|---|
| ID | REV-NET-008 |
| Severidad | **Baja** (seguridad) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Program.cs:79` y `:86-89` |
| Evidencia | [`Program.cs#L79`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Program.cs#L79): `AllowAnyOrigin().AllowAnyHeader().AllowAnyMethod()`. [`Program.cs#L86-L89`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Program.cs#L86-L89): Swagger sin condición de entorno. **Probado en vivo:** con `Origin: https://sitio-malicioso.example` responde `Access-Control-Allow-Origin: *`; `/swagger/v1/swagger.json` → HTTP 200 |
| Impacto en producción | Riesgo **bajo** hoy porque el token viaja en un encabezado y no en una cookie (un sitio ajeno no puede usar la sesión del usuario). Pero si mañana se pasa a cookies, el CORS abierto se vuelve un problema real sin que nadie toque esta línea. Swagger público le entrega a un atacante el mapa completo de la API |
| Recomendación | Lista de orígenes permitidos por configuración; Swagger solo en desarrollo o detrás de autenticación |
| Esfuerzo | S |
| ¿Bloquea el merge? | No. *Aceptable en un proyecto de ejemplo*; en producción se configura por entorno |

Origen: Claude Code (lectura del código y prueba en vivo); pendiente de confirmación humana. Antigravity lo reportó como **Crítica** por "CSRF", lo que no aplica porque el token no viaja en una cookie; se corrigió a Baja.

#### REV-NET-009 — Los errores de formato exponen tipos internos de .NET y salen del contrato de errores

| Campo | Contenido |
|---|---|
| ID | REV-NET-009 |
| Severidad | **Baja** (errores / seguridad) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Infrastructure/ValidatorActionFilter.cs:18-21` |
| Evidencia | [`ValidatorActionFilter.cs#L18-L21`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Infrastructure/ValidatorActionFilter.cs#L18-L21) copia tal cual las claves y los mensajes del `ModelState`. **Probado en vivo** (prueba 7 de `aspnetcore.sh`): un título con bytes no UTF-8 devolvió `{"errors":{"command":["The command field is required."],"$.article.title":["The JSON value could not be converted to System.String. Path: $.article.title \| LineNumber: 0 \| BytePositionInLine: 26."]}}` |
| Impacto en producción | El cliente recibe claves (`command`, `$.article.title`) que no existen en el contrato (`title`), así que el frontend no puede mostrar el error junto al campo. Los mensajes revelan el lenguaje, la librería y la posición del parser, información útil para quien prueba la API buscando fallas |
| Recomendación | Mapear los errores de binding al mismo formato que `ErrorHandlingMiddleware` (`ToFieldName`) con un mensaje genérico ("is invalid"), y registrar el detalle solo en el log |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (encontrado por accidente durante la prueba en vivo del slug); pendiente de confirmación humana.

#### REV-NET-010 — Comentar o borrar un comentario carga todos los comentarios del artículo

| Campo | Contenido |
|---|---|
| ID | REV-NET-010 |
| Severidad | **Baja** (rendimiento) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Features/Comments/Create.cs:36-38`; `src/Conduit/Features/Comments/Delete.cs:27-31`; `src/Conduit/Features/Articles/Create.cs:59-68` y `:75-82` |
| Evidencia | [`Comments/Create.cs#L36-L38`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Comments/Create.cs#L36-L38) y [`Comments/Delete.cs#L27-L31`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Comments/Delete.cs#L27-L31): `Include(x => x.Comments)` (y sus autores) para agregar o borrar **uno**. [`Articles/Create.cs#L59-L68`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Articles/Create.cs#L59-L68): un `FindAsync` y un `SaveChangesAsync` por etiqueta; [`#L75-L82`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Features/Articles/Create.cs#L75-L82): busca un slug libre con una consulta por cada artículo que ya tenga el mismo título |
| Impacto en producción | El costo de comentar crece con el número de comentarios: justo los artículos más populares son los más lentos. Un artículo con 50 etiquetas hace ~100 viajes a la base dentro de una transacción serializable en SQLite, bloqueando otras escrituras mientras tanto |
| Recomendación | Buscar el artículo por slug sin `Include` e insertar el comentario con su `ArticleId`; borrar con `Where(CommentId, ArticleId)`. Etiquetas en una sola consulta (`Where(TagId in ...)`) y un único `SaveChanges`. Slug único con sufijo aleatorio corto en vez del bucle |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (lectura del código); pendiente de confirmación humana.

#### REV-NET-011 — `launchSettings.json` define variables que el código ya no lee y una ruta de otra máquina

| Campo | Contenido |
|---|---|
| ID | REV-NET-011 |
| Severidad | **Baja** (mantenibilidad) |
| Repositorio y SHA | `paopadp123/aspnetcore-realworld-example-app@8aa617a` |
| Archivo y línea | `src/Conduit/Properties/launchSettings.json:23-24` |
| Evidencia | [`launchSettings.json#L23-L24`](https://github.com/paopadp123/aspnetcore-realworld-example-app/blob/8aa617abed5dd645f8f9d99578a3e07b4e2e52e7/src/Conduit/Properties/launchSettings.json#L23-L24): `DB_CONNECTION_STRING` (con `C:\Users\USER\source\repos\...`) y `DB_TYPE`. El código lee `Database:Provider`, `Conduit_DatabaseProvider`, `ConnectionStrings:Conduit` y `Conduit_ConnectionString` (`ServicesExtensions.cs:32-38`); ninguna de las dos variables se usa |
| Impacto en producción | Un desarrollador nuevo cambia `DB_TYPE` a `sqlserver` desde el IDE, la aplicación sigue en SQLite sin avisar y pierde tiempo depurando. Es configuración muerta que contradice la real |
| Recomendación | Quitar las dos variables o reemplazarlas por `Database__Provider` y `ConnectionStrings__Conduit` |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (lectura del código); pendiente de confirmación humana.

**Fortalezas observadas** (útiles para la comparación entre stacks):
- La clave de firma del JWT es obligatoria y se valida: mínimo 32 bytes, variedad de bytes, sin patrones repetidos y sin la clave de ejemplo histórica; si falla, la aplicación no arranca (`ServicesExtensions.cs:97-120`). `docker-compose.yml:5` exige definirla. Es exactamente lo que le falta al repositorio de Go (REV-GO-001).
- Validación estricta del token: algoritmo fijado a HS256, emisor, audiencia, vencimiento, formato del `sub` y **verificación de que el usuario todavía existe** (`ServicesExtensions.cs:133-194`). Cubre lo que en Go es REV-GO-003 y REV-GO-004.
- Contraseñas con PBKDF2 de ASP.NET Identity (210 000 iteraciones) y migración transparente de los hashes antiguos al iniciar sesión (`PasswordHasher.cs:13-15`, `Login.cs:74-79`).
- Índices únicos en usuario, correo y slug, y las violaciones se traducen a 409 en vez de 500 (`ConduitContext.cs:171-201`): la condición de carrera entre "verificar y luego insertar" queda resuelta por la base.
- Imagen con usuario no root (`Dockerfile:17`) y restauración de paquetes con archivo de bloqueo (`--locked-mode`).

**Pista revisada y descartada:** `AGENTS.md:7` advierte que parte de las pruebas de autenticación usa EF Core InMemory. En este SHA la suite principal (`SliceFixture.cs`) usa SQLite en memoria, que sí aplica restricciones relacionales; InMemory queda solo en tres archivos de pruebas del token (`SigningKeyTests`, `TokenFormatTests`, `TokenIdentityTests`), cuya única consulta (si el usuario existe, por id) se comporta igual en InMemory que en una base real. No se reporta como hallazgo.

### 3. next-realworld-example-app (Next.js + TypeScript)

Repositorio sin cambios desde el 2020-08-06 y sin licencia (ver "Diferencias con el Anexo A"). Se revisó el código y se ejecutó la aplicación (Next 9.5 en modo desarrollo, con `--openssl-legacy-provider` en Node 24) contra la API de C# local, porque el backend que trae configurado ya no existe. Las pruebas en vivo están en [`pruebas-en-vivo/next.sh`](pruebas-en-vivo/next.sh).

#### REV-NEXT-001 — El Markdown se protege con una opción obsoleta: actualizar la librería abre un XSS almacenado, y el token vive en `localStorage`

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-001 |
| Severidad | **Alta** (seguridad) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `pages/article/[pid].tsx:29-31` y `:44`; `components/profile/LoginForm.tsx:34` |
| Evidencia | [`[pid].tsx#L29-L31`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/pages/article/%5Bpid%5D.tsx#L29-L31): `__html: marked(article.body, { sanitize: true })`, que se inyecta con `dangerouslySetInnerHTML` en la línea 44. La opción `sanitize` está obsoleta desde marked 0.7.0 (la propia librería avisa en consola que "should not be used and will be removed"). [`LoginForm.tsx#L34`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/components/profile/LoginForm.tsx#L34) guarda el usuario con su token en `localStorage`. **Probado en vivo:** con la versión fijada (1.1.1), `<img src=x onerror=...>` sale escapado; con la versión actual (18.1.2), la opción se ignora en silencio y sale **la etiqueta tal cual** |
| Impacto en producción | Hoy la defensa se sostiene, pero es frágil: marked 1.1.1 tiene CVE de denegación de servicio (REV-NEXT-003), así que la actualización es obligatoria, y la que propondría `npm audit fix` o Dependabot convierte cualquier artículo en un **XSS almacenado** sin que nadie toque este archivo. Como el token está en `localStorage`, el script de un atacante lo lee y **toma la cuenta** de cada lector del artículo |
| Recomendación | Sanitizar el HTML explícitamente con DOMPurify (o renderizar con `react-markdown` sin HTML crudo) y agregar una prueba con payloads XSS que falle si alguien quita la protección. A mediano plazo, sesión en una cookie `httpOnly` detrás de un BFF |
| Esfuerzo | S (sanitizador y prueba), M (cookie httpOnly) |
| ¿Bloquea el merge? | **Sí** cualquier actualización de `marked` que no venga con su sanitizador. *Aceptable en un ejemplo de 2020*; **inaceptable en producción** |

Origen: Claude Code (lectura del código y prueba de la librería en las dos versiones); pendiente de confirmación humana. Antigravity lo vio en parte: señaló la opción obsoleta, pero afirmó que hoy pasa un `<script>`, lo que es falso con la versión fijada. No vio que el riesgo real está en la actualización.

#### REV-NEXT-002 — El backend y los estilos están fijos en el código y apuntan a dominios que ya no funcionan

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-002 |
| Severidad | **Alta** (producción) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `lib/utils/constant.ts:1` y `:10`; `pages/_document.tsx:145-149` |
| Evidencia | [`constant.ts#L1`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/utils/constant.ts#L1): `` SERVER_BASE_URL = `https://conduit.productionready.io/api` ``, sin variable de entorno. [`_document.tsx#L145-L149`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/pages/_document.tsx#L145-L149): hojas de estilo de dominios externos, sin `integrity` (SRI). **Probado en vivo:** la API redirige (307) a `api.realworld.io`, que responde **HTTP 530**; `demo.productionready.io/main.css` responde **404**. La aplicación se ve sin estilos y no carga datos |
| Impacto en producción | La aplicación **dejó de funcionar sin que nadie cambiara su código**: depende de servicios de terceros que se apagaron. No hay forma de apuntarla a otro backend (staging, producción) sin recompilar. Si el dominio de los estilos cambiara de dueño, podría servir CSS malicioso a todos los usuarios, porque no hay SRI |
| Recomendación | `NEXT_PUBLIC_API_URL` por entorno, validada al arrancar. Estilos e íconos empaquetados con la aplicación o, si se quedan en un CDN, con `integrity` y `crossorigin` |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí**: tal como está, la aplicación no funciona |

Origen: Claude Code (lectura del código y peticiones en vivo a los dominios); pendiente de confirmación humana. Antigravity lo reportó como Media por la falta de configuración por entorno, sin notar que los dominios ya no funcionan.

#### REV-NEXT-003 — Repositorio abandonado: 106 vulnerabilidades conocidas en las dependencias

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-003 |
| Severidad | **Alta** (seguridad / mantenimiento) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `package.json:10-17` (versiones resueltas en `package-lock.json`) |
| Evidencia | [`package.json#L10-L17`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/package.json#L10-L17): `next` 9.5.1, `axios` 0.19.2, `marked` 1.1.1, `react` 16.13.1. Último commit: 2020-08-06. **Probado:** `npm audit --package-lock-only` → **106 vulnerabilidades: 10 críticas, 39 altas**, 51 moderadas y 6 bajas. Entre ellas: Next.js (redirección abierta, caída del servidor, SSRF), axios (SSRF, ReDoS, fuga de credenciales) y marked (ReDoS) |
| Impacto en producción | Exposición a fallas públicas con exploits conocidos y sin parches posibles dentro de la versión mayor actual. Next 9 está fuera de soporte desde hace años: actualizar es una migración de varias versiones mayores de Next y React, no un cambio menor. Para un equipo es deuda que bloquea cualquier evolución |
| Recomendación | No adoptarlo como base. Si se usa como referencia, planear la migración por etapas con pruebas antes de tocar dependencias. Activar Dependabot y `npm audit` en CI con umbral en "high" |
| Esfuerzo | L |
| ¿Bloquea el merge? | **Sí** para producción. *Aceptable como material de estudio*, **inaceptable como base de un producto** |

Origen: Claude Code (`npm audit` sobre el lockfile del SHA); pendiente de confirmación humana.

#### REV-NEXT-004 — El manejo de errores de los formularios está roto: publicar un artículo válido rompe el editor

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-004 |
| Severidad | **Media** (errores / experiencia del usuario) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `pages/editor/new.tsx:37-48`; `components/common/ListErrors.tsx:5`; `pages/editor/[pid].tsx:43-59`; `components/profile/SettingsForm.tsx:46-61` |
| Evidencia | [`new.tsx#L44-L45`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/pages/editor/new.tsx#L44-L45): `if (status !== 200) setErrors(data.errors)`, pero crear responde **201** y sin `errors`, así que el estado queda en `undefined` y [`ListErrors.tsx#L5`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/components/common/ListErrors.tsx#L5) hace `Object.keys(undefined)`. En el caso contrario (422, 401), axios lanza una excepción que nadie captura ([`[pid].tsx#L43-L59`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/pages/editor/%5Bpid%5D.tsx#L43-L59), [`SettingsForm.tsx#L46-L61`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/components/profile/SettingsForm.tsx#L46-L61)): la rama `status !== 200` nunca se ejecuta y `setLoading(false)` tampoco. **Probado en vivo:** al publicar un artículo válido, la API respondió 201 y lo creó, y el navegador registró `TypeError: Cannot convert undefined or null to object at ListErrors` |
| Impacto en producción | El camino feliz (publicar) termina en una excepción de React; en un build de producción, sin un error boundary propio, eso desmonta la página (inferido, no se probó el build). En los errores reales (título vacío, sesión vencida), el usuario no ve ningún mensaje y el botón queda deshabilitado: cree que se colgó, recarga y **pierde lo que escribió**. Tickets de soporte y abandono justo en la acción principal del producto |
| Recomendación | Un solo cliente HTTP que convierta las respuestas de error en un resultado tipado; `ListErrors` tolerante a `undefined`; `try/finally` para `setLoading`. Pruebas de componente para 201, 422 y 401 |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí**: rompe la acción principal |

Origen: Claude Code (lectura del código y prueba en el navegador contra la API de C# local); pendiente de confirmación humana.

#### REV-NEXT-005 — No hay capa de sesión ni de cliente HTTP: el token se lee de tres formas y se arma a mano en 13 lugares

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-005 |
| Severidad | **Media** (arquitectura) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `lib/utils/storage.ts:1-4`; `lib/utils/fetcher.ts:6-10`; `lib/api/user.ts:7-8`, `:52-62`; `lib/api/article.ts:32-33`, `:47-48` |
| Evidencia | La sesión se lee con `storage("user")` vía SWR ([`storage.ts#L1-L4`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/utils/storage.ts#L1-L4)), con `window.localStorage.user` ([`fetcher.ts#L6-L10`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/utils/fetcher.ts#L6-L10)) y con `getItem` sin `JSON.parse` ([`user.ts#L7-L8`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/api/user.ts#L7-L8), que manda `Token undefined` a una URL relativa). El encabezado `Authorization` se arma a mano **13 veces en 8 archivos**, unas con `encodeURIComponent` y otras sin él. `UserAPI.save` y `ArticleAPI.favorite/unfavorite` ([`user.ts#L52-L62`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/api/user.ts#L52-L62), [`article.ts#L32-L33`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/api/article.ts#L32-L33)) no envían el token: no fallan hoy solo porque **nadie las usa**; los componentes llaman a axios por su cuenta |
| Impacto en producción | Cambiar algo de la sesión (renovar el token, pasar a cookie, manejar el 401 de forma global) obliga a tocar 8 archivos, y cada copia diverge. La capa `lib/api` parece la oficial pero es código muerto con errores: el próximo desarrollador que la reutilice introduce fallas de autenticación difíciles de rastrear |
| Recomendación | Una instancia de axios con un interceptor que agregue el token y maneje el 401; un hook `useSession` como única fuente de la sesión; que los componentes usen solo `lib/api`; borrar el código muerto |
| Esfuerzo | M |
| ¿Bloquea el merge? | No, pero es requisito antes de cambiar el manejo de la sesión (ver REV-NEXT-001) |

Origen: Claude Code (lectura del código y conteo con grep); pendiente de confirmación humana. Antigravity reportó dos piezas sueltas de este hallazgo (`user.ts:7-8` como **Crítica** y `favorite/unfavorite` como Alta) sin notar que ese código no se usa; uno de sus fragmentos incluía una línea que no existe en el archivo.

#### REV-NEXT-006 — Sin pruebas, sin linter, sin CI y con TypeScript en modo permisivo

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-006 |
| Severidad | **Media** (pruebas) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `package.json:5-9`; `tsconfig.json:16`, `:22-23` |
| Evidencia | [`package.json#L5-L9`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/package.json#L5-L9): solo `dev`, `build` y `start`; no hay pruebas, ESLint ni `.github/workflows`. [`tsconfig.json#L16`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/tsconfig.json#L16) y [`#L22-L23`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/tsconfig.json#L22-L23): `noImplicitAny: false`, `strict: false`, `suppressImplicitAnyIndexErrors: true`. La mayoría de los props y parámetros no tienen tipo (`({ article })`, `async (slug)`) |
| Impacto en producción | REV-NEXT-004 y los errores de REV-NEXT-005 son justo lo que detectaría una prueba de componente o `strict: true` (por ejemplo, `Object.keys` de un posible `undefined`). Sin CI, nada impide que una regresión llegue a producción; TypeScript da la apariencia de seguridad de tipos sin darla |
| Recomendación | `strict: true` de forma gradual, ESLint con las reglas de Next y React Hooks, pruebas con Testing Library para login, editor y artículo (incluido el XSS de REV-NEXT-001), y un workflow que corra lint, tipos, pruebas y `npm audit` en cada PR |
| Esfuerzo | M |
| ¿Bloquea el merge? | No por sí solo, pero ningún cambio de los anteriores debería entrar sin las pruebas que lo cubran |

Origen: Claude Code (lectura del código); pendiente de confirmación humana. Antigravity reportó la falta de tipos en `login` (Media), que es un síntoma de este hallazgo.

#### REV-NEXT-007 — Sin licencia: no se puede reutilizar legalmente

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-007 |
| Severidad | **Media** (cumplimiento) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | Raíz del repositorio (no hay `LICENSE`); `package.json` sin campo `license` |
| Evidencia | [Árbol del repositorio en el SHA](https://github.com/paopadp123/next-realworld-example-app/tree/be9ef569bc395975d98c1973fac23ba9277797e7): no hay archivo de licencia; `package.json` no declara `license`. Los otros dos repositorios revisados son MIT |
| Impacto en producción | Sin licencia, el código queda con todos los derechos reservados por su autor: copiarlo o usarlo como base de un producto comercial expone a la empresa a un reclamo legal. Un equipo que lo tome "porque es público" asume ese riesgo sin saberlo |
| Recomendación | No reutilizar el código. Usarlo solo como referencia de ideas o pedirle una licencia al autor. Agregar a la DoD del equipo la verificación de licencias de dependencias y de código de terceros |
| Esfuerzo | S |
| ¿Bloquea el merge? | **Sí** para incorporar su código a un producto |

Origen: detectado al contrastar el Anexo A (ver "Diferencias con el Anexo A"); se registra aquí como hallazgo con su impacto.

#### REV-NEXT-008 — URL armadas sin codificar en la mitad de las llamadas a la API

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-008 |
| Severidad | **Baja** (errores) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `lib/api/user.ts:73`, `:91`, `:103`; `lib/api/article.ts:26`, `:45`, `:52` |
| Evidencia | [`user.ts#L103`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/api/user.ts#L103): `` `${SERVER_BASE_URL}/profiles/${username}` ``; [`article.ts#L45`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/lib/api/article.ts#L45): `` `/articles/${slug}` ``. En el mismo archivo, `byAuthor` y `byTag` sí usan `encodeURIComponent` |
| Impacto en producción | Un nombre de usuario con `?`, `#` o `/` (la API de C# solo limita la longitud) hace que la petición llegue a otro endpoint o pierda parte del valor: perfiles que no cargan y botones de seguir que fallan para algunos usuarios |
| Recomendación | Codificar siempre los segmentos variables, mejor con un helper único en el cliente HTTP de REV-NEXT-005 |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (lectura del código); pendiente de confirmación humana.

#### REV-NEXT-009 — El zoom está bloqueado en móviles

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-009 |
| Severidad | **Baja** (accesibilidad) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `pages/_app.tsx:17-20` |
| Evidencia | [`_app.tsx#L17-L20`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/pages/_app.tsx#L17-L20): `maximum-scale=1, user-scalable=0` |
| Impacto en producción | Las personas con baja visión no pueden ampliar el texto en el celular. Incumple WCAG 2.1 (criterio 1.4.4) y es un riesgo de reclamo en productos con obligaciones de accesibilidad |
| Recomendación | Dejar solo `width=device-width, initial-scale=1` |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: Claude Code (lectura del código); pendiente de confirmación humana.

#### REV-NEXT-010 — Ningún formulario tiene etiquetas: los campos solo se identifican por el placeholder

| Campo | Contenido |
|---|---|
| ID | REV-NEXT-010 |
| Severidad | **Baja** (accesibilidad) |
| Repositorio y SHA | `paopadp123/next-realworld-example-app@be9ef56` |
| Archivo y línea | `components/profile/LoginForm.tsx:52-58` y `:62-68`; igual en `RegisterForm.tsx`, `SettingsForm.tsx` y los editores |
| Evidencia | [`LoginForm.tsx#L52-L58`](https://github.com/paopadp123/next-realworld-example-app/blob/be9ef569bc395975d98c1973fac23ba9277797e7/components/profile/LoginForm.tsx#L52-L58): `<input ... type="email" placeholder="Email" ... />` sin `<label>` ni `aria-label`. Un `grep` de `<label` y `aria-label` en `components/` y `pages/` no encuentra **ninguno** en todo el repositorio |
| Impacto en producción | El placeholder desaparece al escribir y tiene bajo contraste: el usuario pierde la referencia de qué campo está llenando. Los navegadores usan el placeholder como nombre accesible solo como último recurso, así que con algunos lectores de pantalla el campo se anuncia sin propósito claro. Incumple WCAG 2.1 (criterios 1.3.1 y 3.3.2) en el login, el registro y la edición |
| Recomendación | `<label htmlFor>` visible (o visualmente oculto) en cada campo; revisar con axe en CI junto con REV-NEXT-009 |
| Esfuerzo | S |
| ¿Bloquea el merge? | No |

Origen: IA (Antigravity) **confirmado y ampliado**: lo reportó solo en el login y con severidad Media; se verificó que ningún formulario de la aplicación tiene etiquetas. La afirmación de que los lectores de pantalla "no anuncian el propósito" se matizó, porque el placeholder funciona como nombre de respaldo.

**Fortalezas observadas:**
- La protección contra XSS existe y, con la versión fijada, funciona (`sanitize: true` escapa el HTML); el problema es que se apoya en una opción obsoleta (REV-NEXT-001).
- Las rutas con datos del usuario más visibles sí se codifican: `pages/article/[pid].tsx:22`, `ArticleAPI.byAuthor` y `byTag`.
- La caché con SWR y `initialData` desde `getInitialProps` da una primera carga renderizada en el servidor y actualización en el cliente, un patrón razonable para la época.

**Pista no comprobada:** `pages/user/settings.tsx:9-21` decide en el servidor si hay sesión leyendo `localStorage` (que no existe en el servidor), y `getInitialProps` devuelve el objeto `res` para serializarlo. Es probable que al recargar `/user/settings` siempre redirija al inicio, pero no se probó y no se reporta como hallazgo.

## Comparación entre los tres stacks

Los tres repositorios implementan el mismo producto (RealWorld / Conduit). Las diferencias entre ellos no vienen de la funcionalidad, sino de cómo cada comunidad resolvió lo mismo y de cuánto mantenimiento recibió cada uno.

| Área (5.2) | Go + Gin + GORM | C# + ASP.NET Core + EF Core | Next.js + TypeScript |
|---|---|---|---|
| **Autenticación y secretos** | Secreto del JWT en el código, con la alarma del linter silenciada (REV-GO-001). El middleware confía en el token y no verifica que el usuario exista (REV-GO-003, 004) | Lo mejor de los tres: clave obligatoria y validada al arrancar, token con algoritmo, emisor, audiencia y usuario verificados. Lo que falta está alrededor: login sin límite de intentos (REV-NET-002) y token de 5 minutos sin renovación (REV-NET-003) | Token en `localStorage`, leído de tres formas y armado a mano 13 veces (REV-NEXT-005). Junto con un sanitizador obsoleto, un XSS se convierte en toma de cuentas (REV-NEXT-001) |
| **Errores y caminos de excepción** | Conversiones de tipo sin verificar que terminan en `panic` (REV-GO-002, 003); errores descartados con `_` (REV-GO-007) o impresos sin detener el arranque (REV-GO-008). Recovery de Gin evita la caída, pero responde 500 | Un middleware central traduce excepciones a 404, 409, 422 y 500, y los choques con índices únicos salen como 409. Falla en los bordes: `limit=-1` (REV-NET-001), slug vacío (REV-NET-004), mensajes internos de .NET (REV-NET-009) | Axios lanza en 4xx y nadie lo captura; publicar un artículo válido rompe el editor (REV-NEXT-004). Ninguna página maneja el estado de error |
| **Capas y testabilidad** | Paquetes por módulo (`users/`, `articles/`) con una base global (`common.DB`) y un contrato implícito en el contexto de Gin (REV-GO-005): difícil de probar sin base real | *Vertical slices* con Mediator, validación y transacción como *pipeline behaviors*. Cada caso de uso es una clase que se prueba sola. Es el diseño más limpio de los tres | Sin capa: los componentes llaman a axios directamente y `lib/api` es en parte código muerto (REV-NEXT-005) |
| **Pruebas** | 87–99 % de cobertura sin un solo camino de error (REV-GO-006): la cifra da una falsa tranquilidad | Integración con SQLite en memoria y `WebApplicationFactory`, más la suite oficial de RealWorld en CI. Huecos en los bordes de entrada | Ninguna prueba, ningún linter y `strict: false` (REV-NEXT-006) |
| **Estado y datos en el cliente** | No aplica (solo API) | No aplica (solo API); CORS abierto y Swagger público (REV-NET-008) | Sesión en `localStorage` y SWR como caché; sin protección del contenido generado por usuarios más allá de una opción obsoleta |
| **Preparación para producción** | SQLite, sin apagado ordenado (REV-GO-009); CI con `golangci-lint`, `gofmt`, cobertura y Newman | Analizadores con advertencias como errores, formato y restauración con lockfile en CI, imagen no root. Falla en la operación: migraciones al arrancar con SQLite (REV-NET-006) y logs duplicados en texto plano (REV-NET-007) | Backend y estilos fijos en dominios que ya no existen (REV-NEXT-002); 106 vulnerabilidades (REV-NEXT-003); sin CI |
| **Mantenimiento y licencia** | Modernizado por el agente Copilot (2026-01-04). MIT | Activo, con Dependabot (último commit 2026-10-15). MIT | Abandonado desde 2020-08-06. **Sin licencia** (REV-NEXT-007) |
| **Hallazgos** | 9: 1 Crítica, 4 Medias, 4 Bajas | 11: 2 Altas, 5 Medias, 4 Bajas | 10: 3 Altas, 4 Medias, 3 Bajas |

**Lectura de líder:**
- **El mantenimiento pesa más que el lenguaje.** El repositorio de C# recibió endurecimiento reciente y sus hallazgos están en los bordes y en la operación. El de Next lleva seis años sin cambios y no funciona tal como está. Antes de adoptar código de terceros, el primer filtro es su actividad, su licencia y su `npm audit`, no su diseño.
- **El framework decide cuánto error humano cabe.** ASP.NET Core trae de serie lo que en Go hay que escribir a mano: validación declarativa, manejo central de excepciones, inyección de dependencias y analizadores que frenan el build. En Go, la disciplina depende del equipo. Por eso aparecen allí los `panic` y los `_`: el lenguaje no los impide, y el linter estaba configurado pero silenciado donde más importaba.
- **La cobertura no mide lo que importa.** Go tiene la cifra más alta y ninguna prueba de errores; C# tiene menos ruido y prueba contra la especificación oficial. Para el equipo, la regla es exigir un caso de error por cada camino feliz, no un porcentaje (va al checklist de PR).
- **El frontend concentra el riesgo de la sesión.** Por bien que el backend valide el token, si el cliente lo guarda donde un script lo puede leer y renderiza HTML de usuarios, el eslabón débil está en el navegador. Para nuestra plataforma, eso refuerza la decisión de la HU-09: BFF con cookie `httpOnly`.
- **Lo que se lleva al equipo:** (1) secretos obligatorios y validados al arrancar, como en C#; (2) un manejador central de errores con contrato fijo; (3) validación con límites en toda entrada pública (paginación, longitudes); (4) pruebas de los caminos de error; (5) un cliente HTTP único en el frontend; (6) revisar actividad, licencia y vulnerabilidades antes de adoptar una dependencia.

**Licencias citadas:** golang-gin-realworld-example-app y aspnetcore-realworld-example-app, MIT (archivo `LICENSE` en la raíz de cada fork). next-realworld-example-app no declara licencia. Este informe solo cita fragmentos breves con fines de revisión y no redistribuye código de ninguno de los tres.
