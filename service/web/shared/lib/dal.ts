// Data Access Layer (Next.js authentication guide): the only server-side way to read the session and call the Core
// API. Server Components and Server Actions use it; Client Components never import it.
import "server-only";
import { cache } from "react";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { needsRefresh, SESSION_COOKIE, verify, type Session } from "@ac/web/lib/session";
import type { DomainError } from "@ac/web/lib/ops";

/** The session from the signed cookie, or null. proxy.ts refreshes tokens before pages render (Server Components
 * cannot write cookies), so an expired token here means the refresh failed. Memoized per render pass. */
export const getSession = cache(async (): Promise<Session | null> => {
  const s = verify<Session>((await cookies()).get(SESSION_COOKIE)?.value);
  return s && !needsRefresh(s) ? s : null;
});

/** The session, or a redirect to sign-in. */
export const verifySession = cache(async (): Promise<Session> => {
  const s = await getSession();
  if (!s) redirect("/login");
  return s;
});

export class CoreError extends Error {
  constructor(public readonly status: number, public readonly error: DomainError) {
    super(`${error.code}: ${error.messageKey}`);
  }
}

export type CoreOptions = { write?: boolean; expectedVersion?: number; idempotencyKey?: string };

/** Calls one Core API operation as the signed-in membership and returns `data` (DomainError → CoreError). */
export async function coreOp<T>(operation: string, input: unknown, opts: CoreOptions = {}): Promise<T> {
  return (await coreCall<T>(operation, input, opts)).data;
}

/** session.get once per render pass: the principal and the response meta. */
const coreSession = cache(() => coreCall<{ permissions?: string[]; userId?: string; membershipId?: string; tenantId?: string }>("session.get", {}, {}));

/** The signed-in principal (session.get): user, membership and tenant, e.g. to block self-grants before the API does. */
export const corePrincipal = cache(async () => {
  const { data } = await coreSession();
  return { userId: data.userId ?? "", membershipId: data.membershipId ?? "", tenantId: data.tenantId ?? "" };
});

/** The Core API business clock (Meta.snapshotAt; the demo scenario clock in the demo environment, IR36). */
export const coreNow = cache(async (): Promise<Date> => {
  const { meta } = await coreSession();
  return meta?.snapshotAt ? new Date(meta.snapshotAt) : new Date();
});

/** The signed-in membership's permissions (session.get), for permission-scoped sections; the API still authorizes. */
export const corePermissions = cache(async (): Promise<Set<string>> => new Set((await coreSession()).data.permissions ?? []));

async function coreCall<T>(operation: string, input: unknown, opts: CoreOptions): Promise<{ data: T; meta?: { snapshotAt?: string } }> {
  const s = await verifySession();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${s.accessToken}`,
    "X-Tenant-Id": s.tenantId,
    "X-Membership-Id": s.membershipId,
  };
  if (opts.write) headers["Idempotency-Key"] = opts.idempotencyKey ?? crypto.randomUUID();
  if (opts.expectedVersion !== undefined) headers["X-Expected-Version"] = String(opts.expectedVersion);
  const res = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/ops/${operation}`, {
    method: "POST", headers, body: JSON.stringify(input ?? {}), cache: "no-store",
  });
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new CoreError(res.status, body ?? { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
  return body as { data: T; meta?: { snapshotAt?: string } };
}

export const apiMode = () => process.env.DATA_SOURCE === "api";
