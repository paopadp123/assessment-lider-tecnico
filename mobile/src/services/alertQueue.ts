import { type DriverAlert, type SendResult } from '../domain/alerts';

/** Lo que la cola necesita del almacenamiento (AsyncStorage lo cumple). */
export interface KeyValueStore {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
}

export interface QueueDeps {
  storage: KeyValueStore;
  send: (alert: DriverAlert) => Promise<SendResult>;
  now: () => number;
  random: () => number;
}

export const STORAGE_KEY = 'driver.alerts.v1';
export const MAX_HISTORY = 50;

/**
 * Cola offline de alertas. Cada alerta se guarda en el teléfono antes de intentar enviarla; los envíos van de
 * a uno y la cola se detiene en la primera falla transitoria (si el servidor no responde, no tiene sentido
 * insistir con las demás: eso es el backpressure del lado del teléfono).
 */
export class AlertQueue {
  constructor(private readonly deps: QueueDeps) {}

  /** Historial guardado, de la más reciente a la más antigua. */
  async load(): Promise<DriverAlert[]> {
    throw new Error('not implemented');
  }

  /** Guarda la alerta y luego intenta enviar lo pendiente. */
  async enqueue(alert: DriverAlert): Promise<DriverAlert[]> {
    throw new Error('not implemented');
  }

  /** Envía las alertas pendientes cuyo reintento ya venció. Dos llamadas simultáneas no duplican envíos. */
  async flush(): Promise<DriverAlert[]> {
    throw new Error('not implemented');
  }
}
