"use client";

// Una sola conexión SSE para toda la consola. Mientras el flujo está abierto, EventSource reconecta solo y
// manda el Last-Event-ID; el BFF lo reenvía y el orquestador entrega lo que se perdió. Si el BFF responde un
// error en vez del flujo (p. ej. orquestador caído), el navegador deja de reintentar: aquí se abre otra
// conexión con backoff, pasando el último id en la URL.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import type { OrderEvent } from "@/lib/api/types";
import { parseOrderEvent, reconnectDelay, streamUrl } from "@/lib/live";

export type LiveStatus = "connecting" | "open" | "closed";

type Listener = (event: OrderEvent) => void;

interface LiveEvents {
  status: LiveStatus;
  subscribe: (fn: Listener) => () => void;
}

const LiveEventsContext = createContext<LiveEvents | null>(null);

export function LiveEventsProvider({ children }: { children: ReactNode }) {
  const listeners = useRef(new Set<Listener>());
  const [status, setStatus] = useState<LiveStatus>("connecting");

  useEffect(() => {
    let source: EventSource | null = null;
    let retry: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    let lastId: string | null = null;
    let stopped = false;

    const connect = () => {
      source = new EventSource(streamUrl(lastId));
      source.onopen = () => {
        attempt = 0;
        setStatus("open");
      };
      source.onerror = () => {
        if (source?.readyState !== EventSource.CLOSED) {
          setStatus("connecting"); // el navegador reintenta solo
          return;
        }
        // Cerrada: el navegador no va a reintentar. Si la sesión venció, la consulta del breaker de la barra
        // superior recibe el 401 y manda al login.
        setStatus("closed");
        source.close();
        if (!stopped) {
          retry = setTimeout(connect, reconnectDelay(attempt++));
        }
      };
      source.addEventListener("order-status", (msg: MessageEvent<string>) => {
        lastId = msg.lastEventId === "" ? lastId : msg.lastEventId;
        const event = parseOrderEvent(msg.data);
        if (event === null) {
          return;
        }
        for (const fn of listeners.current) {
          fn(event);
        }
      });
    };

    connect();
    return () => {
      stopped = true;
      if (retry !== null) {
        clearTimeout(retry);
      }
      source?.close();
    };
  }, []);

  const subscribe = useCallback((fn: Listener) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);
  const value = useMemo(() => ({ status, subscribe }), [status, subscribe]);

  return <LiveEventsContext.Provider value={value}>{children}</LiveEventsContext.Provider>;
}

function useLiveEvents(): LiveEvents {
  const ctx = useContext(LiveEventsContext);
  if (ctx === null) {
    throw new Error("useLiveEvents fuera de LiveEventsProvider");
  }
  return ctx;
}

export function useLiveStatus(): LiveStatus {
  return useLiveEvents().status;
}

/** Llama a fn con cada cambio de estado de cualquier orden. */
export function useOrderEvents(fn: Listener): void {
  const { subscribe } = useLiveEvents();
  const latest = useRef(fn);
  useEffect(() => {
    latest.current = fn;
  });
  useEffect(
    () =>
      subscribe((e) => {
        latest.current(e);
      }),
    [subscribe],
  );
}
