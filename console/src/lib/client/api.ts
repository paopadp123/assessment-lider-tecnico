// Llamadas del navegador al BFF (/api/*). Nunca llaman al orquestador directo ni manejan el token.
import type { ApiError } from "@/lib/api/types";

export class RequestError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function send<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  const res = await fetch(path, { ...init, cache: "no-store", headers });
  if (res.status === 401) {
    // La sesión venció o se cerró: el BFF ya borró la cookie.
    window.location.assign("/login");
    throw new RequestError(401, "Tu sesión terminó. Vuelve a iniciar sesión.");
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ApiError | null;
    throw new RequestError(res.status, body?.error ?? `Error ${res.status}`);
  }
  // El contrato lo garantiza el orquestador: sus respuestas se validan contra el OpenAPI en sus pruebas.
  return (await res.json()) as T;
}

export function apiGet<T>(path: string): Promise<T> {
  return send<T>(path);
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return send<T>(path, {
    method: "POST",
    ...(body === undefined ? {} : { body: JSON.stringify(body), headers: { "Content-Type": "application/json" } }),
  });
}
