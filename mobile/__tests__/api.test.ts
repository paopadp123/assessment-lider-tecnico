import { createAlert } from '../src/domain/alerts';
import { checkHealth, sendAlert, type ApiDeps } from '../src/services/api';

type Call = { url: string; init: RequestInit | undefined };

function deps(respond: (call: Call) => Promise<Response>): ApiDeps & { calls: Call[] } {
  const calls: Call[] = [];
  return {
    baseUrl: 'http://api.test',
    token: 'device-token',
    timeoutMs: 50,
    calls,
    fetch: ((url: string, init?: RequestInit) => {
      const call = { url, init };
      calls.push(call);
      return respond(call);
    }) as typeof fetch,
  };
}

const status = (code: number) => () => Promise.resolve({ status: code, ok: code >= 200 && code < 300 } as Response);

describe('envío de alertas', () => {
  it('envía la alerta con el token del dispositivo y el contrato del orquestador', async () => {
    const d = deps(status(201));
    const alert = createAlert('6f1c2a7e-0000-4000-8000-000000000001', 'trip-1', 'ABC123', Date.parse('2026-10-27T20:00:00Z'));

    const result = await sendAlert(d, alert);

    expect(result).toEqual({ kind: 'sent' });
    const call = d.calls[0]!;
    expect(call.url).toBe('http://api.test/driver/alerts');
    expect(call.init?.method).toBe('POST');
    expect((call.init?.headers as Record<string, string>).Authorization).toBe('Bearer device-token');
    expect(JSON.parse(String(call.init?.body))).toEqual({
      alertId: '6f1c2a7e-0000-4000-8000-000000000001',
      plate: 'ABC123',
      tripId: 'trip-1',
      type: 'PANIC',
      occurredAt: '2026-10-27T20:00:00.000Z',
    });
  });

  it('sin red o sin respuesta a tiempo, reintentar más tarde', async () => {
    const down = await sendAlert(deps(() => Promise.reject(new TypeError('Network request failed'))), createAlert('a', 't', 'ABC123', 0));
    const hang = await sendAlert(
      deps(({ init }) => new Promise((_, reject) => init?.signal?.addEventListener('abort', () => reject(new Error('aborted'))))),
      createAlert('a', 't', 'ABC123', 0),
    );

    expect(down).toEqual({ kind: 'retry', reason: 'sin conexión' });
    expect(hang).toEqual({ kind: 'retry', reason: 'sin conexión' });
  });
});

describe('estado de conexión', () => {
  it('listo, no listo o sin respuesta', async () => {
    expect(await checkHealth(deps(status(200)))).toBe('online');
    expect(await checkHealth(deps(status(503)))).toBe('degraded');
    expect(await checkHealth(deps(() => Promise.reject(new TypeError('Network request failed'))))).toBe('offline');
  });

  it('consulta /health/ready', async () => {
    const d = deps(status(200));
    await checkHealth(d);
    expect(d.calls[0]?.url).toBe('http://api.test/health/ready');
  });
});
