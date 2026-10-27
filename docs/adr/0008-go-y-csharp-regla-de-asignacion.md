# ADR 0008 — Go y C#: regla de asignación y un solo dueño por regla de negocio

- **Estado:** Propuesto (se revisa con el equipo en el día 31–60 del plan de entrada)
- **Fecha:** 2026-10-27
- **Responsable:** Líder técnico

## Contexto
El equipo (8 personas: 2 senior, 4 semi senior, 2 practicantes) mantiene servicios en Go y en C# y dos frontends en Next.js con TypeScript. Hay **lógica de dominio duplicada** entre los dos lenguajes de backend: la misma regla vive en un servicio Go y en otro C#. Ejemplos supuestos, hasta tener el inventario del día 30: el cálculo de la prima o los estados de una póliza. Cada cambio de negocio se implementa dos veces, y cuando las dos copias divergen, el cliente ve un precio en la cotización y otro en la póliza.

Lo que pesa en la decisión:
- **El problema es la duplicación, no la cantidad de lenguajes.** Dos servicios en Go también pueden duplicar una regla.
- **Reescribir cuesta mucho y no da valor al cliente.** Migrar los servicios de un lenguaje al otro serían meses de un equipo pequeño sin funcionalidades nuevas, con el riesgo de introducir errores en flujos de dinero.
- **Cada lenguaje tiene un lugar natural en este dominio.** Go para procesos concurrentes y de larga vida (consumidores de colas, orquestación, telemetría); C# para integraciones empresariales y reglas con mucho modelo (aseguradoras, SOAP, librerías .NET de terceros). Este mismo repositorio lo aplica: el orquestador está en Go y la capa anticorrupción del emisor en C# (ADR 0003, HU-07).
- **Personas:** dos lenguajes exigen que cada persona sea competente en uno y lea el otro, no que domine los dos. La facilidad de contratar en cada uno se mide, no se supone (ver los criterios de reversión).
- **TypeScript ya existe** en los frontends y en herramientas internas (como el simulador de terceros de este repositorio). Esta decisión es sobre la **lógica de dominio en el backend**; TypeScript se queda en el frontend y en las herramientas.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **Estandarizar en un solo lenguaje** | Un solo stack de herramientas, CI, librerías y guardias; rotación más fácil entre servicios | Reescritura de meses sin valor de negocio; riesgo en flujos de dinero; se pierde el encaje de cada lenguaje; el equipo que domina el otro pierde productividad o se va |
| **Mantener ambos sin regla** | Cero costo de cambio | La duplicación sigue creciendo; cada servicio nuevo se escribe en el lenguaje de quien lo empieza |
| **Regla de asignación + un dueño por regla de negocio** | Ataca la duplicación directamente; no reescribe nada que funcione; decisión predecible para servicios nuevos | Dos stacks de CI, observabilidad y librerías; hay que mantener contratos entre servicios |

## Decisión
**Regla de asignación + un solo dueño por regla de negocio.**
1. **Un dueño por regla.** Cada regla de dominio (prima, estados de la póliza, validación del vehículo) vive en **un solo servicio**, y los demás lo consultan por su API o reaccionan a sus eventos. No se comparten librerías de dominio entre lenguajes ni se copia la regla "para no hacer una llamada".
2. **Asignación para servicios nuevos:** Go si el servicio es concurrente, de larga vida o de alto volumen (colas, orquestación, telemetría, gateways). C# si el núcleo son reglas de negocio con mucho modelo o integraciones empresariales (.NET de terceros, SOAP, aseguradoras). Si no encaja en ninguno, se elige el lenguaje del equipo que lo va a mantener. Un tercer lenguaje para servicios de backend requiere un ADR propio.
3. **Contratos explícitos** entre servicios: OpenAPI para HTTP y esquemas versionados para eventos, con pruebas de contrato en el CI de ambos lados.
4. **Eliminar la duplicación existente por prioridad de riesgo:** primero las reglas que mueven dinero (prima, cobro, emisión). Cada eliminación se registra como deuda técnica con su costo (ver `lead/README.md`, sección 5). No se reescribe un servicio completo para eliminar una regla: se mueve la regla a su dueño.
5. **Paridad de prácticas entre stacks:** las mismas puertas de calidad en los dos (formato, análisis estático, pruebas, revisión) y el mismo checklist de PR (`review/checklist-pr.md`).

## Consecuencias
- **Buenas:** un cambio de negocio se hace una vez; desaparece la clase de incidente "la cotización y la póliza no coinciden"; los servicios nuevos ya no dependen de quién los empiece; nadie se va porque su lenguaje "perdió".
- **Malas:** se mantienen dos cadenas de herramientas (CI, imágenes base, librerías de observabilidad, actualizaciones de seguridad). Una regla que antes era local ahora es una llamada de red: hay que tratar su latencia y su caída (timeouts, breaker, caché con TTL corto), como el orquestador hace con la ACL.
- **Medición:** número de reglas de dominio duplicadas (inventario inicial en el día 30 del plan), cambios de negocio que tocaron dos servicios, incidentes por divergencia.

## Criterios de reversión
Se reabre la decisión y se evalúa **estandarizar** en el lenguaje con más servicios críticos si se cumple cualquiera de estas condiciones:
- El equipo baja a **5 personas o menos**: dos stacks dejan a cada uno con una sola persona que lo conoce.
- Después de 2 trimestres, **más del 25 % de los cambios de negocio** siguen tocando servicios de los dos lenguajes, o hubo **2 o más incidentes** por divergencia de una regla: la regla de dueño único no está funcionando.
- Mantener los dos stacks (actualizaciones, CI, librerías) cuesta **más del 15 % de la capacidad** del equipo durante 2 trimestres seguidos.
- Durante 6 meses no se logra contratar en uno de los dos lenguajes.

Si se estandariza, se hace por **estrangulamiento** (cada servicio se migra cuando de todas formas hay que cambiarlo mucho), nunca con una reescritura total.
