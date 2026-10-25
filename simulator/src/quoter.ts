import type { FastifyInstance } from "fastify";

import type { Deps } from "./deps.js";

export function registerQuoter(app: FastifyInstance, _deps: Deps): void {
  app.all("/quoter-pendiente", (_req, reply) => reply.code(501).send());
}
