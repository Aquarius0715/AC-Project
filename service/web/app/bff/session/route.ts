import { NextResponse } from "next/server";
import { readSession } from "@/lib/session";

// GET /bff/session: tells the browser which data source this deployment uses and who is signed in (no tokens).
// serverNow is the Core API clock (the demo scenario clock in the demo environment, IR36) from Meta.snapshotAt.
export async function GET() {
  const s = await readSession();
  const api = process.env.DATA_SOURCE === "api";
  let serverNow: string | null = null;
  if (api && s) {
    try {
      const r = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/ops/session.get`, {
        method: "POST", cache: "no-store", body: "{}",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${s.accessToken}`, "X-Tenant-Id": s.tenantId, "X-Membership-Id": s.membershipId },
      });
      if (r.ok) serverNow = ((await r.json()) as { meta?: { snapshotAt?: string } }).meta?.snapshotAt ?? null;
    } catch {
      serverNow = null;
    }
  }
  return NextResponse.json({ dataSource: api ? "api" : "mock", signedIn: !!s, role: s?.role ?? null, membershipId: s?.membershipId ?? null, serverNow });
}
