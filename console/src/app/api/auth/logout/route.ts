import type { NextRequest } from "next/server";

import { logout } from "@/lib/server/bff";

export function POST(req: NextRequest): Response {
  return logout(req);
}
