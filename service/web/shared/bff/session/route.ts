import { NextResponse } from "next/server";
import { readSession } from "@ac/web/lib/session";

// GET /bff/session: tells the browser which data source this deployment uses and who is signed in (no tokens).
// serverNow is the Core API clock (the demo scenario clock in the demo environment, IR36) from Meta.snapshotAt;
// clientRole (owner / member) decides the owner-only Users item of the customer app (IR210).
export async function GET() {
  const s = await readSession();
  const api = process.env.DATA_SOURCE === "api";
  let serverNow: string | null = null;
  let clientRole: string | null = null;
  if (api && s) {
    try {
      const r = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/session`, {
        method: "GET", cache: "no-store",
        headers: { Authorization: `Bearer ${s.accessToken}`, "X-Tenant-Id": s.tenantId, "X-Membership-Id": s.membershipId },
      });
      if (r.ok) {
        const body = (await r.json()) as { data?: { clientRole?: string | null }; meta?: { snapshotAt?: string } };
        serverNow = body.meta?.snapshotAt ?? null;
        clientRole = body.data?.clientRole ?? null;
      }
    } catch {
      serverNow = null;
      clientRole = null;
    }
  }
  return NextResponse.json({ dataSource: api ? "api" : "mock", signedIn: !!s, role: s?.role ?? null, membershipId: s?.membershipId ?? null, clientRole, serverNow });
}
