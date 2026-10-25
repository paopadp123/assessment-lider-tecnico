import type { FastifyBaseLogger } from "fastify";

/** Programa una tarea para más tarde. En las pruebas se reemplaza por uno manual que avanza a pedido. */
export interface Scheduler {
  after(ms: number, task: () => Promise<void>): void;
}

export const timerScheduler: Scheduler = {
  after(ms, task) {
    setTimeout(() => void task(), ms);
  },
};

/** Entrega un webhook y devuelve el status HTTP, o 0 si no hubo respuesta. */
export type Deliver = (url: string, body: string) => Promise<number>;

export const httpDeliver: Deliver = async (url, body) => {
  try {
    const res = await fetch(url, {
      method: "POST",
      body,
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(5000),
    });
    await res.body?.cancel();
    return res.status;
  } catch {
    return 0;
  }
};

export interface PaymentEvent {
  paymentId: string;
  status: "APPROVED" | "REJECTED";
  occurredAt: string;
  signature: string;
}

/**
 * Entrega los webhooks del PSP. Si el orquestador no responde 2xx reintenta con espera exponencial
 * (1 s, 2 s, 4 s…) hasta `retries()` veces; después lo abandona, como haría un PSP real.
 */
export class WebhookDispatcher {
  constructor(
    private readonly url: string,
    private readonly deliver: Deliver,
    private readonly scheduler: Scheduler,
    private readonly retries: () => number,
    private readonly log: FastifyBaseLogger,
  ) {}

  later(delayMs: number, event: PaymentEvent): void {
    this.scheduler.after(delayMs, () => this.now(event));
  }

  async now(event: PaymentEvent, attempt = 0): Promise<void> {
    const status = await this.deliver(this.url, JSON.stringify(event));
    if (status >= 200 && status < 300) {
      this.log.info({ paymentId: event.paymentId, status: event.status, attempt }, "webhook delivered");
      return;
    }
    if (attempt >= this.retries()) {
      this.log.warn({ paymentId: event.paymentId, httpStatus: status, attempt }, "webhook abandoned");
      return;
    }
    this.log.warn({ paymentId: event.paymentId, httpStatus: status, attempt }, "webhook failed, retrying");
    this.scheduler.after(1000 * 2 ** attempt, () => this.now(event, attempt + 1));
  }
}
