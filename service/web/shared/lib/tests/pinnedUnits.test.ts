import { describe, expect, it } from "vitest";
import { MAX_PINS, parsePins, togglePin } from "@ac/web/lib/pinnedUnits";

// IR814: the pins are read back only as a valid list, a new pin goes first and the list stays short
describe("pinned ACs", () => {
  it("reads only a valid stored list", () => {
    expect([parsePins(null), parsePins("not json"), parsePins('{"a":1}'), parsePins('["u1", 2]'), parsePins('["u1","u2"]')]).toStrictEqual([[], [], [], [], ["u1", "u2"]]);
    expect(parsePins(JSON.stringify(Array.from({ length: MAX_PINS + 1 }, (_, i) => `u${String(i)}`)))).toStrictEqual([]);
  });
  it("pins first, unpins, and keeps at most MAX_PINS", () => {
    expect([togglePin(["a"], "b"), togglePin(["a", "b"], "a")]).toStrictEqual([["b", "a"], ["b"]]);
    const full = Array.from({ length: MAX_PINS }, (_, i) => `u${String(i)}`);
    expect(togglePin(full, "new")).toStrictEqual(["new", ...full.slice(0, MAX_PINS - 1)]);
  });
});
