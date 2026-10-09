// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { readDemo, writeDemo } from "@ac/web/lib/demoStore";

describe("Phase 1A demo stores (FR-X05, IR257)", () => {
  afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); });
  const navigation = (type: string) => vi.spyOn(performance, "getEntriesByType").mockReturnValue([{ type } as unknown as PerformanceEntry]);

  it("keeps the demo data in the tab across navigations", () => {
    navigation("navigate");
    writeDemo("k", [{ id: "j1" }]);
    expect(readDemo("k")).toEqual([{ id: "j1" }]);
    expect(localStorage.getItem("k")).toBeNull(); // per tab, not per browser
  });

  it("brings back the seed after a reload", () => {
    writeDemo("k", [{ id: "j1" }]);
    navigation("reload");
    expect(readDemo("k")).toBeNull();
    expect(sessionStorage.getItem("k")).toBeNull();
  });

  it("falls back to the seed when storage is blocked or the value is unreadable", () => {
    navigation("navigate");
    sessionStorage.setItem("k", "{not json");
    expect(readDemo("k")).toBeNull();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
    expect(() => writeDemo("k", 1)).not.toThrow();
  });
});
