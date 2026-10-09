import { describe, expect, it } from "vitest";
import {
  attentionCard, emissionsCard, energyCard, hourlyPoints, nextRunText, overviewRows, overviewScope, periodNote, periodOf, previousRange, whenText, type OverviewUnit,
} from "@ac/web/lib/customerOverview";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiEnergySummary } from "@ac/web/lib/energy";

const NOW = Date.parse("2026-09-14T01:12:30Z"); // Mon 09:12:30 KL
const m = (metric: string, value: number | null, over: Record<string, unknown> = {}) => ({
  id: metric, unitId: "u1", sensorId: "s", metric, value, unit: metric === "power" ? "kW" : metric === "temperature" ? "°C" : "%", observedAt: "2026-09-14T01:12:00Z", origin: "measured" as const, quality: "valid" as const, qualityReason: null, ...over,
});
const unit = (id: string, over: Partial<OverviewUnit> = {}): OverviewUnit => ({
  id, version: 1, customerOrgId: "o", propertyId: "home", spaceId: null, displayName: `AC ${id}`, modelId: "m", type: "split", installedAt: null, serviceScope: ["indoor"], alertPolicyIds: [],
  archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: "2026-09-14T01:12:00Z", warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, latestMeasurements: [], ...over,
});
const props = [{ id: "home", version: 1, customerOrgId: "o", kind: "home" as const, name: "Home A", address: null, accessInstructions: null, archived: false, updatedAt: "" },
  { id: "office", version: 1, customerOrgId: "o", kind: "office" as const, name: "Office A", address: null, accessInstructions: null, archived: false, updatedAt: "" }];
const sum = (kWh: number | null, over: Partial<ApiEnergySummary["totals"]> = {}, factor = true) => ({
  period: { from: "", to: "" }, unitIds: [], currency: "MYR", baselineRef: null, factorRef: null, baselineSnapshot: null, tariffVersion: "t", boundaryId: "ac_input_electricity", boundary: "", coverage: 1, qualityWarnings: [],
  totals: { kWh, amountMinor: null, savedKWh: null, deltaKWh: null, savingPercentage: null, savedAmountMinor: null, emissionsKg: kWh === null ? null : kWh * 0.7, savedEmissionsKg: null, ...over },
  factorSnapshot: factor ? { id: "f", version: 1, region: "MY", year: 2026, kgCO2ePerKWh: 0.7, source: "demo" } : null,
} as ApiEnergySummary);
const alert = (id: string, unitId: string, severity: ApiAlert["severity"], status: string, detectedAt: string, over: Partial<ApiAlert> = {}): ApiAlert =>
  ({ id, unitId, type: "sensor", severity, status, causeCode: "unknown", evidenceKind: "inferred", evidenceText: "", detectedAt, ...over });

describe("customer overview", () => {
  it("scopes to the URL property and unit and states the period", () => {
    const us = [unit("u1"), unit("u2", { propertyId: "office" }), unit("u3", { archived: true })];
    expect(overviewScope(us, props, {})).toMatchObject({ propertyId: null, unitId: null, propertyName: null });
    expect(overviewScope(us, props, {}).scope.map((u) => u.id)).toEqual(["u1", "u2"]);
    const s = overviewScope(us, props, { propertyId: "office", unitId: "u1" }); // a unit of another property is ignored
    expect([s.propertyName, s.unitId, s.scope.map((u) => u.id)]).toEqual(["Office A", null, ["u2"]]);
    expect(overviewScope(us, props, { propertyId: "nope", unitId: "u2" }).scope.map((u) => u.id)).toEqual(["u2"]);
    expect([periodOf("7d"), periodOf("x"), periodOf(undefined)]).toEqual(["7d", "today", "today"]);
    expect(periodNote("today", { from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:12:00Z" })).toBe("Today = 00:00–09:12 · Asia/Kuala_Lumpur");
    expect(previousRange({ from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:12:00Z" })).toEqual({ from: "2026-09-13T06:48:00.000Z", to: "2026-09-13T16:00:00Z" });
  });

  it("shows the measured room temperature, humidity and power — never the AC setting", () => {
    const [r] = overviewRows([unit("u1", { latestMeasurements: [m("temperature", 28), m("humidity", 60.04), m("power", 0.68)] })], () => "Home A › Bedroom");
    expect([r.temp, r.hum, r.power, r.observed, r.state, r.place]).toEqual(["28.0", "60.0", "680", "09:12", "running", "Home A › Bedroom"]);
    const [stale] = overviewRows([unit("u2", { latestMeasurements: [m("temperature", 30, { quality: "stale" })], connection: "offline", effectivePowerState: "unknown", lastSeenAt: "2026-09-12T10:40:00Z" })], () => "");
    expect([stale.temp, stale.hum, stale.power, stale.observed, stale.state, stale.conn]).toEqual([null, null, null, "last seen Sep 12 18:40", "unknown", "offline"]);
  });

  it("compares the energy of the period and keeps missing values apart from 0", () => {
    const e = energyCard("7d", sum(18.6), sum(20.2), [{ label: "Mon", kWh: 3 }, { label: "Tue", kWh: null }], [2, 2.5], "all 5 units");
    expect([e.sub, e.total, e.delta, e.labels, e.series, e.gaps]).toEqual(["Last 7 days · all 5 units", "18.6", { text: "↓ 8% vs the 7 days before (20.2 kWh)", tone: "ok" }, ["Mon", "Tue"], [[2, 2.5], [3, 0]], true]);
    expect(energyCard("today", sum(null), sum(1), [], [], "AC u1")).toMatchObject({ total: null, delta: null, sub: "Today · AC u1" });
    expect(energyCard("today", sum(2), sum(1), [], [], "x").delta).toEqual({ text: "↑ 100% vs yesterday at this time (1.0 kWh)", tone: "warn" });
    expect(emissionsCard(sum(10, { emissionsKg: 13.2, savedEmissionsKg: 2.1 }), "demo_fixed")).toEqual({ total: "13.2", factor: "MY 2026 · 0.7 kgCO₂e/kWh (demo factor)", baseline: "15.3 kgCO₂e (demo_fixed)", saved: "2.1 kgCO₂e" });
    expect(emissionsCard(sum(null, {}, false), null)).toEqual({ total: null, factor: null, baseline: null, saved: null });
  });

  it("lists the unresolved warnings and counts reminders separately (IR51)", () => {
    const us = [unit("u1", { displayName: "Bedroom AC" }), unit("u2", { displayName: "Kitchen AC" })];
    const card = attentionCard([
      alert("a1", "u1", "warning", "open", "2026-09-14T01:12:00Z", { causeCode: "window_open" }), alert("a2", "u2", "critical", "acknowledged", "2026-09-13T14:40:00Z"),
      alert("a3", "u1", "normal", "open", "2026-09-10T01:00:00Z", { type: "maintenance" }), alert("a4", "u1", "warning", "resolved", "2026-09-14T00:00:00Z"), alert("a5", "x", "critical", "open", "2026-09-14T00:00:00Z"),
    ], us, NOW);
    expect(card.items).toEqual([{ id: "a2", title: "Sensor alert", severity: "critical", where: "Kitchen AC · Yesterday 22:40" }, { id: "a1", title: "Possible open window", severity: "warning", where: "Bedroom AC · 09:12 today" }]);
    expect([card.more, card.info]).toEqual([0, { count: 1, text: "filter cleaning reminder" }]);
    expect([whenText("2026-09-10T01:00:00Z", NOW), nextRunText("2026-09-14T10:00:00Z", NOW), nextRunText("2026-09-14T23:30:00Z", NOW), nextRunText("2026-09-20T10:00:00Z", NOW)])
      .toEqual(["Sep 10 09:00", "today 18:00", "tomorrow 07:30", "Sep 20 18:00"]);
  });

  it("averages the valid CO2 readings per hour of the last 24 h", () => {
    const pts = hourlyPoints([m("co2", 900, { observedAt: "2026-09-14T00:40:00Z" }), m("co2", 1100, { observedAt: "2026-09-14T00:50:00Z" }), m("co2", 5000, { observedAt: "2026-09-14T00:55:00Z", quality: "suspect" }), m("co2", 700, { observedAt: "2026-09-12T00:00:00Z" })], NOW);
    expect([pts.length, pts[23], pts.filter((p) => p !== null).length]).toEqual([24, 1000, 1]);
  });
});
