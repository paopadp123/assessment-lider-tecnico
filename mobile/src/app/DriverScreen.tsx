import React, { useEffect, useState } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Icon, type IconName } from '../components/icons';
import { Badge, Button, Card, EmptyState, Note, SectionHeader } from '../components/ui';
import { HEALTH_EVERY_MS } from '../config';
import type { DriverAlert } from '../domain/alerts';
import { formatElapsed } from '../domain/trip';
import { PanicButton } from '../features/PanicButton';
import { NO_TRIP, useDriverStore } from '../state/driverStore';
import { colors, radii, size, space, type } from '../theme';

const CONNECTION: Record<string, { label: string; icon: IconName; color: string; detail: string }> = {
  online: { label: 'Conectado', icon: 'online', color: colors.successText, detail: 'Las alertas llegan al centro de operación.' },
  degraded: { label: 'Servidor degradado', icon: 'degraded', color: colors.secondary, detail: 'El servidor responde pero no está listo. Las alertas se guardan y se reintentan.' },
  offline: { label: 'Sin conexión', icon: 'offline', color: colors.dangerText, detail: 'Las alertas se guardan en el teléfono y se envían al reconectar.' },
  checking: { label: 'Verificando…', icon: 'update', color: colors.textMuted, detail: 'Consultando el estado del servidor.' },
};

/** Reloj que avanza cada segundo, para el viaje y "hace N s". */
function useNow(everyMs = 1000): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), everyMs);
    return () => clearInterval(t);
  }, [everyMs]);
  return now;
}

export function DriverScreen() {
  const insets = useSafeAreaInsets();
  const { plate, connection, lastCheckAt, trip, alerts, sending, panicError, init, refresh, beginTrip, endTrip, panic } = useDriverStore();
  const now = useNow();

  useEffect(() => {
    init();
    const t = setInterval(() => refresh(), HEALTH_EVERY_MS);
    return () => clearInterval(t);
  }, [init, refresh]);

  const conn = CONNECTION[connection] ?? CONNECTION.checking!;
  const pending = alerts.filter(a => a.status === 'pending').length;

  return (
    <ScrollView style={s.screen} contentContainerStyle={[s.content, { paddingTop: insets.top + space.lg, paddingBottom: insets.bottom + space.xxl }]}>
      <View style={s.header}>
        <View>
          <Text style={[type.display, s.text]}>Conductor</Text>
          <Text style={[type.subtitle, s.muted]}>Póliza Digital · prototipo</Text>
        </View>
        <View style={s.plate} accessibilityLabel={`Placa ${plate}`}>
          <Icon name="car" size={size.iconSm} color={colors.textOnSecondary} />
          <Text style={[type.label, s.plateText]}>{plate}</Text>
        </View>
      </View>

      <SectionHeader>Estado de conexión</SectionHeader>
      <Card style={s.row}>
        <Icon name={conn.icon} size={28} color={conn.color} />
        <View style={s.flex}>
          <Text style={[type.cardTitle, { color: conn.color }]} accessibilityLiveRegion="polite">
            {conn.label}
          </Text>
          <Text style={[type.body, s.muted]}>{conn.detail}</Text>
          {lastCheckAt !== null && <Text style={[type.caption, s.muted]}>Verificado hace {Math.max(0, Math.round((now - lastCheckAt) / 1000))} s</Text>}
        </View>
        {pending > 0 && <Badge value={pending} />}
      </Card>
      {pending > 0 && <Note>{`${pending} ${pending === 1 ? 'alerta pendiente' : 'alertas pendientes'}: se reintentan solas.`}</Note>}

      <SectionHeader>Viaje actual</SectionHeader>
      {trip ? (
        <Card>
          <View style={s.row}>
            <Icon name="clock" size={28} color={colors.accent} />
            <View style={s.flex}>
              <Text style={[type.display, s.text]}>{formatElapsed(now - trip.startedAt)}</Text>
              <Text style={[type.caption, s.muted]}>
                {trip.id} · {trip.plate}
              </Text>
            </View>
          </View>
          <Button label="Terminar viaje" icon="stop" variant="danger" onPress={endTrip} style={s.action} />
        </Card>
      ) : (
        <View style={s.stack}>
          <EmptyState icon="car" message="No hay un viaje en curso." />
          <Button label="Iniciar viaje" icon="play" variant="primary" size="lg" onPress={beginTrip} />
        </View>
      )}

      <SectionHeader>Emergencia</SectionHeader>
      <PanicButton onTrigger={panic} disabled={sending} />
      {panicError !== null ? (
        <Text style={[type.label, s.error]} accessibilityRole="alert">
          {panicError}
        </Text>
      ) : (
        <Note>La alerta se guarda primero en el teléfono: llega aunque no haya señal en este momento.</Note>
      )}

      <SectionHeader>Historial de alertas</SectionHeader>
      {alerts.length === 0 ? (
        <EmptyState icon="history" message="Todavía no has enviado alertas." />
      ) : (
        <View style={s.stack}>
          {alerts.map(a => (
            <AlertRow key={a.id} alert={a} />
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const STATUS: Record<DriverAlert['status'], { label: string; icon: IconName; color: string }> = {
  sent: { label: 'Enviada', icon: 'check', color: colors.successText },
  pending: { label: 'Pendiente', icon: 'clock', color: colors.secondary },
  rejected: { label: 'Rechazada', icon: 'close', color: colors.dangerText },
};

function AlertRow({ alert }: { alert: DriverAlert }) {
  const st = STATUS[alert.status];
  const when = new Date(alert.occurredAt);
  const time = `${String(when.getHours()).padStart(2, '0')}:${String(when.getMinutes()).padStart(2, '0')}:${String(when.getSeconds()).padStart(2, '0')}`;
  const detail = alert.status === 'pending' && alert.attempts > 0 ? ` · ${alert.attempts} ${alert.attempts === 1 ? 'intento' : 'intentos'}` : '';
  return (
    <Card style={s.row}>
      <Icon name={st.icon} size={22} color={st.color} />
      <View style={s.flex}>
        <Text style={[type.cardTitle, s.text]}>Pánico · {time}</Text>
        <Text style={[type.caption, s.muted]}>{alert.tripId === NO_TRIP ? 'Sin viaje en curso' : alert.tripId}</Text>
        {alert.status !== 'sent' && alert.lastError ? <Text style={[type.caption, { color: st.color }]}>Último error: {alert.lastError}</Text> : null}
      </View>
      <Text style={[type.label, { color: st.color }]}>
        {st.label}
        {detail}
      </Text>
    </Card>
  );
}

const s = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { paddingHorizontal: space.lg },
  header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  plate: { flexDirection: 'row', alignItems: 'center', gap: space.xs, backgroundColor: colors.secondary, borderRadius: radii.pill, paddingHorizontal: space.md, paddingVertical: space.xs },
  plateText: { color: colors.textOnSecondary, fontWeight: '800', letterSpacing: 1 },
  row: { flexDirection: 'row', alignItems: 'center', gap: space.md },
  flex: { flex: 1, gap: 2 },
  stack: { gap: space.sm },
  action: { marginTop: space.md },
  text: { color: colors.text },
  muted: { color: colors.textMuted },
  error: { color: colors.dangerText, marginTop: space.sm },
});
