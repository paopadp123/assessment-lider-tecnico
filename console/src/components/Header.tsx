"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import type { BreakerStatus } from "@/lib/api/types";
import { apiGet, apiPost } from "@/lib/client/api";

import { useLiveStatus } from "./LiveEvents";

const LIVE_LABEL = { connecting: "Reconectando…", open: "En vivo", closed: "Sin conexión" } as const;
const BREAKER_LABEL = {
  closed: "operando",
  open: "abierto: emisiones en espera",
  "half-open": "probando de nuevo",
  unknown: "estado desconocido",
} as const;

type BreakerView = Pick<BreakerStatus, "consecutiveFailures"> & { state: keyof typeof BREAKER_LABEL };

// El breaker no emite eventos por el flujo SSE; se consulta cada 5 s, lo mismo que tarda en cambiar algo útil.
const BREAKER_POLL_MS = 5000;

export function Header() {
  const live = useLiveStatus();
  const [breaker, setBreaker] = useState<BreakerView | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      apiGet<BreakerStatus[]>("/api/breakers")
        .then((list) => {
          if (active) {
            setBreaker(list[0] ?? null);
          }
        })
        .catch(() => {
          // Si la consulta falla, se dice que no se sabe; ocultar el indicador haría pensar que todo está bien.
          if (active) {
            setBreaker({ state: "unknown", consecutiveFailures: 0 });
          }
        });
    };
    load();
    const timer = setInterval(load, BREAKER_POLL_MS);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);

  const logout = () => {
    void apiPost("/api/auth/logout").finally(() => {
      window.location.assign("/login");
    });
  };

  return (
    <header className="header">
      <Link href="/" className="brand">
        Consola de operaciones
      </Link>
      <div className="header-status">
        {breaker !== null && (
          <span className={`pill breaker-${breaker.state}`} title={`${breaker.consecutiveFailures} fallas seguidas`}>
            Emisor: {BREAKER_LABEL[breaker.state]}
          </span>
        )}
        <span className={`pill live-${live}`} role="status" aria-live="polite">
          <span className="dot" aria-hidden="true" /> {LIVE_LABEL[live]}
        </span>
        <button type="button" className="btn btn-small" onClick={logout}>
          Cerrar sesión
        </button>
      </div>
    </header>
  );
}
