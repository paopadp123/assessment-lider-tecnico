// Textos y formatos de la consola: español de Colombia, pesos y hora de Bogotá (sin i18n, ver README).
import type { OrderStatus, ThirdPartyCall } from "@/lib/api/types";
import type { OperatorAction } from "@/lib/orders";

export const STATUS_LABEL: Record<OrderStatus, string> = {
  CREATED: "Creada",
  QUOTED: "Cotizada",
  PAYMENT_PENDING: "Pago pendiente",
  PAID: "Pagada",
  ISSUING: "Emitiendo",
  ISSUED: "Emitida",
  FAILED: "Fallida",
  REFUND_REQUIRED: "Requiere reverso",
};

/** Tono visual del estado: lo que pide atención del operador se distingue de lo que avanza solo. */
export const STATUS_TONE: Record<OrderStatus, "neutral" | "progress" | "ok" | "warn" | "danger"> = {
  CREATED: "neutral",
  QUOTED: "neutral",
  PAYMENT_PENDING: "progress",
  PAID: "progress",
  ISSUING: "warn",
  ISSUED: "ok",
  FAILED: "danger",
  REFUND_REQUIRED: "danger",
};

export const PARTY_LABEL: Record<ThirdPartyCall["party"], string> = {
  quoter: "Cotizador",
  psp: "Pasarela de pagos",
  issuer: "Emisor",
};

export const OPERATION_LABEL: Record<ThirdPartyCall["operation"], string> = {
  quote: "Cotizar",
  create_payment: "Crear pago",
  issue: "Emitir póliza",
  find_policy: "Consultar póliza",
};

export const OUTCOME_LABEL: Record<ThirdPartyCall["outcome"], string> = {
  OK: "OK",
  REJECTED: "Rechazado",
  UNAVAILABLE: "No disponible",
  TIMEOUT: "Sin respuesta a tiempo",
  CIRCUIT_OPEN: "No se llamó (breaker abierto)",
  NOT_FOUND: "No existe",
};

export const ACTION_LABEL: Record<OperatorAction, string> = {
  "retry-issuance": "Reintentar emisión",
  "mark-for-reversal": "Marcar para reverso",
};

export const ACTION_CONFIRM: Record<OperatorAction, string> = {
  "retry-issuance":
    "Se volverá a encolar la emisión. Antes de emitir se consulta al emisor, así que no se duplica la póliza.",
  "mark-for-reversal":
    "La orden quedará en «Requiere reverso». El reverso del pago lo ejecuta operaciones fuera del sistema.",
};

const money = new Intl.NumberFormat("es-CO", { style: "currency", currency: "COP", maximumFractionDigits: 0 });
const dateTime = new Intl.DateTimeFormat("es-CO", { dateStyle: "short", timeStyle: "medium", timeZone: "America/Bogota" });
const time = new Intl.DateTimeFormat("es-CO", { timeStyle: "medium", timeZone: "America/Bogota" });

export function formatMoney(amount: number, currency: string): string {
  return currency === "COP" ? money.format(amount) : `${amount.toFixed(2)} ${currency}`;
}

export function formatDateTime(iso: string): string {
  return dateTime.format(new Date(iso));
}

export function formatTime(iso: string): string {
  return time.format(new Date(iso));
}

export function formatDuration(ms: number): string {
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}
