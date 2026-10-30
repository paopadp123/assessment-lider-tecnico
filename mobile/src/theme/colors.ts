// Paleta oscura tomada de Acelerex CV (ADL). ÚNICO lugar con literales de color en la app.
export const colors = {
  bg: '#1e1f24',
  surface: '#2a2c33',
  surfaceActive: '#33363e',
  border: '#3a3d46',
  disabled: '#555861',
  accent: '#f28c28',
  secondary: '#ffc933',
  text: '#e8e8e8',
  textMuted: 'rgba(232,232,232,0.6)',
  success: '#4caf50',
  danger: '#e53935',

  // Derivados
  accentPressed: '#d97a1e',
  textOnAccent: '#1e1f24', // texto oscuro sobre naranja (blanco no llega a 4.5:1)
  textOnSecondary: '#1e1f24',
  textOnDanger: '#ffffff',
  dangerText: '#ff8a80', // rojo legible como texto sobre fondo oscuro
  dangerPressed: '#b71c1c',
  successText: '#81c784',
  shadow: '#000000',
  transparent: 'transparent',
} as const;

export type ColorToken = keyof typeof colors;
