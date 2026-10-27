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
  return { id, tripId, plate, type: 'PANIC', occurredAt: new Date(now).toISOString(), status: 'pending', attempts: 0, nextAttemptAt: now };
}

/** Espera antes del siguiente intento: exponencial con tope y jitter completo. */
export function retryDelay(attempts: number, random: () => number): number {
  const ceiling = Math.min(RETRY_MAX_MS, RETRY_BASE_MS * 2 ** Math.max(0, attempts - 1));
  return Math.floor(random() * ceiling);
}

/** Aplica el resultado de un envío a la alerta. */
export function applyResult(alert: DriverAlert, result: SendResult, now: number, random: () => number): DriverAlert {
  const attempts = alert.attempts + 1;
  switch (result.kind) {
    case 'sent':
      return { ...alert, attempts, status: 'sent', sentAt: new Date(now).toISOString(), lastError: undefined };
    case 'rejected':
      return { ...alert, attempts, status: 'rejected', lastError: result.reason };
    case 'retry':
      return { ...alert, attempts, status: 'pending', lastError: result.reason, nextAttemptAt: now + Math.max(1, retryDelay(attempts, random)) };
  }
}

/** Traduce la respuesta del servidor. Solo 400 y 409 son definitivos: reintentar no los arregla. */
export function classifyStatus(status: number): SendResult {
  if (status === 200 || status === 201) {
    return { kind: 'sent' };
  }
  if (status === 400 || status === 409) {
    return { kind: 'rejected', reason: `HTTP ${status}` };
  }
  return { kind: 'retry', reason: `HTTP ${status}` };
}

export function isDue(alert: DriverAlert, now: number): boolean {
  return alert.status === 'pending' && alert.nextAttemptAt <= now;
}
