import { classifyStatus, type DriverAlert, type SendResult } from '../domain/alerts';

export type Connection = 'online' | 'degraded' | 'offline';

export interface ApiDeps {
  baseUrl: string;
  token: string;
  timeoutMs: number;
  fetch: typeof fetch;
}

/** fetch con tiempo límite: en un túnel la conexión no falla, se queda colgada. */
async function request(deps: ApiDeps, path: string, init: RequestInit = {}): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), deps.timeoutMs);
  try {
    return await deps.fetch(`${deps.baseUrl}${path}`, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

/** Envía una alerta al orquestador (POST /driver/alerts). Nunca lanza: devuelve qué hacer con ella. */
export async function sendAlert(deps: ApiDeps, alert: DriverAlert): Promise<SendResult> {
  try {
    const res = await request(deps, '/driver/alerts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${deps.token}` },
      body: JSON.stringify({ alertId: alert.id, plate: alert.plate, tripId: alert.tripId, type: alert.type, occurredAt: alert.occurredAt }),
    });
    return classifyStatus(res.status);
  } catch {
    return { kind: 'retry', reason: 'sin conexión' };
  }
}

/**
 * Estado del backend según /health/ready: listo (online), responde pero no está listo (degraded) o no
 * responde (offline). Mide lo que le importa al conductor: si una alerta llegaría ahora.
 */
export async function checkHealth(deps: ApiDeps): Promise<Connection> {
  try {
    const res = await request(deps, '/health/ready');
    return res.ok ? 'online' : 'degraded';
  } catch {
    return 'offline';
  }
}
