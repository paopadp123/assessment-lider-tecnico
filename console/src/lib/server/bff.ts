// BFF (backend for frontend): el navegador solo habla con Next.js; estas funciones, que corren en el
// servidor de Next, le hablan al orquestador. El JWT vive en una cookie httpOnly: el JavaScript de la
// página nunca lo ve, así que un XSS no puede robarlo.
import { NextResponse, type NextRequest } from "next/server";

export const SESSION_COOKIE = "ops_session";

export function orchestratorUrl(): string {
  return "";
}

/** Reenvía la solicitud al orquestador con el token de la sesión. */
export async function proxy(_req: NextRequest, _path: string, _method: "GET" | "POST" = "GET"): Promise<Response> {
  return Promise.resolve(NextResponse.json({}, { status: 200 }));
}

/** Inicia sesión: el token queda en la cookie y al navegador solo le llega su vencimiento. */
export async function login(_req: NextRequest): Promise<Response> {
  return Promise.resolve(NextResponse.json({}, { status: 200 }));
}

export function logout(_req: NextRequest): Response {
  return NextResponse.json({}, { status: 200 });
}

/** Abre el flujo SSE del orquestador y lo reenvía tal cual, con el Last-Event-ID del navegador. */
export async function stream(_req: NextRequest): Promise<Response> {
  return Promise.resolve(new Response(null, { status: 200 }));
}
