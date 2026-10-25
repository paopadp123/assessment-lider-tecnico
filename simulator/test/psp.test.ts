import { createHmac } from "node:crypto";

import { describe, expect, it } from "vitest";

import { isBroken, paymentBody, secret, sim } from "./helpers.js";

function pay(s: ReturnType<typeof sim>, key: string, body: object = paymentBody) {
  return s.app.inject({ method: "POST", url: "/v1/payments", headers: { "idempotency-key": key }, payload: body });
}

describe("PSP", () => {
  it("responde 202 PENDING y después envía el webhook firmado al orquestador", async () => {
    const s = sim();

    const res = await pay(s, "order-1");

    expect(res.statusCode).toBe(202);
    const { paymentId, status } = res.json<{ paymentId: string; status: string }>();
    expect(status).toBe("PENDING");
    expect(s.deliveries).toHaveLength(0); // todavía no: llega después

    await s.scheduler.runAll();

    expect(s.deliveries).toHaveLength(1);
    const delivery = s.deliveries[0];
    expect(delivery?.url).toBe("http://orchestrator.test/webhooks/payments");
    expect(delivery?.at).toBe(1000);
    const event = delivery?.body ?? {};
    expect(event.paymentId).toBe(paymentId);
    expect(event.status).toBe("APPROVED");
    const expected = createHmac("sha256", secret).update(`${paymentId}|APPROVED|${event.occurredAt ?? ""}`).digest("hex");
    expect(event.signature).toBe(expected);
  });

  it("exige Idempotency-Key", async () => {
    const s = sim();

    const res = await s.app.inject({ method: "POST", url: "/v1/payments", payload: paymentBody });

    expect(res.statusCode).toBe(400);
  });

  it("la misma Idempotency-Key devuelve el mismo pago y no genera otro webhook", async () => {
    const s = sim();

    const first = await pay(s, "order-1");
    const second = await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(second.statusCode).toBe(202);
    expect(second.json()).toEqual(first.json());
    expect(s.deliveries).toHaveLength(1);
  });

  it("la misma Idempotency-Key con otro pago es un conflicto", async () => {
    const s = sim();
    await pay(s, "order-1");

    const res = await pay(s, "order-1", { ...paymentBody, amount: 1 });

    expect(res.statusCode).toBe(409);
  });

  it("una respuesta con formato inválido sí crea el pago: repetir con la misma clave lo recupera", async () => {
    const s = sim({ psp: { invalidRate: 1 } });

    const broken = await pay(s, "order-1");
    s.deps.chaos.update({ psp: { invalidRate: 0 } });
    const retry = await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(broken.statusCode).toBe(202);
    expect(isBroken(broken.body)).toBe(true);
    expect(retry.json<{ paymentId: string }>().paymentId).toBe(s.deliveries[0]?.body.paymentId);
    expect(s.deliveries).toHaveLength(1);
  });

  it("fuera de orden: el webhook puede llegar antes que la respuesta 202", async () => {
    const s = sim({ psp: { earlyWebhookRate: 1 } });

    const res = await pay(s, "order-1");

    expect(s.deliveries).toHaveLength(1); // ya llegó, sin avanzar el tiempo
    expect(s.deliveries[0]?.body.paymentId).toBe(res.json<{ paymentId: string }>().paymentId);
  });

  it("si el orquestador no responde el webhook temprano, el 202 sale igual después de 1 s", async () => {
    const s = sim({ psp: { earlyWebhookRate: 1 } });
    s.webhookStatuses.push(-1);

    const res = await pay(s, "order-1");

    expect(res.statusCode).toBe(202);
    expect(s.sleeps).toContain(1000);
  });

  it("con approveRate en 0 todos los pagos se rechazan", async () => {
    const s = sim({ psp: { approveRate: 0 } });

    await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(s.deliveries[0]?.body.status).toBe("REJECTED");
  });

  it("un duplicado es el mismo evento, con la misma firma, entregado más tarde", async () => {
    const s = sim({ psp: { duplicateRate: 1 } });

    await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(s.deliveries).toHaveLength(2);
    const [first, second] = s.deliveries;
    expect(second?.body).toEqual(first?.body);
    expect(second?.at).toBeGreaterThan(first?.at ?? Infinity);
  });

  it("con los valores del enunciado: ~10% de webhooks duplicados y ~5% de respuestas inválidas", async () => {
    const s = sim({ psp: { duplicateRate: 0.1, invalidRate: 0.05, approveRate: 0.9 } });
    const total = 2000;
    let broken = 0;

    for (let i = 0; i < total; i++) {
      const res = await pay(s, `order-${i}`);
      if (isBroken(res.body)) {
        broken++;
      }
    }
    await s.scheduler.runAll();

    const duplicates = s.deliveries.length - total;
    const rejected = s.deliveries.filter((d) => d.body.status === "REJECTED").length;
    expect(duplicates / total).toBeGreaterThan(0.08);
    expect(duplicates / total).toBeLessThan(0.12);
    expect(broken / total).toBeGreaterThan(0.035);
    expect(broken / total).toBeLessThan(0.065);
    expect(rejected / s.deliveries.length).toBeGreaterThan(0.08);
    expect(rejected / s.deliveries.length).toBeLessThan(0.12);
  });

  it("reintenta el webhook con espera creciente si el orquestador no responde 2xx", async () => {
    const s = sim();
    s.webhookStatuses.push(503, 0); // un error y luego sin respuesta; el tercero entra

    await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(s.deliveries.map((d) => d.at)).toEqual([1000, 2000, 4000]);
  });

  it("abandona el webhook después del último reintento", async () => {
    const s = sim({ psp: { webhookRetries: 2 } });
    s.webhookStatuses.push(500, 500, 500, 500, 500);

    await pay(s, "order-1");
    await s.scheduler.runAll();

    expect(s.deliveries).toHaveLength(3); // el intento original + 2 reintentos
    expect(s.scheduler.pending).toBe(0);
  });
});
