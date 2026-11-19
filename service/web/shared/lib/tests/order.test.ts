// The comparators for toSorted: generated files sort by character codes whatever the locale, and ISO times compare as
// text.
import { describe, expect, it } from "vitest";
import { byCodeUnit, byNumber, isAfter, isBefore } from "@ac/web/lib/order";

describe("comparators (order.ts)", () => {
  it("orders by character codes as a sort without a comparator does, a prefix first", () => {
    const words = ["b", "abc", "B", "ab", "a", "", "_"];
    expect(words.toSorted(byCodeUnit)).toStrictEqual(["", "B", "_", "a", "ab", "abc", "b"]);
    expect([byCodeUnit("ab", "abc") < 0, byCodeUnit("abc", "ab") > 0, byCodeUnit("ab", "ab")]).toStrictEqual([true, true, 0]);
  });

  it("orders numbers by value, not as text", () => {
    expect([10, 9, 100, -1].toSorted(byNumber)).toStrictEqual([-1, 9, 10, 100]);
  });

  it("compares ISO times as text, equal times being neither before nor after", () => {
    const early = "2026-09-08T09:00:00Z";
    const late = "2026-09-08T10:00:00Z";
    expect([isBefore(early, late), isAfter(late, early), isBefore(late, early), isAfter(early, late)]).toStrictEqual([true, true, false, false]);
    expect([isBefore(early, early), isAfter(early, early)]).toStrictEqual([false, false]);
    expect(isBefore("2026-09-08", "2026-09-08T00:00:00Z")).toBe(true);
  });
});
