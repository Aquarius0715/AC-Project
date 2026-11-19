import { describe, expect, it } from "vitest";
import { MAX_PINS, togglePin } from "@ac/web/lib/pinnedUnits";

// IR814 / IR819: a new pin goes first, unpinning removes it, and the list stays at MAX_PINS
describe("pinned ACs", () => {
  it("pins first, unpins, and keeps at most MAX_PINS", () => {
    expect([togglePin(["a"], "b"), togglePin(["a", "b"], "a")]).toStrictEqual([["b", "a"], ["b"]]);
    const full = Array.from({ length: MAX_PINS }, (_, i) => `u${String(i)}`);
    expect(togglePin(full, "new")).toStrictEqual(["new", ...full.slice(0, MAX_PINS - 1)]);
  });
});
