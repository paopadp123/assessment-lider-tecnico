// Enmascarado para los logs, igual que en el orquestador (orchestrator/internal/order/identity.go):
// la placa deja ver la serie y el documento sus últimos 4 dígitos.

export function maskPlate(plate: string): string {
  return plate.length < 3 ? "***" : `${plate.slice(0, 3)}***`;
}

export function maskDocument(doc: string): string {
  return doc.length < 4 ? "****" : `****${doc.slice(-4)}`;
}
