// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { z } from "zod";
import { readDemoAs, writeDemo } from "@ac/web/lib/demoStore";

const idRows = z.array(z.object({ id: z.string() }));

const navigation = (type: string) => vi.spyOn(performance, "getEntriesByType").mockReturnValue([Object.assign(performance.mark("navigation"), { type })]);

describe("Phase 1A demo stores (FR-X05, IR257)", () => {
  afterEach(() => { vi.restoreAllMocks(); sessionStorage.clear(); });

  it("keeps the demo data in the tab across navigations", () => {
    navigation("navigate");
    writeDemo("k", [{ id: "j1" }]);
    expect(readDemoAs("k", idRows)).toStrictEqual([{ id: "j1" }]);
    expect(localStorage.getItem("k")).toBeNull(); // per tab, not per browser
  });

  it("brings back the seed after a reload", () => {
    writeDemo("k", [{ id: "j1" }]);
    navigation("reload");
    expect(readDemoAs("k", z.unknown())).toBeNull();
    expect(sessionStorage.getItem("k")).toBeNull();
  });

  it("falls back to the seed when storage is blocked or the value is unreadable", () => {
    navigation("navigate");
    sessionStorage.setItem("k", "{not json");
    expect(readDemoAs("k", z.unknown())).toBeNull();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
    expect(() => { writeDemo("k", 1); }).not.toThrow();
  });

  it("keeps a saved value only when it has the expected shape", () => {
    navigation("navigate");
    const rows = z.array(z.object({ id: z.string(), n: z.number() }));
    writeDemo("k", [{ id: "j1", n: 1 }]);
    expect(readDemoAs("k", rows)).toStrictEqual([{ id: "j1", n: 1 }]);
    writeDemo("k", [{ id: "j1", n: "1" }]); // an older release stored the number as text
    expect(readDemoAs("k", rows)).toBeNull();
    expect(readDemoAs("none", rows)).toBeNull();
  });
});
