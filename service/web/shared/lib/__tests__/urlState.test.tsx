// @vitest-environment jsdom
// URL query state and stored values as external stores (D10 URL state): a write replaces the URL without navigating and
// re-renders every reader; a stored value is read back, an unchanged write notifies nobody, and blocked storage reads
// as null instead of throwing.
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setSearchParam, setStoredValue, useSearchParam, useStoredValue } from "@ac/web/lib/urlState";

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("URL query state", () => {
  it("sets and removes a parameter without navigation and re-renders the readers", () => {
    window.history.replaceState(null, "", "/customer/units?tab=list");
    const { result } = renderHook(() => useSearchParam("unitId"));
    expect(result.current).toBeNull();
    const length = window.history.length;
    act(() => setSearchParam("unitId", "u-1"));
    expect(result.current).toBe("u-1");
    expect(window.location.search).toBe("?tab=list&unitId=u-1");
    expect(window.history.length).toBe(length); // replaced, not pushed
    act(() => setSearchParam("unitId", null));
    expect(result.current).toBeNull();
    expect(window.location.search).toBe("?tab=list");
  });

  it("follows the back and forward buttons (popstate)", () => {
    const { result } = renderHook(() => useSearchParam("tab"));
    window.history.replaceState(null, "", "/?tab=history");
    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(result.current).toBe("history");
  });
});

describe("stored values", () => {
  it("reads a written value back and notifies only on a change", () => {
    const { result } = renderHook(() => useStoredValue("ac:lang"));
    expect(result.current).toBeNull();
    act(() => setStoredValue("ac:lang", "ms"));
    expect(result.current).toBe("ms");
    const events = vi.fn();
    window.addEventListener("ac:storagechange", events);
    act(() => setStoredValue("ac:lang", "ms"));
    window.removeEventListener("ac:storagechange", events);
    expect(events).not.toHaveBeenCalled();
  });

  it("reads null and writes nothing when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useStoredValue("ac:lang"));
    expect(result.current).toBeNull();
    expect(() => setStoredValue("ac:lang", "en")).not.toThrow();
  });
});
