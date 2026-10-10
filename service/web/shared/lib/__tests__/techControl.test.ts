import { describe, expect, it } from "vitest";
import {
  actionCode, actionWord, blockedText, capabilityText, connectionText, fieldText, historyRows, observedText, refusal, restrictionText, runActive, runBanner, stateTiles, windowText,
  type ApiCommandRow, type ApiRun,
} from "@ac/web/lib/techControl";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const cmd = (over: Partial<ApiCommandRow> = {}): ApiCommandRow => ({ id: "cmd-00000001", action: { kind: "set_mode", mode: "cool" }, status: "acknowledged", requestedAt: "2026-09-14T01:00:00Z", acknowledgedAt: "2026-09-14T01:00:05Z", failureCode: null, reason: "test", diagnosticRunId: null, source: "diagnostic", ...over });
const run = (over: Partial<ApiRun> = {}): ApiRun => ({
  id: "run-1", version: 1, jobId: "job", unitId: "unit", startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false }, durationMinutes: 5, reason: "test",
  state: "running", startCommandId: "cmd-start", endCommandId: null, startedAt: "2026-09-14T01:05:00Z", endAt: "2026-09-14T01:10:00Z", failureCode: null, createdAt: "2026-09-14T01:04:00Z", ...over,
});

describe("action text", () => {
  it("codes every unit action", () => {
    expect(actionCode({ kind: "set_power", power: true })).toBe("set_power = ON");
    expect(actionCode({ kind: "set_temperature", celsius: 24 })).toBe("set_temperature = 24 °C");
    expect(actionCode({ kind: "set_mode", mode: "cool" })).toBe("set_mode = cool");
    expect(actionCode({ kind: "set_fan", fanLevel: "high" })).toBe("set_fan = high");
    expect(actionCode({ kind: "ventilate", level: "boost" })).toBe("ventilate = boost");
    expect(actionCode({ kind: "unknown_kind" })).toBe("unknown_kind");
    expect(actionWord({ kind: "set_power", power: false })).toBe("Power OFF");
    expect(actionWord({ kind: "set_temperature", celsius: 22 })).toBe("set_temperature = 22 °C");
  });
});

describe("historyRows", () => {
  it("describes plain commands by their device outcome", () => {
    const rows = historyRows([
      cmd(), cmd({ id: "cmd-00000002", status: "failed", failureCode: "DEVICE_REJECTED", acknowledgedAt: null }), cmd({ id: "cmd-00000003", status: "expired", acknowledgedAt: null }),
      cmd({ id: "cmd-00000004", status: "requested", acknowledgedAt: null }), cmd({ id: "cmd-00000005", status: "cancelled", acknowledgedAt: null }),
    ], []);
    expect(rows[0]).toEqual({ id: "cmd-00000001", at: "9:00 am MYT", title: "set_mode = cool", sub: "cmd-0000 · acknowledged 9:00 am MYT", badge: { text: "Acked", tone: "ok" }, run: false });
    expect(rows[1]).toMatchObject({ sub: "cmd-0000 · rejected by the device (DEVICE_REJECTED)", badge: { text: "Rejected", tone: "crit" } });
    expect(rows[2]).toMatchObject({ sub: "cmd-0000 · no device response (expired)", badge: { text: "No response", tone: "crit" } });
    expect(rows[3]).toMatchObject({ sub: "cmd-0000 · waiting for the device", badge: { text: "Waiting", tone: "warn" } });
    expect(rows[4]).toMatchObject({ sub: "cmd-0000 · cancelled", badge: { text: "Cancelled", tone: "muted" } });
  });
  it("names the test run of its start and end commands", () => {
    const r = run({ endCommandId: "cmd-end" });
    const rows = historyRows([cmd({ id: "cmd-start", requestedAt: "2026-09-14T01:04:00Z", acknowledgedAt: "2026-09-14T01:05:00Z" }), cmd({ id: "cmd-end", status: "requested", acknowledgedAt: null })], [r]);
    expect(rows[0]).toMatchObject({ title: "test run start (Power ON, 5 min)", sub: "cmd-star · acknowledged 9:05 am MYT · ends 9:10 am MYT", badge: { text: "Running", tone: "primary" }, run: true });
    expect(rows[1]).toMatchObject({ title: "test run end (Power OFF)", badge: { text: "Waiting", tone: "warn" }, run: true });
    expect(historyRows([cmd({ id: "cmd-start" })], [run({ state: "completed" })])[0].badge.text).toBe("Acked");
  });
});

describe("run banner and state", () => {
  it("follows the DiagnosticRun state", () => {
    expect(runBanner(null)).toBeNull();
    expect(runBanner(run({ state: "awaiting_start" }))).toMatchObject({ tone: "primary", title: "Starting — waiting for the device" });
    expect(runBanner(run())).toMatchObject({ tone: "warn", title: "Running — ends 9:10 am MYT (start acknowledged 9:05 am MYT)", text: "DiagnosticRun running · endAt = startedAt + 5 min. The end command (Power OFF) is sent at 9:10 am MYT." });
    expect(runBanner(run({ state: "end_requested" }))).toMatchObject({ tone: "warn", title: "End requested 9:10 am MYT — awaiting device response" });
    expect(runBanner(run({ state: "completed" }))).toMatchObject({ tone: "ok", title: "End acknowledged — test run completed", text: "The device confirmed Power OFF." });
    expect(runBanner(run({ state: "start_failed", failureCode: "TIMEOUT" }))).toMatchObject({ tone: "crit", text: "start_failed (TIMEOUT) · no end command is needed." });
    expect(runBanner(run({ state: "end_failed" }))?.text).toBe("end_failed · the unit may still be running; check it on site.");
    expect(runBanner(run({ state: "end_blocked" }))?.title).toMatch(/^End blocked/);
    expect([null, run({ state: "completed" }), run({ state: "end_failed" })].map(runActive)).toEqual([false, false, false]);
    expect(["awaiting_start", "running", "end_requested"].map((s) => runActive(run({ state: s as ApiRun["state"] })))).toEqual([true, true, true]);
  });
});

describe("state tiles", () => {
  it("shows the observed state and the room temperature with its quality", () => {
    const tiles = stateTiles({ observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "auto", observedAt: "2026-09-14T01:00:00Z" }, latestMeasurements: [{ metric: "temperature", value: 31.26, quality: "valid" }] });
    expect(tiles.map((t) => [t.label, t.value])).toEqual([["Power", "On"], ["Mode", "Cool"], ["Setpoint", "24 °C"], ["Fan", "Auto"], ["Room", "31.3 °C"]]);
    expect(tiles[4].warn).toBe(true);
    const unknown = stateTiles({ observedState: { power: null, celsius: null, mode: null, fanLevel: null, observedAt: null }, latestMeasurements: [{ metric: "temperature", value: 25, quality: "stale" }] });
    expect(unknown.map((t) => t.value)).toEqual(["Unknown", "—", "—", "—", "25.0 °C (stale)"]);
    expect(unknown[4].warn).toBe(false);
    expect(stateTiles({ observedState: { power: false, celsius: 22, mode: "dry", fanLevel: "low", observedAt: null }, latestMeasurements: [] })[4].value).toBe("—");
  });
  it("prints the observation time and the work window", () => {
    expect(observedText("2026-09-14T01:00:05Z")).toBe("observed 9:00 am MYT");
    expect(observedText(null)).toBe("no observation yet");
    expect(windowText("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z")).toBe("14 Sept, 8:00 am – 20 Sept, 8:00 am MYT");
  });
});

describe("capability, restriction and refusals (IR286)", () => {
  const unit = {
    capabilityVersion: 3, connection: "online",
    capabilities: { modeControl: true, modes: ["cool", "dry", "fan"], temperature: { min: 16, max: 30, step: 1 }, fanControl: true, fanLevels: ["low", "mid", "high"] },
    effectiveControlPolicy: { state: "restricted", phase: "active", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 24 } },
    controlAvailability: { state: "blocked", reasonKey: "control.reconciliation_required" },
  } as unknown as ApiUnitDetail;
  it("words what the unit allows and why control waits", () => {
    expect(capabilityText(unit)).toBe("Capability v3 allows: set_mode (cool/dry/fan), set_temperature 16–30 °C, set_fan (low/mid/high), test run 1–15 min.");
    expect([restrictionText(unit), restrictionText({ effectiveControlPolicy: { state: "restricted", phase: "active", policy: { kind: "power_off" } } } as unknown as ApiUnitDetail), restrictionText({ effectiveControlPolicy: { state: "unrestricted" } } as ApiUnitDetail)])
      .toEqual(["min 24 °C", "power off only", null]);
    expect(blockedText(unit)).toBe("the unit must be reconciled after a restriction");
    expect(blockedText({ controlAvailability: { state: "available" } } as ApiUnitDetail)).toBeNull();
    expect(connectionText("offline")).toBe("Connection: Offline.");
  });
  it("words a refused write and its fields", () => {
    expect(refusal({ code: "FORBIDDEN", messageKey: "errors.restriction_active" })).toMatch(/^FORBIDDEN — the active restriction/);
    expect(refusal({ code: "OFFLINE", messageKey: "error.offline" })).toMatch(/^OFFLINE — the device is not reachable/);
    expect(refusal({ code: "VALIDATION", messageKey: "error.validation" })).toBeNull(); // the fields say it
    expect(refusal({ code: "RATE_LIMITED", messageKey: "error.rateLimited" })).toBe("RATE_LIMITED — error.rateLimited");
    expect([fieldText("error.range"), fieldText("errors.unit_busy")]).toEqual(["1–15 minutes", "unit busy"]);
  });
  it("speaks Malay with the display time zone", () => {
    expect(historyRows([cmd()], [], MS)[0]).toMatchObject({ at: "10:00 PG GMT+9", sub: "cmd-0000 · diakui 10:00 PG GMT+9", badge: { text: "Diakui" } });
    expect(runBanner(run(), MS)?.title).toBe("Berjalan — tamat 10:10 PG GMT+9 (permulaan diakui 10:05 PG GMT+9)");
    expect(stateTiles({ observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: null }, latestMeasurements: [] }, MS.t).map((x) => x.label)).toEqual(["Kuasa", "Mod", "Titik set", "Kipas", "Bilik"]);
    expect(capabilityText(unit, MS.t)).toBe("Versi keupayaan v3 membenarkan: set_mode (cool/dry/fan), set_temperature 16–30 °C, set_fan (low/mid/high), larian ujian 1–15 min.");
  });
});
