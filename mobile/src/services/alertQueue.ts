import { applyResult, isDue, type DriverAlert, type SendResult } from '../domain/alerts';

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
  private alerts: DriverAlert[] | null = null;
  private loading: Promise<void> | null = null;
  private running: Promise<DriverAlert[]> | null = null;

  constructor(private readonly deps: QueueDeps) {}

  /** Historial guardado, de la más reciente a la más antigua. */
  async load(): Promise<DriverAlert[]> {
    // Una sola lectura compartida: dos lecturas en paralelo pisarían una alerta recién agregada.
    if (this.loading === null) {
      this.loading = this.read().then(
        saved => {
          this.alerts = saved;
        },
        (e: unknown) => {
          this.loading = null; // la próxima llamada vuelve a intentar leer
          throw e;
        },
      );
    }
    await this.loading;
    return [...(this.alerts ?? [])];
  }

  /** Guarda la alerta y luego intenta enviar lo pendiente. */
  async enqueue(alert: DriverAlert): Promise<DriverAlert[]> {
    await this.load();
    // Leer y escribir sin un await de por medio: un envío que termine en el medio no se revierte.
    this.alerts = trim([alert, ...(this.alerts ?? [])]);
    await this.persist();
    return this.flush();
  }

  /** Envía las alertas pendientes cuyo reintento ya venció. Dos llamadas simultáneas no duplican envíos. */
  flush(): Promise<DriverAlert[]> {
    if (this.running === null) {
      this.running = this.drain().finally(() => {
        this.running = null;
      });
    }
    return this.running;
  }

  private async drain(): Promise<DriverAlert[]> {
    await this.load();
    // Se repite hasta que no quede nada vencido: una alerta encolada durante un envío en curso sale en esta
    // misma pasada. En orden de creación, y se detiene en la primera falla transitoria.
    for (;;) {
      const next = [...(this.alerts ?? [])].reverse().find(a => isDue(a, this.deps.now()));
      if (next === undefined) {
        break;
      }
      const result = await this.deps.send(next).catch((e: unknown): SendResult => ({ kind: 'retry', reason: String(e) }));
      const updated = applyResult(next, result, this.deps.now(), this.deps.random);
      this.alerts = (this.alerts ?? []).map(a => (a.id === updated.id ? updated : a));
      await this.persist();
      if (result.kind === 'retry') {
        break;
      }
    }
    return [...(this.alerts ?? [])];
  }

  /**
   * Si el almacenamiento falla, el error sube: tratarlo como historial vacío haría que la siguiente escritura
   * borrara las alertas pendientes. Un JSON corrupto sí se descarta: no se puede leer de ninguna forma y no debe
   * impedir enviar alertas nuevas.
   */
  private async read(): Promise<DriverAlert[]> {
    const raw = await this.deps.storage.getItem(STORAGE_KEY);
    try {
      const parsed: unknown = raw === null ? [] : JSON.parse(raw);
      return Array.isArray(parsed) ? (parsed as DriverAlert[]) : [];
    } catch {
      return [];
    }
  }

  private persist(): Promise<void> {
    return this.deps.storage.setItem(STORAGE_KEY, JSON.stringify(this.alerts ?? []));
  }
}

/** Limita el historial descartando las terminadas más antiguas; nunca descarta una alerta pendiente. */
function trim(alerts: DriverAlert[]): DriverAlert[] {
  const kept = [...alerts];
  for (let i = kept.length - 1; i >= 0 && kept.length > MAX_HISTORY; i--) {
    if (kept[i]?.status !== 'pending') {
      kept.splice(i, 1);
    }
  }
  return kept;
}
