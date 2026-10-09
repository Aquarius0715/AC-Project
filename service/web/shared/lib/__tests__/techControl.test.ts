import { describe, expect, it } from "vitest";
import { actionCode, actionWord, historyRows, observedText, runActive, runBanner, stateTiles, windowText, type ApiCommandRow, type ApiRun } from "@ac/web/lib/techControl";

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
    expect(rows[0]).toEqual({ id: "cmd-00000001", at: "09:00", title: "set_mode = cool", sub: "cmd-0000 · acknowledged 09:00:05", badge: { text: "Acked", tone: "ok" } });
    expect(rows[1]).toMatchObject({ sub: "cmd-0000 · rejected by the device (DEVICE_REJECTED)", badge: { text: "Rejected", tone: "crit" } });
    expect(rows[2]).toMatchObject({ sub: "cmd-0000 · no device response (expired)", badge: { text: "No response", tone: "crit" } });
    expect(rows[3]).toMatchObject({ sub: "cmd-0000 · waiting for the device", badge: { text: "Waiting", tone: "warn" } });
    expect(rows[4]).toMatchObject({ sub: "cmd-0000 · cancelled", badge: { text: "Cancelled", tone: "muted" } });
  });
  it("names the test run of its start and end commands", () => {
    const r = run({ endCommandId: "cmd-end" });
    const rows = historyRows([cmd({ id: "cmd-start", requestedAt: "2026-09-14T01:04:00Z", acknowledgedAt: "2026-09-14T01:05:00Z" }), cmd({ id: "cmd-end", status: "requested", acknowledgedAt: null })], [r]);
    expect(rows[0]).toMatchObject({ title: "test run start (Power ON, 5 min)", sub: "cmd-star · acknowledged 09:05:00 · ends 09:10", badge: { text: "Running", tone: "primary" } });
    expect(rows[1]).toMatchObject({ title: "test run end (Power OFF)", badge: { text: "Waiting", tone: "warn" } });
    expect(historyRows([cmd({ id: "cmd-start" })], [run({ state: "completed" })])[0].badge.text).toBe("Acked");
  });
});

describe("run banner and state", () => {
  it("follows the DiagnosticRun state", () => {
    expect(runBanner(null)).toBeNull();
    expect(runBanner(run({ state: "awaiting_start" }))).toMatchObject({ tone: "primary", title: "Starting — waiting for the device" });
    expect(runBanner(run())).toMatchObject({ tone: "warn", title: "Running — ends 09:10 (start acknowledged 09:05)" });
    expect(runBanner(run({ state: "end_requested" }))).toMatchObject({ tone: "warn", title: "End requested 09:10 — awaiting device response" });
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
    expect(observedText("2026-09-14T01:00:05Z")).toBe("observed 09:00:05");
    expect(observedText(null)).toBe("no observation yet");
    expect(windowText("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z")).toBe("09/14 08:00 – 09/20 08:00");
  });
});
