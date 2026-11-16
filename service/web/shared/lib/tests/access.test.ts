import { describe, expect, it } from "vitest";
import { canOpenRestrictions } from "@ac/web/lib/access";

describe("links to the Restrictions screen (UI/UX loop R57)", () => {
  it("are shown to a caller with any restriction permission", () => {
    expect(canOpenRestrictions(new Set(["restriction.read"]))).toBe(true);
    expect(canOpenRestrictions(new Set(["restriction.override"]))).toBe(true);
    expect(canOpenRestrictions(new Set(["billing.read", "asset.read"]))).toBe(false);
  });
});
