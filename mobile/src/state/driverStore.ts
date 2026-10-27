import AsyncStorage from '@react-native-async-storage/async-storage';
import { create } from 'zustand';
import { API_URL, DEVICE_TOKEN, REQUEST_TIMEOUT_MS, VEHICLE_PLATE } from '../config';
import { createAlert, type DriverAlert } from '../domain/alerts';
import { uuidV4 } from '../domain/id';
import { startTrip, type Trip } from '../domain/trip';
import { AlertQueue } from '../services/alertQueue';
import { checkHealth, sendAlert, type ApiDeps, type Connection } from '../services/api';

const TRIP_KEY = 'driver.trip.v1';
/** Id de viaje cuando el conductor presiona el pánico sin un viaje iniciado: la alerta nunca se bloquea. */
export const NO_TRIP = 'sin-viaje';

const api: ApiDeps = { baseUrl: API_URL, token: DEVICE_TOKEN, timeoutMs: REQUEST_TIMEOUT_MS, fetch: (...args) => fetch(...args) };
const queue = new AlertQueue({ storage: AsyncStorage, send: a => sendAlert(api, a), now: Date.now, random: Math.random });

interface DriverState {
  plate: string;
  connection: Connection | 'checking';
  lastCheckAt: number | null;
  trip: Trip | null;
  alerts: DriverAlert[];
  sending: boolean;
  /** Error al guardar una alerta en el teléfono (muy raro): se muestra para que el conductor llame. */
  panicError: string | null;
  init: () => Promise<void>;
  /** Consulta la salud del backend y, si está listo, envía lo pendiente que ya venció. */
  refresh: () => Promise<void>;
  beginTrip: () => Promise<void>;
  endTrip: () => Promise<void>;
  panic: () => Promise<void>;
}

export const useDriverStore = create<DriverState>((set, get) => ({
  plate: VEHICLE_PLATE,
  connection: 'checking',
  lastCheckAt: null,
  trip: null,
  alerts: [],
  sending: false,
  panicError: null,

  init: async () => {
    const [alerts, rawTrip] = await Promise.all([queue.load(), AsyncStorage.getItem(TRIP_KEY).catch(() => null)]);
    set({ alerts, trip: parseTrip(rawTrip) });
    await get().refresh();
  },

  refresh: async () => {
    const connection = await checkHealth(api);
    set({ connection, lastCheckAt: Date.now() });
    if (connection === 'online' && get().alerts.some(a => a.status === 'pending')) {
      set({ alerts: await queue.flush().catch(() => get().alerts) });
    }
  },

  beginTrip: async () => {
    const trip = startTrip(`trip-${uuidV4(Math.random).slice(0, 8)}`, get().plate, Date.now());
    set({ trip });
    await AsyncStorage.setItem(TRIP_KEY, JSON.stringify(trip));
  },

  endTrip: async () => {
    set({ trip: null });
    await AsyncStorage.removeItem(TRIP_KEY);
  },

  panic: async () => {
    const { trip, plate } = get();
    const alert = createAlert(uuidV4(Math.random), trip?.id ?? NO_TRIP, plate, Date.now());
    // Se muestra de inmediato como pendiente: el conductor ve que el teléfono la registró.
    set(s => ({ alerts: [alert, ...s.alerts], sending: true, panicError: null }));
    try {
      set({ alerts: await queue.enqueue(alert) });
    } catch {
      set(s => ({
        alerts: s.alerts.filter(a => a.id !== alert.id),
        panicError: 'No se pudo guardar la alerta en el teléfono. Intenta de nuevo o llama a la línea de emergencia.',
      }));
    } finally {
      set({ sending: false });
    }
  },
}));

function parseTrip(raw: string | null): Trip | null {
  try {
    const t: unknown = raw === null ? null : JSON.parse(raw);
    return t !== null && typeof t === 'object' && 'id' in t && 'startedAt' in t ? (t as Trip) : null;
  } catch {
    return null;
  }
}
