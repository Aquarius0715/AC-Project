import { describe, expect, it } from "vitest";
import { advanceTarget, CLOCK_JUMPS, clockAt, clockText, demoRefusal, deviceOptions, jumpDone, jumpDoneDemo, jumpLabel, nextSequence, openFault, type ApiDeviceEvent } from "@ac/web/lib/demoPanel";
import { translator } from "@ac/web/lib/i18n";

const event = (over: Partial<ApiDeviceEvent>): ApiDeviceEvent => ({ id: "e", eventType: "communication_lost", sequence: 1, occurredAt: "2026-09-14T01:00:00Z", restoredAt: null, ...over });

const f = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}) => ({ code, messageKey, fieldErrors });

/** The button, toast and demo toast texts of some jumps in a language. */
const words = (locale: "en" | "ms" | "ja" | "id") => {
  const t = translator(locale);
  return [jumpLabel(1, t), jumpLabel(60, t), jumpLabel(120, t), jumpLabel(1440, t), jumpDone(1, t), jumpDone(60, t), jumpDone(480, t), jumpDoneDemo(60, t), jumpDoneDemo(720, t)];
};

describe("Demo controls in API mode (FR-X05, IR249)", () => {
  it("shows the scenario clock in UTC and moves it forward", () => {
    expect(clockText("2026-09-14T01:41:07.123Z")).toBe("2026-09-14 01:41 UTC");
    expect([advanceTarget(new Date("2026-09-14T01:41:00Z"), 1), advanceTarget(new Date("2026-09-14T23:30:00Z"), 60)]).toStrictEqual(["2026-09-14T01:42:00.000Z", "2026-09-15T00:30:00.000Z"]);
  });

  it("lets the shown clock run at real-time speed from the rendered time", () => {
    expect([clockAt("2026-09-14T01:41:07Z", 0), clockAt("2026-09-14T01:41:07Z", 61_000), clockAt("2026-09-14T23:59:30Z", 60_000)]).toStrictEqual(["2026-09-14 01:41 UTC", "2026-09-14 01:42 UTC", "2026-09-15 00:00 UTC"]);
  });

  // IR641: a minute, then 1, 2, 4, 8, 12 and 24 hours
  it("offers jumps of a minute and of 1, 2, 4, 8, 12 and 24 hours, worded in each language", () => {
    expect(CLOCK_JUMPS).toStrictEqual([1, 60, 120, 240, 480, 720, 1440]);
    expect(advanceTarget(new Date("2026-09-14T01:41:00Z"), 1440)).toBe("2026-09-15T01:41:00.000Z");
    expect(words("en")).toStrictEqual(["+1 min", "+1 hour", "+2 hours", "+24 hours", "Clock advanced 1 minute", "Clock advanced 1 hour", "Clock advanced 8 hours", "Clock advanced +1 hour (demo)", "Clock advanced +12 hours (demo)"]);
    expect(words("ms")).toStrictEqual(["+1 min", "+1 jam", "+2 jam", "+24 jam", "Jam dimajukan 1 minit", "Jam dimajukan 1 jam", "Jam dimajukan 8 jam", "Jam dimajukan +1 jam (demo)", "Jam dimajukan +12 jam (demo)"]);
    expect(words("ja")).toStrictEqual(["+1 分", "+1 時間", "+2 時間", "+24 時間", "時計を1分進めました", "時計を1時間進めました", "時計を 8 時間進めました", "時計を +1 時間進めました（デモ）", "時計を +12 時間進めました（デモ）"]);
    expect(words("id")).toStrictEqual(["+1 mnt", "+1 jam", "+2 jam", "+24 jam", "Jam dimajukan 1 menit", "Jam dimajukan 1 jam", "Jam dimajukan 8 jam", "Jam dimajukan +1 jam (demo)", "Jam dimajukan +12 jam (demo)"]);
  });

  it("finds the open connection fault and the next sequence", () => {
    const events = [event({ id: "old", sequence: 3, restoredAt: "2026-09-14T02:00:00Z" }), event({ id: "open", sequence: 7 }), event({ id: "tamper", eventType: "tamper", sequence: 9 }), event({ id: "back", eventType: "restored", sequence: 4 })];
    expect(openFault(events)?.id).toBe("open");
    expect([openFault([event({ restoredAt: "2026-09-14T02:00:00Z" })]), openFault([])]).toStrictEqual([null, null]);
    expect([nextSequence(events), nextSequence([])]).toStrictEqual([10, 1]);
  });

  it("lists bound devices with their unit and open fault", () => {
    const devices = [
      { id: "d1", serial: "SN-1", unitId: "u1", bindingId: "b1", connection: "online" },
      { id: "d2", serial: "SN-2", unitId: "u2", bindingId: "b2", connection: "offline" },
      { id: "d3", serial: "SN-3", unitId: null, bindingId: null, connection: "unknown" }, // unbound: not offered
    ];
    expect(deviceOptions(devices, new Map([["u1", "Bedroom AC"]]), new Map([["d2", "f2"]]))).toStrictEqual([
      { id: "d1", unit: "Bedroom AC", label: "Bedroom AC · SN-1 · online", connection: "online", fault: null },
      { id: "d2", unit: "Unit", label: "Unit · SN-2 · offline", connection: "offline", fault: "f2" },
    ]);
  });

  it("explains refused demo operations", () => {
    expect([
      demoRefusal(f("UNAVAILABLE", "errors.demo_only")), demoRefusal(f("VALIDATION", "error.validation", { to: "errors.clock_backwards" })),
      demoRefusal(f("VALIDATION", "error.validation", { bindingId: "errors.binding_mismatch" })), demoRefusal(f("CONFLICT", "errors.recovery_source_not_current")),
      demoRefusal(f("NOT_FOUND", "error.notFound")), demoRefusal(f("UNAVAILABLE", "error.unavailable")),
    ]).toStrictEqual(["Demo operations are off in this environment", "The clock only moves forward — reload and try again", "The device was rebound — reload to pick it again",
      "The fault was already restored or replaced — reload", "This device is not visible to you any more", "Try again"]);
    // the view words a refusal "Not done: {reason}", so no reason repeats "Not done" (IR743)
    expect(demoRefusal(f("UNAVAILABLE", "error.unavailable")).startsWith("Not done")).toBe(false);
  });
});
