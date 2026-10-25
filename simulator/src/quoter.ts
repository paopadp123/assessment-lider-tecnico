import type { FastifyInstance } from "fastify";

import { sendBroken } from "./broken.js";
import type { Deps } from "./deps.js";
import { maskDocument, maskPlate } from "./mask.js";
import { between, randomId } from "./random.js";

interface QuoteRequest {
  plate: string;
  vehicleYear: number;
  cityCode: string;
  documentId: string;
}

const quoteSchema = {
  body: {
    type: "object",
    required: ["plate", "vehicleYear", "cityCode", "documentId"],
    properties: {
      plate: { type: "string", minLength: 1 },
      vehicleYear: { type: "integer" },
      cityCode: { type: "string", minLength: 1 },
      documentId: { type: "string", minLength: 1 },
    },
  },
} as const;

const quoteValidity = 30 * 60 * 1000;

/** Cotizador (Anexo B): POST /v1/quotes → 200 { quoteId, premium, currency, validUntil }. */
export function registerQuoter(app: FastifyInstance, deps: Deps): void {
  app.post<{ Body: QuoteRequest }>("/v1/quotes", { schema: quoteSchema }, async (req, reply) => {
    const chaos = deps.chaos.get().quoter;
    await deps.sleep(between(deps.random, chaos.minLatencyMs, chaos.maxLatencyMs));

    const roll = deps.random.next();
    if (roll < chaos.errorRate) {
      req.log.warn("quote failed (chaos: 500)");
      return reply.code(500).send({ error: "internal error" });
    }
    const quote = {
      quoteId: randomId(deps.random, "qt"),
      premium: between(deps.random, 250, 900) * 1000,
      currency: "COP",
      validUntil: new Date(deps.now() + quoteValidity).toISOString(),
    };
    req.log.info(
      { quoteId: quote.quoteId, plate: maskPlate(req.body.plate), documentId: maskDocument(req.body.documentId) },
      "quote issued",
    );
    if (roll < chaos.errorRate + chaos.invalidRate) {
      req.log.warn({ quoteId: quote.quoteId }, "quote sent with invalid format (chaos)");
      return sendBroken(reply, 200, quote);
    }
    return reply.code(200).send(quote);
  });
}
