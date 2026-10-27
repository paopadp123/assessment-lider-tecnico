// Alertas del conductor y su política de reintentos. Funciones puras: la cola (services/alertQueue) las usa.

export type AlertStatus = 'pending' | 'sent' | 'rejected';

export interface DriverAlert {
  /** UUID generado en el teléfono al crear la alerta: es la clave de idempotencia en el servidor. */
  id: string;
  tripId: string;
  plate: string;
  type: 'PANIC';
  occurredAt: string;
  status: AlertStatus;
  attempts: number;
  /** Cuándo puede volver a intentarse (ms epoch). */
  nextAttemptAt: number;
  sentAt?: string;
  lastError?: string;
}

/** Resultado de un envío: enviada, rechazada para siempre o reintentar más tarde. */
export type SendResult = { kind: 'sent' } | { kind: 'rejected'; reason: string } | { kind: 'retry'; reason: string };

export const RETRY_BASE_MS = 2_000;
export const RETRY_MAX_MS = 60_000;

export function createAlert(id: string, tripId: string, plate: string, now: number): DriverAlert {
  throw new Error('not implemented');
}

/** Espera antes del siguiente intento: exponencial con tope y jitter completo. */
export function retryDelay(attempts: number, random: () => number): number {
  throw new Error('not implemented');
}

/** Aplica el resultado de un envío a la alerta. */
export function applyResult(alert: DriverAlert, result: SendResult, now: number, random: () => number): DriverAlert {
  throw new Error('not implemented');
}

/** Traduce la respuesta del servidor. Solo 400 y 409 son definitivos: reintentar no los arregla. */
export function classifyStatus(status: number): SendResult {
  throw new Error('not implemented');
}

export function isDue(alert: DriverAlert, now: number): boolean {
  throw new Error('not implemented');
}
