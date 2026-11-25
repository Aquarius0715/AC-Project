import { describe, expect, it } from "vitest";
import { jobRows, liveTiles, metricChoices, metricOf, openAlerts, otherMetrics, registerRows, seriesCharts, windowEvents, type ApiTechUnit } from "@ac/web/lib/techUnit";
import { componentGroups } from "@ac/web/lib/techJob";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";

// The technician's unit screen (FR-T02, FR-T03, DD-T02, DD-T03): the latest-value tiles for a missing, empty or doubtful
// reading and each operation state, the register's "Not registered" values, the metric the chart opens on, the other
// metrics while offline or cut, the period's events (a recovery, an unknown severity), the jobs in order and the open
// alerts worst first.

const NOW = Date.parse("2026-09-14T01:35:00Z");
type Reading = ApiTechUnit["latestMeasurements"][number];
const m = (metric: Reading["metric"], value: number | null, unit: Reading["unit"], quality: Reading["quality"] = "valid"): Reading =>
  ({ metric, value, unit, quality, id: metric, unitId: "u1", sensorId: "s", origin: "measured", qualityReason: null, observedAt: "2026-09-14T01:30:00Z" });
const unit: ApiTechUnit = {
  id: "u1", installState: "installed", modelId: "model-1", version: 1, spaceId: null, alertPolicyIds: [], effectivePowerState: "on", displayName: "Bedroom AC", type: "split", connection: "online",
  lastSeenAt: "2026-09-14T01:34:00Z", installedAt: "2025-03-01T00:00:00Z", capabilityVersion: 3, serviceScope: ["indoor"], components: [],
  location: { pathLabels: ["Home A", "Bedroom"], address: null, accessInstructions: null },
  capabilities: { manufacturer: "AC-Co", model: "M1", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool"], fanLevels: ["low"], ventilation: false, ventilationLevels: [] },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: "2026-09-14T01:34:30Z" },
  latestMeasurements: [m("temperature", 24.5, "°C")],
};
const at = (over: Partial<ApiTechUnit>): ApiTechUnit => ({ ...unit, ...over });
const alert = (id: string, severity: ApiAlert["severity"], over: Partial<ApiAlert> = {}): ApiAlert => ({
  id, unitId: "u1", type: "sensor", severity, status: "open", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Seen.", detectedAt: "2026-09-14T01:00:00Z", ...over,
});
const event = (id: string, eventType: ApiDeviceEventFull["eventType"], recovery: ApiDeviceEventFull["recovery"] = null): ApiDeviceEventFull => ({
  id, eventType, recovery, version: 1, deviceId: "d1", evidenceSource: "heartbeat", occurredAt: "2026-09-14T01:10:00Z", restoredAt: null, alertIds: [], responseNotes: [],
});

const operation = (observedState: Partial<ApiTechUnit["observedState"]>) => liveTiles(at({ observedState: { ...unit.observedState, ...observedState } }), NOW).operation.value;
const spec = (metric: string, u: string) => ({ metric, unit: u, items: [], total: 0, complete: false });
const summary = (id: string, status: string, startAt: string | null): ApiTechJobRow => ({
  id, status, projection: "summary", version: 1, unitId: "u1", type: "reactive", severity: "normal", dueAt: "2026-09-16T00:00:00Z", requestedSlot: { startAt: "2026-09-15T00:00:00Z", endAt: "2026-09-15T02:00:00Z" },
  scheduledSlot: startAt ? { startAt, endAt: "2026-09-20T02:00:00Z" } : null,
});

describe("the latest values", () => {
  it("says a reading is missing, empty, or doubtful, and marks it", () => {
    const tiles = liveTiles(at({ latestMeasurements: [m("temperature", null, "°C"), m("humidity", 50, "%")] }), NOW);
    expect([tiles.temperature.value, tiles.temperature.sub.startsWith("observed"), tiles.power.value, tiles.power.sub]).toStrictEqual(["—", true, "—", "no reading"]);
    const doubtful = liveTiles(at({ latestMeasurements: [m("temperature", 31, "°C", "suspect")] }), NOW).temperature;
    expect([doubtful.value, doubtful.tone]).toStrictEqual(["31.0 °C (suspect)", "warn"]);
    expect(liveTiles(at({ latestMeasurements: [m("temperature", 31, "°C", "missing")] }), NOW).temperature.value).toBe("31.0 °C (missing)");
  });

  it("words the operation: unknown, off, its mode, or on without one", () => {
    expect([operation({ power: null }), operation({ power: false }), operation({ mode: "dry" }), operation({ mode: null })]).toStrictEqual(["—", "Off", "Drying", "On"]);
  });

  it("marks every time as last known while offline, and a unit never seen", () => {
    const tiles = liveTiles(at({ connection: "offline", lastSeenAt: null, observedState: { ...unit.observedState, observedAt: null } }), NOW);
    expect([tiles.connection.value, tiles.connection.tone, tiles.connection.sub, tiles.operation.sub, tiles.temperature.sub.startsWith("last known · observed")]).toStrictEqual(["Offline", "crit", "never seen", "—", true]);
    expect(liveTiles(at({ connection: "connecting" }), NOW).connection.value).toBe("Connecting");
  });
});

describe("the register and the chart's metric", () => {
  it("says what is not registered and adds the access instructions", () => {
    const r = registerRows(at({
      location: { pathLabels: [], address: null, accessInstructions: "Gate code 1234" }, type: null, installedAt: null, serviceScope: [],
      capabilities: { ...unit.capabilities, manufacturer: "", model: "M1" },
    }));
    expect(r.rows).toStrictEqual([
      ["Location", "Not registered"], ["Manufacturer / model", "Not registered"], ["Configuration", "Not registered"], ["Installed", "Not registered"], ["Capability version", "3"],
      ["Maintenance scope", "Not registered"], ["Access", "Gate code 1234"],
    ]);
    expect(r.missing).toBe(true);
    expect(registerRows(at({ type: "cassette" })).rows.find(([k]) => k === "Configuration")).toStrictEqual(["Configuration", "cassette"]); // a type without a word: as stored
    const one = componentGroups[0];
    expect(registerRows(at({ components: one ? [...one.keys] : [] })).note).toMatch(/components in 1 group/);
  });

  it("opens on the asked metric when measured, else temperature, else the first, else temperature", () => {
    const humid = at({ latestMeasurements: [m("humidity", 50, "%"), m("co2", 600, "ppm")] });
    expect([metricOf(humid, "co2"), metricOf(humid, "pm25"), metricOf(at({ latestMeasurements: [] }))]).toStrictEqual(["co2", "humidity", "temperature"]);
    expect(metricChoices(humid, ["humidity"], (s) => s).map((c) => [c.metric, c.on])).toStrictEqual([["humidity", true], ["co2", false]]);
  });

  it("names several metrics of one chart for the period", () => {
    expect(seriesCharts([spec("temperature", "°C"), spec("humidity", "%")], "24h", NOW).title).toBe("Temperature (°C) · Humidity (%) · last 24 h (rolling)");
    expect(seriesCharts([spec("temperature", "°C"), spec("custom_metric", "")], "7d", NOW).title).toBe("Temperature (°C) · custom metric · last 7 days (Kuala Lumpur calendar days)");
    expect(seriesCharts([spec("temperature", "°C"), spec("humidity", "%")], "1h", NOW).lines.map((l) => l.axis)).toStrictEqual(["left", "right"]);
    expect(seriesCharts([], "1h", NOW)).toMatchObject({ labels: [], times: [], lines: [] });
  });
});

describe("the other metrics and the period's events", () => {
  it("shows the other metrics as last known while offline, an empty value, a doubtful one, and when readings were cut", () => {
    const d = at({ connection: "offline", latestMeasurements: [m("temperature", 24, "°C"), m("humidity", null, "%"), m("co2", 900, "ppm", "stale")] });
    const rows = otherMetrics(d, ["temperature"], { co2: [{ value: 900, observedAt: "2026-09-14T01:00:00Z" }] }, "1h", NOW, undefined, { co2: 5 });
    expect(rows.map((r) => [r.metric, r.value, r.tone ?? "none", r.cut, r.sub.startsWith("last known")])).toStrictEqual([["humidity", "—", "none", false, true], ["co2", "900 ppm (stale)", "warn", true, true]]);
  });

  it("words a recovery by what came back, an alert of unknown severity as information, and leaves out what is outside the period", () => {
    const events = windowEvents(
      [alert("a1", "warning", { acknowledgedAt: "2026-09-14T01:20:00Z", resolvedAt: "2026-09-14T01:30:00Z", resolutionReason: null }), alert("a-old", "critical", { detectedAt: "2026-09-01T00:00:00Z" })],
      [event("e1", "restored", { axis: "power", sourceEventId: "e0", value: "on" }), event("e2", "restored", { axis: "tamper", sourceEventId: "e0", value: "closed" }), event("e3", "restored", { axis: "connection", sourceEventId: "e0", value: "online" }), event("e4", "operation_failed")],
      "1h", NOW,
    );
    expect(events.map((e) => [e.title, e.badge.tone])).toStrictEqual([
      ["Alert resolved — Sensor alert", "ok"], ["Alert acknowledged — Sensor alert", "primary"], ["Power signal restored", "ok"], ["Tamper cleared", "ok"], ["Data resumed", "primary"],
      ["Device operation failed", "muted"], ["Alert raised — Sensor alert", "warn"], // events at the same time by ID; the old alert's raise is outside the hour
    ]);
    expect(events[0]?.sub).toBe(""); // resolved without a reason
  });
});

describe("the jobs and the open alerts", () => {
  it("lists the current jobs by their visit or due time, then the ended ones newest first", () => {
    const rows = jobRows([
      summary("job-a-0001", "assigned", "2026-09-15T00:00:00Z"), summary("job-b-0001", "accepted", null), summary("job-c-0001", "on_hold", "2026-09-17T00:00:00Z"),
      { projection: "history", jobId: "job-h-0001", type: "periodic", status: "cancelled", completedAt: null }, { projection: "history", jobId: "job-h-0002", type: "periodic", status: "completed", completedAt: "2026-08-01T00:00:00Z" },
    ]);
    expect(rows.map((r) => [r.id, r.badge.tone])).toStrictEqual([["job-c-0001", "warn"], ["job-b-0001", "muted"], ["job-a-0001", "primary"], ["job-h-0002", "ok"], ["job-h-0001", "muted"]]);
    expect([rows[1]?.sub, rows[4]?.sub]).toStrictEqual(["not scheduled yet", "your assignment ended"]);
  });

  it("orders the open alerts worst and newest first, and says who acknowledged none", () => {
    const rows = openAlerts([
      alert("w-old", "warning", { detectedAt: "2026-09-13T00:00:00Z" }), alert("n", "normal"), alert("c", "critical", { acknowledgedAt: "2026-09-14T01:20:00Z" }),
      alert("w-new", "warning"), alert("done", "critical", { status: "resolved" }),
    ], NOW);
    expect(rows.map((r) => r.id)).toStrictEqual(["c", "w-new", "w-old", "n"]);
    expect([rows[0]?.ack.startsWith("Acknowledged "), rows[1]?.ack]).toStrictEqual([true, "Acknowledged by nobody yet. Completing the job does not resolve it."]);
  });
});
