import type { FastifyInstance } from "fastify";

import type { Deps } from "./deps.js";

export function registerPsp(app: FastifyInstance, _deps: Deps): void {
  app.all("/psp-pendiente", (_req, reply) => reply.code(501).send());
}
