import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import App from '../App';

// Módulos nativos: en Jest no hay Android. La lógica se prueba aparte (domain, alertQueue y api).
jest.mock('@react-native-async-storage/async-storage', () => {
  const data = new Map<string, string>();
  return {
    getItem: (k: string) => Promise.resolve(data.get(k) ?? null),
    setItem: (k: string, v: string) => {
      data.set(k, v);
      return Promise.resolve();
    },
    removeItem: (k: string) => {
      data.delete(k);
      return Promise.resolve();
    },
  };
});
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('react-native-svg', () => {
  const { View } = require('react-native');
  return { __esModule: true, default: View, Path: View };
});

test('muestra las cuatro secciones del prototipo', async () => {
  globalThis.fetch = jest.fn(() => Promise.resolve({ ok: true, status: 200 } as Response)) as jest.Mock;
  jest.useFakeTimers();
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;

  await ReactTestRenderer.act(async () => {
    tree = ReactTestRenderer.create(<App />);
  });

  const text = JSON.stringify(tree!.toJSON());
  for (const section of ['Estado de conexión', 'Viaje actual', 'ALERTA DE PÁNICO', 'Historial de alertas']) {
    expect(text).toContain(section);
  }
  expect(text).toContain('Conectado');
  await ReactTestRenderer.act(async () => tree!.unmount());
  jest.useRealTimers();
});
