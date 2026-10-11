// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { createSessionCookie, loadSession, SESSION_COOKIE, seal, type Session } from "@ac/web/lib/session";
import { POST } from "@ac/web/bff/auth/logout/route";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));
// the session store, used only where a test sets SESSION_STORE_URL (IR382)
const kv = vi.hoisted(() => new Map<string, string>());
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
  it("on https drops the __Host- session cookie with Secure and Path=/, as a browser requires (IR374)", async () => {
    vi.stubEnv("APP_URL", "https://ac.example.com");
    vi.stubEnv("SESSION_SECRET", "test-".padEnd(32, "s"));
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    vi.resetModules();
    const { POST: post } = await import("@ac/web/bff/auth/logout/route");
    const s = await import("@ac/web/lib/session");
    const res = await post(new NextRequest("https://ac.example.com/bff/auth/logout", {
      method: "POST", headers: { cookie: `${s.SESSION_COOKIE}=${s.seal(session)}`, origin: "https://ac.example.com", host: "ac.example.com", "sec-fetch-site": "same-origin" },
    }));
    const set = res.headers.get("set-cookie") ?? "";
    expect([res.status, set.startsWith("__Host-ac_session_client=;"), /;\s*Secure/i.test(set), /;\s*Path=\/(;|$)/.test(set), /Max-Age=0/.test(set)]).toEqual([303, true, true, true, true]);
    vi.resetModules();
  });
  it("refuses a sign-out another site posts (CSRF, IR358)", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const res = await signOut(seal(session), { origin: "https://evil.example", "sec-fetch-site": "cross-site" });
    expect([res.status, dropped(res)]).toEqual([403, false]);
    expect(fetch).not.toHaveBeenCalled();
  });
});

// IR382: with the session store, sign-out deletes the stored session, so a copied cookie opens nothing afterwards
describe("POST /bff/auth/logout with the session store", () => {
  it("deletes the stored session and ends the provider's session with the stored refresh token", async () => {
    vi.stubEnv("SESSION_STORE_URL", "redis://store.test:6379");
    const fetch = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    const cookie = await createSessionCookie(session);
    const copy = `${cookie}`;
    const res = await signOut(cookie);
    expect([res.status, dropped(res), kv.size]).toEqual([303, true, 0]);
    expect(new URLSearchParams(String((fetch.mock.calls[0] as unknown as [string, RequestInit])[1].body)).get("refresh_token")).toBe("r-1");
    expect(await loadSession(copy)).toBeNull();
  });
});
