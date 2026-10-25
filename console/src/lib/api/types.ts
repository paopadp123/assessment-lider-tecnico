// Tipos del contrato del orquestador. Salen de schema.d.ts, que se genera desde orchestrator/api/openapi.yaml
// (npm run gen:api); el CI falla si quedó desactualizado.
import type { components } from "./schema";

type Schemas = components["schemas"];

export type Order = Schemas["Order"];
export type OrderStatus = Schemas["OrderStatus"];
export type OrderList = Schemas["OrderList"];
export type OrderEvent = Schemas["OrderEvent"];
export type ThirdPartyCall = Schemas["ThirdPartyCall"];
export type BreakerStatus = Schemas["BreakerStatus"];
export type LoginRequest = Schemas["LoginRequest"];
export type LoginResponse = Schemas["LoginResponse"];
export type ApiError = Schemas["Error"];

export const ORDER_STATUSES: readonly OrderStatus[] = [
  "CREATED",
  "QUOTED",
  "PAYMENT_PENDING",
  "PAID",
  "ISSUING",
  "ISSUED",
  "FAILED",
  "REFUND_REQUIRED",
];
