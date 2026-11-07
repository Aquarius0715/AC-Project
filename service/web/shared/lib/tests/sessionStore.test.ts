// @vitest-environment node
import { createHash } from "node:crypto";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Valkey as a map with expiry times (the commands the session store uses: GETEX EX, SET EX [XX], DEL)
const kv = vi.hoisted(() => {
  const data = new Map<string, { value: string; until: number }>();
  const live = (k: string) => { const r = data.get(k); if (r && r.until <= Date.now()) { data.delete(k); } return data.get(k); };
  const api = {
    down: false,
    getEx: vi.fn((k: string, o: { value: number }) => {
      if (api.down) return Promise.reject(new Error("connection lost"));
      const r = live(k);
      if (!r) return Promise.resolve(null);
      r.until = Date.now() + o.value * 1000;
      return Promise.resolve(r.value);
    }),
    set: vi.fn((k: string, v: string, o: { expiration: { value: number }; condition?: "XX" }) => {
      if (api.down) return Promise.reject(new Error("connection lost"));
      if (o.condition === "XX" && !live(k)) return Promise.resolve(null);
      data.set(k, { value: v, until: Date.now() + o.expiration.value * 1000 });
      return Promise.resolve("OK");
    }),
    del: vi.fn((k: string) => Promise.resolve(data.delete(k) ? 1 : 0)),
  };
  return { data, api };
});
vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));
vi.mock("@redis/client", () => ({ createClient: () => { const c = { on: () => c, connect: () => Promise.resolve(c), ...kv.api }; return c; } }));

import { ABSOLUTE_SECONDS, createSessionCookie, dropSession, loadSession, renewSessionCookie, seal, unsealAs, type Session } from "@ac/web/lib/session";
import { z } from "zod";

const sealedSid = z.object({ sid: z.string() });
const anyFields = z.record(z.string(), z.unknown());
import { must } from "./must";

const NOW = Date.parse("2026-09-15T01:00:00Z");
const session = (over: Partial<Session> = {}): Session => ({ accessToken: "header.claims.sig", refreshToken: "refresh:r-1", tenantId: "t-1", membershipId: "m-1", role: "client", expiresAt: NOW / 1000 + 300, ...over });
const later = (minutes: number) => vi.setSystemTime(Date.now() + minutes * 60_000);

beforeEach(() => {
  vi.stubEnv("SESSION_STORE_URL", "redis://store.test:6379");
  vi.useFakeTimers({ now: NOW, toFake: ["Date"] });
  kv.data.clear();
  kv.api.down = false;
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
});

// IR382 (OPEN-CT-03): with SESSION_STORE_URL the session lives in the store and the cookie names it
describe("the server-side session store", () => {
  it("keeps only a sealed random ID in the cookie, and no token in clear in the store", async () => {
    const cookie = await createSessionCookie(session());
    const id = unsealAs(cookie, sealedSid)?.sid ?? "";
    const fields = unsealAs(cookie, anyFields) ?? {};
    expect([Object.keys(fields), id.length]).toStrictEqual([["sid"], 43]);
    const [key, rec] = must([...kv.data][0]);
    expect(key).toBe(`ac:session:${createHash("sha256").update(id).digest("hex")}`); // the key reveals no cookie
    // the markers hold ":" and "." around words, which base64url ciphertext cannot reproduce by chance
    expect([rec.value.includes("header.claims.sig"), rec.value.includes("refresh:r-1"), kv.data.size]).toStrictEqual([false, false, 1]);
    expect(await loadSession(cookie)).toStrictEqual(session());
  });

  it("revokes the session at sign-out: a copy of the cookie stops working at once", async () => {
    const cookie = await createSessionCookie(session());
    const copy = cookie;
    await dropSession(cookie);
    expect([await loadSession(copy), kv.data.size]).toStrictEqual([null, 0]);
  });

  it("slides the idle lifetime with use and ends 30 minutes after the last one", async () => {
    const cookie = await createSessionCookie(session());
    for (let i = 0; i < 4; i++) {
      later(25);
      expect(await loadSession(cookie)).not.toBeNull();
    }
    later(31);
    expect(await loadSession(cookie)).toBeNull();
  });

  it("ends 12 hours after the sign-in however active", async () => {
    const cookie = await createSessionCookie(session());
    for (let m = 20; m < ABSOLUTE_SECONDS / 60; m += 20) {
      later(20);
      expect(await loadSession(cookie), `${String(m)} min`).not.toBeNull();
    }
    later(20);
    expect([await loadSession(cookie), kv.data.size]).toStrictEqual([null, 0]);
  });

  it("stores a refreshed session under the same cookie, but never brings a signed-out one back", async () => {
    const cookie = await createSessionCookie(session());
    const next = session({ accessToken: "new.token.sig", expiresAt: NOW / 1000 + 600 });
    expect(await renewSessionCookie(cookie, next)).toBe(cookie);
    expect(await loadSession(cookie)).toStrictEqual(next);
    await dropSession(cookie); // sign-out in another tab
    expect([await renewSessionCookie(cookie, next), kv.data.size]).toStrictEqual([null, 0]);
  });

  it("starts each sign-in with a new ID and deletes the browser's previous session", async () => {
    const first = await createSessionCookie(session());
    const second = await createSessionCookie(session({ membershipId: "m-2" }), first);
    expect(second).not.toBe(first);
    const dropped = await loadSession(first);
    const kept = await loadSession(second);
    expect([dropped, kept?.membershipId, kv.data.size]).toStrictEqual([null, "m-2", 1]);
  });

  it("refuses a cookie that carries the session itself, an altered cookie and a short ID", async () => {
    const cookie = await createSessionCookie(session());
    const [iv = "", body = ""] = cookie.split(".", 2);
    // an altered cookie: its last two characters replaced by others (a cookie that already ends in "xx" gets "yy")
    const altered = `${iv}.${body.slice(0, -2)}${body.endsWith("xx") ? "yy" : "xx"}`;
    expect([await loadSession(seal(session())), await loadSession(altered), await loadSession(seal({ sid: "short" })), await loadSession(undefined)])
      .toStrictEqual([null, null, null, null]);
  });

  it("counts the session as absent when the store does not answer, and cannot start one", async () => {
    const cookie = await createSessionCookie(session());
    kv.api.down = true;
    expect([await loadSession(cookie), await renewSessionCookie(cookie, session())]).toStrictEqual([null, null]);
    await expect(createSessionCookie(session())).rejects.toThrow("connection lost");
    kv.api.down = false;
    expect(await loadSession(cookie)).toStrictEqual(session()); // back once the store answers again
  });
});
