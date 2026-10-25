import type { NextRequest } from "next/server";

import { login } from "@/lib/server/bff";

export function POST(req: NextRequest): Promise<Response> {
  return login(req);
}
