import { NextResponse, type NextRequest } from "next/server";

import { proxy } from "@/lib/server/bff";
import { orderPath } from "@/lib/server/paths";

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }): Promise<Response> {
  const path = orderPath((await params).id, "/calls");
  return path === null ? NextResponse.json({ error: "order not found" }, { status: 404 }) : proxy(req, path);
}
