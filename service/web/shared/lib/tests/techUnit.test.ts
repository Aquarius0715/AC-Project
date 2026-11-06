import { describe, expect, it } from "vitest";
import {
  componentCards, controlJob, jobRows, liveTiles, metricOf, openAlerts, otherMetrics, periodOf, periodRange, registerRows, seriesChart, stoppedBanner, windowEvents, type ApiTechUnit,
} from "@ac/web/lib/techUnit";
import { componentGroups } from "@ac/web/lib/techJob";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import { i18nOf } from "@ac/web/lib/i18n";
import { must } from "./must";

const TEMPERATURE = "temperature";
const JOB_CONTRACTOR_A = "job-contractor-a";
const NOT_REGISTERED = "Not registered";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = Date.parse("2026-09-14T01:35:00Z"); // 09:35 in Kuala Lumpur
const ALL = componentGroups.flatMap((g) => g.keys);
type Reading = ApiTechUnit["latestMeasurements"][number];
const m = (metric: Reading["metric"], value: number | null, unit: Reading["unit"], quality: Reading["quality"] = "valid", observedAt = "2026-09-14T01:35:00Z"): Reading =>
  ({ id: metric, unitId: "u1", sensorId: "s", origin: "measured", qualityReason: null, metric, value, unit, observedAt, quality });
const unit: ApiTechUnit = {
  id: "unit-online-rto", version: 1, spaceId: null, alertPolicyIds: [], effectivePowerState: "on", displayName: "Bedroom AC", type: "split", connection: "online", lastSeenAt: "2026-09-14T01:34:00Z", installedAt: "2025-03-01T00:00:00Z", capabilityVersion: 3,
  serviceScope: ["indoor", "outdoor", "electrical"], components: ALL,
  location: { pathLabels: ["customer-a", "Home A", "1F", "Bedroom"], address: null, accessInstructions: null },
  capabilities: {
    manufacturer: "AC-Co", model: "ventilation-demo v3", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool", "dry", "fan"],
    fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [], sensors: [],
  },
  effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "mid", observedAt: "2026-09-14T01:34:30Z" },
  latestMeasurements: [m(TEMPERATURE, 24.5, "°C"), m("power", 1.2, "kW"), m("humidity", 60, "%", "stale")],
};
const offline = { ...unit, connection: "offline" } as ApiTechUnit;
const alerts = [
  { id: "a1", unitId: "u1", type: "sensor", severity: "warning", status: "acknowledged", causeCode: "window_open", evidenceKind: "inferred", evidenceText: "Cooling load rose while the outdoor temperature was stable.", detectedAt: "2026-09-14T01:12:00Z", acknowledgedAt: "2026-09-14T01:20:00Z", resolvedAt: null, resolutionReason: null },
  { id: "a2", unitId: "u1", type: "maintenance", severity: "normal", status: "resolved", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Cleaning due.", detectedAt: "2026-09-10T01:00:00Z", acknowledgedAt: null, resolvedAt: "2026-09-13T03:00:00Z", resolutionReason: "Filter cleaned" },
] as ApiAlert[];
const event = (id: string, eventType: ApiDeviceEventFull["eventType"], occurredAt: string, recovery: ApiDeviceEventFull["recovery"] = null) =>
  ({ id, eventType, occurredAt, recovery, version: 1, deviceId: "d1", evidenceSource: "heartbeat", restoredAt: null, alertIds: [], responseNotes: [] }) as ApiDeviceEventFull;
const running: Extract<ApiTechJobRow, { projection: "summary" }> = {
  projection: "summary", id: JOB_CONTRACTOR_A, version: 3, unitId: "u1", type: "reactive", status: "in_progress", severity: "warning", dueAt: "2026-09-15T00:00:00Z", requestedSlot: { startAt: "2026-09-14T02:00:00Z", endAt: "2026-09-14T04:00:00Z" }, scheduledSlot: { startAt: "2026-09-14T02:00:00Z", endAt: "2026-09-14T04:00:00Z" },
};
const jobs: ApiTechJobRow[] = [running, { projection: "history", jobId: "job-c01-0000", type: "preventive", status: "completed", completedAt: "2026-06-12T04:00:00Z" }];

describe("technician unit screen", () => {
  it("reads the register, never filling what is missing (DD-T02)", () => {
    expect(registerRows(unit)).toEqual({
      rows: [["Location", "customer-a › Home A › 1F › Bedroom"], ["Manufacturer / model", "AC-Co / ventilation-demo v3"], ["Configuration", "Split"], ["Installed", "1 Mar 2025"], ["Capability version", "3"], ["Maintenance scope", "Indoor · Outdoor · Electrical"]],
      missing: false, note: "18 components across 3 groups (8 indoor, 5 outdoor, 5 electrical) — inspect them in the job workspace.",
    });
    const bare = { ...unit, type: null, installedAt: null, capabilities: { ...unit.capabilities, manufacturer: "", model: "" }, serviceScope: ["indoor", "electrical"], components: ALL.filter((k) => !["condenser_coil", "compressor", "fan", "blade", "refrigerant_pipe"].includes(k)), location: { ...unit.location, accessInstructions: "Ring the guard" } } as ApiTechUnit;
    const r = registerRows(bare);
    expect(r.rows.map((x) => x[1])).toEqual(["customer-a › Home A › 1F › Bedroom", NOT_REGISTERED, NOT_REGISTERED, NOT_REGISTERED, "3", "Indoor · Electrical", "Ring the guard"]);
    expect([r.missing, r.note]).toEqual([true, "13 components across 2 groups (8 indoor, 5 electrical) — inspect them in the job workspace."]);
    expect(componentCards(bare).map((c) => c.title)).toEqual(["Indoor · 8 components", "Electrical · 5 components"]);
    expect(componentCards(unit)[1]).toEqual({ group: "outdoor", title: "Outdoor · 5 components", names: "Condenser coil, Compressor, Fan, Fan blade, Refrigerant pipe" });
  });

  it("shows the latest values with their times, last known while offline (DD-T03)", () => {
    expect(liveTiles(unit, NOW)).toEqual({
      temperature: { label: "Temperature", value: "24.5 °C", sub: "observed today 9:35 am MYT" },
      power: { label: "Power", value: "1.2 kW", sub: "observed today 9:35 am MYT" },
      operation: { label: "Operation", value: "Cooling", sub: "observed today 9:34 am MYT" },
      connection: { label: "Connection", value: "Online", sub: "last seen today 9:34 am MYT" },
    });
    const down = liveTiles(offline, NOW);
    expect([down.temperature.sub, down.connection.value, down.connection.tone]).toEqual(["last known · observed today 9:35 am MYT", "Offline", "crit"]);
    expect(stoppedBanner(unit, NOW)).toBeNull();
    expect(stoppedBanner(offline, NOW)).toBe("Updates stopped — communication lost today 9:34 am MYT. Showing last known values with their times; nothing here is real-time.");
    expect(stoppedBanner({ ...offline, lastSeenAt: null }, NOW)).toBe("No data has reached the server from this unit yet — nothing here is real-time.");
  });

  it("charts one metric over 1 h, 24 h (rolling) or 7 Kuala Lumpur days with its gaps", () => {
    expect([periodOf("1h"), periodOf("7d"), periodOf("x"), periodOf()]).toEqual(["1h", "7d", "24h", "24h"]);
    expect(new Date(periodRange("7d", NOW).from).toISOString()).toBe("2026-09-07T16:00:00.000Z"); // 8 Sept 00:00 in Kuala Lumpur
    expect(periodRange("1h", NOW)).toEqual({ from: NOW - 3_600_000, to: NOW });
    expect([metricOf(unit, "humidity"), metricOf(unit, "co2"), metricOf(unit), metricOf({ ...unit, latestMeasurements: [m("power", 1, "kW")] })]).toEqual(["humidity", TEMPERATURE, TEMPERATURE, "power"]);
    const items = [
      { value: 25, observedAt: "2026-09-13T02:00:00Z" }, { value: 26, observedAt: "2026-09-13T04:30:00Z" }, { value: 24.5, observedAt: "2026-09-14T01:00:00Z" },
      { value: 40, observedAt: "2026-09-14T01:10:00Z", quality: "suspect" },
    ];
    const c = seriesChart(TEMPERATURE, "°C", items, "24h", NOW);
    expect(c.points).toEqual([25, 26, null, null, null, null, null, null, null, null, null, 24.5]); // the suspect reading is not plotted
    expect([c.title, c.labels[0], c.labels[4], c.count, c.cut, c.empty]).toEqual(["Temperature (°C) · last 24 h (rolling)", "13 Sept, 9:35 am", "now", "4 measurements in this period", null, false]); // the 24 h axis starts with its day (IR598)
    expect(seriesChart(TEMPERATURE, "°C", items, "1h", NOW).labels[0]).toBe("8:35 am");
    expect(c.gap).toBe("no data 13 Sept, 1:35 pm – 14 Sept, 7:35 am MYT (gap not connected)");
    const week = seriesChart(TEMPERATURE, "°C", [], "7d", NOW);
    expect([week.title, week.labels[0], week.empty, week.gap, week.count]).toEqual(["Temperature (°C) · last 7 days (Kuala Lumpur calendar days)", "8 Sept", true, null, "0 measurements in this period"]);
    expect(otherMetrics(unit, TEMPERATURE, { power: [{ value: 1.2, observedAt: "2026-09-14T01:30:00Z" }] }, "24h", NOW).map((x) => [x.metric, x.value, x.tone, x.points[11], x.cut])).toEqual([["power", "1.2 kW", undefined, 1.2, false], ["humidity", "60.0 % (stale)", "warn", null, false]]);
  });

  it("says what a cut series does not load instead of counting the period (IR598)", () => {
    // 400 of the period's 1440 minute readings were loaded: the start of the chart is blank because nothing older was read
    const items = Array.from({ length: 400 }, (_, k) => ({ value: 24, observedAt: new Date(NOW - k * 60_000).toISOString() }));
    const c = seriesChart(TEMPERATURE, "°C", items, "24h", NOW, undefined, 1440);
    expect([c.count, c.cut, c.gap]).toEqual([null, "Showing the latest 400 of 1440 readings — earlier readings in this period are not loaded.", null]);
    expect(seriesChart(TEMPERATURE, "°C", items, "24h", NOW, MS, 1440).cut).toBe("Menunjukkan 400 bacaan terkini daripada 1440 — bacaan lebih awal dalam tempoh ini tidak dimuatkan.");
    expect(seriesChart(TEMPERATURE, "°C", items.slice(0, 60), "1h", NOW, undefined, 60)).toMatchObject({ count: "60 measurements in this period", cut: null });
    // the period's averages (IR606) are complete: they count the period's readings and are never cut
    const averages = Array.from({ length: 288 }, (_, k) => ({ value: 24, observedAt: new Date(NOW - 86_400_000 + k * 300_000).toISOString() }));
    expect(seriesChart(TEMPERATURE, "°C", averages, "24h", NOW, undefined, 1438, true)).toMatchObject({ count: "1438 measurements in this period", cut: null });
    const power = Array.from({ length: 100 }, (_, k) => ({ value: 1, observedAt: new Date(NOW - k * 60_000).toISOString() }));
    expect(otherMetrics(unit, TEMPERATURE, { power }, "24h", NOW, undefined, { power: 1440 }).map((x) => [x.metric, x.cut])).toEqual([["power", true], ["humidity", false]]);
  });

  it("lists the period's alert and device events, newest first", () => {
    const events = [event("e1", "communication_lost", "2026-09-13T19:00:00Z"), event("e2", "restored", "2026-09-13T21:10:00Z", { axis: "connection", sourceEventId: "e1", value: "online" }), event("e3", "tamper", "2026-09-10T00:00:00Z")];
    expect(windowEvents(alerts, events, "24h", NOW).map((e) => [e.time, e.title, e.badge.text])).toEqual([
      ["today 9:20 am MYT", "Alert acknowledged — Possible open window", "Acknowledged"], ["today 9:12 am MYT", "Alert raised — Possible open window", "Warning"],
      ["today 5:10 am MYT", "Data resumed", "Connectivity"], ["today 3:00 am MYT", "Updates stopped", "Connectivity"], ["yesterday 11:00 am MYT", "Alert resolved — Filter cleaning reminder", "Resolved"],
    ]);
    expect(windowEvents([], [event("e4", "restored", "2026-09-14T01:00:00Z", { axis: "tamper", sourceEventId: "e3", value: "clear" })], "1h", NOW)[0]).toMatchObject({ title: "Tamper cleared", badge: { text: "Tamper", tone: "ok" } });
  });

  it("lists the technician's jobs on the unit and the job control works for", () => {
    expect(jobRows(jobs)).toEqual([
      { id: JOB_CONTRACTOR_A, title: "job-cont · Repair", sub: "14 Sept, 10:00 am – 12:00 pm MYT", badge: { text: "in progress", tone: "warn" } },
      { id: "job-c01-0000", title: "job-c01- · Preventive maintenance", sub: "completed 12 Jun 2026", badge: { text: "completed", tone: "ok" } },
    ]);
    const assigned: ApiTechJobRow = { ...running, id: "job-t07", status: "assigned" };
    expect([controlJob(jobs), controlJob([assigned, ...jobs]), controlJob([assigned, ...jobs], "job-t07"), controlJob([must(jobs[1])])]).toEqual([JOB_CONTRACTOR_A, JOB_CONTRACTOR_A, "job-t07", null]);
    expect(openAlerts(alerts, NOW)).toEqual([{
      id: "a1", title: "Possible open window", severity: "warning", sub: "Cooling load rose while the outdoor temperature was stable. · today 9:12 am MYT",
      ack: "Acknowledged today 9:20 am MYT. Completing the job does not resolve it.",
    }]);
  });

  it("words the unit screen in Malay with the display time zone (IR283)", () => {
    expect(registerRows(unit, MS).rows.slice(1, 3)).toEqual([["Pengilang / model", "AC-Co / ventilation-demo v3"], ["Konfigurasi", "Split"]]);
    expect(liveTiles(offline, NOW, MS).connection).toEqual({ label: "Sambungan", value: "Luar talian", sub: "kali terakhir dilihat hari ini 10:34 PG GMT+9", tone: "crit" });
    expect(seriesChart(TEMPERATURE, "°C", [], "24h", NOW, MS).title).toBe("Suhu (°C) · 24 j terakhir (bergerak)");
    expect(openAlerts(alerts, NOW, MS)[0]?.ack).toBe("Diakui hari ini 10:20 PG GMT+9. Menyelesaikan kerja tidak menyelesaikan amaran ini.");
  });
});
