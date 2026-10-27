import type { TextStyle } from 'react-native';

// Fuente de la interfaz: la del sistema (Roboto en Android). Las 17 del CV solo se usan en las tarjetas de Letras.
export const type = {
  display: { fontSize: 24, lineHeight: 30, fontWeight: '700' },
  title: { fontSize: 17, lineHeight: 22, fontWeight: '700' },
  subtitle: { fontSize: 12, lineHeight: 16, fontWeight: '400' },
  cardTitle: { fontSize: 15, lineHeight: 20, fontWeight: '700' },
  body: { fontSize: 14, lineHeight: 20, fontWeight: '400' },
  label: { fontSize: 12, lineHeight: 16, fontWeight: '600' },
  caption: { fontSize: 11, lineHeight: 15, fontWeight: '500' },
  button: { fontSize: 14, lineHeight: 18, fontWeight: '700' },
  buttonLg: { fontSize: 16, lineHeight: 20, fontWeight: '800' },
  overline: { fontSize: 11, lineHeight: 14, fontWeight: '700', letterSpacing: 0.9, textTransform: 'uppercase' },
} satisfies Record<string, TextStyle>;

/** Tope de escalado para elementos compactos (pestañas, contadores). */
export const COMPACT_FONT_SCALE = 1.3;
