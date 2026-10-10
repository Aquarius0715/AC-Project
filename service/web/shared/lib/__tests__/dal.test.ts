import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { coreAll, coreDisplay, CoreError, coreNow, coreOp, corePermissions, corePrincipal, getSession } from "@ac/web/lib/dal";
import { SESSION_COOKIE, sign, type Session } from "@ac/web/lib/session";

// the Data Access Layer (Next.js authentication guide) runs on the server: the cookie jar and redirect come from Next
const jar = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/navigation", () => ({ redirect: (to: string) => { throw new Error(`NEXT_REDIRECT ${to}`); } }));

const NOW = Date.parse("2026-09-15T01:00:00Z");
const session: Session = { accessToken: "access-1", refreshToken: "r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: NOW / 1000 + 300 };
type Call = [string, RequestInit & { headers: Record<string, string> }];
/** The CoreError a call is refused with (a call that succeeds fails the test). */
const refusal = (p: Promise<unknown>) => p.then(() => { throw new Error("the call succeeded"); }, (e: unknown) => e as CoreError);
const reply = (...bodies: [number, unknown][]) => {
  const f = vi.fn(async () => {
    const [status, body] = bodies.length > 1 ? bodies.shift()! : bodies[0];
    return new Response(typeof body === "string" ? body : JSON.stringify(body), { status });
  });
  vi.stubGlobal("fetch", f);
  return f as unknown as { mock: { calls: Call[] } };
};

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, toFake: ["Date"] });
  jar.get.mockImplementation((name: string) => (name === SESSION_COOKIE ? { value: sign(session) } : undefined));
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("calling the Core API as the signed-in membership (IR222)", () => {
  it("sends the operation's REST route with the token, tenant and membership", async () => {
    vi.stubEnv("CORE_API_URL", "http://gateway:8080");
    const f = reply([200, { data: { id: "u1" } }]);
    expect(await coreOp("units.get", { id: "u1" })).toEqual({ id: "u1" });
    const [url, init] = f.mock.calls[0];
    expect([url, init.method, init.body, init.cache]).toEqual(["http://gateway:8080/v1/units/u1", "GET", undefined, "no-store"]);
    expect(init.headers).toEqual({ Authorization: "Bearer access-1", "X-Tenant-Id": "t-1", "X-Membership-Id": "m-1" }); // no body, no write: no other header
  });

  it("marks a write with an idempotency key and the expected version", async () => {
    const f = reply([200, { data: { id: "c1" } }]);
    await coreOp("commands.create", { unitId: "u1", action: { kind: "set_power", power: true }, expectedUnitVersion: 3 }, { write: true, idempotencyKey: "key-1", expectedVersion: 7 });
    const [url, init] = f.mock.calls[0];
    expect([url, init.method, JSON.parse(String(init.body)).unitId]).toEqual(["http://localhost:8080/v1/commands", "POST", "u1"]);
    expect(init.headers).toMatchObject({ "Content-Type": "application/json", "Idempotency-Key": "key-1", "X-Expected-Version": "7" });
    await coreOp("commands.create", { unitId: "u1", action: { kind: "set_power", power: true }, expectedUnitVersion: 3 }, { write: true });
    expect(f.mock.calls[1][1].headers["Idempotency-Key"]).toMatch(/^[0-9a-f-]{36}$/); // a fresh key per intent when none is given
  });

  it("turns a refusal into a CoreError with the API's DomainError, and an unreadable one into UNAVAILABLE", async () => {
    const refused = { code: "VALIDATION", messageKey: "error.invalid", fieldErrors: { action: "error.unsupportedAction" }, correlationId: "c-1", retryAfterSeconds: null };
    reply([422, refused]);
    const e = await refusal(coreOp("units.get", { id: "u1" }));
    expect([e instanceof CoreError, e.status, e.error, e.message]).toEqual([true, 422, refused, "VALIDATION: error.invalid"]);
    reply([502, "<html>bad gateway</html>"]);
    expect((await refusal(coreOp("units.get", { id: "u1" }))).error).toMatchObject({ code: "UNAVAILABLE", messageKey: "error.unavailable" });
  });

  it("refuses an operation outside the catalog without calling anything", async () => {
    const f = reply([200, {}]);
    // @ts-expect-error an operation outside the catalog does not type-check either
    const e = await refusal(coreOp("units.teleport", {}));
    expect([e.status, e.error.messageKey, f.mock.calls.length]).toEqual([404, "error.unknownOperation", 0]);
  });

  it("sends a visitor without a valid session to sign-in", async () => {
    const f = reply([200, { data: {} }]);
    jar.get.mockReturnValue(undefined);
    await expect(coreOp("units.get", { id: "u1" })).rejects.toThrow("NEXT_REDIRECT /login");
    jar.get.mockReturnValue({ value: sign({ ...session, expiresAt: NOW / 1000 + 10 }) }); // expiring: the proxy's refresh failed
    expect(await getSession()).toBeNull();
    await expect(coreOp("units.get", { id: "u1" })).rejects.toThrow("NEXT_REDIRECT /login");
    expect(f.mock.calls.length).toBe(0);
  });
});

describe("reading whole lists (SR14)", () => {
  it("follows nextCursor with the same filters and sort, 100 per page", async () => {
    const f = reply([200, { data: { items: [1, 2], nextCursor: "c2" } }], [200, { data: { items: [3], nextCursor: null } }]);
    expect(await coreAll("units.list", { filters: { propertyId: "p1" }, sort: { field: "createdAt", direction: "asc" } })).toEqual([1, 2, 3]);
    const pages = f.mock.calls.map(([url]) => new URL(url).searchParams);
    expect(pages.map((q) => [q.get("propertyId"), q.get("sort"), q.get("limit"), q.get("cursor")])).toEqual([["p1", "createdAt:asc", "100", null], ["p1", "createdAt:asc", "100", "c2"]]);
  });

  it("stops at the maximum, never past it", async () => {
    const page = (n: number, next: string | null) => [200, { data: { items: Array.from({ length: 100 }, (_, i) => n * 100 + i), nextCursor: next } }] as [number, unknown];
    const f = reply(page(0, "c1"), page(1, "c2"), page(2, null));
    const all = await coreAll<number>("units.list", {}, 150);
    expect([all.length, all.at(-1), f.mock.calls.length]).toEqual([150, 149, 2]);
  });
});

describe("the session's principal, clock and display settings", () => {
  it("reads session.get for the principal, the permissions and the business clock", async () => {
    reply([200, { data: { userId: "u-1", membershipId: "m-1", organizationId: "o-1", tenantId: "t-1", clientRole: "owner", permissions: ["units.read"] }, meta: { snapshotAt: "2026-09-14T01:00:00Z" } }]);
    expect(await corePrincipal()).toEqual({ userId: "u-1", membershipId: "m-1", organizationId: "o-1", tenantId: "t-1", clientRole: "owner", customerId: null });
    expect([...(await corePermissions())]).toEqual(["units.read"]);
    expect((await coreNow()).toISOString()).toBe("2026-09-14T01:00:00.000Z");
  });

  it("uses the saved language and time zone, and the defaults when they are unknown or unreadable", async () => {
    reply([200, { data: { locale: "ms", timezone: "Asia/Tokyo" } }]);
    expect(await coreDisplay()).toEqual({ locale: "ms", timeZone: "Asia/Tokyo" });
    reply([200, { data: { locale: "jp", timezone: "" } }]);
    expect(await coreDisplay()).toEqual({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    reply([503, { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "", retryAfterSeconds: 1 }]);
    expect(await coreDisplay()).toEqual({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  });
});
