import { describe, expect, it } from "vitest";

import type { ChaosConfig } from "../src/chaos.js";
import { issueBody, quoteBody, sim } from "./helpers.js";

describe("control del caos en vivo", () => {
  it("GET /chaos muestra la configuración actual", async () => {
    const s = sim({ issuer: { timeoutButIssuedRate: 0.3 } });

    const res = await s.app.inject({ method: "GET", url: "/chaos" });

    expect(res.json<ChaosConfig>().issuer.timeoutButIssuedRate).toBe(0.3);
  });

  it("tumba y levanta el emisor sin reiniciar", async () => {
    const s = sim();
    const issueStatus = async (ref: string) => (await s.app.inject({ method: "POST", url: "/issue", payload: issueBody(ref) })).statusCode;

    await s.app.inject({ method: "POST", url: "/chaos", payload: { issuer: { down: true } } });
    const whileDown = await issueStatus("order-1");
    await s.app.inject({ method: "POST", url: "/chaos", payload: { issuer: { down: false } } });
    const afterRecovery = await issueStatus("order-2");

    expect(whileDown).toBe(503);
    expect(afterRecovery).toBe(200);
  });

  it("cambia solo lo que se envía", async () => {
    const s = sim();

    const res = await s.app.inject({ method: "POST", url: "/chaos", payload: { quoter: { errorRate: 1 } } });

    expect(res.statusCode).toBe(200);
    const config = res.json<ChaosConfig>();
    expect(config.quoter.errorRate).toBe(1);
    expect(config.quoter.invalidRate).toBe(0);
    expect((await s.app.inject({ method: "POST", url: "/v1/quotes", payload: quoteBody })).statusCode).toBe(500);
  });

  it.each([
    ["una probabilidad fuera de rango", { psp: { duplicateRate: 1.5 } }],
    ["fallas del emisor que suman más de 1", { issuer: { invalidRate: 0.6, timeoutButIssuedRate: 0.6 } }],
    ["latencia mínima mayor que la máxima", { quoter: { minLatencyMs: 500, maxLatencyMs: 100 } }],
    ["un campo desconocido", { issuer: { explode: true } }],
  ])("rechaza con 400 %s y no cambia nada", async (_name, patch) => {
    const s = sim();
    const before = s.deps.chaos.get();

    const res = await s.app.inject({ method: "POST", url: "/chaos", payload: patch });

    expect(res.statusCode).toBe(400);
    expect(s.deps.chaos.get()).toEqual(before);
  });

  it("con la misma semilla se repite exactamente la misma corrida", async () => {
    const run = async () => {
      const s = sim({ quoter: { errorRate: 0.5 } }, 1);
      await s.app.inject({ method: "POST", url: "/chaos", payload: { seed: 7 } });
      const statuses: number[] = [];
      for (let i = 0; i < 20; i++) {
        statuses.push((await s.app.inject({ method: "POST", url: "/v1/quotes", payload: quoteBody })).statusCode);
      }
      return statuses;
    };

    const first = await run();

    expect(await run()).toEqual(first);
    expect(new Set(first).size).toBe(2); // hubo éxitos y fallas
  });
});
