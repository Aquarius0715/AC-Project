import { createCipheriv, randomBytes } from "node:crypto";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SESSION_COOKIE, cookieKey, cookieOptions, endIdpSession, jwtClaims, needsRefresh, oidcConfig, readSession, refresh, roleHome, safeReturnTo, seal, sessionFromTokens, unseal, type Session } from "@ac/web/lib/session";

// the BFF session (backend architecture §5) is server code: next/headers is the request's cookie jar
const jar = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => jar }));

const NOW = Date.parse("2026-09-15T01:00:00Z");
const b64 = (v: unknown) => Buffer.from(JSON.stringify(v)).toString("base64url");
const token = (claims: Record<string, unknown>) => `${b64({ alg: "RS256" })}.${b64(claims)}.signature`;
const claims = { tenant_id: "t-1", membership_id: "m-1", role: "client" };
const session = (over: Partial<Session> = {}): Session => ({ accessToken: token(claims), refreshToken: "r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: NOW / 1000 + 300, ...over });

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, toFake: ["Date"] });
  jar.get.mockReset(); jar.set.mockReset(); jar.delete.mockReset();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("the sealed session cookie (AES-256-GCM, IR333)", () => {
  it("opens what it sealed and refuses anything altered", () => {
    const t = seal({ a: 1, b: "x" });
    expect(unseal(t)).toEqual({ a: 1, b: "x" });
    const [iv, body] = t.split(".");
    const flip = (v: string, i: number) => { const b = Buffer.from(v, "base64url"); b[i] ^= 1; return b.toString("base64url"); };
    expect([unseal(`${flip(iv, 0)}.${body}`), unseal(`${iv}.${flip(body, 0)}`), unseal(`${iv}.${flip(body, Buffer.from(body, "base64url").length - 1)}`),
      unseal(`${iv}.`), unseal(`.${body}`), unseal(iv), unseal(`${t}.x`), unseal(`${iv.slice(2)}.${body}`), unseal(undefined), unseal("")])
      .toEqual([null, null, null, null, null, null, null, null, null, null]);
  });

  it("keeps the tokens unreadable to the browser and never repeats a value", () => {
    const s = session();
    const t = seal(s);
    const plain = Buffer.from(t.split(".")[1], "base64url").toString("latin1");
    expect([plain.includes("accessToken"), plain.includes("r-1"), t.includes(Buffer.from(JSON.stringify(s)).toString("base64url").slice(0, 16))]).toEqual([false, false, false]);
    expect(seal(s)).not.toBe(t); // a fresh IV each time
    expect(unseal<Session>(t)).toEqual(s);
  });

  it("depends on the server's secret", () => {
    vi.stubEnv("SESSION_SECRET", "one");
    const t = seal({ a: 1 });
    vi.stubEnv("SESSION_SECRET", "two");
    expect(unseal(t)).toBeNull();
    vi.stubEnv("SESSION_SECRET", "one");
    expect(unseal(t)).toEqual({ a: 1 });
  });

  it("on https needs a real secret: none, an empty or a short one seals nothing (IR367)", () => {
    vi.stubEnv("APP_URL", "https://ac.example.com");
    for (const v of [undefined, "", "x".repeat(31)]) {
      vi.stubEnv("SESSION_SECRET", v);
      expect(() => cookieKey()).toThrow(/SESSION_SECRET/);
      expect(() => seal({ a: 1 })).toThrow(/SESSION_SECRET/);
    }
    vi.stubEnv("SESSION_SECRET", "test-".padEnd(32, "s"));
    expect(unseal(seal({ a: 1 }))).toEqual({ a: 1 });
    // a cookie sealed before the secret went missing cannot be opened either: the visitor is signed out
    const t = seal({ a: 1 });
    vi.stubEnv("SESSION_SECRET", "");
    expect(unseal(t)).toBeNull();
  });

  it("refuses a correctly sealed body that is not JSON", () => {
    vi.stubEnv("SESSION_SECRET", "s");
    const iv = randomBytes(12);
    const c = createCipheriv("aes-256-gcm", cookieKey(), iv);
    const body = Buffer.concat([c.update("not json", "utf8"), c.final(), c.getAuthTag()]);
    expect(unseal(`${iv.toString("base64url")}.${body.toString("base64url")}`)).toBeNull();
  });

  it("is httpOnly, lax, 30 minutes, and secure only on https", () => {
    expect(cookieOptions()).toEqual({ httpOnly: true, sameSite: "lax", secure: false, path: "/", maxAge: 1800 });
    vi.stubEnv("APP_URL", "https://ac.example.com");
    expect(cookieOptions().secure).toBe(true);
  });
});

describe("a session from the identity provider's tokens", () => {
  it("keeps the tenant, membership and role claims, and times the access token from now", () => {
    expect(sessionFromTokens({ access_token: token(claims), refresh_token: "r-2", expires_in: 300 }))
      .toEqual({ accessToken: token(claims), refreshToken: "r-2", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: NOW / 1000 + 300 });
    expect(sessionFromTokens({ access_token: token(claims), expires_in: 60 }, "r-old")?.refreshToken).toBe("r-old"); // a refresh without a new token keeps the old one
  });

  it("refuses a token without the claims or with an unknown role", () => {
    const bad = [{ ...claims, tenant_id: undefined }, { ...claims, membership_id: "" }, { ...claims, role: undefined }, { ...claims, role: "superuser" },
      { ...claims, role: "constructor" }, { ...claims, role: "toString" }, { ...claims, role: "__proto__" }, { ...claims, role: 1 }];
    expect(bad.map((c) => sessionFromTokens({ access_token: token(c), expires_in: 300 }))).toEqual(bad.map(() => null));
    expect(sessionFromTokens({ access_token: "not-a-jwt", expires_in: 300 })).toBeNull();
    expect(Object.keys(roleHome)).toEqual(["client", "contractor", "technician", "admin"]);
  });

  it("decodes a payload without trusting it, and nothing from a broken token", () => {
    expect(jwtClaims(token({ sub: "u-1" }))).toEqual({ sub: "u-1" });
    expect([jwtClaims("a"), jwtClaims("a.%%%.c"), jwtClaims("")]).toEqual([{}, {}, {}]);
  });
});

describe("refreshing the access token", () => {
  const answer = (status: number, body: unknown) => vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }));

  it("is due 30 seconds before the access token expires", () => {
    expect([needsRefresh(session({ expiresAt: NOW / 1000 + 31 })), needsRefresh(session({ expiresAt: NOW / 1000 + 30 })), needsRefresh(session({ expiresAt: NOW / 1000 - 1 }))]).toEqual([false, true, true]);
  });

  it("posts the refresh grant to the internal token endpoint and keeps the same tenant and membership", async () => {
    vi.stubEnv("OIDC_INTERNAL_ISSUER", "http://keycloak:8080/realms/ac");
    vi.stubEnv("OIDC_APP_CLIENT_SECRET", "test-client-secret");
    const fetch = answer(200, { access_token: token(claims), refresh_token: "r-2", expires_in: 300 });
    vi.stubGlobal("fetch", fetch);
    expect(await refresh(session())).toMatchObject({ refreshToken: "r-2", tenantId: "t-1", membershipId: "m-1", expiresAt: NOW / 1000 + 300 });
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("http://keycloak:8080/realms/ac/protocol/openid-connect/token");
    expect(Object.fromEntries(new URLSearchParams(String(init.body)))).toEqual({ grant_type: "refresh_token", refresh_token: "r-1", client_id: "ac-web", client_secret: "test-client-secret" });
  });

  it("gives nothing for another membership, a refused grant, a network failure or no refresh token", async () => {
    vi.stubGlobal("fetch", answer(200, { access_token: token({ ...claims, membership_id: "m-2" }), expires_in: 300 }));
    expect(await refresh(session())).toBeNull();
    vi.stubGlobal("fetch", answer(400, { error: "invalid_grant" }));
    expect(await refresh(session())).toBeNull();
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("fetch failed"); }));
    expect(await refresh(session())).toBeNull();
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect(await refresh(session({ refreshToken: null }))).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });
});

// IR357: sign-out ends the identity provider's session (backend architecture §6)
describe("ending the identity provider's session", () => {
  it("posts the refresh token with the client's credentials to the internal logout endpoint", async () => {
    vi.stubEnv("OIDC_INTERNAL_ISSUER", "http://keycloak:8080/realms/ac");
    vi.stubEnv("OIDC_APP_CLIENT_SECRET", "test-client-secret");
    const fetch = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    expect(await endIdpSession("r-1")).toBe(true);
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect([url, init.method]).toEqual(["http://keycloak:8080/realms/ac/protocol/openid-connect/logout", "POST"]);
    expect(Object.fromEntries(new URLSearchParams(String(init.body)))).toEqual({ refresh_token: "r-1", client_id: "ac-web", client_secret: "test-client-secret" });
    expect(init.signal).toBeInstanceOf(AbortSignal); // never waits long for the provider
  });
  it("answers false when the provider refuses or cannot be reached", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: "invalid_grant" }), { status: 400 })));
    expect(await endIdpSession("r-1")).toBe(false);
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("fetch failed"); }));
    expect(await endIdpSession("r-1")).toBe(false);
  });
});

describe("reading the session in a route handler", () => {
  it("returns a valid session as it is", async () => {
    const s = session();
    jar.get.mockImplementation((name: string) => (name === SESSION_COOKIE ? { value: seal(s) } : undefined));
    expect(await readSession()).toEqual(s);
    expect([jar.set.mock.calls.length, jar.delete.mock.calls.length]).toEqual([0, 0]);
    expect(SESSION_COOKIE).toBe("ac_session_client"); // one cookie per app: local apps share the host
  });

  it("refreshes an expiring session and rewrites the cookie", async () => {
    jar.get.mockReturnValue({ value: seal(session({ expiresAt: NOW / 1000 + 10 })) });
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ access_token: token(claims), refresh_token: "r-2", expires_in: 300 }))));
    expect(await readSession()).toMatchObject({ refreshToken: "r-2", expiresAt: NOW / 1000 + 300 });
    const [name, value, options] = jar.set.mock.calls[0];
    expect([name, unseal<Session>(value)?.refreshToken, options.maxAge]).toEqual([SESSION_COOKIE, "r-2", 1800]);
  });

  it("clears the cookie when the refresh fails, and ignores a missing or forged cookie", async () => {
    jar.get.mockReturnValue({ value: seal(session({ expiresAt: NOW / 1000 + 10 })) });
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 401 })));
    expect(await readSession()).toBeNull();
    expect(jar.delete).toHaveBeenCalledWith(SESSION_COOKIE);
    jar.get.mockReturnValue(undefined);
    expect(await readSession()).toBeNull();
    jar.get.mockReturnValue({ value: `${b64(session())}.forged` });
    expect(await readSession()).toBeNull();
  });
});

describe("where sign-in returns to", () => {
  it("keeps a same-origin path and refuses absolute, protocol-relative, backslash and control-character forms", () => {
    expect([safeReturnTo("/customer/units/u1?tab=history"), safeReturnTo("/")]).toEqual(["/customer/units/u1?tab=history", "/"]);
    const bad = ["https://evil.example", "//evil.example", "/\\evil.example", "\\\\evil.example", "customer", "/\tevil", "/\nSet-Cookie:x", "/\u0000", "", null, undefined];
    expect(bad.map((v) => safeReturnTo(v))).toEqual(bad.map(() => null));
  });
});

describe("the identity provider's settings", () => {
  it("defaults to the local realm and reads the deployment's environment", () => {
    expect(oidcConfig()).toMatchObject({ issuer: "http://localhost:8081/realms/ac", internalIssuer: "http://localhost:8081/realms/ac", clientId: "ac-web", appUrl: "http://localhost:3000" });
    vi.stubEnv("OIDC_ISSUER", "https://id.example/realms/ac");
    vi.stubEnv("OIDC_APP_CLIENT_ID", "ac-admin-web");
    expect(oidcConfig()).toMatchObject({ issuer: "https://id.example/realms/ac", internalIssuer: "https://id.example/realms/ac", clientId: "ac-admin-web" });
  });
});
