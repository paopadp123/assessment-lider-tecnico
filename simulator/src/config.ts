import type { ChaosConfig } from "./chaos.js";

export interface Config {
  port: number;
  seed: number;
  webhookUrl: string;
  webhookSecret: string;
  chaos: ChaosConfig;
}

type Env = Record<string, string | undefined>;

export class ConfigError extends Error {}

// Los valores por defecto son los del enunciado: 500 en ~5% del cotizador con latencia de 100 ms a 3 s,
// 10% de webhooks duplicados, 5% de respuestas con formato inválido y respuesta a los 8 s en el
// "timeout pero sí emitió".
export function loadConfig(env: Env): Config {
  const webhookSecret = env.PSP_WEBHOOK_SECRET ?? "";
  if (Buffer.byteLength(webhookSecret) < 32) {
    throw new ConfigError("PSP_WEBHOOK_SECRET es obligatorio y debe tener al menos 32 bytes (el mismo del orquestador)");
  }
  const outageStart = env.ISSUER_OUTAGE_START_S;
  const outageDuration = env.ISSUER_OUTAGE_DURATION_S;
  if ((outageStart === undefined) !== (outageDuration === undefined)) {
    throw new ConfigError("ISSUER_OUTAGE_START_S e ISSUER_OUTAGE_DURATION_S van juntas");
  }
  if (outageDuration !== undefined && int(env, "ISSUER_OUTAGE_DURATION_S", 0) === 0) {
    throw new ConfigError("ISSUER_OUTAGE_DURATION_S debe ser mayor que 0");
  }
  return {
    port: int(env, "PORT", 8090),
    seed: int(env, "SIM_SEED", Math.floor(Math.random() * 2 ** 31)),
    webhookUrl: env.PSP_WEBHOOK_URL ?? "http://localhost:8080/webhooks/payments",
    webhookSecret,
    chaos: {
      quoter: {
        errorRate: rate(env, "QUOTER_ERROR_RATE", 0.05),
        invalidRate: rate(env, "QUOTER_INVALID_RATE", 0.05),
        minLatencyMs: int(env, "QUOTER_MIN_LATENCY_MS", 100),
        maxLatencyMs: int(env, "QUOTER_MAX_LATENCY_MS", 3000),
      },
      psp: {
        approveRate: rate(env, "PSP_APPROVE_RATE", 0.9),
        invalidRate: rate(env, "PSP_INVALID_RATE", 0.05),
        duplicateRate: rate(env, "PSP_DUPLICATE_RATE", 0.1),
        earlyWebhookRate: rate(env, "PSP_EARLY_WEBHOOK_RATE", 0.1),
        minWebhookDelayMs: int(env, "PSP_MIN_WEBHOOK_DELAY_MS", 500),
        maxWebhookDelayMs: int(env, "PSP_MAX_WEBHOOK_DELAY_MS", 3000),
        webhookRetries: int(env, "PSP_WEBHOOK_RETRIES", 3),
      },
      issuer: {
        minLatencyMs: int(env, "ISSUER_MIN_LATENCY_MS", 100),
        maxLatencyMs: int(env, "ISSUER_MAX_LATENCY_MS", 1500),
        invalidRate: rate(env, "ISSUER_INVALID_RATE", 0.05),
        technicalErrorRate: rate(env, "ISSUER_TECHNICAL_ERROR_RATE", 0.025),
        businessErrorRate: rate(env, "ISSUER_BUSINESS_ERROR_RATE", 0.025),
        timeoutButIssuedRate: rate(env, "ISSUER_TIMEOUT_BUT_ISSUED_RATE", 0.1),
        timeoutMs: int(env, "ISSUER_TIMEOUT_MS", 8000),
        down: env.ISSUER_DOWN === "true",
        outage:
          outageStart === undefined
            ? null
            : { startMs: int(env, "ISSUER_OUTAGE_START_S", 0) * 1000, durationMs: int(env, "ISSUER_OUTAGE_DURATION_S", 0) * 1000 },
      },
    },
  };
}

function int(env: Env, name: string, fallback: number): number {
  const raw = env[name];
  if (raw === undefined || raw === "") {
    return fallback;
  }
  const value = Number(raw);
  if (!Number.isInteger(value) || value < 0) {
    throw new ConfigError(`${name} debe ser un entero >= 0, llegó "${raw}"`);
  }
  return value;
}

function rate(env: Env, name: string, fallback: number): number {
  const raw = env[name];
  if (raw === undefined || raw === "") {
    return fallback;
  }
  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0 || value > 1) {
    throw new ConfigError(`${name} debe estar entre 0 y 1, llegó "${raw}"`);
  }
  return value;
}
