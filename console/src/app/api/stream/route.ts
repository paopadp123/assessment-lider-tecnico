import type { NextRequest } from "next/server";

import { stream } from "@/lib/server/bff";

// Un flujo SSE nunca se cachea ni se genera al compilar.
export const dynamic = "force-dynamic";

export function GET(req: NextRequest): Promise<Response> {
  return stream(req);
}
