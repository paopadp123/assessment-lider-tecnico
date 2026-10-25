import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SESSION_COOKIE, login, logout, proxy, stream } from "@/lib/server/bff";

const ORCH = "http://orchestrator:8080";
const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => {
  vi.stubEnv("ORCHESTRATOR_URL", ORCH);
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

interface RequestInit {
  method?: string;
  token?: string;
  headers?: Record<string, string>;
  body?: string;
}

function request(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (init.token !== undefined) {
    headers.set("cookie", `${SESSION_COOKIE}=${init.token}`);
  }
  return new NextRequest(`http://localhost:3000${path}`, {
    method: init.method ?? "GET",
    headers,
    ...(init.body === undefined ? {} : { body: init.body }),
  });
}

function upstream(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

function sentHeaders(call = 0): Headers {
  const init = fetchMock.mock.calls[call]?.[1];
  return new Headers(init?.headers);
}

describe("proxy", () => {
  it("sin sesión responde 401 y no llama al orquestador", async () => {
    const res = await proxy(request("/api/orders"), "/orders");
    expect(res.status).toBe(401);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reenvía con el Bearer de la cookie, la consulta y el correlation-id, y devuelve la respuesta tal cual", async () => {
    fetchMock.mockResolvedValue(upstream(200, { items: [] }));

    const res = await proxy(
      request("/api/orders?status=ISSUING&q=pay-1", { token: "jwt-1", headers: { "X-Correlation-ID": "corr-1" } }),
      "/orders",
    );

    expect(fetchMock.mock.calls[0]?.[0]).toBe(`${ORCH}/orders?status=ISSUING&q=pay-1`);
    expect(sentHeaders().get("Authorization")).toBe("Bearer jwt-1");
    expect(sentHeaders().get("X-Correlation-ID")).toBe("corr-1");
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ items: [] });
  });

  it("si el orquestador responde 401 (token vencido) borra la cookie", async () => {
    fetchMock.mockResolvedValue(upstream(401, { error: "invalid token" }));

    const res = await proxy(request("/api/orders", { token: "viejo" }), "/orders");

    expect(res.status).toBe(401);
    expect(res.headers.get("set-cookie")).toMatch(new RegExp(`${SESSION_COOKIE}=;.*(Max-Age=0|Expires=Thu, 01 Jan 1970)`, "i"));
  });

  it("si el orquestador no responde devuelve 502 sin detalles internos", async () => {
    fetchMock.mockRejectedValue(new TypeError("fetch failed: ECONNREFUSED 10.0.0.5"));

    const res = await proxy(request("/api/orders", { token: "jwt-1" }), "/orders");

    expect(res.status).toBe(502);
    expect(JSON.stringify(await res.json())).not.toContain("10.0.0.5");
  });

  it("rechaza un POST que viene de otro sitio (CSRF) sin llamar al orquestador", async () => {
    const req = request("/api/orders/x/actions/retry-issuance", {
      method: "POST",
      token: "jwt-1",
      headers: { Origin: "https://evil.example" },
    });

    const res = await proxy(req, "/orders/x/actions/retry-issuance", "POST");

    expect(res.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("acepta un POST del mismo origen", async () => {
    fetchMock.mockResolvedValue(upstream(200, { id: "x", status: "ISSUING" }));
    const req = request("/api/orders/x/actions/retry-issuance", {
      method: "POST",
      token: "jwt-1",
      headers: { Origin: "http://localhost:3000" },
    });

    const res = await proxy(req, "/orders/x/actions/retry-issuance", "POST");

    expect(res.status).toBe(200);
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("POST");
  });
});

describe("login", () => {
  const body = JSON.stringify({ username: "ana.perez", password: "clave-segura-123" });

  it("guarda el token en una cookie httpOnly, Secure y SameSite=Strict, y no lo devuelve al navegador", async () => {
    fetchMock.mockResolvedValue(upstream(200, { token: "jwt-secreto", expiresAt: "2026-10-24T11:00:00Z" }));

    const res = await login(request("/api/auth/login", { method: "POST", body, headers: { Origin: "http://localhost:3000" } }));

    expect(res.status).toBe(200);
    const json: unknown = await res.json();
    expect(JSON.stringify(json)).not.toContain("jwt-secreto");
    expect(json).toEqual({ expiresAt: "2026-10-24T11:00:00Z" });
    const cookie = res.headers.get("set-cookie") ?? "";
    expect(cookie).toContain(`${SESSION_COOKIE}=jwt-secreto`);
    expect(cookie).toMatch(/HttpOnly/i);
    expect(cookie).toMatch(/Secure/i);
    expect(cookie).toMatch(/SameSite=Strict/i);
    expect(cookie).toMatch(/Path=\//i);
    expect(cookie).toMatch(/Expires=/i);
  });

  it("propaga 401 y 429 (con Retry-After) sin crear la cookie", async () => {
    fetchMock.mockResolvedValueOnce(upstream(401, { error: "invalid credentials" }));
    const denied = await login(request("/api/auth/login", { method: "POST", body }));
    expect(denied.status).toBe(401);
    expect(denied.headers.get("set-cookie")).toBeNull();

    fetchMock.mockResolvedValueOnce(upstream(429, { error: "too many attempts" }, { "Retry-After": "12" }));
    const limited = await login(request("/api/auth/login", { method: "POST", body }));
    expect(limited.status).toBe(429);
    expect(limited.headers.get("Retry-After")).toBe("12");
    expect(limited.headers.get("set-cookie")).toBeNull();
  });

  it("no crea la cookie si el orquestador responde 200 sin token", async () => {
    fetchMock.mockResolvedValue(upstream(200, { expiresAt: "2026-10-24T11:00:00Z" }));
    const res = await login(request("/api/auth/login", { method: "POST", body }));
    expect(res.status).toBe(502);
    expect(res.headers.get("set-cookie")).toBeNull();
  });

  it("rechaza un cuerpo inválido sin llamar al orquestador", async () => {
    const res = await login(request("/api/auth/login", { method: "POST", body: '{"username": 1}' }));
    expect(res.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rechaza un login que viene de otro sitio", async () => {
    const res = await login(request("/api/auth/login", { method: "POST", body, headers: { Origin: "https://evil.example" } }));
    expect(res.status).toBe(403);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("logout", () => {
  it("borra la cookie de sesión", () => {
    const res = logout(request("/api/auth/logout", { method: "POST", token: "jwt-1", headers: { Origin: "http://localhost:3000" } }));
    expect(res.headers.get("set-cookie")).toMatch(new RegExp(`${SESSION_COOKIE}=;`));
  });

  it("rechaza un logout que viene de otro sitio", () => {
    const res = logout(request("/api/auth/logout", { method: "POST", token: "jwt-1", headers: { Origin: "https://evil.example" } }));
    expect(res.status).toBe(403);
    expect(res.headers.get("set-cookie")).toBeNull();
  });
});

describe("stream", () => {
  it("reenvía Last-Event-ID y devuelve el flujo como text/event-stream sin caché", async () => {
    fetchMock.mockResolvedValue(
      new Response("id: 7\nevent: order-status\ndata: {}\n\n", { status: 200, headers: { "Content-Type": "text/event-stream" } }),
    );

    const res = await stream(request("/api/stream", { token: "jwt-1", headers: { "Last-Event-ID": "6" } }));

    expect(fetchMock.mock.calls[0]?.[0]).toBe(`${ORCH}/events/stream`);
    expect(sentHeaders().get("Last-Event-ID")).toBe("6");
    expect(sentHeaders().get("Authorization")).toBe("Bearer jwt-1");
    expect(res.headers.get("Content-Type")).toBe("text/event-stream");
    expect(res.headers.get("Cache-Control")).toContain("no-cache");
    const text = await res.text();
    expect(text.startsWith(": connected\n\n")).toBe(true); // abre la respuesta sin esperar el primer evento
    expect(text).toContain("id: 7");
  });

  it("toma el último id de la URL cuando la consola tuvo que abrir otra conexión", async () => {
    fetchMock.mockResolvedValue(new Response("", { status: 200, headers: { "Content-Type": "text/event-stream" } }));

    await stream(request("/api/stream?lastEventId=41", { token: "jwt-1" }));
    await stream(request("/api/stream?lastEventId=41;DROP", { token: "jwt-1" }));

    expect(sentHeaders(0).get("Last-Event-ID")).toBe("41");
    expect(sentHeaders(1).get("Last-Event-ID")).toBeNull();
  });

  it("sin sesión responde 401", async () => {
    const res = await stream(request("/api/stream"));
    expect(res.status).toBe(401);
  });
});
