// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { servedVersion, UpdateNotice, VERSION_POLL_MS } from "@ac/web/components/UpdateNotice";
import { GET } from "@ac/web/bff/version/route";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const answer = (version: unknown) => vi.fn(() => Promise.resolve(Response.json({ version })));
const tick = async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); };
const MESSAGE = "A new version of the app is available. Reload the page to use it; save what you are entering first.";

// a page compares the build it was loaded with ("dev" in the tests) with the one the server answers (IR856)
describe("the reload offer after a deployment", () => {
  it("appears once the server runs another build, and asks no more", async () => {
    vi.useFakeTimers();
    const fetch = answer("test1.0.0r9");
    vi.stubGlobal("fetch", fetch);
    render(<UpdateNotice />);
    expect(fetch).not.toHaveBeenCalled(); // a page just loaded is the build the server runs
    await tick(VERSION_POLL_MS);
    expect(screen.getByRole("status").textContent).toContain(MESSAGE);
    expect(screen.getByRole("button", { name: "Reload page" })).toBeTruthy();
    await tick(VERSION_POLL_MS * 3);
    expect(fetch).toHaveBeenCalledExactlyOnceWith("/bff/version", { cache: "no-store" });
  });

  it("stays away while the server runs the page's build, or cannot answer", async () => {
    vi.useFakeTimers();
    for (const fetch of [answer("dev"), answer(42), vi.fn(() => Promise.resolve(new Response("", { status: 502 }))), vi.fn(() => Promise.reject(new Error("offline")))]) {
      vi.stubGlobal("fetch", fetch);
      render(<UpdateNotice />);
      await tick(VERSION_POLL_MS);
      expect(fetch).toHaveBeenCalledOnce();
      expect(screen.queryByRole("status")).toBeNull();
      cleanup();
    }
  });

  it("asks when the page comes back into view, and not while it is hidden", async () => {
    vi.useFakeTimers();
    const fetch = answer("test1.0.0r9");
    vi.stubGlobal("fetch", fetch);
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    render(<UpdateNotice />);
    await tick(VERSION_POLL_MS);
    expect(fetch).not.toHaveBeenCalled();
    visibility.mockReturnValue("visible");
    await act(async () => { dispatchEvent(new Event("focus")); await vi.advanceTimersByTimeAsync(0); });
    expect(fetch).toHaveBeenCalledOnce();
    expect(screen.getByRole("status").textContent).toContain(MESSAGE);
    visibility.mockRestore();
  });
});

describe("GET /bff/version", () => {
  it("answers the server's build, never stored", async () => {
    const res = GET();
    expect(res.headers.get("Cache-Control")).toBe("no-store");
    expect(await res.json()).toStrictEqual({ version: "dev" });
    expect(await servedVersion().catch(() => "threw")).toBeNull(); // no fetch in this environment: unknown, not a new build
  });
});
