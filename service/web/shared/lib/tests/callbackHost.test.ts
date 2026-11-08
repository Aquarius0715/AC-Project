// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "@ac/web/bff/auth/callback/route";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => { vi.unstubAllEnvs(); });

/** Where a sign-in that lost its state is sent back to, for a callback that came by `host`. */
async function sentTo(host: string): Promise<string | null> {
  const res = await GET(new NextRequest("https://app.example:3000/bff/auth/callback?code=c&state=s", { headers: { host } }));
  return res.headers.get("location");
}

// IR651: a sign-in that comes back on a listed address is answered on that address; another Host header is not followed
describe("GET /bff/auth/callback", () => {
  it("answers a sign-in that lost its state on the address it came back to, when that address is listed", async () => {
    vi.stubEnv("APP_URL", "https://app.example:3000");
    vi.stubEnv("PUBLIC_HOSTS", "alt.example");
    expect(await sentTo("alt.example:3000")).toBe("https://alt.example:3000/login?error=state");
  });

  it("answers on the canonical address for a host that is not listed", async () => {
    vi.stubEnv("APP_URL", "https://app.example:3000");
    vi.stubEnv("PUBLIC_HOSTS", "alt.example");
    expect(await sentTo("evil.example")).toBe("https://app.example:3000/login?error=state");
  });
});
