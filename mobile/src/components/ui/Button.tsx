import React from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle } from 'react-native';
import { Icon, type IconName } from '../icons';
import { colors, radii, size, space, type } from '../../theme';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

interface Props {
  label: string;
  onPress: () => void;
  variant?: ButtonVariant;
  size?: 'md' | 'lg';
  icon?: IconName;
  disabled?: boolean;
  loading?: boolean;
  style?: StyleProp<ViewStyle>;
  accessibilityHint?: string;
}

const FG: Record<ButtonVariant, string> = {
  primary: colors.textOnAccent,
  secondary: colors.text,
  ghost: colors.accent,
  danger: colors.dangerText,
};

export function Button({ label, onPress, variant = 'secondary', size: sz = 'md', icon, disabled, loading, style, accessibilityHint }: Props) {
  const off = disabled || loading;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityHint={accessibilityHint}
      accessibilityState={{ disabled: !!off, busy: !!loading }}
      disabled={off}
      onPress={onPress}
      style={({ pressed }) => [s.base, sz === 'lg' && s.lg, s[variant], pressed && s[`${variant}Pressed`], off && s.off, style]}>
      {/* El contenido se mantiene (invisible) durante la carga para conservar el ancho del botón. */}
      <View style={[s.row, loading && s.hidden]}>
        {icon && <Icon name={icon} size={sz === 'lg' ? 22 : 18} color={FG[variant]} />}
        <Text style={[sz === 'lg' ? type.buttonLg : type.button, { color: FG[variant] }]} numberOfLines={2}>
          {label}
        </Text>
      </View>
      {loading && <ActivityIndicator style={s.spinner} color={FG[variant]} />}
    </Pressable>
  );
}

const s = StyleSheet.create({
  base: { minHeight: size.touch, borderRadius: radii.md, paddingHorizontal: space.lg, alignItems: 'center', justifyContent: 'center' },
  lg: { minHeight: 56, borderRadius: radii.lg },
  row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: space.sm },
  hidden: { opacity: 0 },
  spinner: { position: 'absolute' },
  primary: { backgroundColor: colors.accent },
  primaryPressed: { backgroundColor: colors.accentPressed },
  secondary: { backgroundColor: colors.surface, borderWidth: size.hairline, borderColor: colors.border },
  secondaryPressed: { backgroundColor: colors.surfaceActive },
  ghost: { backgroundColor: colors.transparent },
  ghostPressed: { backgroundColor: colors.surfaceActive },
  danger: { backgroundColor: colors.surface, borderWidth: size.hairline, borderColor: colors.danger },
  dangerPressed: { backgroundColor: colors.surfaceActive },
  off: { opacity: 0.45 },
});
