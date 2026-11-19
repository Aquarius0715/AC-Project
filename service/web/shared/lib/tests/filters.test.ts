import { describe, expect, it } from "vitest";
import { anySet } from "@ac/web/lib/filters";

describe("an empty list's filter check (UI/UX loop R62)", () => {
  it("is set when any value is", () => {
    expect(anySet(undefined, "", false, null)).toBe(false);
    expect(anySet(undefined, "c1")).toBe(true);
    expect(anySet(true)).toBe(true);
  });
});
