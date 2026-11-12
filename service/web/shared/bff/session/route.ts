import { NextResponse } from "next/server";
import { z } from "zod";
import { readSession } from "@ac/web/lib/session";

const coreSession = z.object({ data: z.object({ clientRole: z.string().nullish() }).optional(), meta: z.object({ snapshotAt: z.string().optional() }).optional() });

// GET /bff/session: tells the browser which data source this deployment uses and who is signed in (no tokens).
// serverNow is the Core API clock (the demo scenario clock in the demo environment, IR36) from Meta.snapshotAt;
// clientRole (owner / member) decides the owner-only Users item of the customer app (IR210).
export async function GET() {
  const s = await readSession();
  const isApi = process.env.DATA_SOURCE === "api";
  let serverNow: string | null = null;
  let clientRole: string | null = null;
  if (isApi && s) {
    try {
      const r = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/session`, {
        cache: "no-store", signal: AbortSignal.timeout(3000), // a stalled Core API must not hang the page's session bootstrap
        headers: { Authorization: `Bearer ${s.accessToken}`, "X-Tenant-Id": s.tenantId, "X-Membership-Id": s.membershipId },
      });
      if (r.ok) {
        const body = coreSession.safeParse(await r.json());
        serverNow = body.success ? body.data.meta?.snapshotAt ?? null : null;
        clientRole = body.success ? body.data.data?.clientRole ?? null : null;
      }
    } catch {
      serverNow = null;
      clientRole = null;
    }
  }
  return NextResponse.json({ dataSource: isApi ? "api" : "mock", signedIn: !!s, role: s?.role ?? null, membershipId: s?.membershipId ?? null, clientRole, serverNow });
}
