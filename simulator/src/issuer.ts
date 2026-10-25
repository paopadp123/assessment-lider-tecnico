import type { FastifyInstance } from "fastify";

import type { Deps } from "./deps.js";

export function registerIssuer(app: FastifyInstance, _deps: Deps): void {
  app.all("/issuer-pendiente", (_req, reply) => reply.code(501).send());
}
