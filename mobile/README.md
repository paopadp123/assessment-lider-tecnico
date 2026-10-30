# App del conductor (prototipo, bonus del Bloque 3)

React Native **0.86.2** sin Expo, con la nueva arquitectura y Hermes. Una pantalla con las cuatro piezas que pide el bloque, conectada al orquestador del Bloque 1:

| Sección | Qué hace |
|---|---|
| **Estado de conexión** | Consulta `GET /health/ready` cada 5 s: *Conectado*, *Servidor degradado* (responde pero no está listo) o *Sin conexión*. Mide si una alerta llegaría ahora, no solo si hay red. Muestra cuántas alertas esperan. |
| **Viaje actual** | Iniciar y terminar un viaje (id, placa y tiempo transcurrido). Se guarda en el teléfono y sobrevive a cerrar la app. |
| **Botón de pánico** | Se activa **manteniéndolo presionado 2 s**, con una barra que se llena: evita disparos accidentales sin pedir una confirmación extra en una emergencia. Funciona aunque no haya un viaje iniciado. |
| **Historial local** | Cada alerta con su estado: *Enviada*, *Pendiente (N intentos)* o *Rechazada*. Persistido en el teléfono. |

## Offline first, en pequeño

Es la respuesta de la sección 5 del README principal, implementada:

1. La alerta se crea con un **UUID generado en el teléfono** y se **guarda en AsyncStorage antes** de intentar enviarla.
2. `POST /driver/alerts` es **idempotente por ese UUID**: 201 la primera vez, 200 en un reintento. Un reintento tras un timeout nunca duplica la alerta (`orchestrator/internal/adapters/postgres/driver_alert_repository.go`, con prueba de reintentos concurrentes).
3. Sin conexión, la alerta queda **pendiente** y se reintenta con **espera exponencial y jitter completo** (2 s a 60 s). Los envíos van de a uno y la cola **se detiene en la primera falla**: si el servidor no responde, no insiste con las demás.
4. Solo 400 y 409 son definitivos (*Rechazada*, con el motivo a la vista): datos inválidos o un id reutilizado con otro contenido. Un 401 (token mal configurado), un 429 o un 5xx se reintentan, y el historial muestra el último error. El servidor acepta la hora de un teléfono con el reloj adelantado (hasta 24 h) y guarda además su propia hora de recepción: un reloj mal configurado no rechaza una alerta de pánico.
5. Si el almacenamiento del teléfono falla al leer, la cola no lo trata como historial vacío (la siguiente escritura borraría lo pendiente): el pánico muestra el error para que el conductor llame.

## Decisiones (y lo que se tomó de ADL)

- **Base de los proyectos de Acelerex Digital Labs:** la misma versión de React Native, la paleta oscura, la tipografía, los tokens y los componentes (`Button`, `Badge`, `Icon`) de Acelerex CV, y el estado con zustand.
- **Pocas dependencias nativas:** AsyncStorage, `react-native-svg` y `safe-area-context`, todas probadas con la nueva arquitectura en apps de ADL publicadas. **Sin NetInfo ni librerías de dispositivo:** en ADL, `react-native-device-info` no registraba su módulo con la nueva arquitectura y *bridgeless*, y crasheaba la app. El UUID se genera en JavaScript por la misma razón.
- **Sin GPS en el prototipo:** el rastreo en segundo plano es la decisión del [memorando](../docs/mobile-memo.md) (Transistorsoft tras un piloto). El prototipo deja listo el lugar donde entraría: el viaje.
- **Autenticación de prototipo:** un token de dispositivo (`DRIVER_DEVICE_TOKEN`). En una app instalada en miles de teléfonos un token compartido no es un secreto; en producción cada vehículo tendría su certificado (AWS IoT Core, [`docs/architecture.md`](../docs/architecture.md)).
- **HTTP sin cifrar solo hacia el backend local** (`10.0.2.2`, `network_security_config.xml`); cualquier otro destino exige HTTPS.

## Correrla

Con el backend arriba (`bash scripts/dev-env.sh && docker compose up --build --wait` en la raíz) y un emulador de Android:

```bash
npm ci
export DRIVER_API_URL=http://10.0.2.2:8080          # el host visto desde el emulador
export DRIVER_DEVICE_TOKEN=$(grep '^DRIVER_DEVICE_TOKEN=' ../.env | cut -d= -f2)
npx react-native run-android                        # depuración, con Metro
# o, como se verificó: cd android && ./gradlew assembleRelease && adb install -r app/build/outputs/apk/release/app-release.apk
```

La configuración entra al compilar (`babel.config.js` inyecta solo esas variables). Si se cambia, hay que volver a empaquetar.

**Demo de la cola offline:** con la app abierta, `docker compose stop orchestrator` → la conexión pasa a *Sin conexión* → mantener el botón de pánico → la alerta queda *Pendiente* → `docker compose start orchestrator` → en unos segundos pasa a *Enviada*, y en la base hay **una sola fila**:

```bash
docker compose exec postgres psql -U app -d policies -c "SELECT id, trip_id, received_at FROM driver_alerts ORDER BY received_at DESC LIMIT 5;"
```

## Pruebas

```bash
npm run lint && npm run typecheck && npm test   # también en el CI (job mobile)
```

La lógica se prueba sin el dispositivo: dominio (estados, espera con jitter, UUID, viaje), la cola (guarda antes de enviar, reintento único al reconectar, orden, se detiene en la primera falla, rechazos, dos `flush` simultáneos, carga concurrente, historial corrupto, límite sin perder pendientes) y el cliente HTTP (contrato, token, timeout). Las pruebas se escribieron primero, contra stubs (rojo), y dos se validaron con mutaciones.

**Fuera del prototipo:** iOS (no se compiló), GPS, notificaciones al centro de operación y la vista de alertas en la consola.
