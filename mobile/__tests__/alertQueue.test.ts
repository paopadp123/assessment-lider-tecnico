import { createAlert, type DriverAlert, type SendResult } from '../src/domain/alerts';
import { AlertQueue, type KeyValueStore, MAX_HISTORY, STORAGE_KEY } from '../src/services/alertQueue';

const START = Date.parse('2026-10-27T20:00:00.000Z');

class MemoryStore implements KeyValueStore {
  data = new Map<string, string>();
  writes = 0;
  getItem(key: string) {
    return Promise.resolve(this.data.get(key) ?? null);
  }
  setItem(key: string, value: string) {
    this.writes++;
    this.data.set(key, value);
    return Promise.resolve();
  }
  saved(): DriverAlert[] {
    return JSON.parse(this.data.get(STORAGE_KEY) ?? '[]');
  }
}

/** Servidor falso: responde según `online` y registra cada envío. */
function fakeServer() {
  const server = {
    online: true,
    sent: [] as string[],
    storeAtSend: [] as number[],
    send: (alert: DriverAlert): Promise<SendResult> => {
      server.sent.push(alert.id);
      return Promise.resolve(server.online ? { kind: 'sent' } : { kind: 'retry', reason: 'sin conexión' });
    },
  };
  return server;
}

function setup() {
  const storage = new MemoryStore();
  const server = fakeServer();
  const clock = { now: START };
  const queue = new AlertQueue({ storage, send: a => server.send(a), now: () => clock.now, random: () => 1 });
  return { storage, server, clock, queue };
}

describe('cola offline de alertas', () => {
  it('guarda la alerta en el teléfono antes de enviarla', async () => {
    const { storage, server, queue } = setup();
    let savedBeforeSend = -1;
    const original = server.send;
    server.send = a => {
      savedBeforeSend = storage.saved().filter(x => x.id === a.id).length;
      return original(a);
    };

    const list = await queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));

    expect(savedBeforeSend).toBe(1);
    expect(list[0]).toMatchObject({ id: 'a-1', status: 'sent' });
    expect(storage.saved()[0]?.status).toBe('sent');
  });

  it('sin conexión queda pendiente y se envía una sola vez cuando vuelve', async () => {
    const { server, clock, queue } = setup();
    server.online = false;

    const offline = await queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));
    expect(offline[0]).toMatchObject({ status: 'pending', attempts: 1, lastError: 'sin conexión' });

    // Antes de que venza la espera no se reintenta: no se martilla al servidor.
    await queue.flush();
    expect(server.sent).toEqual(['a-1']);

    server.online = true;
    clock.now = offline[0]!.nextAttemptAt;
    const back = await queue.flush();
    await queue.flush();

    expect(back[0]).toMatchObject({ status: 'sent', attempts: 2 });
    expect(server.sent).toEqual(['a-1', 'a-1']); // el mismo id: el servidor lo trata como reintento
  });

  it('se detiene en la primera falla transitoria y envía en orden de creación', async () => {
    const { server, clock, queue } = setup();
    server.online = false;
    await queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));
    clock.now += 1;
    await queue.enqueue(createAlert('a-2', 'trip-1', 'ABC123', clock.now));
    server.sent = [];

    clock.now += 120_000;
    await queue.flush();
    expect(server.sent).toEqual(['a-1']); // la primera falló: no intenta la segunda

    server.online = true;
    clock.now += 120_000;
    await queue.flush();
    expect(server.sent).toEqual(['a-1', 'a-1', 'a-2']);
  });

  it('una alerta rechazada no se reintenta', async () => {
    const { server, clock, queue } = setup();
    server.send = a => {
      server.sent.push(a.id);
      return Promise.resolve({ kind: 'rejected', reason: 'HTTP 409' });
    };

    const list = await queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));
    clock.now += 3_600_000;
    await queue.flush();

    expect(list[0]).toMatchObject({ status: 'rejected', lastError: 'HTTP 409' });
    expect(server.sent).toEqual(['a-1']);
  });

  it('dos flush simultáneos no envían dos veces la misma alerta', async () => {
    const { storage, clock } = setup();
    const sent: string[] = [];
    let release: () => void = () => undefined;
    const queue = new AlertQueue({
      storage,
      now: () => clock.now,
      random: () => 1,
      send: a => {
        sent.push(a.id);
        return new Promise(resolve => {
          release = () => resolve({ kind: 'sent' });
        });
      },
    });

    const first = queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));
    for (let i = 0; i < 100 && sent.length === 0; i++) {
      await Promise.resolve(); // hasta que el primer envío esté en curso
    }
    const second = queue.flush();
    release();
    await Promise.all([first, second]);

    expect(sent).toEqual(['a-1']);
  });

  it('una alerta encolada mientras se carga el historial no se pierde', async () => {
    const { storage, server, clock } = setup();
    storage.data.set(STORAGE_KEY, JSON.stringify([createAlert('vieja', 'trip-0', 'ABC123', START - 1000)]));
    // Lecturas lentas y desordenadas, como AsyncStorage: la segunda lectura termina después de que enqueue
    // ya agregó la alerta. Si la cola leyera dos veces, la segunda lectura la borraría.
    const delays = [5, 40];
    const slow: KeyValueStore = {
      getItem: key => {
        const snapshot = storage.data.get(key) ?? null; // el valor se lee al llamar, no al terminar
        return new Promise(resolve => setTimeout(() => resolve(snapshot), delays.shift() ?? 0));
      },
      setItem: (key, value) => storage.setItem(key, value),
    };
    const queue = new AlertQueue({ storage: slow, send: a => server.send(a), now: () => clock.now, random: () => 1 });

    const [afterEnqueue] = await Promise.all([queue.enqueue(createAlert('nueva', 'trip-1', 'ABC123', START)), queue.flush()]);

    expect(afterEnqueue.map(a => a.id)).toEqual(['nueva', 'vieja']);
    expect(storage.saved().map(a => a.id)).toEqual(['nueva', 'vieja']);
  });

  it('el historial sobrevive a reiniciar la app y conserva lo pendiente', async () => {
    const { storage, server, clock, queue } = setup();
    server.online = false;
    await queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));

    const reopened = new AlertQueue({ storage, send: a => server.send(a), now: () => clock.now, random: () => 1 });
    const loaded = await reopened.load();

    expect(loaded).toHaveLength(1);
    expect(loaded[0]).toMatchObject({ id: 'a-1', status: 'pending' });
  });

  it('si el almacenamiento falla al leer, no borra el historial guardado', async () => {
    const { storage, server, clock } = setup();
    storage.data.set(STORAGE_KEY, JSON.stringify([createAlert('pendiente', 'trip-0', 'ABC123', START - 1000)]));
    let failReads = 1;
    const flaky: KeyValueStore = {
      getItem: key => (failReads-- > 0 ? Promise.reject(new Error('disk I/O')) : storage.getItem(key)),
      setItem: (key, value) => storage.setItem(key, value),
    };
    const queue = new AlertQueue({ storage: flaky, send: a => server.send(a), now: () => clock.now, random: () => 1 });

    await expect(queue.enqueue(createAlert('nueva', 'trip-1', 'ABC123', START))).rejects.toThrow('disk I/O');
    expect(storage.saved().map(a => a.id)).toEqual(['pendiente']);

    // La lectura se reintenta en la siguiente operación y no se pierde nada.
    await queue.enqueue(createAlert('nueva', 'trip-1', 'ABC123', START));
    expect(storage.saved().map(a => a.id)).toEqual(['nueva', 'pendiente']);
  });

  it('una alerta encolada durante un envío en curso sale en la misma pasada', async () => {
    const { storage, clock } = setup();
    const sent: string[] = [];
    let release: () => void = () => undefined;
    const queue = new AlertQueue({
      storage,
      now: () => clock.now,
      random: () => 1,
      send: a => {
        sent.push(a.id);
        if (sent.length > 1) {
          return Promise.resolve({ kind: 'sent' });
        }
        return new Promise(resolve => {
          release = () => resolve({ kind: 'sent' });
        });
      },
    });

    const first = queue.enqueue(createAlert('a-1', 'trip-1', 'ABC123', START));
    for (let i = 0; i < 100 && sent.length === 0; i++) {
      await Promise.resolve();
    }
    const second = queue.enqueue(createAlert('a-2', 'trip-1', 'ABC123', START));
    for (let i = 0; i < 100; i++) {
      await Promise.resolve(); // deja que a-2 quede encolada mientras a-1 sigue en vuelo
    }
    release();
    const [, list] = await Promise.all([first, second]);

    expect(sent).toEqual(['a-1', 'a-2']);
    expect(list.map(a => a.status)).toEqual(['sent', 'sent']);
  });

  it('un historial corrupto no rompe la app', async () => {
    const { storage, queue } = setup();
    storage.data.set(STORAGE_KEY, '{no es json');

    await expect(queue.load()).resolves.toEqual([]);
  });

  it('el historial se limita, pero nunca descarta una alerta pendiente', async () => {
    const { storage, server, clock, queue } = setup();
    server.online = false;
    await queue.enqueue(createAlert('pendiente', 'trip-1', 'ABC123', clock.now));
    server.online = true;
    for (let i = 0; i < MAX_HISTORY + 5; i++) {
      clock.now += 1;
      await queue.enqueue(createAlert(`a-${i}`, 'trip-1', 'ABC123', clock.now));
    }

    const saved = storage.saved();
    expect(saved.length).toBeLessThanOrEqual(MAX_HISTORY);
    expect(saved.some(a => a.id === 'pendiente')).toBe(true);
  });
});
