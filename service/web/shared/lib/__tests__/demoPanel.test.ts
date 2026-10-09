import { describe, expect, it } from "vitest";
import { advanceTarget, clockText, demoRefusal, deviceOptions, nextSequence, openFault, type ApiDeviceEvent } from "@ac/web/lib/demoPanel";

const ev = (over: Partial<ApiDeviceEvent>): ApiDeviceEvent => ({ id: "e", eventType: "communication_lost", sequence: 1, occurredAt: "2026-09-14T01:00:00Z", restoredAt: null, ...over });

describe("Demo controls in API mode (FR-X05, IR249)", () => {
  it("shows the scenario clock in UTC and moves it forward", () => {
    expect(clockText("2026-09-14T01:41:07.123Z")).toBe("2026-09-14 01:41 UTC");
    expect([advanceTarget(new Date("2026-09-14T01:41:00Z"), 1), advanceTarget(new Date("2026-09-14T23:30:00Z"), 60)]).toEqual(["2026-09-14T01:42:00.000Z", "2026-09-15T00:30:00.000Z"]);
  });

  it("finds the open connection fault and the next sequence", () => {
    const events = [ev({ id: "old", sequence: 3, restoredAt: "2026-09-14T02:00:00Z" }), ev({ id: "open", sequence: 7 }), ev({ id: "tamper", eventType: "tamper", sequence: 9 }), ev({ id: "back", eventType: "restored", sequence: 4 })];
    expect(openFault(events)?.id).toBe("open");
    expect([openFault([ev({ restoredAt: "2026-09-14T02:00:00Z" })]), openFault([])]).toEqual([null, null]);
    expect([nextSequence(events), nextSequence([])]).toEqual([10, 1]);
  });

  it("lists bound devices with their unit and open fault", () => {
    const devices = [
      { id: "d1", serial: "SN-1", unitId: "u1", bindingId: "b1", connection: "online" },
      { id: "d2", serial: "SN-2", unitId: "u2", bindingId: "b2", connection: "offline" },
      { id: "d3", serial: "SN-3", unitId: null, bindingId: null, connection: "unknown" }, // unbound: not offered
    ];
    expect(deviceOptions(devices, new Map([["u1", "Bedroom AC"]]), new Map([["d2", "f2"]]))).toEqual([
      { id: "d1", unit: "Bedroom AC", label: "Bedroom AC · SN-1 · online", connection: "online", fault: null },
      { id: "d2", unit: "Unit", label: "Unit · SN-2 · offline", connection: "offline", fault: "f2" },
    ]);
  });

  it("explains refused demo operations", () => {
    const f = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}) => ({ code, messageKey, fieldErrors });
    expect([
      demoRefusal(f("UNAVAILABLE", "errors.demo_only")), demoRefusal(f("VALIDATION", "error.validation", { to: "errors.clock_backwards" })),
      demoRefusal(f("VALIDATION", "error.validation", { bindingId: "errors.binding_mismatch" })), demoRefusal(f("CONFLICT", "errors.recovery_source_not_current")),
      demoRefusal(f("NOT_FOUND", "error.notFound")), demoRefusal(f("UNAVAILABLE", "error.unavailable")),
    ]).toEqual(["Demo operations are off in this environment", "The clock only moves forward — reload and try again", "The device was rebound — reload to pick it again",
      "The fault was already restored or replaced — reload", "This device is not visible to you any more", "Not done — try again"]);
  });
});
