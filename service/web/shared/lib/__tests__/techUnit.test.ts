import { describe, expect, it } from "vitest";
import {
  componentCards, controlJob, jobRows, liveTiles, metricOf, openAlerts, otherMetrics, periodOf, periodRange, registerRows, seriesChart, stoppedBanner, windowEvents, type ApiTechUnit,
} from "@ac/web/lib/techUnit";
import { componentGroups } from "@ac/web/lib/techJob";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = Date.parse("2026-09-14T01:35:00Z"); // 09:35 in Kuala Lumpur
const ALL = componentGroups.flatMap((g) => g.keys);
const m = (metric: string, value: number | null, unit: string, quality = "valid", observedAt = "2026-09-14T01:35:00Z") =>
  ({ id: metric, unitId: "u1", sensorId: "s", metric, value, unit, observedAt, origin: "measured", quality, qualityReason: null });
const unit = {
  id: "unit-online-rto", displayName: "Bedroom AC", type: "split", connection: "online", lastSeenAt: "2026-09-14T01:34:00Z", installedAt: "2025-03-01T00:00:00Z", capabilityVersion: 3,
  serviceScope: ["indoor", "outdoor", "electrical"], components: ALL,
  location: { pathLabels: ["customer-a", "Home A", "1F", "Bedroom"], address: null, accessInstructions: null },
  capabilities: { manufacturer: "AC-Co", model: "ventilation-demo v3" },
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "mid", observedAt: "2026-09-14T01:34:30Z" },
  latestMeasurements: [m("temperature", 24.5, "°C"), m("power", 1.2, "kW"), m("humidity", 60, "%", "stale")],
} as unknown as ApiTechUnit;
const offline = { ...unit, connection: "offline" } as ApiTechUnit;
const alerts = [
  { id: "a1", unitId: "u1", type: "sensor", severity: "warning", status: "acknowledged", causeCode: "window_open", evidenceKind: "inferred", evidenceText: "Cooling load rose while the outdoor temperature was stable.", detectedAt: "2026-09-14T01:12:00Z", acknowledgedAt: "2026-09-14T01:20:00Z", resolvedAt: null, resolutionReason: null },
  { id: "a2", unitId: "u1", type: "maintenance", severity: "normal", status: "resolved", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Cleaning due.", detectedAt: "2026-09-10T01:00:00Z", acknowledgedAt: null, resolvedAt: "2026-09-13T03:00:00Z", resolutionReason: "Filter cleaned" },
] as ApiAlert[];
const ev = (id: string, eventType: ApiDeviceEventFull["eventType"], occurredAt: string, recovery: ApiDeviceEventFull["recovery"] = null) =>
  ({ id, version: 1, deviceId: "d1", eventType, evidenceSource: "heartbeat", occurredAt, restoredAt: null, alertIds: [], recovery, responseNotes: [] }) as ApiDeviceEventFull;
const jobs = [
  { projection: "summary", id: "job-contractor-a", version: 3, unitId: "u1", type: "reactive", status: "in_progress", severity: "warning", dueAt: "2026-09-15T00:00:00Z", requestedSlot: { startAt: "2026-09-14T02:00:00Z", endAt: "2026-09-14T04:00:00Z" }, scheduledSlot: { startAt: "2026-09-14T02:00:00Z", endAt: "2026-09-14T04:00:00Z" } },
  { projection: "history", jobId: "job-c01-0000", type: "preventive", status: "completed", completedAt: "2026-06-12T04:00:00Z" },
] as ApiTechJobRow[];

describe("technician unit screen", () => {
  it("reads the register, never filling what is missing (DD-T02)", () => {
    expect(registerRows(unit)).toEqual({
      rows: [["Location", "customer-a › Home A › 1F › Bedroom"], ["Manufacturer / model", "AC-Co / ventilation-demo v3"], ["Configuration", "Split"], ["Installed", "1 Mar 2025"], ["Capability version", "3"], ["Maintenance scope", "Indoor · Outdoor · Electrical"]],
      missing: false, note: "18 components across 3 groups (8 indoor, 5 outdoor, 5 electrical) — inspect them in the job workspace.",
    });
    const bare = { ...unit, type: null, installedAt: null, capabilities: { ...unit.capabilities, manufacturer: "", model: "" }, serviceScope: ["indoor", "electrical"], components: ALL.filter((k) => !["condenser_coil", "compressor", "fan", "blade", "refrigerant_pipe"].includes(k)), location: { ...unit.location, accessInstructions: "Ring the guard" } } as ApiTechUnit;
    const r = registerRows(bare);
    expect(r.rows.map((x) => x[1])).toEqual(["customer-a › Home A › 1F › Bedroom", "Not registered", "Not registered", "Not registered", "3", "Indoor · Electrical", "Ring the guard"]);
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
    expect([metricOf(unit, "humidity"), metricOf(unit, "co2"), metricOf(unit), metricOf({ ...unit, latestMeasurements: [m("power", 1, "kW")] } as ApiTechUnit)]).toEqual(["humidity", "temperature", "temperature", "power"]);
    const items = [
      { value: 25, observedAt: "2026-09-13T02:00:00Z" }, { value: 26, observedAt: "2026-09-13T04:30:00Z" }, { value: 24.5, observedAt: "2026-09-14T01:00:00Z" },
      { value: 40, observedAt: "2026-09-14T01:10:00Z", quality: "suspect" },
    ];
    const c = seriesChart("temperature", "°C", items, "24h", NOW);
    expect(c.points).toEqual([25, 26, null, null, null, null, null, null, null, null, null, 24.5]); // the suspect reading is not plotted
    expect([c.title, c.labels[0], c.labels[4], c.count, c.empty]).toEqual(["Temperature (°C) · last 24 h (rolling)", "9:35 am", "now", "4 measurements in this period", false]);
    expect(c.gap).toBe("no data 13 Sept, 1:35 pm – 14 Sept, 7:35 am MYT (gap not connected)");
    const week = seriesChart("temperature", "°C", [], "7d", NOW);
    expect([week.title, week.labels[0], week.empty, week.gap, week.count]).toEqual(["Temperature (°C) · last 7 days (Kuala Lumpur calendar days)", "8 Sept", true, null, "0 measurements in this period"]);
    expect(otherMetrics(unit, "temperature", { power: [{ value: 1.2, observedAt: "2026-09-14T01:30:00Z" }] }, "24h", NOW).map((x) => [x.metric, x.value, x.tone, x.points[11]])).toEqual([["power", "1.2 kW", undefined, 1.2], ["humidity", "60.0 % (stale)", "warn", null]]);
  });

  it("lists the period's alert and device events, newest first", () => {
    const events = [ev("e1", "communication_lost", "2026-09-13T19:00:00Z"), ev("e2", "restored", "2026-09-13T21:10:00Z", { axis: "connection", sourceEventId: "e1", value: "online" }), ev("e3", "tamper", "2026-09-10T00:00:00Z")];
    expect(windowEvents(alerts, events, "24h", NOW).map((e) => [e.time, e.title, e.badge.text])).toEqual([
      ["today 9:20 am MYT", "Alert acknowledged — Possible open window", "Acknowledged"], ["today 9:12 am MYT", "Alert raised — Possible open window", "Warning"],
      ["today 5:10 am MYT", "Data resumed", "Connectivity"], ["today 3:00 am MYT", "Updates stopped", "Connectivity"], ["yesterday 11:00 am MYT", "Alert resolved — Filter cleaning reminder", "Resolved"],
    ]);
    expect(windowEvents([], [ev("e4", "restored", "2026-09-14T01:00:00Z", { axis: "tamper", sourceEventId: "e3", value: "clear" })], "1h", NOW)[0]).toMatchObject({ title: "Tamper cleared", badge: { text: "Tamper", tone: "ok" } });
  });

  it("lists the technician's jobs on the unit and the job control works for", () => {
    expect(jobRows(jobs)).toEqual([
      { id: "job-contractor-a", title: "job-cont · Repair", sub: "14 Sept, 10:00 am – 12:00 pm MYT", badge: { text: "in progress", tone: "warn" } },
      { id: "job-c01-0000", title: "job-c01- · Preventive maintenance", sub: "completed 12 Jun 2026", badge: { text: "completed", tone: "ok" } },
    ]);
    const assigned = { ...jobs[0], id: "job-t07", status: "assigned" } as ApiTechJobRow;
    expect([controlJob(jobs), controlJob([assigned, ...jobs]), controlJob([assigned, ...jobs], "job-t07"), controlJob([jobs[1]])]).toEqual(["job-contractor-a", "job-contractor-a", "job-t07", null]);
    expect(openAlerts(alerts, NOW)).toEqual([{
      id: "a1", title: "Possible open window", severity: "warning", sub: "Cooling load rose while the outdoor temperature was stable. · today 9:12 am MYT",
      ack: "Acknowledged today 9:20 am MYT. Completing the job does not resolve it.",
    }]);
  });

  it("words the unit screen in Malay with the display time zone (IR283)", () => {
    expect(registerRows(unit, MS).rows.slice(1, 3)).toEqual([["Pengilang / model", "AC-Co / ventilation-demo v3"], ["Konfigurasi", "Split"]]);
    expect(liveTiles(offline, NOW, MS).connection).toEqual({ label: "Sambungan", value: "Luar talian", sub: "kali terakhir dilihat hari ini 10:34 PG GMT+9", tone: "crit" });
    expect(seriesChart("temperature", "°C", [], "24h", NOW, MS).title).toBe("Suhu (°C) · 24 j terakhir (bergerak)");
    expect(openAlerts(alerts, NOW, MS)[0].ack).toBe("Diakui hari ini 10:20 PG GMT+9. Menyelesaikan kerja tidak menyelesaikan amaran ini.");
  });
});
