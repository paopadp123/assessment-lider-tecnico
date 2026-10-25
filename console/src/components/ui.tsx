"use client";

import { useEffect, useRef, type ReactNode } from "react";

import type { OrderStatus } from "@/lib/api/types";
import { STATUS_LABEL, STATUS_TONE } from "@/lib/format";

export function StatusBadge({ status }: { status: OrderStatus }) {
  return <span className={`badge tone-${STATUS_TONE[status]}`}>{STATUS_LABEL[status]}</span>;
}

export function ErrorBanner({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="banner banner-error">
      {children}
    </p>
  );
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  children: ReactNode;
  confirmLabel: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * Diálogo de confirmación con el <dialog> nativo: atrapa el foco, se cierra con Escape y el lector de
 * pantalla lo anuncia como modal. El foco inicial va a "Cancelar" para que Enter no confirme por accidente.
 */
export function ConfirmDialog({ open, title, children, confirmLabel, busy, onConfirm, onCancel }: ConfirmDialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return;
    }
    if (open && !dialog.open) {
      dialog.showModal();
      cancelRef.current?.focus();
    } else if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  return (
    <dialog
      ref={ref}
      className="dialog"
      aria-labelledby="confirm-title"
      onCancel={(e) => {
        // Escape: no se cierra mientras la acción está en curso, para que el resultado no quede sin mostrar.
        e.preventDefault();
        if (!busy) {
          onCancel();
        }
      }}
    >
      <h2 id="confirm-title">{title}</h2>
      <div className="dialog-body">{children}</div>
      <div className="dialog-actions">
        <button ref={cancelRef} type="button" className="btn" onClick={onCancel} disabled={busy}>
          Cancelar
        </button>
        <button type="button" className="btn btn-primary" onClick={onConfirm} disabled={busy}>
          {busy ? "Enviando…" : confirmLabel}
        </button>
      </div>
    </dialog>
  );
}
