import { describe, expect, it } from "vitest";

import { isBroken, issueBody, sim } from "./helpers.js";

type Legacy = { result: string; code: string; message?: string; data?: { policy: { policyNumber: string; number: string } } };
type Lookup = { result: string; code: string; policies: { policyNumber: string }[] };

function issue(s: ReturnType<typeof sim>, body: object) {
  return s.app.inject({ method: "POST", url: "/issue", payload: body });
}

function lookup(s: ReturnType<typeof sim>, ref: string) {
  return s.app.inject({ method: "GET", url: `/policies?externalRef=${ref}` });
}

describe("emisor legado", () => {
  it("emite con el contrato verboso: 200 OK con la póliza y campos redundantes", async () => {
    const s = sim();

    const res = await issue(s, issueBody("order-1"));

    expect(res.statusCode).toBe(200);
    const body = res.json<Legacy>();
    expect(body.result).toBe("OK");
    expect(body.code).toBe("00");
    expect(body.data?.policy.policyNumber).toBe("POL-2026-000001");
    expect(body.data?.policy.number).toBe(body.data?.policy.policyNumber);
  });

  it("reemitir el mismo externalRef devuelve la misma póliza", async () => {
    const s = sim();

    const first = await issue(s, issueBody("order-1"));
    const second = await issue(s, issueBody("order-1"));

    expect(second.json<Legacy>().data?.policy.policyNumber).toBe(first.json<Legacy>().data?.policy.policyNumber);
  });

  it.each([
    ["externalReference distinto de externalRef", issueBody("order-1", { externalReference: "order-2" })],
    ["plateNumber distinto de plate", issueBody("order-1", { plateNumber: "XYZ987" })],
    ["sin policyRequest", { header: { transactionId: "tx" } }],
  ])("esconde en un 200 el error E-100 si la solicitud es inconsistente: %s", async (_name, body) => {
    const s = sim();

    const res = await issue(s, body);

    expect(res.statusCode).toBe(200);
    expect(res.json<Legacy>()).toMatchObject({ result: "ERROR", code: "E-100" });
  });

  it("esconde en un 200 el rechazo de negocio por placa inválida (E-104)", async () => {
    const s = sim();

    const res = await issue(s, issueBody("order-1", { plate: "12345" }));

    expect(res.json<Legacy>()).toMatchObject({ result: "ERROR", code: "E-104" });
  });

  it("timeout pero sí emitió: responde después de 8 s y la póliza ya se puede consultar", async () => {
    const s = sim({ issuer: { timeoutButIssuedRate: 1 } });

    const res = await issue(s, issueBody("order-1"));
    const found = await lookup(s, "order-1");

    expect(s.sleeps).toContain(8000);
    expect(res.json<Legacy>().result).toBe("OK");
    expect(found.json<Lookup>().policies.map((p) => p.policyNumber)).toEqual(["POL-2026-000001"]);
  });

  it("formato inválido: emite, pero la respuesta no se puede leer; la consulta la encuentra", async () => {
    const s = sim({ issuer: { invalidRate: 1 } });

    const res = await issue(s, issueBody("order-1"));
    const found = await lookup(s, "order-1");

    expect(res.statusCode).toBe(200);
    expect(isBroken(res.body)).toBe(true);
    expect(found.json<Lookup>().policies).toHaveLength(1);
  });

  it.each([
    ["técnico", { technicalErrorRate: 1 }, "E-901"],
    ["de negocio", { businessErrorRate: 1 }, "E-102"],
  ])("un error %s escondido en un 200 no emite", async (_name, chaos, code) => {
    const s = sim({ issuer: chaos });

    const res = await issue(s, issueBody("order-1"));
    const found = await lookup(s, "order-1");

    expect(res.statusCode).toBe(200);
    expect(res.json<Legacy>()).toMatchObject({ result: "ERROR", code });
    expect(found.json<Lookup>()).toEqual({ result: "OK", code: "00", policies: [] });
  });

  it("la consulta sin externalRef es una solicitud incompleta, también en un 200", async () => {
    const s = sim();

    const res = await s.app.inject({ method: "GET", url: "/policies" });

    expect(res.json<Legacy>()).toMatchObject({ result: "ERROR", code: "E-100" });
  });

  it("con la configuración por defecto: cada falla aparece en su proporción", async () => {
    const s = sim({
      issuer: { invalidRate: 0.05, technicalErrorRate: 0.025, businessErrorRate: 0.025, timeoutButIssuedRate: 0.1, minLatencyMs: 100, maxLatencyMs: 1500 },
    });
    const total = 2000;
    const counts = { broken: 0, technical: 0, business: 0 };

    for (let i = 0; i < total; i++) {
      const res = await issue(s, issueBody(`order-${i}`));
      if (isBroken(res.body)) {
        counts.broken++;
        continue;
      }
      const code = res.json<Legacy>().code;
      if (code === "E-901") counts.technical++;
      if (code === "E-102") counts.business++;
    }
    const timeouts = s.sleeps.filter((ms) => ms === 8000).length;

    expect(counts.broken / total).toBeGreaterThan(0.035);
    expect(counts.broken / total).toBeLessThan(0.065);
    expect(counts.technical / total).toBeGreaterThan(0.012);
    expect(counts.technical / total).toBeLessThan(0.038);
    expect(counts.business / total).toBeGreaterThan(0.012);
    expect(counts.business / total).toBeLessThan(0.038);
    expect(timeouts / total).toBeGreaterThan(0.08);
    expect(timeouts / total).toBeLessThan(0.12);
    const latencies = s.sleeps.filter((ms) => ms !== 8000);
    expect(Math.min(...latencies)).toBeGreaterThanOrEqual(100);
    expect(Math.max(...latencies)).toBeLessThanOrEqual(1500);
    expect(Math.max(...latencies) - Math.min(...latencies)).toBeGreaterThan(1200); // latencia variable
  });

  describe("caída total", () => {
    it("encendida a mano: 503 en la emisión y en la consulta", async () => {
      const s = sim({ issuer: { down: true } });

      const issued = await issue(s, issueBody("order-1"));
      const found = await lookup(s, "order-1");

      expect(issued.statusCode).toBe(503);
      expect(found.statusCode).toBe(503);
    });

    it("por ventana: cae al empezar la ventana y se recupera al terminar", async () => {
      const s = sim({ issuer: { outage: { startMs: 60_000, durationMs: 30_000 } } });
      const statusAt = async (offsetMs: number) => {
        s.clock.now = Date.parse("2026-10-24T10:00:00.000Z") + offsetMs;
        return (await issue(s, issueBody(`order-${offsetMs}`))).statusCode;
      };

      expect(await statusAt(59_999)).toBe(200);
      expect(await statusAt(60_000)).toBe(503);
      expect(await statusAt(89_999)).toBe(503);
      expect(await statusAt(90_000)).toBe(200);
    });
  });
});
