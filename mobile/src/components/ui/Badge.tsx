import React from 'react';
import { StyleSheet, Text, View } from 'react-native';
import { COMPACT_FONT_SCALE, colors, radii, space, type } from '../../theme';

/** Contador amarillo con texto oscuro. */
export function Badge({ value }: { value: number | string }) {
  return (
    <View style={s.badge} accessible={false}>
      <Text style={[type.caption, s.txt]} maxFontSizeMultiplier={COMPACT_FONT_SCALE}>
        {value}
      </Text>
    </View>
  );
}

const s = StyleSheet.create({
  badge: { minWidth: 26, paddingHorizontal: space.sm, paddingVertical: 2, borderRadius: radii.pill, backgroundColor: colors.secondary, alignItems: 'center' },
  txt: { color: colors.textOnSecondary, fontWeight: '800' },
});
