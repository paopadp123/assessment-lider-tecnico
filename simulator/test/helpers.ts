import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import { ChaosState, type ChaosConfig, type ChaosPatch } from "../src/chaos.js";
import type { Deps } from "../src/deps.js";
import { SeededRandom } from "../src/random.js";
import type { Scheduler } from "../src/webhook.js";

export const secret = "test-secret-with-at-least-32-bytes!!";
export const start = Date.parse("2026-10-24T10:00:00.000Z");

/** Sin caos ni esperas: cada prueba enciende solo la falla que verifica. */
export function calm(): ChaosConfig {
  return {
    quoter: { errorRate: 0, invalidRate: 0, minLatencyMs: 0, maxLatencyMs: 0 },
    psp: {
      approveRate: 1,
      invalidRate: 0,
      duplicateRate: 0,
      earlyWebhookRate: 0,
      minWebhookDelayMs: 1000,
      maxWebhookDelayMs: 1000,
      webhookRetries: 3,
    },
    issuer: {
      minLatencyMs: 0,
      maxLatencyMs: 0,
      invalidRate: 0,
      technicalErrorRate: 0,
      businessErrorRate: 0,
      timeoutButIssuedRate: 0,
      timeoutMs: 8000,
      down: false,
      outage: null,
    },
  };
}

/** Avanza el tiempo a pedido: ejecuta las tareas programadas en el orden en que vencen. */
export class ManualScheduler implements Scheduler {
  private tasks: { due: number; seq: number; task: () => Promise<void> }[] = [];
  private seq = 0;
  time = 0;

  after(ms: number, task: () => Promise<void>): void {
    this.tasks.push({ due: this.time + ms, seq: this.seq++, task });
  }

  get pending(): number {
    return this.tasks.length;
  }

  async runAll(): Promise<void> {
    for (let guard = 0; this.tasks.length > 0; guard++) {
      if (guard > 100_000) {
        throw new Error("demasiadas tareas: ¿un bucle de reintentos?");
      }
      this.tasks.sort((a, b) => a.due - b.due || a.seq - b.seq);
      const next = this.tasks.shift();
      if (next === undefined) {
        return;
      }
      this.time = next.due;
      await next.task();
    }
  }
}

export interface Delivery {
  at: number;
  url: string;
  body: Record<string, string>;
}

export interface TestSim {
  app: FastifyInstance;
  deps: Deps;
  scheduler: ManualScheduler;
  deliveries: Delivery[];
  sleeps: number[];
  logs: string[];
  /** Status que devuelve el orquestador falso a cada webhook, en orden (-1 = no responde); después 200. */
  webhookStatuses: number[];
  clock: { now: number };
}

export function sim(patch: ChaosPatch = {}, seed = 42): TestSim {
  const scheduler = new ManualScheduler();
  const deliveries: Delivery[] = [];
  const sleeps: number[] = [];
  const logs: string[] = [];
  const webhookStatuses: number[] = [];
  const clock = { now: start };
  const chaos = new ChaosState(calm(), start);
  chaos.update(patch);
  const deps: Deps = {
    chaos,
    random: new SeededRandom(seed),
    now: () => clock.now,
    sleep: (ms) => {
      sleeps.push(ms);
      return Promise.resolve();
    },
    scheduler,
    deliver: (url, body) => {
      deliveries.push({ at: scheduler.time, url, body: JSON.parse(body) as Record<string, string> });
      const status = webhookStatuses.shift() ?? 200;
      // -1: el orquestador nunca responde.
      return status === -1 ? new Promise<number>(() => undefined) : Promise.resolve(status);
    },
    webhookUrl: "http://orchestrator.test/webhooks/payments",
    webhookSecret: secret,
  };
  const app = buildApp(deps, { level: "info", stream: { write: (line: string) => logs.push(line) } });
  return { app, deps, scheduler, deliveries, sleeps, logs, webhookStatuses, clock };
}

export const quoteBody = { plate: "ABC123", vehicleYear: 2020, cityCode: "11001", documentId: "1020304050" };

export const paymentBody = { quoteId: "qt_1", amount: 412000, currency: "COP", customerRef: "order-1" };

export function issueBody(externalRef: string, overrides: { plate?: string; plateNumber?: string; externalReference?: string } = {}) {
  const plate = overrides.plate ?? "ABC123";
  return {
    header: { transactionId: `tx-${externalRef}`, channel: "DIGITAL", requestDate: "20261024100000" },
    policyRequest: {
      externalRef,
      externalReference: overrides.externalReference ?? externalRef,
      product: { code: "SOAT", productCode: "SOAT" },
      vehicle: { plate, plateNumber: overrides.plateNumber ?? plate, modelYear: "2020", cityCode: "11001" },
      holder: { document: { type: "CC", number: "1020304050" }, documentNumber: "1020304050" },
      premium: { amount: "412000.00", value: 412000, currency: "COP" },
    },
  };
}

/** true si el cuerpo no se puede leer como JSON (formato inválido). */
export function isBroken(body: string): boolean {
  try {
    JSON.parse(body);
    return false;
  } catch {
    return true;
  }
}

/** Proporción observada, para comparar con la configurada con un margen. */
export function ratio(count: number, total: number): number {
  return count / total;
}
