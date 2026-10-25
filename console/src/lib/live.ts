import type { OrderEvent } from "./api/types";

/** Espera antes de abrir otra conexión SSE cuando el navegador ya no va a reintentar solo. */
export function reconnectDelay(_attempt: number): number {
  return 0;
}

/** URL del flujo; con el último id recibido, el orquestador entrega lo que se perdió. */
export function streamUrl(_lastEventId: string | null): string {
  return "";
}

/** Lee un mensaje order-status; null si no tiene la forma esperada. */
export function parseOrderEvent(_data: string): OrderEvent | null {
  return null;
}
