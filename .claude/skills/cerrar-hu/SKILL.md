---
name: cerrar-hu
description: Cierra una HU antes de proponer commits: corre las pruebas del servicio, invoca al agente qa-reviewer, actualiza la guía de estudio y propone mensajes de commit sin ejecutarlos. Usar cuando el usuario diga "cerrar HU-xx" o /cerrar-hu.
---

# /cerrar-hu HU-xx

## Pasos
1. **Pruebas reales** del servicio que tocó la HU: Go `go vet ./... && go test -race ./...` en `orchestrator/`; C# `dotnet build` + `dotnet test` en `issuing-service/`; Next/simulador `npm run lint && npm test`. Reporta la salida tal cual; si algo falla, se arregla antes de seguir.
2. **QA**: invoca el agente `qa-reviewer` con la HU y sus criterios de aceptación (del `PLAN_DE_TRABAJO.md`). Corrige lo marcado como BLOQUEA. Lo que no bloquea se anota como deuda en la HU.
3. **Guía de estudio** (`GUIA_ESTUDIO.md`, fuera de git): agrega la sección de la HU con:
   - Qué hace, en 3–4 líneas.
   - Para Go: los conceptos del lenguaje usados y por qué (goroutines, `context`, interfaces, manejo de `error`), con la línea del código donde aparecen.
   - Decisiones y alternativas descartadas.
   - 3 preguntas probables de la entrevista, con su respuesta corta.
4. **Commits propuestos**: Conventional Commits en español, incrementales (p. ej. `test(orquestador): …` antes de `feat(orquestador): …` para mostrar rojo→verde), con `Refs #<issue>` y la línea `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. **No ejecutar commit ni push**: el usuario los revisa con otra IA y los aprueba.
5. **Resumen final** (máximo 8 líneas): pruebas, resultado del QA, deuda anotada, commits propuestos y el siguiente clip de video si aplica (ver la sección 10 del plan).
