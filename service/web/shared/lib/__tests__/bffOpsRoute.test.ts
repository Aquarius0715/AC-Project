// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { POST } from "@ac/web/bff/ops/[operation]/route";

vi.mock("server-only", () => ({}));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: () => undefined, set: vi.fn(), delete: vi.fn() }) }));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const relay = (operation: string, body: unknown, headers: Record<string, string> = { origin: "http://localhost:3000", host: "localhost:3000", "sec-fetch-site": "same-origin" }) =>
  POST(new NextRequest(`http://localhost:3000/bff/ops/${operation}`, { method: "POST", headers, body: JSON.stringify(body) }), { params: Promise.resolve({ operation }) });

// IR358: the BFF relay serves the app's own pages only (CSRF)
describe("POST /bff/ops/<operation>", () => {
  it("relays a read to its Core API route", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    vi.stubEnv("CORE_API_URL", "http://core:8080");
    const fetch = vi.fn(async () => new Response(JSON.stringify({ data: { id: "u-1" } }), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    const res = await relay("units.get", { id: "00000000-0000-4000-8000-000000000001" });
    expect([res.status, await res.json()]).toEqual([200, { data: { id: "u-1" } }]);
    expect((fetch.mock.calls[0] as unknown as [string, RequestInit])[0]).toBe("http://core:8080/v1/units/00000000-0000-4000-8000-000000000001");
  });
  it("refuses another site's read or write without calling the Core API", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const evil = { origin: "https://evil.example", host: "localhost:3000", "sec-fetch-site": "cross-site" };
    for (const res of [await relay("units.get", { id: "x" }, evil), await relay("jobs.cancel", { jobId: "00000000-0000-4000-8000-000000000001", cancelReason: "x" }, evil)]) {
      expect([res.status, (await res.json()).messageKey]).toEqual([403, "errors.cross_site_request"]);
    }
    expect(fetch).not.toHaveBeenCalled();
  });
});

// IR370: the relay reads at most 1 MiB, the Core API's limit for an operation without files
describe("the relay's input limit", () => {
  const same = { origin: "http://localhost:3000", host: "localhost:3000", "sec-fetch-site": "same-origin" };
  const note = (bytes: number) => {
    const shell = JSON.stringify({ jobId: "00000000-0000-4000-8000-000000000001", message: "", visibility: "internal" });
    return JSON.stringify({ jobId: "00000000-0000-4000-8000-000000000001", message: "x".repeat(bytes - shell.length), visibility: "internal" });
  };
  const post = (body: BodyInit, headers: Record<string, string> = same) =>
    POST(new NextRequest("http://localhost:3000/bff/ops/jobs.addNote", { method: "POST", headers, body, duplex: "half" } as never), { params: Promise.resolve({ operation: "jobs.addNote" }) });

  it("passes an input of exactly 1 MiB on and refuses one byte more without calling the Core API", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    const fetch = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    expect(note(1 << 20).length).toBe(1 << 20);
    expect((await post(note(1 << 20))).status).toBe(200);
    expect(fetch).toHaveBeenCalledTimes(1);
    const res = await post(note((1 << 20) + 1));
    expect([res.status, (await res.json()).messageKey]).toEqual([422, "error.bodyTooLarge"]);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("stops reading a body without a length as soon as it passes the limit", async () => {
    vi.stubEnv("DATA_SOURCE", "api");
    vi.stubGlobal("fetch", vi.fn());
    let pulled = 0;
    const chunk = new Uint8Array(64 * 1024).fill(0x20);
    const endless = new ReadableStream<Uint8Array>({ pull(c) { pulled++; c.enqueue(chunk); } }); // a client that never stops sending
    const res = await post(endless);
    expect([res.status, (await res.json()).messageKey]).toEqual([422, "error.bodyTooLarge"]);
    expect(pulled).toBeLessThan(20); // 1 MiB is 16 chunks: reading stopped there
  });
});
