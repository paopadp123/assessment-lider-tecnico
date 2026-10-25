import { NextResponse, type NextRequest } from "next/server";

import { proxy } from "@/lib/server/bff";
import { isOperatorAction, orderPath } from "@/lib/server/paths";

type Params = { params: Promise<{ id: string; action: string }> };

export async function POST(req: NextRequest, { params }: Params): Promise<Response> {
  const { id, action } = await params;
  const path = isOperatorAction(action) ? orderPath(id, `/actions/${action}`) : null;
  return path === null ? NextResponse.json({ error: "not found" }, { status: 404 }) : proxy(req, path, "POST");
}
