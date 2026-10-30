import { colors } from './colors';
import { type, COMPACT_FONT_SCALE } from './typography';
import { space, size } from './spacing';
import { radii } from './radii';
import { motion } from './motion';

export { colors, type, space, size, radii, motion, COMPACT_FONT_SCALE };
export const theme = { colors, type, space, size, radii, motion } as const;
