import { randomUUID } from "node:crypto";

import Fastify, { LogController, type FastifyInstance, type FastifyServerOptions } from "fastify";

import { registerControl } from "./control.js";
import type { Deps } from "./deps.js";
import { registerIssuer } from "./issuer.js";
import { registerPsp } from "./psp.js";
import { registerQuoter } from "./quoter.js";

// Mismo formato que acepta el orquestador (orchestrator/internal/trace): un valor raro no llega a los logs.
const validCorrelationId = /^[A-Za-z0-9._:-]{1,64}$/;

/**
 * Arma el simulador de los 3 terceros en un solo servicio. Cada log de una solicitud lleva el
 * X-Correlation-ID que envía el orquestador (o uno generado), para cruzarlo con los logs de la compra.
 */
export function buildApp(deps: Deps, logger: FastifyServerOptions["logger"] = true): FastifyInstance {
  const app = Fastify({
    logger,
    // Por defecto Fastify borra en silencio los campos que el esquema no conoce. En /chaos un error de
    // tipeo ({ "dwon": true }) respondería 200 sin cambiar nada; así se rechaza con 400.
    // allowUnionTypes: el monto del PSP llega como número o como texto.
    ajv: { customOptions: { removeAdditional: false, allowUnionTypes: true } },
    logController: new LogController({ requestIdLogLabel: "correlationId" }),
    requestIdHeader: false,
    genReqId: (req) => {
      const id = req.headers["x-correlation-id"];
      return typeof id === "string" && validCorrelationId.test(id) ? id : randomUUID();
    },
  });
  registerQuoter(app, deps);
  registerPsp(app, deps);
  registerIssuer(app, deps);
  registerControl(app, deps);
  return app;
}
