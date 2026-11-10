import { describe, expect, it } from "vitest";
import { axisRange, axisTicks } from "@ac/web/lib/chartScale";

describe("the y range of a chart with several lines (IR689, IR741)", () => {
  it("keeps room around the values in their own size, not a whole unit", () => {
    const power = axisRange([0.4, 0.8, 1.2], null); // kW of an air conditioner
    expect(power.min).toBeGreaterThanOrEqual(0);
    expect(power.max).toBeLessThanOrEqual(1.5);
    expect((1.2 - 0.4) / (power.max - power.min)).toBeGreaterThan(0.6); // the line fills most of the height
  });
  it("draws no axis below zero for values that are all zero or more", () => {
    expect(axisRange([0, 1, 3], null).min).toBe(0); // PM2.5
    expect(axisRange([0, 0], null)).toStrictEqual({ min: 0, max: 1 });
  });
  it("keeps the ranges of the larger metrics as they were", () => {
    expect(axisRange([22, 24, 26], null)).toStrictEqual({ min: 20, max: 28 }); // °C
    expect(axisRange([24, 24], null)).toStrictEqual({ min: 22, max: 26 });
    expect(axisRange([], null)).toStrictEqual({ min: 0, max: 1 });
    expect(axisRange([-5, 5], null).min).toBeLessThan(-5); // a value below zero keeps its room
  });
  it("puts round ticks inside the range", () => {
    const { min, max } = axisRange([0.4, 0.8, 1.2], null);
    const ticks = axisTicks(min, max);
    expect(ticks).toStrictEqual([0.5, 1]); // 0.2–1.4: two gridlines of the 0.5 step axisTicks picks
    expect(ticks.every((t) => t >= min && t <= max)).toBe(true);
  });
});
