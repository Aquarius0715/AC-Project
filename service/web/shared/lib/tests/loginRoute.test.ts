// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "@ac/web/bff/auth/login/route";

vi.mock("server-only", () => ({}));

const start = async (query: string) => {
  const res = await GET(new NextRequest(`http://localhost:3000/bff/auth/login?${query}`));
  const to = new URL(res.headers.get("location") ?? "");
  return { to, params: to.searchParams, cookie: res.headers.get("set-cookie") ?? "" };
};

// IR646: the sign-in page gives an address and a language; the BFF starts the authorization-code flow with PKCE
describe("GET /bff/auth/login", () => {
  it("redirects to the identity provider with PKCE, the address as login hint and the language as ui_locales", async () => {
    const { to, params, cookie } = await start("login_hint=mia%40example.com&returnTo=%2Fcustomer%2Fautomations&ui_locales=ja");
    expect(to.pathname).toBe("/realms/ac/protocol/openid-connect/auth");
    expect([params.get("response_type"), params.get("code_challenge_method"), params.get("login_hint"), params.get("ui_locales"), params.get("prompt")])
      .toStrictEqual(["code", "S256", "mia@example.com", "ja", null]);
    expect(params.get("state")?.length).toBeGreaterThan(10);
    expect(cookie).toContain("ac_oidc_");
  });

  it("passes on neither a hint that is no address nor a language that is not a display language", async () => {
    const { params } = await start("login_hint=customer-a&ui_locales=fr");
    expect([params.has("login_hint"), params.has("ui_locales")]).toStrictEqual([false, false]);
  });

  it("asks for the password again after a revocation (reauth=1)", async () => {
    const { params } = await start("reauth=1");
    expect(params.get("prompt")).toBe("login");
  });
});
