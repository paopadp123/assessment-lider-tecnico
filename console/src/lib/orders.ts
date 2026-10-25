import type { Order, OrderEvent, OrderStatus } from "./api/types";

export type OperatorAction = "retry-issuance" | "mark-for-reversal";

/** Acciones que el orquestador acepta en cada estado (mismas reglas que app/operator_actions.go). */
export function allowedActions(status: OrderStatus): OperatorAction[] {
  const actions: OperatorAction[] = [];
  if (status === "ISSUING" || status === "REFUND_REQUIRED") {
    actions.push("retry-issuance");
  }
  if (status === "PAID" || status === "ISSUING") {
    actions.push("mark-for-reversal");
  }
  return actions;
}

/** Aplica un evento del flujo SSE a la página de órdenes visible: actualiza estado y fecha de la orden. */
export function patchOrders(orders: Order[], event: OrderEvent): Order[] {
  const i = orders.findIndex((o) => o.id === event.orderId);
  const current = orders[i];
  if (current === undefined) {
    return orders;
  }
  const next = [...orders];
  next[i] = { ...current, status: event.to, updatedAt: event.occurredAt };
  return next;
}

/** Agrega un evento a la línea de tiempo sin duplicarlo (el flujo puede reenviar tras reconectar). */
export function mergeTimeline(events: OrderEvent[], event: OrderEvent): OrderEvent[] {
  if (events.some((e) => e.id === event.id)) {
    return events;
  }
  return [...events, event].sort((a, b) => a.id - b.id);
}
