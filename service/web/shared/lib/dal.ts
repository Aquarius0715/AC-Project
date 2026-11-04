// Data Access Layer (Next.js authentication guide): the only server-side way to read the session and call the Core
// API. Server Components and Server Actions use it; Client Components never import it.
import "server-only";
import { cache } from "react";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { dropSession, loadSession, needsRefresh, SESSION_COOKIE, type Session } from "@ac/web/lib/session";
import type { DomainError } from "@ac/web/lib/ops";
import { coreRequest } from "@ac/web/lib/rest";
import { DEFAULT_DISPLAY, isLocale, type Display } from "@ac/web/lib/i18n";
import type { ListArgs, OpArgs } from "@ac/web/lib/opTypes";

/** The session of the cookie (sealed, or stored under the sealed ID it holds, IR382), or null. proxy.ts refreshes tokens
before pages render (Server Components cannot write cookies), so an expired token here means the refresh failed.
Memoized per render pass. */
export const getSession = cache(async (): Promise<Session | null> => {
  const jar = await cookies();
  const s = await loadSession(jar.get(SESSION_COOKIE)?.value);
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

export interface CoreOptions { write?: boolean; expectedVersion?: number; idempotencyKey?: string }

/** Calls one Core API operation as the signed-in membership at its REST route (IR222) and returns `data`
(DomainError → CoreError). The input is typed by the contract (IR313). */
export async function coreOp<T>(...[operation, input, options = {}]: OpArgs<CoreOptions>): Promise<T> {
  const { data } = await coreCall<T>(operation, input, options);
  return data;
}

/** Every item of a list operation: follows nextCursor with the same filters and sort (SR14), up to `max` items. */
export async function coreAll<T>(...[operation, query = {}, max = 1000]: ListArgs): Promise<T[]> {
  const out: T[] = [];
  let cursor: string | null = null;
  do {
    const response: { data: { items: T[]; nextCursor: string | null } } = await coreCall<{ items: T[]; nextCursor: string | null }>(operation, { ...query, limit: 100, ...(cursor && { cursor }) }, {});
    const page = response.data;
    out.push(...page.items);
    cursor = page.nextCursor;
  } while (cursor && out.length < max);
  return out.slice(0, max);
}

/** Every item of a list and how many there are. */
export interface AllItems<T> { items: T[]; total: number }

/** coreAll as a page: every item and their count. */
export async function coreAllPage<T>(...args: ListArgs): Promise<AllItems<T>> {
  const items = await coreAll<T>(...args);
  return { items, total: items.length };
}

/** The items of one page of a list operation. */
export async function coreItems<T>(...args: OpArgs<CoreOptions>): Promise<T[]> {
  const page = await coreOp<{ items: T[] }>(...args);
  return page.items;
}

/** A read a page can do without: `fallback` when the Core API refuses it, except UNAVAILABLE and TIMEOUT, which
still fail the page (it is not readable now, not refused). */
export async function orWhenRefused<T, F>(read: Promise<T>, fallback: F): Promise<T | F> {
  try {
    return await read;
  } catch (error) {
    if (error instanceof CoreError && error.error.code !== "UNAVAILABLE" && error.error.code !== "TIMEOUT") return fallback;
    throw error;
  }
}

/** `fallback` when the session may not read it (FORBIDDEN); every other failure still fails. */
export async function orWhenForbidden<T, F>(read: Promise<T>, fallback: F): Promise<T | F> {
  try {
    return await read;
  } catch (error) {
    if (error instanceof CoreError && error.error.code === "FORBIDDEN") return fallback;
    throw error;
  }
}

/** null for a record the session cannot see (FORBIDDEN, or NOT_FOUND: archived or another tenant's). */
export async function orAbsent<T>(read: Promise<T>): Promise<T | null> {
  try {
    return await read;
  } catch (error) {
    if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null;
    throw error;
  }
}

/** session.get once per render pass: the principal and the response meta. */
const coreSession = cache(() => coreCall<{ permissions?: string[]; userId?: string; membershipId?: string; tenantId?: string; clientRole?: "owner" | "member" | null; displayName?: string; organizationName?: string; organizationId?: string; customerId?: string | null }>("session.get", {}, {}));

/** The signed-in principal (session.get): user, membership, its organization and tenant, e.g. to block self-grants
before the API does or to tell the own company from another one in a URL (IR276). */
export const corePrincipal = cache(async () => {
  const { data } = await coreSession();
  return { userId: data.userId ?? "", membershipId: data.membershipId ?? "", organizationId: data.organizationId ?? "", tenantId: data.tenantId ?? "", clientRole: data.clientRole ?? null, customerId: data.customerId ?? null };
});

/** Who is signed in, for the shell: the user's display name and the membership's organization (session.get, IR241). */
export const coreIdentity = cache(async () => {
  const { data } = await coreSession();
  return { displayName: data.displayName ?? "", organizationName: data.organizationName ?? "" };
});

/** The Core API business clock (Meta.snapshotAt; the demo scenario clock in the demo environment, IR36). */
export const coreNow = cache(async (): Promise<Date> => {
  const { meta } = await coreSession();
  return meta?.snapshotAt ? new Date(meta.snapshotAt) : new Date();
});

/** The Core API clock read afresh (session.get, not memoized for the render pass): for a Server Action that has just
moved the demo clock and waits until the other services show it (IR168, IR249). */
export async function coreClockFresh(): Promise<Date> {
  const { meta } = await coreCall<unknown>("session.get", {}, {});
  return meta?.snapshotAt ? new Date(meta.snapshotAt) : new Date();
}

/** The signed-in user's display language and time zone (preferences.get, FR-X01, IR258), once per render pass: the
shell and the screens that show times read the same answer. Unreadable preferences show the defaults. */
export const coreDisplay = cache(async (): Promise<Display> => {
  try {
    const p = await coreOp<{ locale?: string; timezone?: string }>("preferences.get", {});
    return { locale: isLocale(p.locale) ? p.locale : DEFAULT_DISPLAY.locale, timeZone: p.timezone || DEFAULT_DISPLAY.timeZone };
  } catch {
    return DEFAULT_DISPLAY;
  }
});

/** The signed-in membership's permissions (session.get), for permission-scoped sections; the API still authorizes. */
export const corePermissions = cache(async (): Promise<Set<string>> => {
  const { data } = await coreSession();
  return new Set(data.permissions);
});

async function coreCall<T>(operation: string, input: unknown, options: CoreOptions): Promise<{ data: T; meta?: { snapshotAt?: string } }> {
  const s = await verifySession();
  const req = coreRequest(operation, input);
  if (!req) throw new CoreError(404, { code: "NOT_FOUND", messageKey: "error.unknownOperation", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
  const headers: Record<string, string> = {
    Authorization: `Bearer ${s.accessToken}`,
    "X-Tenant-Id": s.tenantId,
    "X-Membership-Id": s.membershipId,
  };
  if (req.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.write) headers["Idempotency-Key"] = options.idempotencyKey ?? crypto.randomUUID();
  if (options.expectedVersion !== undefined) headers["X-Expected-Version"] = String(options.expectedVersion);
  const res = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}${req.path}`, { method: req.method, body: req.body, cache: "no-store", headers });
  const body = await res.json().catch(() => null);
  // the Core API no longer takes this sign-in (HQ signed the user out everywhere, IR384, or the membership ended): the
  // stored session goes too, so the next page asks to sign in; the caller still gets the refusal (IR385)
  if (res.status === 401) {
    const jar = await cookies();
    await dropSession(jar.get(SESSION_COOKIE)?.value);
  }
  if (!res.ok) throw new CoreError(res.status, body ?? { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
  return body as { data: T; meta?: { snapshotAt?: string } };
}

export const apiMode = () => process.env.DATA_SOURCE === "api";
