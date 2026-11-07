import { describe, expect, it } from "vitest";

import { isNone } from "@ac/web/lib/isNone";

describe("isNone", () => {
  it("is true for null and undefined only", () => {
    expect(isNone(null)).toBe(true);
    expect(isNone(undefined)).toBe(true);
    expect([0, "", false, NaN, [], {}].map((value) => isNone(value))).toEqual([false, false, false, false, false, false]);
  });
});
