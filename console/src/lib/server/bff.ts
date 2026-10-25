// BFF (backend for frontend): el navegador solo habla con Next.js; estas funciones, que corren en el
// servidor de Next, le hablan al orquestador. El JWT vive en una cookie httpOnly: el JavaScript de la
// página nunca lo ve, así que un XSS no puede robarlo.
import { NextResponse, type NextRequest } from "next/server";

export const SESSION_COOKIE = "ops_session";

const correlationFormat = /^[A-Za-z0-9._:-]{1,64}$/;
const eventIdFormat = /^[0-9]+$/;

export function orchestratorUrl(): string {
  return (process.env.ORCHESTRATOR_URL ?? "http://localhost:8080").replace(/\/+$/, "");
}

// Secure por defecto. Los navegadores aceptan cookies Secure en http://localhost, así que la demo local
// funciona igual; COOKIE_SECURE=false solo existe para servir la consola por http en otra máquina.
function cookieSecure(): boolean {
  return process.env.COOKIE_SECURE !== "false";
}

function json(status: number, body: unknown): NextResponse {
  return NextResponse.json(body, { status });
}

function clearSession(res: NextResponse): NextResponse {
  res.cookies.set(SESSION_COOKIE, "", { httpOnly: true, secure: cookieSecure(), sameSite: "strict", path: "/", maxAge: 0 });
  return res;
}

// La cookie es SameSite=Strict, así que otro sitio no puede usarla. Además se rechaza toda solicitud que
// declare venir de otro origen: es la segunda defensa contra CSRF si un navegador viejo ignora SameSite.
function crossSite(req: NextRequest): boolean {
  const origin = req.headers.get("origin");
  if (origin === null) {
    return false;
  }
  try {
    return new URL(origin).host !== (req.headers.get("host") ?? req.nextUrl.host);
  } catch {
    return true;
  }
}

function forwardHeaders(req: NextRequest, token: string): Headers {
  const headers = new Headers({ Authorization: `Bearer ${token}`, Accept: "application/json" });
  const corr = req.headers.get("x-correlation-id");
  headers.set("X-Correlation-ID", corr !== null && correlationFormat.test(corr) ? corr : crypto.randomUUID());
  return headers;
}

/** Reenvía la solicitud al orquestador con el token de la sesión. */
export async function proxy(req: NextRequest, path: string, method: "GET" | "POST" = "GET"): Promise<Response> {
  if (method !== "GET" && crossSite(req)) {
    return json(403, { error: "cross-site request" });
  }
  const token = req.cookies.get(SESSION_COOKIE)?.value;
  if (token === undefined || token === "") {
    return json(401, { error: "not authenticated" });
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${orchestratorUrl()}${path}${req.nextUrl.search}`, {
      method,
      headers: forwardHeaders(req, token),
      cache: "no-store",
    });
  } catch (err) {
    // El detalle (host, IP) queda en el log del servidor de Next, nunca en la respuesta.
    console.error("orchestrator request failed", path, err);
    return json(502, { error: "orchestrator unavailable" });
  }

  const res = new NextResponse(await upstream.text(), {
    status: upstream.status,
    headers: { "Content-Type": upstream.headers.get("content-type") ?? "application/json" },
  });
  const corr = upstream.headers.get("x-correlation-id");
  if (corr !== null) {
    res.headers.set("X-Correlation-ID", corr);
  }
  // Token vencido o inválido: se borra la cookie para que la consola vuelva al login.
  return upstream.status === 401 ? clearSession(res) : res;
}

function isCredentials(v: unknown): v is { username: string; password: string } {
  if (typeof v !== "object" || v === null) {
    return false;
  }
  const { username, password } = v as Record<string, unknown>;
  return (
    typeof username === "string" &&
    typeof password === "string" &&
    username.length > 0 &&
    password.length > 0 &&
    username.length <= 200 &&
    password.length <= 200
  );
}

/** Inicia sesión: el token queda en la cookie y al navegador solo le llega su vencimiento. */
export async function login(req: NextRequest): Promise<Response> {
  if (crossSite(req)) {
    return json(403, { error: "cross-site request" });
  }
  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return json(400, { error: "invalid JSON body" });
  }
  if (!isCredentials(body)) {
    return json(400, { error: "username and password are required" });
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${orchestratorUrl()}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: body.username, password: body.password }),
      cache: "no-store",
    });
  } catch (err) {
    console.error("orchestrator login failed", err);
    return json(502, { error: "orchestrator unavailable" });
  }

  if (!upstream.ok) {
    const res = new NextResponse(await upstream.text(), {
      status: upstream.status,
      headers: { "Content-Type": upstream.headers.get("content-type") ?? "application/json" },
    });
    const retryAfter = upstream.headers.get("retry-after");
    if (retryAfter !== null) {
      res.headers.set("Retry-After", retryAfter);
    }
    return res;
  }

  const session: unknown = await upstream.json().catch(() => null);
  if (!isSession(session)) {
    console.error("orchestrator login returned an unexpected body");
    return json(502, { error: "orchestrator unavailable" });
  }
  const { token, expiresAt } = session;
  const res = json(200, { expiresAt });
  res.cookies.set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure: cookieSecure(),
    sameSite: "strict",
    path: "/",
    expires: new Date(expiresAt), // la cookie vence con el token
  });
  return res;
}

function isSession(v: unknown): v is { token: string; expiresAt: string } {
  if (typeof v !== "object" || v === null) {
    return false;
  }
  const { token, expiresAt } = v as Record<string, unknown>;
  return typeof token === "string" && token !== "" && typeof expiresAt === "string" && !Number.isNaN(Date.parse(expiresAt));
}

export function logout(req: NextRequest): Response {
  // Un sitio ajeno no puede cerrarle la sesión al operador.
  if (crossSite(req)) {
    return json(403, { error: "cross-site request" });
  }
  return clearSession(json(200, { ok: true }));
}

/** Abre el flujo SSE del orquestador y lo reenvía tal cual, con el Last-Event-ID del navegador. */
export async function stream(req: NextRequest): Promise<Response> {
  const token = req.cookies.get(SESSION_COOKIE)?.value;
  if (token === undefined || token === "") {
    return json(401, { error: "not authenticated" });
  }
  const headers = forwardHeaders(req, token);
  headers.set("Accept", "text/event-stream");
  // EventSource reenvía el último id al reconectar (cabecera). Si la consola tuvo que crear otro
  // EventSource (el anterior se cerró), lo manda en la URL, porque a uno nuevo no se le pueden poner cabeceras.
  const last = req.headers.get("last-event-id") ?? req.nextUrl.searchParams.get("lastEventId");
  if (last !== null && eventIdFormat.test(last)) {
    headers.set("Last-Event-ID", last);
  }

  let upstream: Response;
  try {
    // Si el navegador cierra la pestaña, req.signal corta también la conexión con el orquestador.
    upstream = await fetch(`${orchestratorUrl()}/events/stream`, { headers, signal: req.signal, cache: "no-store" });
  } catch (err) {
    console.error("orchestrator stream failed", err);
    return json(502, { error: "orchestrator unavailable" });
  }
  if (!upstream.ok || upstream.body === null) {
    const res = json(upstream.status, { error: "stream unavailable" });
    return upstream.status === 401 ? clearSession(res) : res;
  }
  // Next no envía la respuesta hasta el primer trozo del cuerpo, y el orquestador no manda nada hasta el
  // primer evento o el ping de los 15 s: sin esto el navegador espera hasta 15 s para ver la conexión abierta.
  // Un comentario SSE (": ...") lo ignora EventSource y abre la respuesta de inmediato.
  const { readable, writable } = new TransformStream<Uint8Array, Uint8Array>();
  const writer = writable.getWriter();
  const upstreamBody = upstream.body;
  void writer
    .write(new TextEncoder().encode(": connected\n\n"))
    .then(() => {
      writer.releaseLock();
      // Si el navegador se va, se cancela readable y pipeTo cancela también el flujo del orquestador.
      return upstreamBody.pipeTo(writable);
    })
    .catch(() => undefined);
  return new Response(readable, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      "X-Accel-Buffering": "no",
    },
  });
}
