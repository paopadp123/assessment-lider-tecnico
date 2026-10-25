// Azar con semilla: con la misma semilla, la misma secuencia de fallas. Las pruebas fijan la semilla y la
// demo puede repetir una corrida exacta con SIM_SEED o POST /chaos { "seed": n }.
export interface Random {
  /** Número en [0, 1). */
  next(): number;
  reseed(seed: number): void;
}

/** mulberry32: pequeño, rápido y suficiente para inyectar fallas (no es criptográfico). */
export class SeededRandom implements Random {
  private state: number;

  constructor(seed: number) {
    this.state = seed >>> 0;
  }

  next(): number {
    this.state = (this.state + 0x6d2b79f5) >>> 0;
    let t = this.state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }

  reseed(seed: number): void {
    this.state = seed >>> 0;
  }
}

/** Entero en [min, max]. */
export function between(random: Random, min: number, max: number): number {
  return min + Math.floor(random.next() * (max - min + 1));
}

/** Identificador corto para quoteId y paymentId. */
export function randomId(random: Random, prefix: string): string {
  let id = "";
  for (let i = 0; i < 12; i++) {
    id += Math.floor(random.next() * 36).toString(36);
  }
  return `${prefix}_${id}`;
}
