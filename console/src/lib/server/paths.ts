import type { OperatorAction } from "@/lib/orders";

const uuidFormat = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

/**
 * Ruta del orquestador para una orden. Solo acepta un UUID: un id como "../ops/breakers" no puede
 * convertirse en otra ruta del orquestador con el token del operador.
 */
export function orderPath(id: string, suffix = ""): string | null {
  return uuidFormat.test(id) ? `/orders/${id}${suffix}` : null;
}

const actions: readonly OperatorAction[] = ["retry-issuance", "mark-for-reversal"];

export function isOperatorAction(v: string): v is OperatorAction {
  return (actions as readonly string[]).includes(v);
}
