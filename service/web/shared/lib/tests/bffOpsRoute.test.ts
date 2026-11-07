// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { sentCall } from "./http";
import { POST } from "@ac/web/bff/ops/[operation]/route";

const LOCALHOST_3000 = "localhost:3000";
const DATA_SOURCE_2 = "DATA_SOURCE";
const TEXT_00000000_0000_4000_8000_000000000001 = "00000000-0000-4000-8000-000000000001";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve({ get: vi.fn(), set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const SAME_ORIGIN = { origin: "http://localhost:3000", host: LOCALHOST_3000, "sec-fetch-site": "same-origin" };
const relay = (operation: string, body: unknown, headers: Record<string, string> = SAME_ORIGIN) =>
  POST(new NextRequest(`http://localhost:3000/bff/ops/${operation}`, { method: "POST", body: JSON.stringify(body), headers }), { params: Promise.resolve({ operation }) });

// IR358: the BFF relay serves the app's own pages only (CSRF)
describe("POST /bff/ops/<operation>", () => {
  it("relays a read to its Core API route", async () => {
    vi.stubEnv(DATA_SOURCE_2, "api");
    vi.stubEnv("CORE_API_URL", "https://core:8080");
    const fetch = vi.fn(() => Promise.resolve(Response.json({ data: { id: "u-1" } }, { status: 200 })));
    vi.stubGlobal("fetch", fetch);
    const res = await relay("units.get", { id: TEXT_00000000_0000_4000_8000_000000000001 });
    expect([res.status, await res.json()]).toEqual([200, { data: { id: "u-1" } }]);
    expect(sentCall(fetch.mock.calls)[0]).toBe("https://core:8080/v1/units/00000000-0000-4000-8000-000000000001");
  });
  it("refuses another site's read or write without calling the Core API", async () => {
    vi.stubEnv(DATA_SOURCE_2, "api");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const evil = { origin: "https://evil.example", host: LOCALHOST_3000, "sec-fetch-site": "cross-site" };
    for (const res of [await relay("units.get", { id: "x" }, evil), await relay("jobs.cancel", { jobId: TEXT_00000000_0000_4000_8000_000000000001, cancelReason: "x" }, evil)]) {
      const body: unknown = await res.json();
      expect([res.status, body]).toMatchObject([403, { messageKey: "errors.cross_site_request" }]);
    }
    expect(fetch).not.toHaveBeenCalled();
  });
});

// IR370: the relay reads at most 1 MiB, the Core API's limit for an operation without files
const note = (bytes: number) => {
  const shell = JSON.stringify({ jobId: TEXT_00000000_0000_4000_8000_000000000001, message: "", visibility: "internal" });
  return JSON.stringify({ jobId: TEXT_00000000_0000_4000_8000_000000000001, message: "x".repeat(bytes - shell.length), visibility: "internal" });
};

describe("the relay's input limit", () => {
  const same = SAME_ORIGIN;
  const post = (body: BodyInit) => {
    const init = { method: "POST", duplex: "half" as const, headers: same, body };
    return POST(new NextRequest("http://localhost:3000/bff/ops/jobs.addNote", init), { params: Promise.resolve({ operation: "jobs.addNote" }) });
  };

  it("passes an input of exactly 1 MiB on and refuses one byte more without calling the Core API", async () => {
    vi.stubEnv(DATA_SOURCE_2, "api");
    const fetch = vi.fn(() => Promise.resolve(new Response("{}", { status: 200 })));
    vi.stubGlobal("fetch", fetch);
    expect(note(1 << 20)).toHaveLength(1 << 20);
    const accepted = await post(note(1 << 20));
    expect(accepted.status).toBe(200);
    expect(fetch).toHaveBeenCalledTimes(1);
    const res = await post(note((1 << 20) + 1));
    const body: unknown = await res.json();
    expect([res.status, body]).toMatchObject([422, { messageKey: "error.bodyTooLarge" }]);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("stops reading a body without a length as soon as it passes the limit", async () => {
    vi.stubEnv(DATA_SOURCE_2, "api");
    vi.stubGlobal("fetch", vi.fn());
    let pulled = 0;
    const chunk = new Uint8Array(64 * 1024).fill(0x20);
    const endless = new ReadableStream<Uint8Array>({ pull(c) { pulled++; c.enqueue(chunk); } }); // a client that never stops sending
    const res = await post(endless);
    const body: unknown = await res.json();
    expect([res.status, body]).toMatchObject([422, { messageKey: "error.bodyTooLarge" }]);
    expect(pulled).toBeLessThan(20); // 1 MiB is 16 chunks: reading stopped there
  });
});
