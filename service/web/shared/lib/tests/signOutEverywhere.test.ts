// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

// the request's cookie jar and a session store kept in memory (GETEX / SET / DEL)
const jar = vi.hoisted(() => ({ value: null as string | null, get: vi.fn(), set: vi.fn(), delete: vi.fn() }));
const kv = vi.hoisted(() => new Map<string, string>());
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ ...jar, get: () => (jar.value ? { value: jar.value } : undefined) }) }));
vi.mock("next/navigation", () => ({ redirect: (to: string) => { throw new Error(`redirect ${to}`); } }));
vi.mock("@redis/client", () => {
  return {
    createClient: () => {
      const c = {
        on: () => c, connect: () => Promise.resolve(c),
        getEx: (k: string) => Promise.resolve(kv.get(k) ?? null),
        set: (k: string, v: string, o: { condition?: string }) => Promise.resolve(o.condition === "XX" && !kv.has(k) ? null : (kv.set(k, v), "OK")),
        del: (k: string) => Promise.resolve(kv.delete(k) ? 1 : 0),
      };
      return c;
    },
  };
});

import { coreOp, CoreError } from "@ac/web/lib/dal";
import { createSessionCookie, loadSession, OIDC_COOKIE, seal, unseal, type Session } from "@ac/web/lib/session";
import { signedOutReason, signInAgainHref } from "@ac/web/lib/login";
import { GET as login } from "@ac/web/bff/auth/login/route";
import { GET as callback } from "@ac/web/bff/auth/callback/route";
import { must } from "./must";
const locationOf = (res: Response) => new URL(must(res.headers.get("location"))); // where a redirect leads

const b64 = (v: unknown) => Buffer.from(JSON.stringify(v)).toString("base64url");
const token = `${b64({ alg: "RS256" })}.${b64({ tenant_id: "t-1", membership_id: "m-1", role: "client" })}.sig`;
const session: Session = { accessToken: token, refreshToken: "r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: Math.floor(Date.now() / 1000) + 300 };
const json = (status: number, body: unknown) => Response.json(body, { status, headers: { "Content-Type": "application/json" } });
const revoked = { code: "UNAUTHENTICATED", messageKey: "errors.session_revoked", fieldErrors: {}, correlationId: "c", retryAfterSeconds: null };

beforeEach(() => {
  vi.stubEnv("SESSION_STORE_URL", "redis://store.test:6379");
  vi.stubEnv("DATA_SOURCE", "api");
  kv.clear();
  jar.value = null;
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

// IR385: after HQ signed the user out everywhere (IR384), the apps let the refused session go and ask for the password
describe("a session the Core API refuses", () => {
  it("is dropped from the store by the DAL, and the caller still gets the refusal", async () => {
    jar.value = await createSessionCookie(session);
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(json(401, revoked))));
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
    const url = locationOf(again);
    const state = unseal<{ reauth?: boolean; returnTo: string | null }>(again.cookies.get(OIDC_COOKIE)?.value);
    expect([url.searchParams.get("prompt"), state?.reauth, state?.returnTo]).toEqual(["login", true, "/customer"]);
    const plain = await login(new NextRequest("http://localhost:3000/bff/auth/login?returnTo=/customer"));
    expect(locationOf(plain).searchParams.get("prompt")).toBeNull();
  });

  const signIn = (pending: { reauth?: boolean; returnTo?: string | null }, coreAnswer: Response) => {
    vi.stubGlobal("fetch", vi.fn((url: string) => Promise.resolve(url.includes("/protocol/openid-connect/token") ? json(200, { access_token: token, refresh_token: "r-1", expires_in: 300 }) : coreAnswer)));
    const cookie = seal({ state: "s-1", verifier: "v-1", returnTo: "/customer/alerts", ...pending });
    return callback(new NextRequest("http://localhost:3000/bff/auth/callback?code=c-1&state=s-1", { headers: { cookie: `${OIDC_COOKIE}=${cookie}` } }));
  };

  it("turns a single sign-on answer with the old sign-in into one password prompt", async () => {
    const res = await signIn({}, json(401, revoked));
    const to = locationOf(res);
    expect([to.pathname, to.searchParams.get("reauth"), to.searchParams.get("returnTo"), kv.size]).toEqual(["/bff/auth/login", "1", "/customer/alerts", 0]);
  });

  it("gives up after the password prompt rather than looping", async () => {
    const res = await signIn({ reauth: true }, json(401, revoked));
    expect([locationOf(res).pathname + locationOf(res).search, kv.size]).toEqual(["/login?error=revoked", 0]);
  });

  it("starts the session when the Core API takes the sign-in", async () => {
    const res = await signIn({ reauth: true }, json(200, { data: {} }));
    expect([locationOf(res).pathname, kv.size]).toEqual(["/customer/alerts", 1]);
  });

  // open redirects (security check R380): a crafted returnTo goes through the sign-in, the password prompt after a
  // refused single sign-on, and the second sign-in; every redirect on the way stays on the app's own origin
  it("never leaves the app's origin, whatever returnTo says", async () => {
    const crafted = ["https://evil.example/x", "//evil.example", "/\\evil.example", "/.//evil.example", "/customer/..//evil.example", "/%2F%2Fevil.example", "/\t/evil.example", "javascript:alert(1)", "/@evil.example", " //evil.example"];
    const origin = "http://localhost:3000";
    const kept = async (url: string) => {
      const res = await login(new NextRequest(url));
      return must(unseal<{ returnTo: string | null; reauth?: boolean }>(res.cookies.get(OIDC_COOKIE)?.value));
    };
    for (const returnTo of crafted) {
      const first = await kept(`${origin}/bff/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
      const refused = await signIn({ returnTo: first.returnTo }, json(401, revoked));
      const hop = locationOf(refused);
      const second = await kept(hop.toString());
      const signedIn = await signIn({ returnTo: second.returnTo, reauth: second.reauth }, json(200, { data: {} }));
      const home = locationOf(signedIn);
      expect([returnTo, hop.origin, hop.pathname, home.origin]).toEqual([returnTo, origin, "/bff/auth/login", origin]);
    }
  });
});
