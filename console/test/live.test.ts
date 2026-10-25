import { describe, expect, it } from "vitest";

import { parseOrderEvent, reconnectDelay, streamUrl } from "@/lib/live";

describe("reconnectDelay", () => {
  it("duplica la espera y no pasa de 30 s", () => {
    expect([0, 1, 2, 3, 4, 5, 6, 20].map(reconnectDelay)).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000]);
  });
});

describe("streamUrl", () => {
  it("manda el último id recibido para no perder eventos", () => {
    expect(streamUrl(null)).toBe("/api/stream");
    expect(streamUrl("41")).toBe("/api/stream?lastEventId=41");
  });
});

describe("parseOrderEvent", () => {
  it("lee un evento válido", () => {
    const e = parseOrderEvent('{"id":7,"orderId":"a","from":"PAID","to":"ISSUING","reason":"r","actor":"system","occurredAt":"2026-10-24T10:00:00Z"}');
    expect(e?.id).toBe(7);
    expect(e?.to).toBe("ISSUING");
  });

  it("ignora mensajes rotos o con otra forma", () => {
    expect(parseOrderEvent("{no es json")).toBeNull();
    expect(parseOrderEvent("null")).toBeNull();
    expect(parseOrderEvent('{"id":"7","orderId":"a"}')).toBeNull();
  });
});
