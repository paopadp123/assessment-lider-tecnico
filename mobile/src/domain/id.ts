/**
 * UUID v4 a partir de una fuente aleatoria. Hermes no trae crypto.randomUUID y una librería nativa más es un
 * riesgo con la nueva arquitectura (lección de ADL con react-native-device-info). Para una clave de
 * idempotencia basta con que no choque: no es un secreto.
 */
export function uuidV4(random: () => number): string {
  throw new Error('not implemented');
}
