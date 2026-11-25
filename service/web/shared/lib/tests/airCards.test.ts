import { describe, expect, it } from "vitest";
import {
  airChartGroups, airGuidance, airRooms, airRows, airSelection, airSlots, allergenView, metricCard, ventRow, ventStrip, windowTitle,
  type AirChartSeries, type ApiAllergen, type ApiMeasurement, type ApiVentilationLog,
} from "@ac/web/lib/air";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";

// The customer's air quality (FR-C06, IR98, IR99, IR110): the rooms of archived places left out and the selection
// from the URL, each metric card for a stale, suspect, empty or estimated reading and a high PM2.5, the guidance when
// nothing is measured, the strips without fresh air, the allergen with or without its value and unit, the chart's
// groups, the readings outside the window, a gap that turns into unloaded slots, and a ventilation log of another
// method or unit.

const m = (metric: string, value: number | null, over: Partial<ApiMeasurement> = {}): ApiMeasurement => ({
  id: "m", unitId: "u", sensorId: "s", metric, value, unit: "ppm", observedAt: "2026-09-14T01:12:30Z", origin: "measured", quality: "valid", qualityReason: null, ...over,
});
const property = (id: string, archived = false): ApiPropertyRow => ({
  id, archived, name: `Home ${id}`, version: 1, customerOrgId: "org-1", kind: "home", address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, updatedAt: "2026-09-01T00:00:00Z",
});
const space = (id: string, propertyId: string, archived = false): ApiSpaceRow => ({ id, propertyId, archived, parentSpaceId: null, kind: "room", name: `Room ${id}`, version: 1 });
const unit = (id: string, propertyId: string, spaceId: string | null): ApiUnitRow => ({
  id, propertyId, spaceId, displayName: `AC ${id}`, version: 1, customerOrgId: "org-1", modelId: "model-1", type: "split", installState: "installed", installedAt: null, serviceScope: [], alertPolicyIds: [],
  archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0,
});
const line = (metric: AirChartSeries["metric"], u: string): AirChartSeries => ({ metric, unit: u, name: metric, points: [], guide: null, axis: "left", hasSensor: true, valid: false, readings: 0 });

const slot = (h: number, avg: number | null) => ({ avg, start: `2026-09-14T0${String(h)}:00:00.000Z`, n: avg === null ? 0 : 1 });
const log = (over: Partial<ApiVentilationLog>): ApiVentilationLog => ({
  id: "v1", spaceId: "r1", unitId: null, method: "window_opened", durationMinutes: 15, co2AtLog: null, loggedByMembershipId: "m-1", loggedAt: "2026-09-14T01:00:00Z", ...over,
});

describe("the rooms and the selection", () => {
  const rooms = airRooms(
    [property("a"), property("gone", true)],
    [space("r1", "a"), space("r-old", "a", true), space("r-gone", "gone")],
    [unit("u1", "a", "r1"), unit("u2", "a", "r-old"), unit("u3", "gone", "r-gone")],
  );

  it("leaves out an archived room and a room of an archived property, and lists no loose units when there are none", () => {
    expect(rooms.map((r) => r.key)).toStrictEqual(["r1"]);
  });

  it("shows the URL's unit, else the URL room's first unit, else the first room's", () => {
    expect(airSelection(rooms, "u1")?.unitId).toBe("u1");
    expect([airSelection(rooms, "u-gone", "r1")?.unitId, airSelection(rooms, undefined, "r-x")?.unitId]).toStrictEqual(["u1", "u1"]);
    expect([airSelection([]), airSelection([{ key: "k", spaceId: null, label: "", path: "", units: [] }])]).toStrictEqual([null, null]);
  });
});

describe("a metric card", () => {
  it("never shows a stale, empty or suspect reading as current, and names an estimated one", () => {
    expect(metricCard("co2", m("co2", null, { quality: "stale" }), true).sub).toMatch(/^No reading since /);
    const suspect = metricCard("co2", m("co2", null, { quality: "suspect", qualityReason: "clock_skew" }), true);
    expect([suspect.value, suspect.sub.endsWith("· unreliable reading"), suspect.advice]).toStrictEqual([null, true, "Not used for advice"]);
    expect(metricCard("co2", m("co2", null), true).sub).toMatch(/^No reading \(null\) · /);
    expect(metricCard("co2", m("co2", 600, { origin: "estimated" }), true).sub).toMatch(/ · estimated$/);
    expect(metricCard("co2", m("co2", 600, { origin: "inspection" }), true).sub).toMatch(/ · inspection$/);
  });

  it("recommends cleaning the filter at a high PM2.5", () => {
    expect(metricCard("pm25", m("pm25", 40), true)).toMatchObject({ advice: "Filter cleaning recommended (≥ 35 µg/m³)", warn: true });
    expect(metricCard("pm25", m("pm25", 20), true)).toMatchObject({ advice: "No current advice", warn: false });
  });
});

describe("the guidance and the strips", () => {
  it("has no guidance without a valid CO2 or PM2.5, and none to give when both are low", () => {
    expect([airGuidance(undefined, undefined, true), airGuidance(m("co2", 500), undefined, true), airGuidance(m("co2", 1200), undefined, false)]).toStrictEqual([["unavailable"], ["none"], ["ventilate_manual"]]);
  });

  it("says how to ventilate without data and without a fresh-air function", () => {
    expect(ventStrip(["unavailable"], undefined, "Bedroom", "Bedroom AC", false).text).toMatch(/Manual ventilation: Bedroom AC has no fresh-air function/);
    expect(ventStrip(["unavailable"], undefined, "Bedroom", "Bedroom AC", true).text).toMatch(/You can still log a manual ventilation\.$/);
  });
});

describe("the allergen", () => {
  const seen: ApiAllergen = { availability: "available", substance: "Dust mite", value: 12, unit: "ng/m³", sourceLabel: "Lab kit", observedAt: "2026-09-14T01:00:00Z", evidenceText: "Sampled." };

  it("says not measured, and shows a value without its unit as unknown or no value at all", () => {
    expect(allergenView({ ...seen, availability: "not_measured" }).badge.text).toBe("Not measured");
    expect(allergenView({ ...seen, unit: null }).text).toMatch(/^Value unknown \(no unit\) · Source: Lab kit/);
    expect(allergenView({ ...seen, value: null }).text).toMatch(/^Source: Lab kit · /);
    expect(allergenView(seen).text).toMatch(/^12 ng\/m³ · /);
  });
});

describe("the chart and the table", () => {
  it("draws no chart without lines, and one chart for one or two units", () => {
    expect(airChartGroups([])).toStrictEqual([]);
    expect(airChartGroups([line("co2", "ppm"), line("temperature", "°C")])).toHaveLength(1);
    expect(windowTitle("7d")).toBe("last 7 days");
  });

  it("counts only the valid readings inside the window", () => {
    const w = { period: "1h" as const, from: "2026-09-14T00:00:00.000Z", to: "2026-09-14T00:10:00.000Z", slotMs: 300_000 };
    const slots = airSlots([m("co2", 500, { observedAt: "2026-09-14T00:01:00Z" }), m("co2", 9999, { observedAt: "2026-09-14T00:10:00Z" }), m("co2", 800, { observedAt: "2026-09-13T23:59:00Z" }), m("co2", 700, { observedAt: "2026-09-14T00:02:00Z", quality: "stale" })], w);
    expect(slots.map((s) => [s.avg, s.n])).toStrictEqual([[500, 1], [null, 0]]);
  });

  it("splits a run of empty slots where the loaded readings begin", () => {
    const rows = airRows([slot(0, null), slot(1, null), slot(2, null), slot(3, 500)], "co2", 3_600_000, undefined, "2026-09-14T01:30:00Z");
    expect(rows.map((r) => [r.value, r.note])).toStrictEqual([["500 ppm", "1 reading"], ["No data", "Gap — not joined"], ["Not loaded", "Earlier readings are not loaded"]]);
  });
});

describe("a ventilation log row", () => {
  it("names another method as stored and an unknown unit, and a log without a CO2 reading", () => {
    const names = new Map([["u1", "Bedroom AC"]]);
    expect(ventRow(log({ unitId: "u-gone" }), "m-1", names, Date.parse("2026-09-14T02:00:00Z"))).toMatchObject({ text: "Window opened · 15 min · another unit", co2: "CO2 not measured at logging", by: "by you" });
    expect(ventRow(log({ unitId: "u1", loggedByMembershipId: "m-2" }), "m-1", names, Date.parse("2026-09-14T02:00:00Z"))).toMatchObject({ text: "Window opened · 15 min · Bedroom AC", by: "by another member" });
  });
});
