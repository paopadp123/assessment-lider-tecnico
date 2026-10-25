import type { ChaosState } from "./chaos.js";
import type { Random } from "./random.js";
import type { Deliver, Scheduler } from "./webhook.js";

/** Lo que el simulador recibe desde afuera: así las pruebas controlan el azar, el reloj y las esperas. */
export interface Deps {
  chaos: ChaosState;
  random: Random;
  now: () => number;
  sleep: (ms: number) => Promise<void>;
  scheduler: Scheduler;
  deliver: Deliver;
  webhookUrl: string;
  webhookSecret: string;
}

export const realSleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));
