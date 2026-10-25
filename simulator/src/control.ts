import type { FastifyInstance } from "fastify";

import type { Deps } from "./deps.js";

export function registerControl(app: FastifyInstance, _deps: Deps): void {
  app.get("/health", () => ({ status: "ok" }));
}
