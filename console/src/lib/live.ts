import type { OrderEvent } from "./api/types";

const MAX_RECONNECT_MS = 30_000;

/**
 * Espera antes de abrir otra conexión SSE cuando el navegador ya no va a reintentar solo (el BFF respondió
 * un error en vez del flujo, p. ej. con el orquestador caído): 1 s, 2 s, 4 s… hasta 30 s.
 */
export function reconnectDelay(attempt: number): number {
  return Math.min(MAX_RECONNECT_MS, 1000 * 2 ** Math.max(0, attempt));
}

/** URL del flujo; con el último id recibido, el orquestador entrega lo que se perdió mientras no hubo conexión. */
export function streamUrl(lastEventId: string | null): string {
  return lastEventId === null ? "/api/stream" : `/api/stream?lastEventId=${encodeURIComponent(lastEventId)}`;
}

/** Lee un mensaje order-status. Devuelve null si no tiene la forma esperada, en vez de romper la consola. */
export function parseOrderEvent(data: string): OrderEvent | null {
  let v: unknown;
  try {
    v = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof v !== "object" || v === null) {
    return null;
  }
  const e = v as Record<string, unknown>;
  if (typeof e.id !== "number" || typeof e.orderId !== "string" || typeof e.to !== "string" || typeof e.occurredAt !== "string") {
    return null;
  }
  return v as OrderEvent;
}
