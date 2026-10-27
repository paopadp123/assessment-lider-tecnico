# Filtro técnico para un senior fullstack

**Formato:** ejercicio en casa de 3 h (con tope declarado) y sustentación en vivo de 60 min con dos personas del equipo. El mismo ejercicio y una rúbrica escrita de antemano para todos los candidatos.

## El ejercicio y por qué ese

> *Te damos un servicio pequeño (Go o C#, a elección) que recibe el webhook de pago del PSP y emite la póliza llamando a un emisor simulado. Hazlo seguro para producción: el PSP puede reenviar el mismo evento, mandarlo fuera de orden o con la firma inválida, y el emisor a veces tarda 30 s o responde 503. Ninguna compra debe cobrarse dos veces ni emitirse dos pólizas. Entrega un PR con pruebas y un README de una página con lo que dejaste fuera y por qué. Puedes usar IA; dinos cómo.*

El equipo prepara ese repositorio con **dos trampas sembradas** y verificadas antes de usarlo: una prueba verde que valida un comportamiento incorrecto y un secreto en el código.

**Por qué este y no otro:** es el riesgo real de la plataforma (dinero, duplicados y terceros inestables) en 3 horas. Premia decidir qué proteger, no escribir mucho.

## Señales

| Avanzar | Descartar, aunque el código funcione |
|---|---|
| Idempotencia en la base (clave única, transacción), no solo en memoria | Idempotencia con un `map` o un flag que se pierde al reiniciar |
| Pruebas de los caminos de error: duplicado, fuera de orden, timeout | Solo el camino feliz, o una cobertura alta sin casos de error |
| Encuentra las trampas, o explica por qué no las tocó | No ve el secreto, o "arregla" la prueba para que pase |
| El README dice qué no hizo y qué riesgo deja | Promete más de lo que entrega; el README contradice el código |
| Antes de reemitir, consulta al emisor por la referencia; pregunta quién decide un reembolso | Reintenta sin límite, reemite a ciegas o reembolsa sin que nadie lo apruebe |

## Cómo evalúo a alguien que lo resolvió con IA

La IA está permitida; evalúo el criterio. En la sustentación:

1. **Cambio en vivo:** "El PSP ahora manda dos eventos por compra. ¿Qué cambias?" Sin IA los primeros 10 minutos.
2. **Una línea al azar:** elijo una línea no trivial y pregunto qué pasa si la quito.
3. **La decisión descartada:** "¿Qué alternativa consideraste y por qué no?" Copiar y pegar no deja alternativas.
4. **El error de la IA:** "Muéstrame algo que la IA hizo mal y cómo lo detectaste." Quien la usó con criterio tiene un ejemplo.
5. **Revisión de un PR ajeno:** 15 minutos con un PR que tiene cinco defectos.

## Qué no evaluar aquí, y cómo cubrirlo

| No se evalúa en la prueba | Cómo se cubre |
|---|---|
| Algoritmos de memoria, trivia del framework, velocidad bajo presión | No se cubren: no predicen el trabajo; el ritmo se ve con objetivos a 30-60-90 días |
| Colaboración y feedback sostenidos | Referencias con preguntas concretas y *pairing* durante la primera semana |
| Diseño de sistemas a gran escala | Conversación aparte de 45 minutos sobre un incidente real que haya vivido |
| Ajuste al dominio de seguros | Se enseña. Se evalúa la curiosidad: si preguntó por el negocio en el ejercicio |
