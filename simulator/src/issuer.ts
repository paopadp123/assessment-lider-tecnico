import type { FastifyInstance, FastifyReply } from "fastify";

import { sendBroken } from "./broken.js";
import type { Deps } from "./deps.js";
import { maskPlate } from "./mask.js";
import { between } from "./random.js";

/**
 * Emisor legado (Anexo B). Contrato feo a propósito, que la ACL en C# (HU-07) envuelve:
 * - campos redundantes que deben coincidir (externalRef/externalReference, plate/plateNumber…),
 * - errores de negocio y técnicos escondidos en un 200 { result: "ERROR", code, message },
 * - "timeout pero sí emitió": emite y responde después de 8 s,
 * - GET /policies?externalRef= para verificar antes de reintentar (lista vacía si no existe, nunca 404).
 * Reemitir el mismo externalRef devuelve la misma póliza. El estado vive en memoria.
 */

interface IssueInput {
  externalRef: string;
  plate: string;
  documentNumber: string;
  amount: string;
  currency: string;
}

export interface LegacyPolicy {
  policyNumber: string;
  number: string; // redundante: igual a policyNumber
  statusCode: "A";
  status: "ACTIVA";
  externalRef: string;
  issueDate: string; // dd/MM/yyyy, formato del sistema legado
  premium: { amount: string; currency: string };
}

type LegacyError = { result: "ERROR"; code: string; message: string };

const errors = {
  incomplete: { result: "ERROR", code: "E-100", message: "SOLICITUD INCOMPLETA O INCONSISTENTE" },
  activePolicy: { result: "ERROR", code: "E-102", message: "VEHICULO CON POLIZA VIGENTE" },
  invalidPlate: { result: "ERROR", code: "E-104", message: "PLACA NO VALIDA" },
  unavailable: { result: "ERROR", code: "E-901", message: "SISTEMA NO DISPONIBLE, INTENTE MAS TARDE" },
} satisfies Record<string, LegacyError>;

const plateFormat = /^[A-Z]{3}[0-9]{2}[0-9A-Z]$/;

export function registerIssuer(app: FastifyInstance, deps: Deps): void {
  const policies = new Map<string, LegacyPolicy>();
  let sequence = 0;

  const issue = (input: IssueInput): LegacyPolicy => {
    sequence += 1;
    const number = `POL-${new Date(deps.now()).getUTCFullYear()}-${String(sequence).padStart(6, "0")}`;
    const policy: LegacyPolicy = {
      policyNumber: number,
      number,
      statusCode: "A",
      status: "ACTIVA",
      externalRef: input.externalRef,
      issueDate: legacyDate(deps.now()),
      premium: { amount: input.amount, currency: input.currency },
    };
    policies.set(input.externalRef, policy);
    return policy;
  };

  const down = (reply: FastifyReply): FastifyReply =>
    reply.code(503).type("text/html").send("<html><body><h1>503 Service Unavailable</h1></body></html>");

  app.post("/issue", async (req, reply) => {
    if (deps.chaos.issuerDown(deps.now())) {
      req.log.warn("issue rejected (chaos: issuer down)");
      return down(reply);
    }
    const chaos = deps.chaos.get().issuer;
    await deps.sleep(between(deps.random, chaos.minLatencyMs, chaos.maxLatencyMs));

    const input = parseIssue(req.body);
    if (input === null) {
      return reply.code(200).send(errors.incomplete);
    }
    const log = req.log.child({ externalRef: input.externalRef, plate: maskPlate(input.plate) });

    const existing = policies.get(input.externalRef);
    if (existing !== undefined) {
      log.info({ policyNumber: existing.policyNumber }, "policy already issued, returning it");
      return reply.code(200).send(ok(existing));
    }
    if (!plateFormat.test(input.plate)) {
      return reply.code(200).send(errors.invalidPlate);
    }

    const roll = deps.random.next();
    let edge = chaos.invalidRate;
    if (roll < edge) {
      const policy = issue(input);
      log.warn({ policyNumber: policy.policyNumber }, "policy issued, response sent with invalid format (chaos)");
      return sendBroken(reply, 200, ok(policy));
    }
    edge += chaos.technicalErrorRate;
    if (roll < edge) {
      log.warn("issue failed (chaos: technical error)");
      return reply.code(200).send(errors.unavailable);
    }
    edge += chaos.businessErrorRate;
    if (roll < edge) {
      log.warn("issue rejected (chaos: business error)");
      return reply.code(200).send(errors.activePolicy);
    }
    edge += chaos.timeoutButIssuedRate;
    const policy = issue(input);
    if (roll < edge) {
      log.warn({ policyNumber: policy.policyNumber, delayMs: chaos.timeoutMs }, "policy issued, answering late (chaos: timeout but issued)");
      await deps.sleep(chaos.timeoutMs);
    } else {
      log.info({ policyNumber: policy.policyNumber }, "policy issued");
    }
    return reply.code(200).send(ok(policy));
  });

  app.get<{ Querystring: { externalRef?: string } }>("/policies", async (req, reply) => {
    if (deps.chaos.issuerDown(deps.now())) {
      return down(reply);
    }
    const chaos = deps.chaos.get().issuer;
    await deps.sleep(between(deps.random, chaos.minLatencyMs, chaos.maxLatencyMs));
    const ref = req.query.externalRef;
    if (ref === undefined || ref === "") {
      return reply.code(200).send(errors.incomplete);
    }
    const policy = policies.get(ref);
    return reply.code(200).send({ result: "OK", code: "00", policies: policy === undefined ? [] : [policy] });
  });
}

function ok(policy: LegacyPolicy) {
  return { result: "OK", code: "00", message: "EMITIDA", data: { policy } };
}

function legacyDate(ms: number): string {
  const d = new Date(ms);
  const dd = String(d.getUTCDate()).padStart(2, "0");
  const mm = String(d.getUTCMonth() + 1).padStart(2, "0");
  return `${dd}/${mm}/${d.getUTCFullYear()}`;
}

/** Lee la solicitud verbosa. Devuelve null si falta un campo o si los campos redundantes no coinciden. */
export function parseIssue(body: unknown): IssueInput | null {
  const header = field(body, "header");
  const request = field(body, "policyRequest");
  const vehicle = field(request, "vehicle");
  const holder = field(request, "holder");
  const premium = field(request, "premium");

  const transactionId = text(header, "transactionId");
  const externalRef = text(request, "externalRef");
  const externalReference = text(request, "externalReference");
  const productCode = text(field(request, "product"), "code");
  const plate = text(vehicle, "plate");
  const plateNumber = text(vehicle, "plateNumber");
  const documentNumber = text(field(holder, "document"), "number");
  const holderDocument = text(holder, "documentNumber");
  const amount = text(premium, "amount");
  const currency = text(premium, "currency");

  if (
    transactionId === null ||
    externalRef === null ||
    productCode === null ||
    plate === null ||
    documentNumber === null ||
    amount === null ||
    currency === null ||
    externalReference !== externalRef ||
    plateNumber !== plate ||
    holderDocument !== documentNumber
  ) {
    return null;
  }
  return { externalRef, plate, documentNumber, amount, currency };
}

function field(value: unknown, key: string): unknown {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  return (value as Record<string, unknown>)[key];
}

function text(value: unknown, key: string): string | null {
  const v = field(value, key);
  return typeof v === "string" && v !== "" ? v : null;
}
