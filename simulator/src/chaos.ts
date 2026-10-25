// Configuración del caos. Arranca con las variables de entorno y se puede cambiar en vivo con POST /chaos,
// para que la demo muestre cada falla cuando se necesita y no cuando cae por azar.

export interface QuoterChaos {
  errorRate: number; // 500
  invalidRate: number; // 200 con JSON roto
  minLatencyMs: number;
  maxLatencyMs: number;
}

export interface PspChaos {
  approveRate: number; // el resto es REJECTED
  invalidRate: number; // 202 con JSON roto (el pago sí se crea)
  duplicateRate: number; // el mismo webhook se entrega dos veces
  earlyWebhookRate: number; // el webhook llega antes que la respuesta 202
  minWebhookDelayMs: number;
  maxWebhookDelayMs: number;
  webhookRetries: number; // reintentos si el orquestador no responde 2xx
}

export interface IssuerChaos {
  minLatencyMs: number;
  maxLatencyMs: number;
  invalidRate: number; // emite y responde JSON roto
  technicalErrorRate: number; // 200 { result: "ERROR", code: "E-9xx" }, sin emitir
  businessErrorRate: number; // 200 { result: "ERROR", code: "E-1xx" }, sin emitir
  timeoutButIssuedRate: number; // emite y responde después de timeoutMs
  timeoutMs: number;
  down: boolean; // caída total encendida a mano
  outage: { startMs: number; durationMs: number } | null; // ventana de caída desde el arranque
}

export interface ChaosConfig {
  quoter: QuoterChaos;
  psp: PspChaos;
  issuer: IssuerChaos;
}

export type ChaosPatch = {
  [K in keyof ChaosConfig]?: Partial<ChaosConfig[K]>;
} & { seed?: number };

export class InvalidChaosError extends Error {}

export class ChaosState {
  private config: ChaosConfig;

  constructor(
    initial: ChaosConfig,
    private readonly startedAt: number,
  ) {
    validate(initial);
    this.config = structuredClone(initial);
  }

  get(): ChaosConfig {
    return structuredClone(this.config);
  }

  /** Aplica un cambio parcial. Si el resultado es inválido no cambia nada. */
  update(patch: ChaosPatch): ChaosConfig {
    const next: ChaosConfig = {
      quoter: { ...this.config.quoter, ...patch.quoter },
      psp: { ...this.config.psp, ...patch.psp },
      issuer: { ...this.config.issuer, ...patch.issuer },
    };
    validate(next);
    this.config = next;
    return this.get();
  }

  /** El emisor está caído si se encendió a mano o si `now` cae dentro de la ventana configurada. */
  issuerDown(now: number): boolean {
    const { down, outage } = this.config.issuer;
    if (down) {
      return true;
    }
    if (outage === null) {
      return false;
    }
    const elapsed = now - this.startedAt;
    return elapsed >= outage.startMs && elapsed < outage.startMs + outage.durationMs;
  }
}

function validate(c: ChaosConfig): void {
  const rates: [string, number][] = [
    ["quoter.errorRate", c.quoter.errorRate],
    ["quoter.invalidRate", c.quoter.invalidRate],
    ["psp.approveRate", c.psp.approveRate],
    ["psp.invalidRate", c.psp.invalidRate],
    ["psp.duplicateRate", c.psp.duplicateRate],
    ["psp.earlyWebhookRate", c.psp.earlyWebhookRate],
    ["issuer.invalidRate", c.issuer.invalidRate],
    ["issuer.technicalErrorRate", c.issuer.technicalErrorRate],
    ["issuer.businessErrorRate", c.issuer.businessErrorRate],
    ["issuer.timeoutButIssuedRate", c.issuer.timeoutButIssuedRate],
  ];
  for (const [name, value] of rates) {
    if (!Number.isFinite(value) || value < 0 || value > 1) {
      throw new InvalidChaosError(`${name} debe estar entre 0 y 1`);
    }
  }
  // Una sola tirada decide la respuesta de cada solicitud: las probabilidades de un mismo tercero se suman.
  if (c.quoter.errorRate + c.quoter.invalidRate > 1) {
    throw new InvalidChaosError("las fallas del cotizador suman más de 1");
  }
  const i = c.issuer;
  if (i.invalidRate + i.technicalErrorRate + i.businessErrorRate + i.timeoutButIssuedRate > 1) {
    throw new InvalidChaosError("las fallas del emisor suman más de 1");
  }
  const ranges: [string, number, number][] = [
    ["quoter latency", c.quoter.minLatencyMs, c.quoter.maxLatencyMs],
    ["psp webhook delay", c.psp.minWebhookDelayMs, c.psp.maxWebhookDelayMs],
    ["issuer latency", i.minLatencyMs, i.maxLatencyMs],
  ];
  for (const [name, min, max] of ranges) {
    if (!Number.isInteger(min) || !Number.isInteger(max) || min < 0 || min > max) {
      throw new InvalidChaosError(`${name}: se espera 0 <= min <= max (enteros, en ms)`);
    }
  }
  if (!Number.isInteger(c.psp.webhookRetries) || c.psp.webhookRetries < 0 || c.psp.webhookRetries > 10) {
    throw new InvalidChaosError("psp.webhookRetries debe ser un entero entre 0 y 10");
  }
  if (!Number.isInteger(i.timeoutMs) || i.timeoutMs < 0) {
    throw new InvalidChaosError("issuer.timeoutMs debe ser un entero >= 0");
  }
  if (i.outage !== null && (i.outage.startMs < 0 || i.outage.durationMs <= 0)) {
    throw new InvalidChaosError("issuer.outage: se espera startMs >= 0 y durationMs > 0");
  }
}
