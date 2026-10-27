import React from 'react';
import { StyleSheet, Text, View, type StyleProp, type ViewStyle } from 'react-native';
import { Icon, type IconName } from '../icons';
import { colors, radii, size, space, type } from '../../theme';

/** Título de grupo (mayúsculas, gris), como en Acelerex CV. */
export function SectionHeader({ children }: { children: string }) {
  return (
    <Text style={[type.overline, s.header]} accessibilityRole="header">
      {children}
    </Text>
  );
}

/** Tarjeta de superficie con borde fino. */
export function Card({ children, style }: { children: React.ReactNode; style?: StyleProp<ViewStyle> }) {
  return <View style={[s.card, style]}>{children}</View>;
}

export function EmptyState({ icon, message }: { icon: IconName; message: string }) {
  return (
    <View style={s.empty}>
      <Icon name={icon} size={28} color={colors.textMuted} />
      <Text style={[type.body, s.muted, s.centerText]}>{message}</Text>
    </View>
  );
}

/** Nota corta gris. */
export function Note({ children }: { children: string }) {
  return <Text style={[type.caption, s.note]}>{children}</Text>;
}

const s = StyleSheet.create({
  header: { color: colors.textMuted, marginTop: space.xl, marginBottom: space.sm, marginLeft: 2 },
  card: { backgroundColor: colors.surface, borderRadius: radii.lg, borderWidth: size.hairline, borderColor: colors.border, padding: space.lg },
  muted: { color: colors.textMuted },
  centerText: { textAlign: 'center' },
  empty: { alignItems: 'center', gap: space.sm, paddingVertical: space.lg, borderRadius: radii.md, borderWidth: size.hairline, borderStyle: 'dashed', borderColor: colors.border },
  note: { color: colors.textMuted, marginTop: space.sm, marginLeft: 2 },
});
