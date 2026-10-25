import type { NextRequest } from "next/server";

import { proxy } from "@/lib/server/bff";

export function GET(req: NextRequest): Promise<Response> {
  return proxy(req, "/orders");
}
