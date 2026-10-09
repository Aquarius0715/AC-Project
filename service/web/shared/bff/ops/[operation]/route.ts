import { NextResponse, type NextRequest } from "next/server";
import { readSession } from "@ac/web/lib/session";
import { coreRequest } from "@ac/web/lib/rest";

const OP = /^[a-zA-Z]+(\.[a-zA-Z]+)+$/;

function domainError(status: number, code: string, messageKey: string) {
  return NextResponse.json({ code, messageKey, fieldErrors: {}, correlationId: crypto.randomUUID(), retryAfterSeconds: null }, { status });
}

// POST /bff/ops/<operation>: relays one operation call of the browser (its JSON input) to the operation's REST route
// of the Core API (IR222) with the session token and the selected tenant / membership (backend architecture §5,
// §15). DomainError bodies and status codes pass through unchanged.
export async function POST(req: NextRequest, ctx: { params: Promise<{ operation: string }> }) {
  if (process.env.DATA_SOURCE !== "api") return domainError(503, "UNAVAILABLE", "errors.api_data_source_disabled");
  const { operation } = await ctx.params;
  if (!OP.test(operation)) return domainError(404, "NOT_FOUND", "error.unknownOperation");
  let input: unknown = {};
  const raw = await req.text();
  if (raw.trim()) {
    try {
      input = JSON.parse(raw);
    } catch {
      return NextResponse.json({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { _: "error.malformedInput" }, correlationId: crypto.randomUUID(), retryAfterSeconds: null }, { status: 422 });
    }
  }
  const call = coreRequest(operation, input);
  if (!call) return domainError(404, "NOT_FOUND", "error.unknownOperation");
  const session = await readSession();
  const headers: Record<string, string> = call.body !== undefined ? { "Content-Type": "application/json" } : {};
  if (session) {
    headers.Authorization = `Bearer ${session.accessToken}`;
    headers["X-Tenant-Id"] = session.tenantId;
    headers["X-Membership-Id"] = session.membershipId;
  }
  for (const h of ["Idempotency-Key", "X-Expected-Version"]) {
    const v = req.headers.get(h);
    if (v) headers[h] = v;
  }
  const base = process.env.CORE_API_URL ?? "http://localhost:8080";
  try {
    const upstream = await fetch(`${base}${call.path}`, { method: call.method, headers, body: call.body, cache: "no-store" });
    return new NextResponse(await upstream.text(), { status: upstream.status, headers: { "Content-Type": "application/json" } });
  } catch {
    return domainError(503, "UNAVAILABLE", "error.unavailable");
  }
}
