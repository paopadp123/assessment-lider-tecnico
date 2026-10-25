import { describe, expect, it } from "vitest";

import { ConfigError, loadConfig } from "../src/config.js";
import { secret } from "./helpers.js";

describe("configuración", () => {
  it("por defecto usa los valores del enunciado", () => {
    const c = loadConfig({ PSP_WEBHOOK_SECRET: secret }).chaos;

    expect(c.quoter).toMatchObject({ errorRate: 0.05, invalidRate: 0.05, minLatencyMs: 100, maxLatencyMs: 3000 });
    expect(c.psp).toMatchObject({ duplicateRate: 0.1, invalidRate: 0.05 });
    expect(c.issuer).toMatchObject({ invalidRate: 0.05, timeoutMs: 8000, down: false, outage: null });
  });

  it("exige el secreto del webhook con al menos 32 bytes", () => {
    expect(() => loadConfig({})).toThrow(ConfigError);
    expect(() => loadConfig({ PSP_WEBHOOK_SECRET: "corto" })).toThrow(ConfigError);
  });

  it("lee cada probabilidad, latencia y la ventana de caída", () => {
    const c = loadConfig({
      PSP_WEBHOOK_SECRET: secret,
      QUOTER_ERROR_RATE: "0.2",
      ISSUER_MAX_LATENCY_MS: "4000",
      ISSUER_OUTAGE_START_S: "120",
      ISSUER_OUTAGE_DURATION_S: "60",
      SIM_SEED: "7",
    });

    expect(c.chaos.quoter.errorRate).toBe(0.2);
    expect(c.chaos.issuer.maxLatencyMs).toBe(4000);
    expect(c.chaos.issuer.outage).toEqual({ startMs: 120_000, durationMs: 60_000 });
    expect(c.seed).toBe(7);
  });

  it.each([
    ["una probabilidad mayor que 1", { QUOTER_ERROR_RATE: "5" }],
    ["un número inválido", { ISSUER_TIMEOUT_MS: "ocho" }],
    ["la ventana de caída a medias", { ISSUER_OUTAGE_START_S: "120" }],
    ["una ventana de caída de duración 0", { ISSUER_OUTAGE_START_S: "120", ISSUER_OUTAGE_DURATION_S: "0" }],
  ])("falla al arrancar con %s", (_name, env) => {
    expect(() => loadConfig({ PSP_WEBHOOK_SECRET: secret, ...env })).toThrow(ConfigError);
  });
});
