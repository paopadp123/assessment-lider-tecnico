// Healthcheck del contenedor: solo dice que el servidor de Next responde (no depende del orquestador).
export function GET(): Response {
  return Response.json({ status: "ok" });
}
