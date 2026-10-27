// Viaje actual del conductor. Local en el prototipo: el GPS queda fuera (ver docs/mobile-memo.md).

export interface Trip {
  id: string;
  plate: string;
  startedAt: number;
}

export function startTrip(id: string, plate: string, now: number): Trip {
  return { id, plate, startedAt: now };
}

/** Duración como HH:MM:SS. */
export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(Math.floor(total / 3600))}:${pad(Math.floor((total % 3600) / 60))}:${pad(total % 60)}`;
}
