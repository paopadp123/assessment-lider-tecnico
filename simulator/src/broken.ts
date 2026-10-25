import type { FastifyReply } from "fastify";

/**
 * Respuesta con formato inválido: dice ser JSON pero llega cortada. Es el "5% de peticiones con formato
 * inválido" del enunciado; quien la recibe no debe tomarla como éxito ni como rechazo.
 */
export function sendBroken(reply: FastifyReply, status: number, validJson: unknown): FastifyReply {
  const text = JSON.stringify(validJson);
  return reply.code(status).type("application/json").send(text.slice(0, Math.max(1, Math.floor(text.length / 2))));
}
