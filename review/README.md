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
| aspnetcore | `AGENTS.md` | Sin inyección. Describe las puertas de calidad del repositorio (restauración con lock, analizadores, advertencias como errores, formateador) |

Lo que sí aportan como **pistas para verificar en el código** (no son hallazgos hasta comprobarlos):
- golang-gin `AGENTS.md:112` reconoce que el secreto del JWT está definido en `common/utils.go`.
- golang-gin `AGENTS.md:70` exige no ignorar errores con `_`: se contrasta contra el código.
- golang-gin `BACKEND_INSTRUCTIONS.md:1` y `MOBILE_INSTRUCTIONS.md:1` dicen "Delete this file before publishing your app!" y siguen en el repositorio.
- aspnetcore `AGENTS.md:7`: parte de las pruebas de autenticación usa EF Core InMemory, que no se comporta como una base real.

## Hallazgos

Cada hallazgo usa la plantilla del Anexo C. Los marcados como **probado en vivo** se reprodujeron con la aplicación corriendo; el script está en [`pruebas-en-vivo/golang-gin.sh`](pruebas-en-vivo/golang-gin.sh).

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
| Impacto en producción | La cobertura alta da una **falsa sensación de seguridad**: los cuatro defectos de este informe (tres probados en vivo con 500 o acceso indebido) pasan el CI sin que nadie se entere. Un equipo que use la cobertura como meta de calidad aprobaría estos cambios. Además, al depender del orden, un caso que falla arrastra a los siguientes y cuesta encontrar la causa |
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

## Comparación entre los tres stacks

<!-- Pendiente. -->
