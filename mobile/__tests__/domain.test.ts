import { applyResult, classifyStatus, createAlert, isDue, retryDelay, RETRY_BASE_MS, RETRY_MAX_MS } from '../src/domain/alerts';
import { uuidV4 } from '../src/domain/id';
import { formatElapsed, startTrip } from '../src/domain/trip';

const NOW = Date.parse('2026-10-27T20:00:00.000Z');
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe('alertas', () => {
  it('se crean pendientes, listas para el primer intento y con la hora del teléfono', () => {
    const a = createAlert('id-1', 'trip-1', 'ABC123', NOW);

    expect(a).toMatchObject({ id: 'id-1', tripId: 'trip-1', plate: 'ABC123', type: 'PANIC', status: 'pending', attempts: 0 });
    expect(a.occurredAt).toBe('2026-10-27T20:00:00.000Z');
    expect(isDue(a, NOW)).toBe(true);
  });

  it('la espera crece exponencialmente con tope y jitter completo', () => {
    expect(retryDelay(1, () => 0.999)).toBeLessThanOrEqual(RETRY_BASE_MS * 2);
    expect(retryDelay(3, () => 0.999)).toBeGreaterThan(retryDelay(1, () => 0.999));
    expect(retryDelay(30, () => 0.999)).toBeLessThanOrEqual(RETRY_MAX_MS);
    // Jitter completo: con azar 0 no espera; así miles de teléfonos no reintentan en el mismo segundo.
    expect(retryDelay(5, () => 0)).toBe(0);
  });

  it.each([
    [201, 'sent'],
    [200, 'sent'], // reintento de una alerta que el servidor ya tenía
    [400, 'rejected'],
    [409, 'rejected'],
    [401, 'retry'], // token mal configurado: se arregla en el servidor, la alerta no se pierde
    [429, 'retry'],
    [500, 'retry'],
    [503, 'retry'],
  ])('HTTP %i → %s', (status, kind) => {
    expect(classifyStatus(status).kind).toBe(kind);
  });

  it('una falla transitoria la deja pendiente con el próximo intento programado', () => {
    const a = applyResult(createAlert('id-1', 'trip-1', 'ABC123', NOW), { kind: 'retry', reason: 'sin conexión' }, NOW, () => 0.5);

    expect(a.status).toBe('pending');
    expect(a.attempts).toBe(1);
    expect(a.lastError).toBe('sin conexión');
    expect(isDue(a, NOW)).toBe(false);
    expect(isDue(a, a.nextAttemptAt)).toBe(true);
  });

  it('enviada o rechazada ya no se reintenta', () => {
    const sent = applyResult(createAlert('id-1', 'trip-1', 'ABC123', NOW), { kind: 'sent' }, NOW, Math.random);
    const rejected = applyResult(createAlert('id-2', 'trip-1', 'ABC123', NOW), { kind: 'rejected', reason: 'HTTP 409' }, NOW, Math.random);

    expect(sent).toMatchObject({ status: 'sent', attempts: 1, sentAt: '2026-10-27T20:00:00.000Z' });
    expect(rejected).toMatchObject({ status: 'rejected', lastError: 'HTTP 409' });
    expect(isDue(sent, NOW + RETRY_MAX_MS)).toBe(false);
    expect(isDue(rejected, NOW + RETRY_MAX_MS)).toBe(false);
  });
});

describe('id', () => {
  it('genera UUID v4 válidos y distintos', () => {
    const a = uuidV4(Math.random);
    const b = uuidV4(Math.random);

    expect(a).toMatch(UUID);
    expect(b).toMatch(UUID);
    expect(a).not.toBe(b);
    expect(uuidV4(() => 0)).toMatch(UUID);
    expect(uuidV4(() => 0.9999999)).toMatch(UUID);
  });
});

describe('viaje', () => {
  it('guarda placa e inicio, y muestra la duración como HH:MM:SS', () => {
    expect(startTrip('trip-1', 'ABC123', NOW)).toEqual({ id: 'trip-1', plate: 'ABC123', startedAt: NOW });
    expect(formatElapsed(0)).toBe('00:00:00');
    expect(formatElapsed(3_725_000)).toBe('01:02:05');
    expect(formatElapsed(-5)).toBe('00:00:00');
  });
});
