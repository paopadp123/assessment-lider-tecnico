// Configuración de compilación. Babel la inyecta desde el entorno al empaquetar (babel.config.js):
//   DRIVER_API_URL=http://10.0.2.2:8080 DRIVER_DEVICE_TOKEN=... npx react-native run-android
// 10.0.2.2 es el host visto desde el emulador de Android.
export const API_URL = process.env.DRIVER_API_URL || 'http://10.0.2.2:8080';
export const DEVICE_TOKEN = process.env.DRIVER_DEVICE_TOKEN || '';
/** Placa del vehículo asignado a este teléfono. En producción vendría del aprovisionamiento del dispositivo. */
export const VEHICLE_PLATE = process.env.DRIVER_PLATE || 'ABC123';

export const HEALTH_EVERY_MS = 5_000;
export const REQUEST_TIMEOUT_MS = 5_000;
