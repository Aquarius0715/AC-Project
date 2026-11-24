// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "@ac/web/bff/auth/callback/route";
import { APP_ROLE, LOGIN_HINT_COOKIE, loginHintOf, OIDC_COOKIE, seal } from "@ac/web/lib/session";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

const OTHER_ROLE = APP_ROLE === "contractor" ? "client" : "contractor";

/** An unsigned access token with these claims (the BFF reads claims; the Core API checks the signature). */
const accessToken = (claims: Record<string, string>) =>
  `e30.${new TextEncoder().encode(JSON.stringify({ tenant_id: "t1", membership_id: "m1", ...claims })).toBase64({ alphabet: "base64url", omitPadding: true })}.sig`;

/** The callback's answer when the identity provider signs in an account with `claims`, after a start with `reauth`. */
async function callback(claims: Record<string, string>, reauth: boolean, refreshToken: string | null = "r") {
  vi.stubEnv("APP_URL", "http://app.example:3000");
  const fetch = vi.fn((url: string) => Promise.resolve(url.endsWith("/token")
    ? Response.json({ access_token: accessToken(claims), ...(refreshToken !== null && { refresh_token: refreshToken }), expires_in: 300 })
    : Response.json({})));
  vi.stubGlobal("fetch", fetch);
  const pending = seal({ state: "s", verifier: "v", returnTo: "/somewhere", reauth });
  const res = await GET(new NextRequest("http://app.example:3000/bff/auth/callback?code=c&state=s", { headers: { cookie: `${OIDC_COOKIE}=${pending}` } }));
  const loggedOut = fetch.mock.calls.some(([url]) => url.endsWith("/protocol/openid-connect/logout"));
  return { location: res.headers.get("location"), hint: res.cookies.get(LOGIN_HINT_COOKIE)?.value, oidc: res.cookies.get(OIDC_COOKIE)?.value, loggedOut };
}

// IR876: the four apps share one single sign-on session, so a plain sign-in can come back with another app's account
describe("GET /bff/auth/callback, another role's account", () => {
  it("ends that single sign-on and asks for the password once instead of showing the role error", async () => {
    const r = await callback({ role: OTHER_ROLE, email: "other@ac.local" }, false);
    expect(r.location).toBe("http://app.example:3000/bff/auth/login?reauth=1&returnTo=%2Fsomewhere");
    expect([r.oidc, r.hint, r.loggedOut]).toStrictEqual(["", undefined, true]); // the old state is dropped; the other account is not remembered
  });

  it("asks for the password without ending anything when the sign-in has no refresh token", async () => {
    const r = await callback({ role: OTHER_ROLE, email: "other@ac.local" }, false, null);
    expect([r.location, r.loggedOut]).toStrictEqual(["http://app.example:3000/bff/auth/login?reauth=1&returnTo=%2Fsomewhere", false]);
  });

  it("shows the role error when another role's account comes back from the password form too", async () => {
    const r = await callback({ role: OTHER_ROLE, email: "other@ac.local" }, true);
    expect([r.location, r.loggedOut]).toStrictEqual(["http://app.example:3000/login?error=role", false]); // the account typed is kept
  });
});

describe("GET /bff/auth/callback, this app's account", () => {
  it("remembers the e-mail address of the sign-in for the next password form", async () => {
    const r = await callback({ role: APP_ROLE, email: "me@ac.local" }, false);
    expect([r.location, r.loggedOut]).toStrictEqual(["http://app.example:3000/somewhere", false]);
    expect(loginHintOf(r.hint)).toBe("me@ac.local");
  });

  it("remembers nothing when the token has no e-mail address", async () => {
    const r = await callback({ role: APP_ROLE }, false);
    expect(r.hint).toBeUndefined();
  });
});
