// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

// the request's cookie jar and a session store kept in memory (GETEX / SET / DEL)
const jar = vi.hoisted(() => ({ value: undefined as string | undefined, get: vi.fn(), set: vi.fn(), delete: vi.fn() }));
const kv = vi.hoisted(() => new Map<string, string>());
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => ({ ...jar, get: () => (jar.value ? { value: jar.value } : undefined) }) }));
vi.mock("next/navigation", () => ({ redirect: (to: string) => { throw new Error(`redirect ${to}`); } }));
vi.mock("@redis/client", () => ({
  createClient: () => {
    const c = {
      on: () => c, connect: async () => c,
      getEx: async (k: string) => kv.get(k) ?? null,
      set: async (k: string, v: string, o: { condition?: string }) => (o.condition === "XX" && !kv.has(k) ? null : (kv.set(k, v), "OK")),
      del: async (k: string) => (kv.delete(k) ? 1 : 0),
    };
    return c;
  },
}));

import { coreOp, CoreError } from "@ac/web/lib/dal";
import { createSessionCookie, loadSession, OIDC_COOKIE, seal, unseal, type Session } from "@ac/web/lib/session";
import { signedOutReason, signInAgainHref } from "@ac/web/lib/login";
import { GET as login } from "@ac/web/bff/auth/login/route";
import { GET as callback } from "@ac/web/bff/auth/callback/route";

const b64 = (v: unknown) => Buffer.from(JSON.stringify(v)).toString("base64url");
const token = `${b64({ alg: "RS256" })}.${b64({ tenant_id: "t-1", membership_id: "m-1", role: "client" })}.sig`;
const session: Session = { accessToken: token, refreshToken: "r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: Math.floor(Date.now() / 1000) + 300 };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const revoked = { code: "UNAUTHENTICATED", messageKey: "errors.session_revoked", fieldErrors: {}, correlationId: "c", retryAfterSeconds: null };

beforeEach(() => {
  vi.stubEnv("SESSION_STORE_URL", "redis://store.test:6379");
  vi.stubEnv("DATA_SOURCE", "api");
  kv.clear();
  jar.value = undefined;
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

// IR385: after HQ signed the user out everywhere (IR384), the apps let the refused session go and ask for the password
describe("a session the Core API refuses", () => {
  it("is dropped from the store by the DAL, and the caller still gets the refusal", async () => {
    jar.value = await createSessionCookie(session);
    vi.stubGlobal("fetch", vi.fn(async () => json(401, revoked)));
    await expect(coreOp("units.list", { limit: 5 })).rejects.toBeInstanceOf(CoreError);
    expect([await loadSession(jar.value), kv.size]).toEqual([null, 0]);
  });

  it("sends the browser to sign-in with why, and back to the page it was on", () => {
    expect([signedOutReason("errors.session_revoked"), signedOutReason("error.membershipInvalid"), signedOutReason(undefined)]).toEqual(["revoked", "expired", "expired"]);
    expect(signInAgainHref("revoked", "/admin/units?customerId=c-1")).toBe("/login?error=revoked&returnTo=%2Fadmin%2Funits%3FcustomerId%3Dc-1");
  });
});

describe("signing in again after a revocation", () => {
  it("asks the identity provider for the password when reauth=1 (prompt=login), and only then", async () => {
    const again = await login(new NextRequest("http://localhost:3000/bff/auth/login?reauth=1&returnTo=/customer"));
    const url = new URL(again.headers.get("location")!);
    const state = unseal<{ reauth?: boolean; returnTo: string | null }>(again.cookies.get(OIDC_COOKIE)?.value);
    expect([url.searchParams.get("prompt"), state?.reauth, state?.returnTo]).toEqual(["login", true, "/customer"]);
    const plain = await login(new NextRequest("http://localhost:3000/bff/auth/login?returnTo=/customer"));
    expect(new URL(plain.headers.get("location")!).searchParams.get("prompt")).toBeNull();
  });

  const signIn = (pending: { reauth?: boolean }, coreAnswer: Response) => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => (String(url).includes("/protocol/openid-connect/token") ? json(200, { access_token: token, refresh_token: "r-1", expires_in: 300 }) : coreAnswer)));
    const cookie = seal({ state: "s-1", verifier: "v-1", returnTo: "/customer/alerts", ...pending });
    return callback(new NextRequest("http://localhost:3000/bff/auth/callback?code=c-1&state=s-1", { headers: { cookie: `${OIDC_COOKIE}=${cookie}` } }));
  };

  it("turns a single sign-on answer with the old sign-in into one password prompt", async () => {
    const res = await signIn({}, json(401, revoked));
    const to = new URL(res.headers.get("location")!);
    expect([to.pathname, to.searchParams.get("reauth"), to.searchParams.get("returnTo"), kv.size]).toEqual(["/bff/auth/login", "1", "/customer/alerts", 0]);
  });

  it("gives up after the password prompt rather than looping", async () => {
    const res = await signIn({ reauth: true }, json(401, revoked));
    expect([new URL(res.headers.get("location")!).pathname + new URL(res.headers.get("location")!).search, kv.size]).toEqual(["/login?error=revoked", 0]);
  });

  it("starts the session when the Core API takes the sign-in", async () => {
    const res = await signIn({ reauth: true }, json(200, { data: {} }));
    expect([new URL(res.headers.get("location")!).pathname, kv.size]).toEqual(["/customer/alerts", 1]);
  });
});
