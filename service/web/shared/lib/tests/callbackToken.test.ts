// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "@ac/web/bff/auth/callback/route";
import { OIDC_COOKIE, seal } from "@ac/web/lib/session";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

/** Where a sign-in whose state matches is sent when the identity provider's token endpoint answers with `fetch`. */
async function sentTo(fetch: () => Promise<Response>): Promise<string | null> {
  vi.stubEnv("APP_URL", "http://app.example:3000");
  vi.stubGlobal("fetch", vi.fn(fetch));
  const cookie = seal({ state: "s", verifier: "v", returnTo: null });
  const res = await GET(new NextRequest("http://app.example:3000/bff/auth/callback?code=c&state=s", { headers: { cookie: `${OIDC_COOKIE}=${cookie}` } }));
  return res.headers.get("location");
}

// the code-for-token exchange: an answer that is not the token response, or none in time, is the token error of the
// sign-in page, not the error page (IR719)
describe("GET /bff/auth/callback, the token exchange", () => {
  it("sends a token endpoint that answers something other than JSON to the sign-in page's token error", async () => {
    expect(await sentTo(() => Promise.resolve(new Response("<html>502 Bad Gateway</html>", { status: 200 })))).toBe("http://app.example:3000/login?error=token");
  });

  it("sends a token endpoint that cannot be reached, or does not answer in time, there too", async () => {
    expect(await sentTo(() => Promise.reject(new DOMException("The operation was aborted due to timeout", "TimeoutError")))).toBe("http://app.example:3000/login?error=token");
  });
});
