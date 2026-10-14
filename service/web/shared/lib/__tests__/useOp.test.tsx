// @vitest-environment jsdom
// useOp, the screens' data-source switch: in mock mode the screen keeps its fixture value and nothing is called; in api
// mode it reads the operation through the BFF relay, maps the answer, keeps the last data with the error when a read
// fails, and reads again after a write invalidates the screen.
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const callOp = vi.fn();
vi.mock("@ac/web/lib/ops", async (original) => ({ ...(await original<typeof import("@ac/web/lib/ops")>()), callOp: (...a: unknown[]) => callOp(...a) }));

function session(dataSource: "mock" | "api") {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ dataSource, signedIn: true, role: "client", membershipId: "m-1", clientRole: "owner", serverNow: "2026-09-20T01:00:00Z" }))));
}

// the session is cached per page load: each test loads the module afresh
async function load() {
  vi.resetModules();
  return import("@ac/web/lib/useOp");
}

beforeEach(() => callOp.mockReset());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useOp", () => {
  it("keeps the mock value and calls nothing in mock mode", async () => {
    session("mock");
    const { useOp } = await load();
    const { result } = renderHook(() => useOp<{ n: number }, number>("units.list" as never, {} as never, 7, (d) => d.n));
    await waitFor(() => expect(result.current.source).toBe("mock"));
    expect(result.current).toMatchObject({ data: 7, loading: false, error: null });
    expect(callOp).not.toHaveBeenCalled();
  });

  it("reads and maps the operation in api mode, and reads again after invalidate", async () => {
    session("api");
    callOp.mockResolvedValueOnce({ n: 1 }).mockResolvedValueOnce({ n: 2 });
    const { useOp, invalidate } = await load();
    const { result } = renderHook(() => useOp<{ n: number }, number>("units.list" as never, { limit: 5 } as never, 0, (d) => d.n * 10));
    await waitFor(() => expect(result.current.data).toBe(10));
    expect(result.current).toMatchObject({ loading: false, error: null, source: "api" });
    expect(callOp).toHaveBeenCalledWith("units.list", { limit: 5 });
    act(() => invalidate());
    await waitFor(() => expect(result.current.data).toBe(20));
    expect(callOp).toHaveBeenCalledTimes(2);
  });

  it("keeps the last data with the error when a read fails", async () => {
    session("api");
    const { useOp, invalidate } = await load();
    const { OpError } = await import("@ac/web/lib/ops"); // the instance the freshly loaded useOp sees
    callOp.mockResolvedValueOnce({ n: 3 });
    const { result } = renderHook(() => useOp<{ n: number }, number>("units.list" as never, {} as never, 0, (d) => d.n));
    await waitFor(() => expect(result.current.data).toBe(3));
    callOp.mockRejectedValueOnce(new OpError(503, { code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, correlationId: "c-1", retryAfterSeconds: null }));
    act(() => invalidate());
    await waitFor(() => expect(result.current.error?.status).toBe(503));
    expect(result.current.data).toBe(3);
  });

  it("does not read while disabled", async () => {
    session("api");
    const { useOp } = await load();
    const { result } = renderHook(() => useOp<{ n: number }, number>("units.list" as never, {} as never, 5, (d) => d.n, false));
    await waitFor(() => expect(result.current.source).toBe("api"));
    expect(result.current).toMatchObject({ data: 5, loading: false });
    expect(callOp).not.toHaveBeenCalled();
  });
});
