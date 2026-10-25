import { describe, expect, it } from "vitest";

import { signWebhook } from "../src/sign.js";
import { issueBody, paymentBody, quoteBody, secret, sim } from "./helpers.js";

describe("firma del webhook", () => {
  // Mismo vector que TestSignatureVerifier_SharedVector en orchestrator/internal/httpapi/signature_test.go:
  // si uno de los dos lados cambia el algoritmo, falla su prueba.
  it("coincide con el vector compartido con el orquestador", () => {
    expect(signWebhook(secret, "pay-1", "APPROVED", "2026-10-24T10:00:00Z")).toBe(
      "c5192eb5811eabbd22ca4b83f466e13ecc8df6503e2550aa45d84c52c844cfac",
    );
  });
});

describe("logs", () => {
  it("nunca muestran la placa ni el documento completos, y llevan el correlation-id de la compra", async () => {
    const s = sim({ psp: { duplicateRate: 1 }, issuer: { timeoutButIssuedRate: 1 } });
    const headers = { "x-correlation-id": "corr-compra-1" };

    await s.app.inject({ method: "POST", url: "/v1/quotes", headers, payload: quoteBody });
    await s.app.inject({ method: "POST", url: "/v1/payments", headers: { ...headers, "idempotency-key": "order-1" }, payload: paymentBody });
    await s.app.inject({ method: "POST", url: "/issue", headers, payload: issueBody("order-1") });
    await s.app.inject({ method: "POST", url: "/v1/quotes", headers, payload: { ...quoteBody, vehicleYear: "x" } });
    await s.scheduler.runAll();

    const all = s.logs.join("");
    expect(all).not.toContain("ABC123");
    expect(all).not.toContain("1020304050");
    expect(all).toContain("ABC***");
    expect(all).toContain('"correlationId":"corr-compra-1"');
  });

  it("reemplaza un correlation-id con formato inválido en vez de llevarlo a los logs", async () => {
    const s = sim();

    await s.app.inject({ method: "GET", url: "/chaos", headers: { "x-correlation-id": "x".repeat(200) } });

    expect(s.logs.join("")).not.toContain("x".repeat(200));
  });
});
