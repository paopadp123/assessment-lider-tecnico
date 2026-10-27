/**
 * UUID v4 a partir de una fuente aleatoria. Hermes no trae crypto.randomUUID y una librería nativa más es un
 * riesgo con la nueva arquitectura (lección de ADL con react-native-device-info). Para una clave de
 * idempotencia basta con que no choque: no es un secreto.
 */
export function uuidV4(random: () => number): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const v = Math.min(15, Math.floor(random() * 16));
    // La "y" lleva la variante RFC 4122: 8, 9, a o b.
    return (c === 'x' ? v : 8 + (v % 4)).toString(16);
  });
}
