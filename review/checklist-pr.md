# Checklist de PR y Definition of Done

Derivado de los 30 hallazgos de [`README.md`](README.md): cada punto existe porque falló en al menos uno de los tres repositorios. El autor marca cada punto antes de pedir revisión; el revisor lo verifica en el código, no en la casilla. Si un punto no aplica, se escribe "N/A" y el porqué.

## Checklist de PR

**1. Secretos y configuración**
- [ ] Ningún secreto, clave ni contraseña está en el código, los scripts o los ejemplos. Se leen del entorno o de un gestor de secretos, y la aplicación **no arranca** si faltan o son débiles (REV-GO-001; el patrón correcto está en `aspnetcore`).
- [ ] Nada se configura con URL, dominios o rutas fijas en el código: cada entorno se configura con variables (REV-NEXT-002, REV-NET-011).
- [ ] Ningún aviso de linter o de seguridad se silencia (`#nosec`, `nolint`, `// @ts-ignore`) sin un comentario que justifique la excepción y la aprobación del revisor (REV-GO-001).

**2. Autenticación y autorización**
- [ ] Un token válido no basta: se verifica que el usuario siga existiendo y que tenga permiso sobre el recurso (REV-GO-004).
- [ ] Los endpoints públicos sensibles (login, registro, recuperación) tienen límite de intentos (REV-NET-002).
- [ ] La sesión del navegador no vive en `localStorage`, sino en una cookie `httpOnly` (REV-NEXT-001), además `Secure` y `SameSite` como práctica general.

**3. Entrada y límites**
- [ ] Toda entrada pública tiene validación de tipo, rango y longitud máxima, incluidas la paginación (`limit`, `offset`) y las listas (REV-NET-001, REV-NET-005).
- [ ] El HTML generado por usuarios pasa por un sanitizador explícito (DOMPurify o equivalente). No vale una opción de librería obsoleta (REV-NEXT-001).
- [ ] Los valores en una URL se codifican siempre (REV-NEXT-008).
- [ ] Los identificadores derivados del contenido (slugs) toleran tildes, ñ y entradas vacías (REV-NET-004).

**4. Manejo de errores**
- [ ] Ningún error se descarta (`_`, `catch {}` vacío) ni se imprime para seguir como si nada, sin un comentario que diga por qué (REV-GO-004, REV-GO-007, REV-GO-008).
- [ ] No hay conversiones de tipo que puedan hacer `panic` o lanzar una excepción con datos del cliente (`x.(T)` sin `, ok`) (REV-GO-002, REV-GO-003).
- [ ] Los errores del cliente responden 4xx y los del servidor, 5xx. El mensaje no expone detalles internos (tipos, rutas, stack traces) (REV-GO-002, REV-NET-009).
- [ ] En el frontend, cada llamada a la API maneja el error: muestra un mensaje, libera el estado de carga y no pierde lo que el usuario escribió (REV-NEXT-004).

**5. Pruebas**
- [ ] Cada cambio de comportamiento trae al menos **un caso de error** además del camino feliz. La cobertura no reemplaza esto (REV-GO-006).
- [ ] Un arreglo de un defecto trae la prueba que lo reproduce y que falla sin el arreglo. Si una prueba existente cambia de expectativa, el PR explica por qué (HU-12: dos pruebas validaban el defecto).
- [ ] Las pruebas no dependen del orden ni de una base compartida entre casos.

**6. Dependencias y CI**
- [ ] Las dependencias nuevas tienen licencia compatible, mantenimiento activo y `npm audit` / `govulncheck` / `dotnet list package --vulnerable` sin vulnerabilidades altas (REV-NEXT-003, REV-NEXT-007).
- [ ] Todo recurso externo del CI (colecciones, scripts, acciones) está fijado a una versión o SHA (HU-12: la colección de Postman).
- [ ] El CI corre lint, tipos estrictos y pruebas, y está en verde (REV-NEXT-006).

**7. Operación**
- [ ] Los logs son estructurados, sin datos personales ni secretos, y cada evento se escribe una sola vez (REV-NET-007).
- [ ] Las migraciones son un paso separado del arranque (REV-NET-006) y, como práctica general, compatibles con la versión anterior durante el despliegue.
- [ ] El servicio tiene endpoint de salud y apagado ordenado (REV-GO-009, REV-NET-007).
- [ ] Las consultas no cargan colecciones completas para modificar un solo elemento (REV-NET-010).

**8. Código generado con IA**
- [ ] El autor puede explicar cada línea del PR. Lo generado con IA se revisó igual que el resto, y el PR lo indica.
- [ ] Ninguna cita, API o comportamiento se tomó de la IA sin verificarlo contra el código o la documentación ([`docs/ai/review-ia-vs-humano.md`](../docs/ai/review-ia-vs-humano.md): dos alucinaciones marcadas como "verificado").

## Definition of Done

Una historia está terminada cuando se cumple **todo** lo siguiente:

1. Los criterios de aceptación de la historia se cumplen y se demostraron (prueba automática o evidencia en el PR).
2. El checklist de PR está completo y al menos una persona distinta del autor aprobó el PR.
3. El CI está en verde: lint, tipos estrictos, pruebas unitarias e integración, y escaneo de dependencias.
4. Las reglas críticas del negocio (dinero, emisión, datos personales) tienen pruebas de sus caminos de error, no solo del camino feliz.
5. La configuración nueva está documentada (`.env.example`, README) y existe en todos los entornos antes del despliegue.
6. Los logs y las métricas permiten ver el cambio funcionando en producción. Si agrega una dependencia externa, tiene timeout, reintentos acotados y una alerta.
7. Si el cambio toma una decisión de arquitectura, hay un ADR. Si cambia un contrato, el contrato (OpenAPI) está actualizado.
8. Está desplegado en un entorno de prueba y nadie tuvo que hacer pasos manuales que no estén documentados.
