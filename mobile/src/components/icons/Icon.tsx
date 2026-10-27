import React, { memo } from 'react';
import Svg, { Path } from 'react-native-svg';
import { colors, size as sizes } from '../../theme';

// Íconos de trazo 24×24, con el mismo estilo de Acelerex CV: cada uno es una lista de trazos SVG.
const PATHS = {
  online: ['M2 9a15 15 0 0 1 20 0', 'M5 12.5a10 10 0 0 1 14 0', 'M8.5 16a5 5 0 0 1 7 0', 'M12 19.5h.01'],
  offline: ['M2 9a15 15 0 0 1 6-3.3', 'M22 9a15 15 0 0 0-9.5-3.9', 'M8.5 16a5 5 0 0 1 7 0', 'M12 19.5h.01', 'M3 3l18 18'],
  degraded: ['M12 3 2 20h20z', 'M12 9v5', 'M12 17h.01'],
  car: ['M5 11l1.5-4.5A2 2 0 0 1 8.4 5h7.2a2 2 0 0 1 1.9 1.5L19 11', 'M3 11h18v6H3z', 'M7 17v2', 'M17 17v2', 'M7 14h.01', 'M17 14h.01'],
  play: ['M7 4l13 8-13 8z'],
  stop: ['M6 6h12v12H6z'],
  alert: ['M12 3l7 3v5c0 4.5-3 8.4-7 10-4-1.6-7-5.5-7-10V6z', 'M12 8v5', 'M12 16h.01'],
  history: ['M3 12a9 9 0 1 0 3-6.7L3 8', 'M3 3v5h5', 'M12 7v5l3 2'],
  check: ['M5 12.5l4.5 4.5L19 7.5'],
  clock: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M12 7v5l3 2'],
  close: ['M6 6l12 12', 'M18 6 6 18'],
  update: ['M4 12a8 8 0 0 1 14-5.3L20 9', 'M20 4v5h-5', 'M20 12a8 8 0 0 1-14 5.3L4 15', 'M4 20v-5h5'],
} as const;

export type IconName = keyof typeof PATHS;

interface Props {
  name: IconName;
  size?: number;
  color?: string;
  strokeWidth?: number;
}

/** Ícono decorativo (la etiqueta accesible la pone el elemento que lo contiene). */
export const Icon = memo(function IconView({ name, size = sizes.icon, color = colors.text, strokeWidth = 2 }: Props) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none" accessible={false}>
      {PATHS[name].map((d, i) => (
        <Path key={i} d={d} stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
      ))}
    </Svg>
  );
});
