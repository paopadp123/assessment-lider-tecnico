import type { FastifyInstance } from "fastify";

import { sendBroken } from "./broken.js";
import type { Deps } from "./deps.js";
import { between, randomId } from "./random.js";
import { signWebhook } from "./sign.js";
import { WebhookDispatcher, type PaymentEvent } from "./webhook.js";

interface PaymentRequest {
  quoteId: string;
  amount: number | string;
  currency: string;
  customerRef: string;
}

interface PaymentResponse {
  paymentId: string;
  status: "PENDING";
}

const paymentSchema = {
  headers: {
    type: "object",
    required: ["idempotency-key"],
    properties: { "idempotency-key": { type: "string", minLength: 1 } },
  },
  body: {
    type: "object",
    required: ["quoteId", "amount", "currency", "customerRef"],
    properties: {
      quoteId: { type: "string", minLength: 1 },
      amount: { type: ["number", "string"] },
      currency: { type: "string", minLength: 1 },
      customerRef: { type: "string", minLength: 1 },
    },
  },
} as const;

const earlyWebhookWaitMs = 1000;

/** Lo que se guarda de un pago para responder igual a una repetición con la misma clave. */
interface StoredPayment {
  fingerprint: string;
  response: PaymentResponse;
}

/**
 * Pasarela de pagos (Anexo B): POST /v1/payments → 202 { paymentId, status: "PENDING" }; el resultado
 * llega después por webhook firmado. Respeta Idempotency-Key: la misma clave devuelve el mismo pago y no
 * genera otro webhook. El estado vive en memoria: reiniciar el simulador lo borra.
 */
export function registerPsp(app: FastifyInstance, deps: Deps): void {
  const payments = new Map<string, StoredPayment>();
  const webhooks = new WebhookDispatcher(
    deps.webhookUrl,
    deps.deliver,
    deps.scheduler,
    () => deps.chaos.get().psp.webhookRetries,
    app.log,
  );

  app.post<{ Body: PaymentRequest; Headers: { "idempotency-key": string } }>(
    "/v1/payments",
    { schema: paymentSchema },
    async (req, reply) => {
      const key = req.headers["idempotency-key"];
      const { quoteId, amount, currency, customerRef } = req.body;
      const fingerprint = JSON.stringify([quoteId, String(amount), currency, customerRef]);

      const existing = payments.get(key);
      if (existing !== undefined) {
        if (existing.fingerprint !== fingerprint) {
          return reply.code(409).send({ error: "idempotency key reused with a different payment" });
        }
        req.log.info({ paymentId: existing.response.paymentId }, "payment replayed (same idempotency key)");
        return reply.code(202).send(existing.response);
      }

      const chaos = deps.chaos.get().psp;
      const response: PaymentResponse = { paymentId: randomId(deps.random, "pay"), status: "PENDING" };
      payments.set(key, { fingerprint, response });

      const status = deps.random.next() < chaos.approveRate ? "APPROVED" : "REJECTED";
      const occurredAt = new Date(deps.now()).toISOString();
      const event: PaymentEvent = {
        paymentId: response.paymentId,
        status,
        occurredAt,
        signature: signWebhook(deps.webhookSecret, response.paymentId, status, occurredAt),
      };
      const early = deps.random.next() < chaos.earlyWebhookRate;
      const duplicate = deps.random.next() < chaos.duplicateRate;
      const invalid = deps.random.next() < chaos.invalidRate;
      const delay = between(deps.random, chaos.minWebhookDelayMs, chaos.maxWebhookDelayMs);
      req.log.info({ paymentId: response.paymentId, result: status, early, duplicate, invalid }, "payment accepted");

      if (early) {
        // Fuera de orden: el webhook llega antes de que el orquestador reciba el 202. Se espera la entrega
        // hasta earlyWebhookWaitMs: si el orquestador tarda, el 202 sale igual y el webhook sigue su curso.
        await Promise.race([webhooks.now(event), deps.sleep(earlyWebhookWaitMs)]);
      } else {
        webhooks.later(delay, event);
      }
      if (duplicate) {
        // El mismo evento (misma firma) otra vez, más tarde: el orquestador debe ignorarlo.
        webhooks.later(delay + between(deps.random, 1000, 5000), event);
      }
      if (invalid) {
        return sendBroken(reply, 202, response);
      }
      return reply.code(202).send(response);
    },
  );
}
