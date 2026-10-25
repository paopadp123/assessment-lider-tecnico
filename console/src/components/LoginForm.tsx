"use client";

import { useState, type SubmitEvent } from "react";

export function LoginForm() {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: data.get("username"), password: data.get("password") }),
      });
      if (res.ok) {
        window.location.assign("/");
        return;
      }
      if (res.status === 429) {
        const wait = res.headers.get("Retry-After");
        setError(`Demasiados intentos. Espera ${wait ?? "unos"} segundos y vuelve a intentar.`);
      } else if (res.status === 401 || res.status === 400) {
        setError("Usuario o contraseña incorrectos.");
      } else {
        setError("No se pudo iniciar sesión: el servicio no responde. Intenta de nuevo.");
      }
    } catch {
      setError("No se pudo iniciar sesión: revisa la conexión.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="card login" onSubmit={(e) => void submit(e)}>
      <h1>Consola de operaciones</h1>
      <label htmlFor="username">Usuario</label>
      <input id="username" name="username" autoComplete="username" required autoFocus />
      <label htmlFor="password">Contraseña</label>
      <input id="password" name="password" type="password" autoComplete="current-password" required />
      {error !== null && (
        <p role="alert" className="banner banner-error">
          {error}
        </p>
      )}
      <button type="submit" className="btn btn-primary" disabled={busy}>
        {busy ? "Entrando…" : "Iniciar sesión"}
      </button>
    </form>
  );
}
