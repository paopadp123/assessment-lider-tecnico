import type { FastifyInstance } from "fastify";

import { InvalidChaosError, type ChaosPatch } from "./chaos.js";
import type { Deps } from "./deps.js";

const rate = { type: "number", minimum: 0, maximum: 1 } as const;
const ms = { type: "integer", minimum: 0 } as const;

const patchSchema = {
  body: {
    type: "object",
    additionalProperties: false,
    properties: {
      seed: { type: "integer", minimum: 0 },
      quoter: {
        type: "object",
        additionalProperties: false,
        properties: { errorRate: rate, invalidRate: rate, minLatencyMs: ms, maxLatencyMs: ms },
      },
      psp: {
        type: "object",
        additionalProperties: false,
        properties: {
          approveRate: rate,
          invalidRate: rate,
          duplicateRate: rate,
          earlyWebhookRate: rate,
          minWebhookDelayMs: ms,
          maxWebhookDelayMs: ms,
          webhookRetries: { type: "integer", minimum: 0, maximum: 10 },
        },
      },
      issuer: {
        type: "object",
        additionalProperties: false,
        properties: {
          minLatencyMs: ms,
          maxLatencyMs: ms,
          invalidRate: rate,
          technicalErrorRate: rate,
          businessErrorRate: rate,
          timeoutButIssuedRate: rate,
          timeoutMs: ms,
          down: { type: "boolean" },
          outage: {
            anyOf: [
              { type: "null" },
              {
                type: "object",
                additionalProperties: false,
                required: ["startMs", "durationMs"],
                properties: { startMs: ms, durationMs: { type: "integer", minimum: 1 } },
              },
            ],
          },
        },
      },
    },
  },
} as const;

/**
 * Control del caos en vivo (solo existe en el simulador, que nunca va a producción; por eso no tiene
 * autenticación). GET muestra la configuración; POST cambia solo lo que se envía, p. ej.
 * { "issuer": { "down": true } } para tumbar el emisor durante la demo del circuit breaker.
 */
export function registerControl(app: FastifyInstance, deps: Deps): void {
  app.get("/chaos", () => deps.chaos.get());

  app.post<{ Body: ChaosPatch }>("/chaos", { schema: patchSchema }, (req, reply) => {
    try {
      const config = deps.chaos.update(req.body);
      if (req.body.seed !== undefined) {
        deps.random.reseed(req.body.seed);
      }
      req.log.info({ patch: req.body }, "chaos updated");
      return reply.code(200).send(config);
    } catch (err) {
      if (err instanceof InvalidChaosError) {
        return reply.code(400).send({ error: err.message });
      }
      throw err;
    }
  });

  // Sin log por solicitud: el healthcheck de Docker la llama cada 5 s y taparía los logs del caos.
  app.get("/health", { logLevel: "silent" }, () => ({ status: "ok" }));
}
