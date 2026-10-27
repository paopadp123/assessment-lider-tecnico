# Memorando: librería de ubicación en segundo plano

**Para:** jefe de ingeniería · **De:** líder técnico · **Fecha:** 27 de octubre de 2026
**Asunto:** ¿adoptamos `mauron85/react-native-background-geolocation` para la app del conductor?

## Decisión

**No la adoptaría.** No recibe código desde 2019 y, tal como está, no sirve para una app que hoy se publique en Google Play: falla al iniciar en Android 12+ y no declara lo que Android 14 exige para rastrear en segundo plano. Recomiendo **Transistorsoft v5** con licencia comercial, tras un piloto de dos semanas. Plan B: `expo-location` + `expo-task-manager`, con la cola offline construida por nosotros.

## Evidencia del repositorio

Revisado en `master`, SHA `f0d4342` ([fork propio](https://github.com/paopadp123/react-native-background-geolocation)), el 27 de octubre de 2026, con la API de GitHub y npm.

- **Mantenimiento.** El último commit con código es `6836156` (16-09-2019, *version bump*). El siguiente y último, `f0d4342` (06-10-2020), solo quita las donaciones del README. La última versión en npm es la 0.6.3, del 16-09-2019. Los núcleos nativos son submódulos fijados en `58464fe` (Android, 16-09-2019) y `5149178` (iOS, 17-12-2018). Hay 92 issues y 7 PR abiertos: #585 ("Not working in Android 12, API 31 and above"), #587 (iOS 17), #588 (falla de compilación por el atributo `package` del manifiesto) y #591 ("Release outdated"). `.github/stale.yml` está configurado para cerrar los issues tras 187 días sin actividad, así que los 425 cerrados no indican soluciones.
- **Arquitectura de React Native.** El módulo extiende `ReactContextBaseJavaModule` y emite por `DeviceEventManagerModule` (`BackgroundGeolocationModule.java:30`): es el *bridge* antiguo. La librería se compila contra RN 0.55.3 y Android Gradle Plugin 3.4.1 (`android/build.gradle`). Desde RN 0.82 la nueva arquitectura es la única ([blog de React Native, 08-10-2025](https://reactnative.dev/blog/2025/10/08/react-native-0.82)), así que el módulo dependería de la capa de compatibilidad, y nadie lo ha probado ahí.
- **Android actual.** Crea sus `PendingIntent` sin `FLAG_IMMUTABLE` ni `FLAG_MUTABLE` (`DistanceFilterLocationProvider.java:88`, `:92`, `:96` y `:100` del submódulo). Con `targetSdk` ≥ 31, Android lanza una excepción al crearlos. El manifiesto no declara `ACCESS_BACKGROUND_LOCATION` ni `foregroundServiceType="location"` (obligatorio en Android 14). Su `targetSdk` por defecto es 28 (`VERSIONS.gradle:5`), y desde el 31-08-2026 Google Play exige `targetSdk` 36 para publicar actualizaciones.
- **Licencia.** Apache 2.0: no es el problema. Su diseño (proveedores intercambiables, cola SQLite, envío por lotes) sí es la referencia de lo que necesitamos.

## Alternativas y costo total a 3 años

Supuestos: un desarrollador móvil senior con experiencia nativa cuesta ~US$30 por hora a la empresa, y hay una sola app. Las horas son estimaciones con rango.

| Camino | Inicial | Por año | Total 3 años |
|---|---|---|---|
| **A. mauron85 tal cual** | 0 | — | No viable: falla en Android 12+ |
| **B. Fork propio de mauron85** | 240–400 h: Turbo Modules, Android 12–16, iOS 17+, matriz de dispositivos | 120–160 h: una versión de Android y una de iOS por año, más los fabricantes que matan servicios | **US$18.000–26.400** |
| **C. Transistorsoft v5** | Licencia Starter US$399 (perpetua, 1 app, iOS y Android, 1 año de actualizaciones) + 40–80 h de integración | 20–40 h de actualización + renovación (precio no publicado; supongo US$400 en los años 2 y 3) | **US$4.200–7.200** |
| **D. `expo-location` + `expo-task-manager`** | 120–160 h: cola SQLite, sincronización, detección de viaje | ~40 h | **US$7.200–8.400** |
| **E. Módulo nativo propio** (Kotlin + Swift) | 320–480 h | 120–160 h | **US$20.400–28.800** |

- **C:** la versión 5.8.1 salió el 26-10-2026. Soporta la arquitectura *bridgeless* desde la 4.18.0 y los Turbo Modules desde la 5.0 (CHANGELOG), y tiene complemento para Expo. Su README exige la licencia solo en las compilaciones de release, en iOS y Android. El precio sale de su página de compra (27-10-2026), con una clave por identificador de app. Riesgo: un solo proveedor, con núcleo cerrado.
- **D** optimiza menos la batería (no detecta el movimiento por sí solo). **B y E** cuestan por cada versión anual de Android e iOS, y el conocimiento queda en una o dos personas.

## Recomendación y condiciones de reversión

**Recomiendo C**, tras un piloto de dos semanas con 50 vehículos en **compilación de release con la licencia de prueba de 30 días**, con tres metas medidas: batería ≤ 4 % por hora de viaje, posiciones perdidas ≤ 0,5 % tras túneles y zonas sin señal, y datos ≤ 20 MB por vehículo al mes (cálculo en el README, sección 5). El fork cuesta entre 2,5 y 6 veces más y no es nuestro negocio.

**Revertiría a D (o a E si la ubicación se vuelve el producto) si:**
1. Transistorsoft pasa 9 meses sin versión, o tarda más de 3 meses en soportar una versión de React Native o de Android que Google Play exija.
2. Seguridad o legal no aceptan un binario cerrado que procesa la ubicación de los conductores.
3. El piloto no cumple las metas y el soporte no lo resuelve en dos semanas.
4. El licenciamiento por app deja de ser razonable (por ejemplo, decenas de apps de marca blanca).

La integración va detrás de una interfaz propia (`LocationTracker`: iniciar viaje, detener, posiciones pendientes). Si hay que cambiar de camino, se cambia una implementación, no la app.
