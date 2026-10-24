# ADR 0001 — PostgreSQL como base de datos transaccional

- **Estado:** Aceptado
- **Fecha:** 2026-10-23
- **Responsable:** Líder técnico

## Contexto
El orquestador (Go) persiste órdenes, su máquina de estados, una outbox de eventos y una cola de mensajes muertos. La capa de emisión (C#) persiste su registro de idempotencia por referencia externa. La plataforma corre en AWS y el enunciado no impone un motor. La escala de referencia es de 10.000 órdenes por día con picos de 50 solicitudes por segundo.

## Opciones consideradas
| Opción | A favor | En contra |
|---|---|---|
| **PostgreSQL** | Sin costo de licencia en RDS/Aurora; Aurora PostgreSQL (incluida Serverless v2) disponible; `pgx` es el driver de referencia en Go; `FOR UPDATE SKIP LOCKED` permite varios despachadores de outbox sin bloquearse; imagen de contenedor liviana | El equipo .NET puede tener más experiencia operando SQL Server |
| SQL Server | Integración natural con .NET y herramientas de Microsoft | Licencia incluida en la tarifa de RDS, notablemente más cara a igual capacidad; sin Aurora; driver de Go con menos adopción; imagen pesada (más de 1.5 GB, mínimo 2 GB de RAM) para el entorno local |
| DynamoDB | Escala sin administración | Transacciones y consultas operativas (filtros, búsqueda, línea de tiempo) más costosas de modelar; la outbox requiere Streams; dificulta el entorno local |

## Decisión
PostgreSQL 17 para ambos servicios, con un esquema por servicio (sin tablas compartidas). En AWS: Aurora PostgreSQL.

## Consecuencias
- **Buenas:** la outbox y la cola usan el mismo motor transaccional que la orden, lo que da atomicidad sin un broker extra en local; costo predecible; un solo motor que operar.
- **Malas:** la cola sobre tablas tiene un techo de rendimiento menor que SQS; se mitiga publicando desde la outbox hacia SQS en AWS (ver arquitectura).

## Criterios de reversión
- Si la organización estandariza en SQL Server con licencias y DBAs existentes: la persistencia está aislada detrás de repositorios, así que el cambio queda acotado a esa capa.
- Si el volumen de la outbox supera lo que soporta un despachador sobre tablas (latencia de publicación p95 mayor a 5 s sostenida), se mueve la cola a SQS manteniendo la outbox solo como garantía de atomicidad.
