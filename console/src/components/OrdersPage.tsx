"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState, type SubmitEvent } from "react";

import { ORDER_STATUSES, type Order, type OrderList, type OrderStatus } from "@/lib/api/types";
import { apiGet } from "@/lib/client/api";
import { STATUS_LABEL, formatDateTime, formatMoney } from "@/lib/format";
import { patchOrders } from "@/lib/orders";

import { useOrderEvents } from "./LiveEvents";
import { ErrorBanner, StatusBadge } from "./ui";

const PAGE_SIZE = 20;
// Tras un evento se pide de nuevo la página, una sola vez aunque lleguen muchos seguidos, para que el
// filtro y el orden sigan siendo los del servidor (p. ej. una orden que sale del filtro "Emitiendo").
const REFRESH_DEBOUNCE_MS = 1000;

export function OrdersPage() {
  const [statuses, setStatuses] = useState<OrderStatus[]>([]);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  // cursors[i] es el cursor que abre la página i (la primera no tiene).
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [page, setPage] = useState<OrderList | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Cada solicitud lleva un número; solo la última puede escribir en pantalla. Así una respuesta lenta del
  // filtro anterior no pisa la del filtro nuevo.
  const requestSeq = useRef(0);

  const cursor = cursors[cursors.length - 1];

  const load = useCallback(async () => {
    const mine = ++requestSeq.current;
    const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
    for (const s of statuses) {
      params.append("status", s);
    }
    if (query !== "") {
      params.set("q", query);
    }
    if (cursor !== undefined) {
      params.set("cursor", cursor);
    }
    try {
      const data = await apiGet<OrderList>(`/api/orders?${params.toString()}`);
      if (mine === requestSeq.current) {
        setPage(data);
        setError(null);
      }
    } catch (err) {
      if (mine === requestSeq.current) {
        setError(err instanceof Error ? err.message : "No se pudo cargar el listado.");
      }
    } finally {
      if (mine === requestSeq.current) {
        setLoading(false);
      }
    }
  }, [statuses, query, cursor]);

  // El timer de refresco siempre usa el load vigente (el del filtro actual), no el del momento del evento.
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  }, [load]);

  useEffect(() => {
    if (refreshTimer.current !== null) {
      clearTimeout(refreshTimer.current);
      refreshTimer.current = null;
    }
    setLoading(true);
    void load();
  }, [load]);

  useEffect(
    () => () => {
      if (refreshTimer.current !== null) {
        clearTimeout(refreshTimer.current);
      }
    },
    [],
  );

  useOrderEvents((event) => {
    setPage((p) => (p === null ? p : { ...p, items: patchOrders(p.items, event) }));
    if (refreshTimer.current !== null) {
      clearTimeout(refreshTimer.current);
    }
    refreshTimer.current = setTimeout(() => void loadRef.current(), REFRESH_DEBOUNCE_MS);
  });

  const resetPages = () => {
    setCursors([undefined]);
  };

  const toggleStatus = (s: OrderStatus) => {
    setStatuses((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));
    resetPages();
  };

  const submitSearch = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    setQuery(search.trim());
    resetPages();
  };

  const items: Order[] = page?.items ?? [];
  const next = page?.nextCursor;

  return (
    <main className="main">
      <section className="toolbar" aria-label="Filtros">
        <form className="search" role="search" onSubmit={submitSearch}>
          <label htmlFor="q" className="sr-only">
            Buscar
          </label>
          <input
            id="q"
            type="search"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
            }}
            placeholder="Id de orden, pago, póliza, correlation-id o placa"
            maxLength={100}
          />
          <button type="submit" className="btn">
            Buscar
          </button>
        </form>
        <fieldset className="filters">
          <legend>Estado</legend>
          {ORDER_STATUSES.map((s) => (
            <label key={s} className={`chip ${statuses.includes(s) ? "chip-on" : ""}`}>
              <input
                type="checkbox"
                checked={statuses.includes(s)}
                onChange={() => {
                  toggleStatus(s);
                }}
              />
              {STATUS_LABEL[s]}
            </label>
          ))}
        </fieldset>
      </section>

      {error !== null && <ErrorBanner>{error}</ErrorBanner>}

      <table className="table">
        <caption className="sr-only">Órdenes, de la más reciente a la más antigua</caption>
        <thead>
          <tr>
            <th scope="col">Orden</th>
            <th scope="col">Estado</th>
            <th scope="col">Placa</th>
            <th scope="col">Prima</th>
            <th scope="col">Póliza</th>
            <th scope="col">Motivo</th>
            <th scope="col">Actualizada</th>
          </tr>
        </thead>
        <tbody>
          {items.map((o) => (
            <tr key={o.id}>
              <td>
                <Link href={`/orders/${o.id}`} className="mono">
                  {o.id.slice(0, 8)}
                </Link>
              </td>
              <td>
                <StatusBadge status={o.status} />
              </td>
              <td className="mono">{o.plate}</td>
              <td>{o.quote === undefined ? "—" : formatMoney(o.quote.premium, o.quote.currency)}</td>
              <td className="mono">{o.policyNumber ?? "—"}</td>
              <td className="mono">{o.failureReason ?? "—"}</td>
              <td>{formatDateTime(o.updatedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {!loading && error === null && items.length === 0 && (
        <p className="empty">No hay órdenes {statuses.length > 0 || query !== "" ? "con estos filtros" : "todavía"}.</p>
      )}
      {loading && <p className="empty">Cargando…</p>}

      <nav className="pager" aria-label="Paginación">
        <button
          type="button"
          className="btn"
          disabled={cursors.length === 1}
          onClick={() => {
            setCursors((c) => c.slice(0, -1));
          }}
        >
          ← Anterior
        </button>
        <span>Página {cursors.length}</span>
        <button
          type="button"
          className="btn"
          disabled={next === undefined}
          onClick={() => {
            if (next !== undefined) {
              setCursors((c) => [...c, next]);
            }
          }}
        >
          Siguiente →
        </button>
      </nav>
    </main>
  );
}
