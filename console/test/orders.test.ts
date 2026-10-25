import { describe, expect, it } from "vitest";

import type { Order, OrderEvent } from "@/lib/api/types";
import { allowedActions, mergeTimeline, patchOrders } from "@/lib/orders";

const order = (id: string, status: Order["status"]): Order => ({
  id,
  status,
  plate: "ABC***",
  vehicleYear: 2020,
  cityCode: "11001",
  createdAt: "2026-10-24T10:00:00Z",
  updatedAt: "2026-10-24T10:00:00Z",
});

const event = (id: number, orderId: string, to: OrderEvent["to"], from?: OrderEvent["from"]): OrderEvent => ({
  id,
  orderId,
  ...(from === undefined ? {} : { from }),
  to,
  reason: "test",
  actor: "system",
  occurredAt: `2026-10-24T10:00:0${id}Z`,
});

describe("allowedActions", () => {
  it("permite reintentar solo en ISSUING y REFUND_REQUIRED, y marcar reverso solo en PAID e ISSUING", () => {
    expect(allowedActions("ISSUING")).toEqual(["retry-issuance", "mark-for-reversal"]);
    expect(allowedActions("REFUND_REQUIRED")).toEqual(["retry-issuance"]);
    expect(allowedActions("PAID")).toEqual(["mark-for-reversal"]);
    for (const s of ["CREATED", "QUOTED", "PAYMENT_PENDING", "ISSUED", "FAILED"] as const) {
      expect(allowedActions(s)).toEqual([]);
    }
  });
});

describe("patchOrders", () => {
  it("cambia el estado y la fecha de la orden del evento, sin tocar las demás ni mutar la lista", () => {
    const list = [order("a", "PAID"), order("b", "ISSUING")];
    const out = patchOrders(list, event(5, "b", "ISSUED", "ISSUING"));

    expect(out[1]?.status).toBe("ISSUED");
    expect(out[1]?.updatedAt).toBe("2026-10-24T10:00:05Z");
    expect(out[0]).toBe(list[0]);
    expect(list[1]?.status).toBe("ISSUING");
  });

  it("ignora eventos de órdenes que no están en la página", () => {
    const list = [order("a", "PAID")];
    expect(patchOrders(list, event(1, "zzz", "ISSUED"))).toBe(list);
  });
});

describe("mergeTimeline", () => {
  it("agrega el evento nuevo en orden de id", () => {
    const out = mergeTimeline([event(1, "a", "CREATED"), event(3, "a", "QUOTED")], event(2, "a", "FAILED"));
    expect(out.map((e) => e.id)).toEqual([1, 2, 3]);
  });

  it("no duplica un evento reenviado al reconectar", () => {
    const list = [event(1, "a", "CREATED"), event(2, "a", "QUOTED")];
    expect(mergeTimeline(list, event(2, "a", "QUOTED"))).toBe(list);
  });
});
