# Mensaje al desarrollador

Comentario para la persona senior que escribió `golang-gin-realworld-example-app`, dejado en la revisión del repositorio (máximo 250 palabras).

---

Hola. Revisé el repositorio y la base es buena: la estructura por módulos es clara, validaste el algoritmo del JWT (evita el ataque `alg: none`) y el CI ya tiene lint, formato y cobertura.

**Lo urgente:** el secreto del JWT está escrito en `common/utils.go:41` y publicado, con el aviso de gosec silenciado. Con ese valor cualquiera firma un token para cualquier usuario. Lo corregí en el [PR #1 de mi fork](https://github.com/paopadp123/golang-gin-realworld-example-app/pull/1), junto con el panic por un JSON roto y el token de un usuario que ya no existe. Revísalo hoy: `JWT_SECRET` pasa a ser obligatorio y las sesiones actuales se invalidan.

**Después, en este orden:**
1. **Errores:** el más grave es el `bcrypt` ignorado: una contraseña de más de 72 bytes crea la cuenta y la deja bloqueada para siempre. Además hay conversiones sin `, ok` (`utils.go:71`, `middlewares.go:71`). Propongo una regla: ningún error se ignora sin un comentario que diga por qué.
2. **Pruebas:** la cobertura está entre 87 y 99 %, pero no cubre los caminos de error que encontré, y dos pruebas validaban un defecto. Pidamos un caso de error por handler.
3. **CI:** el job de la API estaba roto desde marzo por una colección sin fijar; ya la fijé. Y antes de silenciar un linter con `#nosec`, hablémoslo.

Son atajos razonables en un ejemplo que se vuelven riesgos en producción. ¿Lo vemos 30 minutos mañana? Quiero tu opinión sobre el contrato implícito del contexto (REV-GO-005), que no tengo resuelto.
