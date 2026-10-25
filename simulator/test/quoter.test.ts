import { describe, expect, it } from "vitest";

import { isBroken, quoteBody, sim } from "./helpers.js";

describe("cotizador", () => {
  it("responde con el contrato del Anexo B", async () => {
    const { app } = sim();

    const res = await app.inject({ method: "POST", url: "/v1/quotes", payload: quoteBody });

    expect(res.statusCode).toBe(200);
    const body = res.json<Record<string, unknown>>();
    expect(Object.keys(body).sort()).toEqual(["currency", "premium", "quoteId", "validUntil"]);
    expect(body.currency).toBe("COP");
    expect(typeof body.premium).toBe("number");
    expect(body.validUntil).toBe("2026-10-24T10:30:00.000Z");
  });

  it("rechaza con 400 una solicitud sin un campo obligatorio", async () => {
    const { app } = sim();
    const res = await app.inject({ method: "POST", url: "/v1/quotes", payload: { ...quoteBody, documentId: undefined } });

    expect(res.statusCode).toBe(400);
  });

  it("con los valores del enunciado: ~5% de 500, ~5% de formato inválido y latencia entre 100 ms y 3 s", async () => {
    const { app, sleeps } = sim({ quoter: { errorRate: 0.05, invalidRate: 0.05, minLatencyMs: 100, maxLatencyMs: 3000 } });
    const total = 2000;
    let errors = 0;
    let broken = 0;

    for (let i = 0; i < total; i++) {
      const res = await app.inject({ method: "POST", url: "/v1/quotes", payload: quoteBody });
      if (res.statusCode === 500) {
        errors++;
      } else if (isBroken(res.body)) {
        broken++;
      }
    }

    expect(errors / total).toBeGreaterThan(0.035);
    expect(errors / total).toBeLessThan(0.065);
    expect(broken / total).toBeGreaterThan(0.035);
    expect(broken / total).toBeLessThan(0.065);
    expect(Math.min(...sleeps)).toBeGreaterThanOrEqual(100);
    expect(Math.max(...sleeps)).toBeLessThanOrEqual(3000);
    expect(Math.max(...sleeps) - Math.min(...sleeps)).toBeGreaterThan(2500); // varía de verdad
  });
});
