// @vitest-environment jsdom
// URL query state and stored values as external stores (D10 URL state): a write replaces the URL without navigating and
// re-renders every reader; a stored value is read back, an unchanged write notifies nobody, and blocked storage reads
// as null instead of throwing.
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setSearchParameter, setStoredValue, useSearchParameter, useStoredValue } from "@ac/web/lib/urlState";

afterEach(() => {
  cleanup();
  history.replaceState(null, "", "/");
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("URL query state", () => {
  it("sets and removes a parameter without navigation and re-renders the readers", () => {
    history.replaceState(null, "", "/customer/units?tab=list");
    const { result } = renderHook(() => useSearchParameter("unitId"));
    expect(result.current).toBeNull();
    const length = history.length;
    act(() => { setSearchParameter("unitId", "u-1"); });
    expect(result.current).toBe("u-1");
    expect(location.search).toBe("?tab=list&unitId=u-1");
    expect(history.length).toBe(length); // replaced, not pushed
    act(() => { setSearchParameter("unitId", null); });
    expect(result.current).toBeNull();
    expect(location.search).toBe("?tab=list");
  });

  it("follows the back and forward buttons (popstate)", () => {
    const { result } = renderHook(() => useSearchParameter("tab"));
    history.replaceState(null, "", "/?tab=history");
    act(() => {
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(result.current).toBe("history");
  });
});

describe("stored values", () => {
  it("reads a written value back and notifies only on a change", () => {
    const { result } = renderHook(() => useStoredValue("ac:lang"));
    expect(result.current).toBeNull();
    act(() => { setStoredValue("ac:lang", "ms"); });
    expect(result.current).toBe("ms");
    const events = vi.fn();
    globalThis.addEventListener("ac:storagechange", events);
    act(() => { setStoredValue("ac:lang", "ms"); });
    globalThis.removeEventListener("ac:storagechange", events);
    expect(events).not.toHaveBeenCalled();
  });

  it("reads null and writes nothing when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useStoredValue("ac:lang"));
    expect(result.current).toBeNull();
    expect(() => { setStoredValue("ac:lang", "en"); }).not.toThrow();
  });
});
