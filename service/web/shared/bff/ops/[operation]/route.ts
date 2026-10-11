import { NextResponse, type NextRequest } from "next/server";
import { readSession } from "@ac/web/lib/session";
import { coreRequest } from "@ac/web/lib/rest";
import { sameOriginRequest } from "@ac/web/lib/securityHeaders";

const OP = /^[a-zA-Z]+(\.[a-zA-Z]+)+$/;

function domainError(status: number, code: string, messageKey: string, fieldErrors: Record<string, string> = {}) {
  return NextResponse.json({ code, messageKey, fieldErrors, correlationId: crypto.randomUUID(), retryAfterSeconds: null }, { status });
}

// The largest input the relay passes on: the Core API's body limit for every operation without files (1 MiB); the apps
// send files through Server Actions, which have their own bodySizeLimit. The proxy, which would cap a body, does not run
// on /bff, so without this cap a body of any size was read into memory before anything else was checked (IR370).
const MAX_INPUT_BYTES = 1 << 20;

/** The request body as text, or null when it is larger than max: by its Content-Length, or as soon as reading it passes
 * max (a chunked body says no length). */
async function readCapped(req: NextRequest, max: number): Promise<string | null> {
  if (Number(req.headers.get("content-length") ?? 0) > max) return null;
  if (!req.body) return "";
  const reader = req.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > max) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  return Buffer.concat(chunks).toString("utf8");
}

// POST /bff/ops/<operation>: relays one operation call of the browser (its JSON input) to the operation's REST route
// of the Core API (IR222) with the session token and the selected tenant / membership (backend architecture §5,
// §15). DomainError bodies and status codes pass through unchanged. Only the app's own pages may call it (CSRF, as
// Next.js protects Server Actions, IR358). The apps send it reads only — their writes are Server Actions, which a
// contract test checks (coreWrites); the E2E suite's restore helpers send writes through it.
export async function POST(req: NextRequest, ctx: { params: Promise<{ operation: string }> }) {
  if (!sameOriginRequest(req.headers)) return domainError(403, "FORBIDDEN", "errors.cross_site_request");
  if (process.env.DATA_SOURCE !== "api") return domainError(503, "UNAVAILABLE", "errors.api_data_source_disabled");
  const { operation } = await ctx.params;
  if (!OP.test(operation)) return domainError(404, "NOT_FOUND", "error.unknownOperation");
  let input: unknown = {};
  const raw = await readCapped(req, MAX_INPUT_BYTES);
  if (raw === null) return domainError(422, "VALIDATION", "error.bodyTooLarge"); // as the Core API answers it
  if (raw.trim()) {
    try {
      input = JSON.parse(raw);
    } catch {
      return domainError(422, "VALIDATION", "error.validation", { _: "error.malformedInput" });
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
