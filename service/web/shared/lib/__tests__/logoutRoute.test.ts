// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { SESSION_COOKIE, seal, type Session } from "@ac/web/lib/session";
import { POST } from "@ac/web/bff/auth/logout/route";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const session: Session = { accessToken: "a.b.c", refreshToken: "r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: 9_999_999_999 };
const signOut = (cookie?: string, extra: Record<string, string> = {}) => POST(new NextRequest("http://localhost:3000/bff/auth/logout", { method: "POST", headers: { ...(cookie ? { cookie: `${SESSION_COOKIE}=${cookie}` } : {}), ...extra } }));
const dropped = (res: Response) => (res.headers.get("set-cookie") ?? "").startsWith(`${SESSION_COOKIE}=;`);

// IR357: sign-out ends the identity provider's session of the sign-in, then drops the BFF session
describe("POST /bff/auth/logout", () => {
  it("ends the provider's session with the session's refresh token and returns to sign-in", async () => {
    const fetch = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    const res = await signOut(seal(session));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(new URLSearchParams(String((fetch.mock.calls[0] as unknown as [string, RequestInit])[1].body)).get("refresh_token")).toBe("r-1");
    expect([res.status, res.headers.get("location"), dropped(res)]).toEqual([303, "http://localhost:3000/login", true]);
  });
  it("still signs out when the provider cannot be reached, and calls nothing without a session", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("fetch failed"); }));
    const failed = await signOut(seal(session));
    expect([failed.status, dropped(failed)]).toEqual([303, true]);
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    for (const res of [await signOut(), await signOut("forged"), await signOut(seal({ ...session, refreshToken: null }))]) expect([res.status, dropped(res)]).toEqual([303, true]);
    expect(fetch).not.toHaveBeenCalled();
  });
  it("refuses a sign-out another site posts (CSRF, IR358)", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const res = await signOut(seal(session), { origin: "https://evil.example", "sec-fetch-site": "cross-site" });
    expect([res.status, dropped(res)]).toEqual([403, false]);
    expect(fetch).not.toHaveBeenCalled();
  });
});
