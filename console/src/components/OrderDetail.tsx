"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";

import type { Order, OrderEvent, ThirdPartyCall } from "@/lib/api/types";
import { RequestError, apiGet, apiPost } from "@/lib/client/api";
import {
  ACTION_CONFIRM,
  ACTION_LABEL,
  OPERATION_LABEL,
  OUTCOME_LABEL,
  PARTY_LABEL,
  STATUS_LABEL,
  formatDateTime,
  formatDuration,
  formatMoney,
  formatTime,
} from "@/lib/format";
import { allowedActions, mergeTimeline, type OperatorAction } from "@/lib/orders";

import { useOrderEvents } from "./LiveEvents";
import { ConfirmDialog, ErrorBanner, StatusBadge } from "./ui";

// Un evento de esta orden también puede traer llamadas nuevas a terceros: se recarga todo, una vez.
const RELOAD_DEBOUNCE_MS = 300;

interface Detail {
  order: Order;
  events: OrderEvent[];
  calls: ThirdPartyCall[];
}

export function OrderDetail({ id }: { id: string }) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [pending, setPending] = useState<OperatorAction | null>(null);
  const [busy, setBusy] = useState(false);
  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Solo la última recarga escribe en pantalla: una respuesta lenta no pisa una más nueva.
  const requestSeq = useRef(0);

  const load = useCallback(async () => {
    const mine = ++requestSeq.current;
    try {
      const [order, events, calls] = await Promise.all([
        apiGet<Order>(`/api/orders/${id}`),
        apiGet<OrderEvent[]>(`/api/orders/${id}/events`),
        apiGet<ThirdPartyCall[]>(`/api/orders/${id}/calls`),
      ]);
      if (mine !== requestSeq.current) {
        return;
      }
      setDetail({ order, events, calls });
      setError(null);
    } catch (err) {
      if (mine !== requestSeq.current) {
        return;
      }
      setError(err instanceof RequestError && err.status === 404 ? "La orden no existe." : "No se pudo cargar la orden.");
    }
  }, [id]);

  useEffect(() => {
    void load();
    return () => {
      if (reloadTimer.current !== null) {
        clearTimeout(reloadTimer.current);
      }
    };
  }, [load]);

  useOrderEvents((event) => {
    if (event.orderId !== id) {
      return;
    }
    // El evento se ve al instante; el resto (estado, póliza, llamadas) llega con la recarga.
    setDetail((d) => (d === null ? d : { ...d, events: mergeTimeline(d.events, event) }));
    if (reloadTimer.current !== null) {
      clearTimeout(reloadTimer.current);
    }
    reloadTimer.current = setTimeout(() => void load(), RELOAD_DEBOUNCE_MS);
  });

  const run = async (action: OperatorAction) => {
    setBusy(true);
    setNotice(null);
    try {
      await apiPost<Order>(`/api/orders/${id}/actions/${action}`);
      setNotice(`${ACTION_LABEL[action]}: acción registrada.`);
    } catch (err) {
      // 409: la orden cambió de estado mientras el operador decidía; se muestra el estado real.
      setNotice(
        err instanceof RequestError && err.status === 409
          ? "La acción ya no aplica: la orden cambió de estado. Se actualizó la información."
          : `No se pudo ejecutar la acción: ${err instanceof Error ? err.message : "error desconocido"}.`,
      );
    } finally {
      setBusy(false);
      setPending(null);
      void load();
    }
  };

  if (error !== null) {
    return (
      <main className="main">
        <Link href="/">← Volver al listado</Link>
        <ErrorBanner>{error}</ErrorBanner>
      </main>
    );
  }
  if (detail === null) {
    return (
      <main className="main">
        <p className="empty">Cargando…</p>
      </main>
    );
  }

  const { order, events, calls } = detail;
  const actions = allowedActions(order.status);

  return (
    <main className="main">
      <Link href="/">← Volver al listado</Link>
      <div className="detail-head">
        <h1>
          Orden <span className="mono">{order.id}</span>
        </h1>
        <StatusBadge status={order.status} />
      </div>

      {notice !== null && (
        <p role="status" className="banner">
          {notice}
        </p>
      )}

      <section className="card">
        <h2>Datos</h2>
        <dl className="facts">
          <dt>Placa</dt>
          <dd className="mono">{order.plate}</dd>
          <dt>Vehículo</dt>
          <dd>
            {order.vehicleYear} · ciudad {order.cityCode}
          </dd>
          <dt>Cotización</dt>
          <dd>
            {order.quote === undefined ? (
              "—"
            ) : (
              <>
                {formatMoney(order.quote.premium, order.quote.currency)} <span className="muted mono">({order.quote.id})</span>
              </>
            )}
          </dd>
          <dt>Pago</dt>
          <dd className="mono">{order.paymentId ?? "—"}</dd>
          <dt>Póliza</dt>
          <dd className="mono">{order.policyNumber ?? "—"}</dd>
          <dt>Motivo de falla</dt>
          <dd className="mono">{order.failureReason ?? "—"}</dd>
          <dt>Creada</dt>
          <dd>{formatDateTime(order.createdAt)}</dd>
        </dl>
        {actions.length > 0 && (
          <div className="actions">
            {actions.map((a) => (
              <button
                key={a}
                type="button"
                className={a === "mark-for-reversal" ? "btn btn-danger" : "btn btn-primary"}
                onClick={() => {
                  setPending(a);
                }}
              >
                {ACTION_LABEL[a]}
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="card">
        <h2>Línea de tiempo</h2>
        <ol className="timeline">
          {events.map((e) => (
            <li key={e.id}>
              <time dateTime={e.occurredAt}>{formatTime(e.occurredAt)}</time>
              <span>
                {e.from === undefined ? "—" : STATUS_LABEL[e.from]} → <strong>{STATUS_LABEL[e.to]}</strong>
              </span>
              <span className="muted">{e.reason}</span>
              <span className={e.actor === "system" ? "muted" : "actor"}>{e.actor === "system" ? "sistema" : e.actor}</span>
            </li>
          ))}
        </ol>
      </section>

      <section className="card">
        <h2>Respuestas de terceros</h2>
        {calls.length === 0 ? (
          <p className="empty">Todavía no hay llamadas registradas.</p>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Hora</th>
                <th scope="col">Tercero</th>
                <th scope="col">Operación</th>
                <th scope="col">Resultado</th>
                <th scope="col">Duración</th>
                <th scope="col">Detalle</th>
              </tr>
            </thead>
            <tbody>
              {calls.map((c) => (
                <tr key={c.id}>
                  <td>{formatTime(c.startedAt)}</td>
                  <td>{PARTY_LABEL[c.party]}</td>
                  <td>{OPERATION_LABEL[c.operation]}</td>
                  <td>
                    <span className={`badge outcome-${c.outcome}`}>{OUTCOME_LABEL[c.outcome]}</span>
                  </td>
                  <td>{formatDuration(c.durationMs)}</td>
                  <td className="mono small">{c.detail ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <ConfirmDialog
        open={pending !== null}
        title={pending === null ? "" : `${ACTION_LABEL[pending]}?`}
        confirmLabel={pending === null ? "" : ACTION_LABEL[pending]}
        busy={busy}
        onConfirm={() => {
          if (pending !== null) {
            void run(pending);
          }
        }}
        onCancel={() => {
          setPending(null);
        }}
      >
        <p>{pending === null ? "" : ACTION_CONFIRM[pending]}</p>
        <p className="muted">Quedará registrada con tu usuario en la línea de tiempo.</p>
      </ConfirmDialog>
    </main>
  );
}
