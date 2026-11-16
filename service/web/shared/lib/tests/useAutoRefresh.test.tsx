// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import { useAutoRefresh } from "@ac/web/lib/useAutoRefresh";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

beforeEach(() => { vi.useFakeTimers(); refresh.mockClear(); });
afterEach(() => { cleanup(); vi.useRealTimers(); });

// IR809: the page re-reads at the interval while it is shown and idle, never while paused or hidden
describe("useAutoRefresh", () => {
  it("refreshes every interval while enabled and visible", () => {
    renderHook(() => { useAutoRefresh(true, 30); });
    vi.advanceTimersByTime(29_000);
    expect(refresh).not.toHaveBeenCalled();
    vi.advanceTimersByTime(31_000);
    expect(refresh).toHaveBeenCalledTimes(2);
  });
  it("does nothing while paused, and stops when paused again", () => {
    const { rerender } = renderHook(({ on }: { on: boolean }) => { useAutoRefresh(on, 30); }, { initialProps: { on: false } });
    vi.advanceTimersByTime(60_000);
    expect(refresh).not.toHaveBeenCalled();
    rerender({ on: true });
    vi.advanceTimersByTime(30_000);
    rerender({ on: false });
    vi.advanceTimersByTime(60_000);
    expect(refresh).toHaveBeenCalledOnce();
  });
  it("skips a tick while the tab is hidden", () => {
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    renderHook(() => { useAutoRefresh(true, 30); });
    vi.advanceTimersByTime(30_000);
    expect(refresh).not.toHaveBeenCalled();
    visibility.mockReturnValue("visible");
    vi.advanceTimersByTime(30_000);
    expect(refresh).toHaveBeenCalledOnce();
  });
});
