---
name: prompt
description: Convierte un pedido escrito en lenguaje natural en un prompt estructurado (contexto, objetivo, restricciones, criterios de aceptación, fuera de alcance, formato), lo muestra para aprobación antes de ejecutarlo y registra ambas versiones en la bitácora de IA. Usar cuando el usuario escribe /prompt o termina su mensaje con "mejóralo" / "mejora el prompt".
---

# /prompt — refinar antes de ejecutar

La prueba exige una bitácora de prompts **reales, tal como se escribieron**. Esta skill no reescribe el pasado: mejora el prompt **antes** de ejecutarlo y guarda las dos versiones, que son reales.

## Pasos

1. **Toma el texto original** (los argumentos de `/prompt`, o el mensaje del usuario sin la palabra "mejóralo"). No lo corrijas ni le cambies la ortografía: se registra tal cual.

2. **Reúne el contexto mínimo necesario** (no más):
   - `PLAN_DE_TRABAJO.md`: identificador de la HU, decisiones técnicas, criterios.
   - Código ya existente que el pedido toque (con Grep/Glob, solo lo relevante).
   - Contratos del Anexo B si el pedido toca terceros (cotizador, PSP, emisor).
   - Preferencias conocidas: C# sin `var`; TS `strict` sin `any`; explicar el código de Go en 3–4 líneas porque el usuario está aprendiendo Go; nunca hacer commit ni push sin pedido explícito.

3. **Escribe el prompt refinado** con esta estructura. Omite una sección solo si de verdad no aplica:

   ```
   [HU-xx] <título corto>
   Contexto: lo que ya existe y las decisiones tomadas (archivos, contratos, SHA si es code review).
   Objetivo: el resultado esperado, en una frase.
   Restricciones: stack, librerías permitidas o prohibidas, estilo.
   Criterios de aceptación: casos verificables, incluidos los bordes (errores, duplicados, timeouts).
   Fuera de alcance: lo que NO se debe hacer.
   Formato de salida: qué entregar (código, pruebas, explicación breve, sin commit).
   ```

   Reglas: no inventar requisitos que no estén en el pedido, el plan o el enunciado. Si falta un dato que cambia el resultado, márcalo como `❓ <pregunta>` en vez de suponerlo.

4. **Muestra el prompt refinado y detente.** Termina con una línea: *"¿Lo ejecuto así, o lo ajustas?"*. **No ejecutes nada todavía.**

5. **Cuando el usuario apruebe** (o mande ajustes y luego apruebe):
   - Agrega una entrada en `docs/ai/prompt-log.md` (crea la carpeta y el archivo si no existen) con este formato:

     ```markdown
     ## <AAAA-MM-DD HH:mm> — [HU-xx] <título>
     **Herramienta:** Claude Code (skill /prompt)

     **Original (tal como se escribió):**
     > <texto original, sin editar>

     **Refinado (ejecutado):**
     ```text
     <prompt final aprobado, con los ajustes del usuario>
     ```

     **Qué se agregó y por qué:** <1–2 líneas: qué contexto o criterio faltaba en el original>
     **Resultado:** _pendiente_
     ```

   - Ejecuta el prompt refinado como la tarea en curso.
   - Al terminar, reemplaza `_pendiente_` por una línea honesta: qué salió bien, qué hubo que corregir y si la IA se equivocó en algo. Esa línea es el insumo de "La alucinación" (Bloque 6.3).

## Notas
- Si el pedido es trivial (renombrar algo, una pregunta corta), dilo y ofrece ejecutarlo directo sin registrarlo: la bitácora debe tener prompts que cuenten algo.
- Nunca pongas secretos, contraseñas ni datos personales en la bitácora: reemplázalos por `<REDACTADO>`.
