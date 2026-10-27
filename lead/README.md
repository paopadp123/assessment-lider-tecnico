# Liderazgo técnico y gobierno de ingeniería (Bloque 5)

## 1. Plan de 30, 60 y 90 días

Equipo: 8 personas (2 senior, 4 semi senior, 2 practicantes), servicios en Go y C# y dos frontends en Next.js.

**Medido desde el día 1** (la línea base, antes de cambiar nada): las cuatro métricas DORA por servicio, el tiempo hasta la primera revisión de un PR, el estado de la rama principal (porcentaje de builds en verde), las alertas de guardia por semana y fuera de horario, y una encuesta anónima de salud del equipo cada mes (5 preguntas, 2 minutos). Cómo se obtienen: sección 4.

| Días | Objetivo | Qué hago | Qué entrego |
|---|---|---|---|
| **1–30** | Entender antes de cambiar | 1:1 con las 8 personas (qué les frena, qué quieren aprender, qué no tocarían) y con producto, negocio y operaciones. Mapa de servicios: dueño, lenguaje, última versión desplegada, pruebas, alertas y quién lo conoce. Acompaño la guardia y leo los incidentes y los PR de los últimos 3 meses. Un practicante con cada senior como mentor. | Un documento de una página: el estado del equipo, los 5 riesgos principales y la línea base de métricas. Dos mejoras rápidas que nadie discuta: checks obligatorios en la rama principal y la plantilla de PR con la DoD (`review/checklist-pr.md`). |
| **31–60** | Acordar cómo trabajamos | Acuerdos del equipo: DoD, revisión en menos de 24 h, PR pequeños, pruebas obligatorias en los flujos de dinero. ADR de Go y C# ([ADR 0008](../docs/adr/0008-go-y-csharp-regla-de-asignacion.md)) discutido con los dos senior. Registro de deuda técnica con costo (sección 5) y una capacidad fija para pagarla. Guardia rotativa con runbooks. SLO para los dos flujos más críticos. | Acuerdos escritos y aceptados por el equipo, no impuestos. Cada senior es dueño de un área (una en Go, una en C#) y cada semi senior de un servicio. |
| **61–90** | Delegar y mostrar resultados | La hoja de ruta del trimestre se negocia con negocio incluyendo la deuda. Los senior dirigen las revisiones de diseño; yo reviso menos código y más decisiones. Ningún servicio depende de una sola persona. Primeras conversaciones de desempeño, con evidencia. | Informe a mi jefe: métricas frente a la línea base, qué cambió, qué no funcionó y el plan del siguiente trimestre. |

Lo que **no** hago en los primeros 30 días: reescribir servicios, cambiar herramientas o fijar metas sobre las métricas.

## 2. ADR

[ADR 0008 — Go y C#: regla de asignación y un solo dueño por regla de negocio](../docs/adr/0008-go-y-csharp-regla-de-asignacion.md). Resumen: no se estandariza, porque el problema es la **duplicación** y no la cantidad de lenguajes. Cada regla de dominio tiene un solo servicio dueño, los servicios nuevos siguen una regla de asignación, y se reabre la decisión si el equipo baja a 5 personas, si después de 2 trimestres más del 25 % de los cambios de negocio siguen tocando los dos lenguajes (o hubo 2 incidentes por divergencia), si mantener dos stacks cuesta más del 15 % de la capacidad o si no se logra contratar en uno de los dos.

## 3. Conversación difícil

**Al desarrollador** (1:1, en privado):
> Tienes razón en algo importante: esas fechas se comprometieron sin preguntarnos, y arreglar eso es mi trabajo, no el tuyo. Pero entregar sin pruebas no te ahorra el costo: se lo pasa al equipo. El siguiente que toca ese código no sabe qué rompe, y los incidentes de pago los atiende quien esté de guardia. Acordemos dos cosas: en los flujos de dinero no se integra nada sin pruebas de la regla de negocio, y si una fecha no alcanza para hacerlo bien, me lo dices antes de comprometerla, no después. Yo me encargo de negociar con negocio. Y quiero que tú definas cuál es el mínimo de pruebas para esos flujos: eres quien mejor los conoce. Lo revisamos en dos semanas.

**A negocio:**
> Las últimas fechas se comprometieron sin consultar a ingeniería. Se cumplieron, pero pidiendo prestado: se entregó sin pruebas, y eso ya nos costó [N] incidentes y [H] horas de corrección este trimestre. Propongo una regla: ninguna fecha que involucre a ingeniería se compromete sin una estimación del equipo, que entregamos en 48 horas y con rango. Si la fecha es fija por una razón de negocio, negociamos el alcance, no la calidad. Para el compromiso actual, propongo entregar [X] en la fecha y [Y] dos semanas después.

## 4. Métricas

| Métrica | Cómo se obtiene |
|---|---|
| **Frecuencia de despliegue** y **tiempo de entrega de cambios** (DORA) | API de GitHub: hora del primer commit del PR y hora del despliegue a producción, que el pipeline registra como un *deployment* de GitHub. Un job programado las agrega por servicio y por semana. |
| **Tasa de fallo de cambios** y **tiempo de recuperación** (DORA) | Incidentes de PagerDuty u Opsgenie etiquetados con el despliegue que los causó (el pipeline anota la versión); reversiones y hotfixes se detectan por etiqueta del PR. |
| **Tiempo hasta la primera revisión** y **tamaño del PR** | API de GitHub (eventos de revisión y líneas cambiadas). |
| **Salud de la rama principal** y **pruebas inestables** | Resultados de GitHub Actions: porcentaje de builds en verde y pruebas que fallan y pasan en el mismo commit. |
| **Carga de guardia** | Alertas por semana y fuera de horario, por rotación (no por persona). |
| **SLO de los flujos críticos** | Los de `docs/architecture.md`, sección 5 (emisión a tiempo, ningún pago sin resolver). |
| **Salud del equipo** | Encuesta anónima mensual. |

**El riesgo de optimizar en vacío** (ley de Goodhart): si la frecuencia de despliegue se vuelve una meta, aparecen PR partidos sin sentido; si la tasa de fallo es una meta, los incidentes se reclasifican o dejan de reportarse; si el tiempo de entrega es una meta, se saltan las pruebas. Cómo lo evito:
- **Métricas del equipo, nunca individuales**, y fuera de las evaluaciones de desempeño.
- **Siempre en pares que se tensionan:** velocidad (frecuencia, tiempo de entrega) junto a estabilidad (tasa de fallo, recuperación, SLO). Mejorar una a costa de la otra no cuenta.
- **Tendencias frente a la línea base, no metas numéricas.** Sirven para hacer preguntas en la retrospectiva, no para responderlas.
- La encuesta de salud del equipo y la carga de guardia muestran el costo humano que las métricas DORA no ven.

## 5. Deuda técnica

**Cómo la hago visible:** un registro en el backlog donde cada deuda tiene dos números. El **interés**: horas por mes que nos cuesta convivir con ella. El **riesgo**: probabilidad por impacto de un incidente. Se prioriza por interés y riesgo, no por gusto técnico.

**Cómo la financio:** una capacidad fija acordada con negocio (15–20 % de cada sprint), más una regla atada al SLO: si un servicio consume su presupuesto de error, se congelan sus funcionalidades hasta recuperarlo. Cada trimestre, al comité le muestro la deuda en dinero y riesgo, junto con lo que ya se pagó y lo que eso ahorró.

**Ejemplo concreto** (la duplicación del ADR 0008; cifras supuestas, a medir en el día 30): la regla de la prima vive en un servicio Go y en uno C#.
- **Interés:** cada cambio de tarifa se hace dos veces, con 2 días-persona extra. 12 cambios al año × 2 días × US$240 por día ≈ **US$5.800 al año**.
- **Riesgo** (supuesto: una divergencia al año): si las copias divergen 3 días antes de detectarlo, con 10.000 órdenes por día y el 0,5 % afectado, son **150 pólizas con una prima distinta de la cobrada**. Cada una exige corrección manual y reembolso (~US$15 de operación), más quejas que pueden escalar al regulador. Por divergencia: **~US$2.300 directos más el riesgo regulatorio y de reputación**.
- **Pagarla:** mover la regla a un solo servicio cuesta ~15 días-persona (**~US$3.600**). Con el interés y una divergencia al año (~US$8.000), **se recupera en menos de 6 meses**; solo con el interés, en ~7 meses. Además elimina el incidente.

Al comité se lo presento así: *"Invertir 3 semanas de una persona ahorra, según lo esperado, ~US$8.000 al año y elimina el riesgo de cobrarle a un cliente una prima distinta de la de su póliza."*

## 6. Estimación del Bloque 1 como épica

**Equipo de 5:** un senior en Go, un semi senior en Go, un senior en C#, un semi senior en Next.js y una persona de QA o DevOps. **Foco real del 70 %:** reuniones, revisiones y soporte consumen el resto.

| Historia | Optimista | Probable | Pesimista (días-persona) |
|---|---|---|---|
| Contratos y sandboxes de los 3 terceros, ambientes | 5 | 8 | 15 |
| Simulador de terceros con caos y docker compose | 3 | 5 | 8 |
| Orden, máquina de estados, idempotencia y outbox | 8 | 12 | 18 |
| Pago: intención, webhook firmado e idempotencia | 6 | 9 | 15 |
| Emisión: capa anticorrupción C# | 7 | 10 | 17 |
| Emisión en el orquestador: breaker, reintentos y DLQ | 3 | 5 | 8 |
| Reconciliación, acciones operativas y login de operadores | 6 | 10 | 15 |
| Consola de operaciones (lista, detalle, eventos en vivo, acciones) | 8 | 12 | 18 |
| Observabilidad: correlation-id, salud, métricas y alertas | 4 | 6 | 10 |
| Pruebas de punta a punta, caos y contrato | 6 | 10 | 15 |
| CI/CD, IaC y secretos | 6 | 10 | 18 |
| Seguridad, pruebas con negocio y salida a producción | 5 | 8 | 15 |
| **Total** | **67** | **105** | **172** |

Con la estimación de tres puntos (PERT), la media es (67 + 4·105 + 172) / 6 ≈ **110 días-persona**, con una desviación de ≈ 17,5. Al 80 % de confianza son ≈ **125 días-persona**. Con 5 personas al 70 % (3,5 personas efectivas), la capacidad alcanza para **6–7 semanas**. **Pero la capacidad no es el límite: la ruta crítica sí.** Orden, pago, emisión en el orquestador y reconciliación (36 días-persona probables) caen en las dos personas de Go, que suman 7 días efectivos por semana, y no empiezan hasta tener los contratos:

| Semanas | Go (2 personas) | C# | Next.js | QA / DevOps |
|---|---|---|---|---|
| 1–1,5 | Contratos con los terceros (todo el equipo) | | | CI/CD, ambientes |
| 1,5–6,5 | Orden → pago → emisión en el orquestador → reconciliación (~5 semanas) | Capa anticorrupción (~3 semanas), luego observabilidad y apoyo en reconciliación | Consola contra un mock del contrato; integración real desde la semana 5 | Simulador con caos, IaC, secretos |
| 6,5–8 | Integración, corrección de defectos | Igual | Igual | Pruebas de punta a punta, caos, seguridad y salida a producción |

**Comunico de 8 a 10 semanas, con compromiso en 9**, y reviso la estimación al terminar la semana 2, cuando ya se conozcan los contratos de los terceros. Agregar gente no acorta la ruta: una tercera persona en Go llegaría cuando ya hay que integrar.

**Los tres supuestos que más pueden hacerla fallar:**
1. **El emisor legado tiene un sandbox estable desde la semana 1 y permite consultar por referencia externa.** Si no lo permite, la emisión idempotente necesita una conciliación manual con la aseguradora, y es la historia más grande. Riesgo: +2 a 3 semanas.
2. **La certificación con la pasarela de pagos** (revisión de seguridad, firma del webhook, pruebas en su ambiente) toma menos de 2 semanas. Depende del tercero, no de nosotros.
3. **El equipo está dedicado** al 70 % sin que lo saquen para incidentes ni para otros proyectos. Si una de las dos personas de Go se pierde 2 semanas, la fecha se mueve ~1 semana, porque Go es la ruta crítica.
