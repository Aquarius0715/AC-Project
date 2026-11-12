// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@ac/web/bff/session/route";

const SESSION = { accessToken: "t", tenantId: "tenant-1", membershipId: "m-1", role: "client" };

vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/session", () => ({ readSession: () => Promise.resolve(SESSION) }));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("GET /bff/session", () => {
  it("gives the Core API call a deadline, so a stalled Core API does not hang the session bootstrap", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    vi.stubEnv("CORE_API_URL", "https://core:8080");
    const fetch = vi.fn((_url: string, init: RequestInit) => {
      expect(init.signal).toBeInstanceOf(AbortSignal);
      return Promise.resolve(Response.json({ data: { clientRole: "owner" }, meta: { snapshotAt: "2026-11-11T00:00:00Z" } }));
    });
    vi.stubGlobal("fetch", fetch);
    const res = await GET();
    const body: unknown = await res.json();
    expect(fetch).toHaveBeenCalledOnce();
    expect(body).toMatchObject({ signedIn: true, clientRole: "owner", serverNow: "2026-11-11T00:00:00Z" });
  });
});
