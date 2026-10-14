// @vitest-environment jsdom
// The URL hooks of the screens (D10 URL state): useUrlTab reads a catalogued tab value back to the page's tab id (an
// unknown or missing value falls back) and writes the catalogued value; useUrlPatch replaces search params in place,
// removes a key given null or "", and drops the "?" when nothing is left.
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const replace = vi.fn();
const router = { replace }; // Next.js keeps one router object across renders
vi.mock("next/navigation", () => ({ useRouter: () => router, usePathname: () => "/admin/billing" }));

import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { useUrlTab } from "@ac/web/lib/useUrlTab";

afterEach(() => {
  cleanup();
  replace.mockReset();
  window.history.replaceState(null, "", "/");
});

describe("useUrlTab", () => {
  const map = { list: "units", history: "history" } as const;

  it("reads the catalogued value, falls back for an unknown or missing one, and writes the catalogued value", () => {
    window.history.replaceState(null, "", "/?tab=history");
    const { result } = renderHook(() => useUrlTab(map, "list"));
    expect(result.current[0]).toBe("history");
    act(() => result.current[1]("list"));
    expect(window.location.search).toBe("?tab=units");
    expect(result.current[0]).toBe("list");
    act(() => window.history.replaceState(null, "", "/?tab=nonsense"));
    const { result: other } = renderHook(() => useUrlTab(map, "list"));
    expect(other.current[0]).toBe("list");
  });
});

describe("useUrlPatch", () => {
  it("sets and removes params in place and drops an empty query", () => {
    window.history.replaceState(null, "", "/admin/billing?from=2026-08&tab=invoices");
    const { result } = renderHook(() => useUrlPatch());
    result.current({ to: "2026-09", from: null });
    expect(replace).toHaveBeenLastCalledWith("/admin/billing?tab=invoices&to=2026-09", { scroll: false });
    result.current({ from: "", tab: null });
    expect(replace).toHaveBeenLastCalledWith("/admin/billing", { scroll: false });
  });

  it("keeps the same function across renders", () => {
    const { result, rerender } = renderHook(() => useUrlPatch());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});
