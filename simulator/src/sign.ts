import { createHmac } from "node:crypto";

/**
 * Firma del webhook: HMAC-SHA256 en hexadecimal sobre "paymentId|status|occurredAt", el mismo algoritmo
 * que verifica el orquestador (orchestrator/internal/httpapi/signature.go). Las pruebas de los dos lados
 * comparten un vector fijo para que no se desalineen.
 */
export function signWebhook(secret: string, paymentId: string, status: string, occurredAt: string): string {
  return createHmac("sha256", secret).update(`${paymentId}|${status}|${occurredAt}`).digest("hex");
}
