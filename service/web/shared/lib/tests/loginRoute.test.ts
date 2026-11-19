// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "@ac/web/bff/auth/login/route";

vi.mock("server-only", () => ({}));

const start = async (query: string, host?: string) => {
  const res = await GET(new NextRequest(`http://localhost:3000/bff/auth/login?${query}`, host ? { headers: { host } } : undefined));
  const to = new URL(res.headers.get("location") ?? "");
  return { to, params: to.searchParams, cookie: res.headers.get("set-cookie") ?? "" };
};

// IR790: the app's /login goes straight here; the BFF starts the authorization-code flow with PKCE
describe("GET /bff/auth/login", () => {
  it("redirects to the identity provider with PKCE and keeps the state in a sealed cookie", async () => {
    const { to, params, cookie } = await start("returnTo=%2Fcustomer%2Fautomations");
    expect(to.pathname).toBe("/realms/ac/protocol/openid-connect/auth");
    expect([params.get("response_type"), params.get("code_challenge_method"), params.has("login_hint"), params.has("ui_locales"), params.get("prompt")])
      .toStrictEqual(["code", "S256", false, false, null]);
    expect(params.get("state")?.length).toBeGreaterThan(10);
    expect(cookie).toContain("ac_oidc_");
  });

  it("asks for the password again after a revocation (reauth=1)", async () => {
    const { params } = await start("reauth=1");
    expect(params.get("prompt")).toBe("login");
  });

  // IR651: a sign-in that starts on a listed address goes to the identity provider on that address and returns to it
  it("uses the listed address the request came by, and the canonical one for any other Host header", async () => {
    vi.stubEnv("OIDC_ISSUER", "https://app.example:8081/realms/ac");
    vi.stubEnv("APP_URL", "https://app.example:3000");
    vi.stubEnv("SESSION_SECRET", "a-session-secret-of-more-than-thirty-two-characters");
    vi.stubEnv("PUBLIC_HOSTS", "alt.example");
    const listed = await start("returnTo=%2Fcustomer", "alt.example:3000");
    expect([listed.to.origin, listed.params.get("redirect_uri")]).toStrictEqual(["https://alt.example:8081", "https://alt.example:3000/bff/auth/callback"]);
    const other = await start("returnTo=%2Fcustomer", "evil.example");
    expect([other.to.origin, other.params.get("redirect_uri")]).toStrictEqual(["https://app.example:8081", "https://app.example:3000/bff/auth/callback"]);
    vi.stubEnv("PUBLIC_HOSTS", "");
    const unlisted = await start("returnTo=%2Fcustomer", "alt.example:3000"); // nothing listed: the canonical address
    expect(unlisted.to.origin).toBe("https://app.example:8081");
  });
});
