import { describe, expect, it } from "vitest";

import { commonRange } from "@ac/web/lib/temperatureRange";

describe("the setpoint range of the chosen units", () => {
  it("is null when no unit states a range", () => {
    expect(commonRange([])).toBeNull();
    expect(commonRange([{}, { temperature: null }])).toBeNull();
  });

  it("is the highest minimum to the lowest maximum, and a unit without a range does not narrow it", () => {
    expect(commonRange([{ temperature: { min: 16, max: 30 } }])).toStrictEqual({ min: 16, max: 30 });
    expect(commonRange([{ temperature: { min: 16, max: 30 } }, { temperature: null }, { temperature: { min: 18, max: 28 } }, { temperature: { min: 17, max: 32 } }])).toStrictEqual({ min: 18, max: 28 });
  });
});
