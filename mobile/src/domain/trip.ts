// Viaje actual del conductor. Local en el prototipo: el GPS queda fuera (ver docs/mobile-memo.md).

export interface Trip {
  id: string;
  plate: string;
  startedAt: number;
}

export function startTrip(id: string, plate: string, now: number): Trip {
  throw new Error('not implemented');
}

/** Duración como HH:MM:SS. */
export function formatElapsed(ms: number): string {
  throw new Error('not implemented');
}
