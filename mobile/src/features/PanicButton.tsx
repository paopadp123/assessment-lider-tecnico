import React, { useRef } from 'react';
import { Animated, Easing, Pressable, StyleSheet, Text, View, Vibration } from 'react-native';
import { Icon } from '../components/icons';
import { colors, radii, space, type } from '../theme';

export const HOLD_MS = 2_000;

/**
 * Botón de pánico: se activa manteniéndolo presionado 2 s, con una barra que se llena mientras tanto. Evita
 * los disparos accidentales (bolsillo, soporte del celular) sin pedir una confirmación extra en una emergencia.
 */
export function PanicButton({ onTrigger, disabled }: { onTrigger: () => void; disabled?: boolean }) {
  const progress = useRef(new Animated.Value(0)).current;

  const start = () => {
    progress.setValue(0);
    Animated.timing(progress, { toValue: 1, duration: HOLD_MS, easing: Easing.linear, useNativeDriver: false }).start();
  };
  const cancel = () => {
    progress.stopAnimation();
    Animated.timing(progress, { toValue: 0, duration: 150, useNativeDriver: false }).start();
  };
  const trigger = () => {
    Vibration.vibrate(400);
    progress.setValue(0);
    onTrigger();
  };

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel="Alerta de pánico"
      accessibilityHint="Mantén presionado dos segundos para enviar la alerta"
      accessibilityState={{ disabled: !!disabled }}
      disabled={disabled}
      delayLongPress={HOLD_MS}
      onPressIn={start}
      onPressOut={cancel}
      onLongPress={trigger}
      style={({ pressed }) => [s.button, pressed && s.pressed, disabled && s.off]}>
      <Animated.View style={[s.fill, { width: progress.interpolate({ inputRange: [0, 1], outputRange: ['0%', '100%'] }) }]} />
      <View style={s.content}>
        <Icon name="alert" size={34} color={colors.textOnDanger} strokeWidth={2.2} />
        <Text style={[type.buttonLg, s.label]}>ALERTA DE PÁNICO</Text>
        <Text style={[type.caption, s.hint]}>Mantén presionado 2 segundos</Text>
      </View>
    </Pressable>
  );
}

const s = StyleSheet.create({
  button: { minHeight: 132, borderRadius: radii.xl, backgroundColor: colors.danger, overflow: 'hidden', justifyContent: 'center' },
  pressed: { backgroundColor: colors.dangerPressed },
  off: { opacity: 0.6 },
  fill: { position: 'absolute', left: 0, top: 0, bottom: 0, backgroundColor: colors.shadow, opacity: 0.25 },
  content: { alignItems: 'center', gap: space.xs, paddingVertical: space.lg },
  label: { color: colors.textOnDanger, letterSpacing: 1 },
  hint: { color: colors.textOnDanger, opacity: 0.85 },
});
