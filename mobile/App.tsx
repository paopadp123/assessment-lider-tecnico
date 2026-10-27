import React from 'react';
import { StatusBar } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { DriverScreen } from './src/app/DriverScreen';
import { colors } from './src/theme';

/** App del conductor (prototipo, HU-20): estado de conexión, viaje actual, botón de pánico e historial local. */
export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="light-content" backgroundColor={colors.bg} />
      <DriverScreen />
    </SafeAreaProvider>
  );
}
