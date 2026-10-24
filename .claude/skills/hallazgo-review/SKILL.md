---
name: hallazgo-review
description: Registra un hallazgo del code review (Bloque 2) con la plantilla del Anexo C y verifica contra GitHub que el archivo y las líneas existen en el SHA revisado antes de escribirlo. Usar cada vez que se agregue un hallazgo a review/README.md.
---

# /hallazgo-review — hallazgo verificado

Un hallazgo inventado o que no corresponde al código real **descalifica la prueba**. Esta skill no escribe nada sin verificarlo primero.

## Entrada
Repositorio (fork propio `paopadp123/<repo>`), SHA revisado, archivo, línea o rango, y la idea del hallazgo. Si falta el SHA, obtenlo con `git -C <clon> rev-parse HEAD`; nunca uses una rama como referencia.

## Pasos
1. **Verificar el código real.** Descarga el archivo exactamente en ese SHA:
   `gh api "repos/paopadp123/<repo>/contents/<archivo>?ref=<SHA>" --jq .content` (decodificar base64), o léelo del clon local en ese SHA.
   Muestra las líneas citadas. Si no existen o no dicen lo que el hallazgo afirma, **detente** y dilo.
2. **Asignar el ID** siguiendo la serie del repositorio: `REV-GO-00n`, `REV-NET-00n`, `REV-NEXT-00n` (revisar los existentes en `review/README.md`).
3. **Clasificar con criterio:**
   - Severidad: Crítica / Alta / Media / Baja.
   - Tipo: seguridad, arquitectura, errores, pruebas, estado del cliente, producción.
   - **Contexto:** "aceptable en un proyecto de ejemplo, inaceptable en producción" cuando aplique. Decirlo explícitamente.
4. **Escribir la fila** en la tabla del repositorio en `review/README.md`:

| Campo | Contenido |
|---|---|
| ID | REV-XX-00n |
| Severidad | … |
| Repositorio y SHA | `paopadp123/<repo>@<SHA corto>` |
| Archivo y línea | `<archivo>:<línea>` |
| Evidencia | enlace permanente `https://github.com/paopadp123/<repo>/blob/<SHA>/<archivo>#L<a>-L<b>` + fragmento breve |
| Impacto en producción | en términos de negocio (dinero, clientes, cumplimiento, reputación) |
| Recomendación | acción concreta |
| Esfuerzo | S / M / L |
| ¿Bloquea el merge? | Sí / No, y por qué |

5. **Registrar el origen** en una línea debajo del hallazgo: `Origen: humano | IA (Antigravity) confirmado | IA corregido`. Es el insumo del punto 9.4 (qué vio la IA, qué vi yo y qué reportó mal).

## Reglas
- Nunca citar líneas de memoria; siempre verificar en el SHA.
- Si la IA reportó algo que resultó falso, no se borra: se anota en `docs/ai/review-ia-vs-humano.md` como "reportado por la IA, descartado", con el motivo.
