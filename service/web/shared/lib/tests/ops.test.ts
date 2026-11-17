import { afterEach, describe, expect, it, vi } from "vitest";
import { callRelay, OpError } from "@ac/web/lib/ops";

const json = (status: number, body: unknown) => Response.json(body, { status });

/** Stubs fetch with one answer and returns the stub, to read the request it got. */
function answer(res: Response) {
  const stub = vi.fn<(url: string, init: RequestInit) => Promise<Response>>(() => Promise.resolve(res));
  vi.stubGlobal("fetch", stub);
  return stub;
}

function request(stub: ReturnType<typeof answer>) {
  const [url, init] = stub.mock.calls[0] ?? [];
  return { url, headers: new Headers(init?.headers), method: init?.method, body: init?.body };
}

/** The error a call throws (null when it resolves). */
const thrown = (call: Promise<unknown>): Promise<unknown> => call.then(() => null).catch((error: unknown) => error);

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("callRelay (the browser's BFF relay)", () => {
  it("posts a read to /bff/ops/<operation> without write headers and returns the envelope's data", async () => {
    const stub = answer(json(200, { data: { items: [1] }, meta: { snapshotAt: "2026-11-17T00:00:00Z" } }));
    await expect(callRelay("units.list", { q: "a" })).resolves.toStrictEqual({ items: [1] });
    const r = request(stub);
    expect([r.url, r.method, r.body, r.headers.get("Content-Type")]).toStrictEqual(["/bff/ops/units.list", "POST", '{"q":"a"}', "application/json"]);
    expect([r.headers.has("Idempotency-Key"), r.headers.has("X-Expected-Version")]).toStrictEqual([false, false]);
  });

  it("sends an Idempotency-Key on a write: a new one each call, or the caller's key kept", async () => {
    let stub = answer(json(200, { data: null }));
    await callRelay("units.save", {}, { write: true });
    const generated = request(stub).headers.get("Idempotency-Key");
    expect(generated).toMatch(/\S{8,}/);
    stub = answer(json(200, { data: null }));
    await callRelay("units.save", {}, { write: true });
    expect(request(stub).headers.get("Idempotency-Key")).not.toBe(generated);
    stub = answer(json(200, { data: null }));
    await callRelay("units.save", {}, { write: true, idempotencyKey: "key-1", expectedVersion: 0 });
    const r = request(stub);
    expect([r.headers.get("Idempotency-Key"), r.headers.get("X-Expected-Version")]).toStrictEqual(["key-1", "0"]);
  });

  it("throws the refusal's DomainError with the HTTP status", async () => {
    answer(json(409, { code: "VERSION_CONFLICT", messageKey: "error.version", fieldErrors: { name: "taken" } }));
    const error = await thrown(callRelay("units.save", {}, { write: true }));
    expect(error).toBeInstanceOf(OpError);
    expect(error instanceof OpError && [error.status, error.error.code, error.error.fieldErrors]).toStrictEqual([409, "VERSION_CONFLICT", { name: "taken" }]);
  });

  it("reads a refusal without a DomainError body, or a success without an envelope, as unavailable", async () => {
    answer(new Response("<html>bad gateway</html>", { status: 502 }));
    const refused = await thrown(callRelay("units.list", {}));
    expect(refused instanceof OpError && [refused.status, refused.error.code]).toStrictEqual([502, "UNAVAILABLE"]);
    answer(json(200, { items: [] }));
    const bare = await thrown(callRelay("units.list", {}));
    expect(bare instanceof OpError && [bare.status, bare.error.code]).toStrictEqual([200, "UNAVAILABLE"]);
  });
});
