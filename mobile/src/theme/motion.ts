import { Easing } from 'react-native';

export const motion = {
  fast: 120,
  normal: 220,
  slow: 500,
  easeOut: Easing.out(Easing.quad),
  easeInOut: Easing.inOut(Easing.quad),
} as const;
