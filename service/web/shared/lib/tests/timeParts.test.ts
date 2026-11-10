import { describe, expect, it } from "vitest";
import { HOURS, MINUTES, joinDateTime, joinTime, minuteOptions, splitDateTime, splitTime } from "@ac/web/lib/timeParts";

// IR638: the pickers work on the strings the native time and datetime-local inputs gave.
describe("a time of day", () => {
  it("splits into hour and minute and joins back to the same string", () => {
    expect(splitTime("09:30")).toStrictEqual({ hour: "09", minute: "30" });
    expect(joinTime("09", "30")).toBe("09:30");
    const { hour, minute } = splitTime("23:45");
    expect(joinTime(hour, minute)).toBe("23:45");
  });
  it("has no hour or minute when the value is empty, unfinished or out of range", () => {
    for (const bad of ["", "9:30", "24:00", "09:60", "09:3", "09:30:00", "abc"]) expect(splitTime(bad)).toStrictEqual({ hour: "", minute: "" });
  });
  it("is no time without an hour, and starts a chosen hour at :00", () => {
    expect(joinTime("", "30")).toBe("");
    expect(joinTime("07", "")).toBe("07:00");
  });
  it("lists 24 hours and the quarter hours", () => {
    expect(HOURS).toHaveLength(24);
    expect(HOURS.at(0)).toBe("00");
    expect(HOURS.at(-1)).toBe("23");
    expect(MINUTES).toStrictEqual(["00", "15", "30", "45"]);
  });
  it("keeps a stored minute off the quarter hours as one more option, and never rounds it", () => {
    expect(minuteOptions("30")).toBe(MINUTES);
    expect(minuteOptions("")).toBe(MINUTES);
    expect(minuteOptions("10")).toStrictEqual(["00", "10", "15", "30", "45"]);
  });
});

describe("a date and time", () => {
  it("splits and joins", () => {
    expect(splitDateTime("2026-09-01T09:30")).toStrictEqual({ date: "2026-09-01", time: "09:30" });
    expect(joinDateTime("2026-09-01", "09:30")).toBe("2026-09-01T09:30");
  });
  it("has no time while only a date is given or the value is empty", () => {
    expect(splitDateTime("2026-09-01")).toStrictEqual({ date: "2026-09-01", time: "" });
    expect(splitDateTime("")).toStrictEqual({ date: "", time: "" });
  });
  it("is no value without a date; a date alone takes the default time", () => {
    expect(joinDateTime("", "09:30")).toBe("");
    expect(joinDateTime("2026-09-01", "")).toBe("2026-09-01T00:00");
    expect(joinDateTime("2026-09-01", "", "09:00")).toBe("2026-09-01T09:00");
  });
});
