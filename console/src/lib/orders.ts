import type { Order, OrderEvent, OrderStatus } from "./api/types";

export type OperatorAction = "retry-issuance" | "mark-for-reversal";

/** Acciones que el orquestador acepta en cada estado (mismas reglas que app/operator_actions.go). */
export function allowedActions(_status: OrderStatus): OperatorAction[] {
  return [];
}

/** Aplica un evento del flujo SSE a la página de órdenes visible: actualiza estado y fecha de la orden. */
export function patchOrders(orders: Order[], _event: OrderEvent): Order[] {
  return orders;
}

/** Agrega un evento a la línea de tiempo sin duplicarlo (el flujo puede reenviar tras reconectar). */
export function mergeTimeline(events: OrderEvent[], _event: OrderEvent): OrderEvent[] {
  return events;
}
