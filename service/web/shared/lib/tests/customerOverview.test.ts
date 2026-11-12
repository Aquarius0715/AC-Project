import { describe, expect, it } from "vitest";
import {
  attentionCard, bucketPoints, bucketTimes, emissionsCard, energyCard, nextRunText, overviewRows, overviewScope, periodNote, periodOf, previousRange, whenText, type OverviewUnit,
} from "@ac/web/lib/customerOverview";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiEnergySummary } from "@ac/web/lib/energy";
import { i18nOf } from "@ac/web/lib/i18n";

const TEMPERATURE = "temperature";
const TEXT_2026_09_14T01_12_00Z = "2026-09-14T01:12:00Z";
const TEXT_2026_09_13T16_00_00Z = "2026-09-13T16:00:00Z";

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const UNIT_OF = new Map([["power", "kW"], [TEMPERATURE, "°C"]]); // humidity in %

const NOW = Date.parse("2026-09-14T01:12:30Z"); // Mon 09:12:30 KL
const m = (metric: string, value: number | null, over: Record<string, unknown> = {}) => {
  return {
    id: metric, unitId: "u1", sensorId: "s", metric, value, unit: UNIT_OF.get(metric) ?? "%", observedAt: TEXT_2026_09_14T01_12_00Z, origin: "measured" as const, quality: "valid" as const, qualityReason: null, ...over,
  };
};
const unit = (id: string, over: Partial<OverviewUnit> = {}): OverviewUnit => {
  return {
    id, version: 1, customerOrgId: "o", propertyId: "home", spaceId: null, displayName: `AC ${id}`, modelId: "m", type: "split", installState: "installed", installedAt: null, serviceScope: ["indoor"], alertPolicyIds: [],
    archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: TEXT_2026_09_14T01_12_00Z, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, latestMeasurements: [], ...over,
  };
};
const props = [{ id: "home", version: 1, customerOrgId: "o", kind: "home" as const, name: "Home A", address: null, accessInstructions: null, archived: false, updatedAt: "" },
  { id: "office", version: 1, customerOrgId: "o", kind: "office" as const, name: "Office A", address: null, accessInstructions: null, archived: false, updatedAt: "" }];
const sum = (kWh: number | null, over: Partial<ApiEnergySummary["totals"]> = {}, factor = true) => {
  return ({
    period: { from: "", to: "" }, unitIds: [], currency: "MYR", baselineRef: null, factorRef: null, baselineSnapshot: null, tariffVersion: "t", boundaryId: "ac_input_electricity", boundary: "", coverage: 1, qualityWarnings: [],
    totals: { kWh, amountMinor: null, savedKWh: null, deltaKWh: null, savingPercentage: null, savedAmountMinor: null, emissionsKg: kWh === null ? null : kWh * 0.7, savedEmissionsKg: null, ...over },
    factorSnapshot: factor ? { id: "f", version: 1, region: "MY", year: 2026, kgCO2ePerKWh: 0.7, source: "demo" } : null,
  } as ApiEnergySummary);
};
const alert = (id: string, unitId: string, severity: ApiAlert["severity"], status: string, detectedAt: string, over: Partial<ApiAlert> = {}): ApiAlert =>
  ({ id, unitId, type: "sensor", severity, status, causeCode: "unknown", evidenceKind: "inferred", evidenceText: "", detectedAt, ...over });

describe("customer overview", () => {
  it("scopes to the URL property and unit and states the period", () => {
    const us = [unit("u1"), unit("u2", { propertyId: "office" }), unit("u3", { archived: true })];
    expect(overviewScope(us, props, {})).toMatchObject({ propertyId: null, unitId: null, propertyName: null });
    expect(overviewScope(us, props, {}).scope.map((u) => u.id)).toStrictEqual(["u1", "u2"]);
    const s = overviewScope(us, props, { propertyId: "office", unitId: "u1" }); // a unit of another property is ignored
    expect([s.propertyName, s.unitId, s.scope.map((u) => u.id)]).toStrictEqual(["Office A", null, ["u2"]]);
    expect(overviewScope(us, props, { propertyId: "nope", unitId: "u2" }).scope.map((u) => u.id)).toStrictEqual(["u2"]);
    expect([periodOf("7d"), periodOf("x"), periodOf()]).toStrictEqual(["7d", "today", "today"]);
    expect(periodNote("today", { from: TEXT_2026_09_13T16_00_00Z, to: TEXT_2026_09_14T01_12_00Z })).toBe("Today = 00:00–09:12 · Asia/Kuala_Lumpur");
    // the periods are Kuala Lumpur days, so the note stays in Kuala Lumpur time in either language
    expect(periodNote("7d", { from: "2026-09-07T16:00:00Z", to: TEXT_2026_09_14T01_12_00Z }, MS_TOKYO)).toBe("7 hari lepas = 8 Sep 00:00 – 14 Sep 09:12 · Asia/Kuala_Lumpur");
    // today is compared with yesterday at this time, the same hours a day earlier (IR726): 00:00–09:12 yesterday, not
    // the 9 h 12 min before midnight; 7 and 30 days with the days just before them
    expect(previousRange("today", { from: TEXT_2026_09_13T16_00_00Z, to: TEXT_2026_09_14T01_12_00Z })).toStrictEqual({ from: "2026-09-12T16:00:00.000Z", to: "2026-09-13T01:12:00.000Z" });
    expect(previousRange("7d", { from: "2026-09-07T16:00:00Z", to: TEXT_2026_09_14T01_12_00Z })).toStrictEqual({ from: "2026-09-01T06:48:00.000Z", to: "2026-09-07T16:00:00Z" });
  });

  it("shows the measured room temperature, humidity and power — never the AC setting", () => {
    const [r] = overviewRows([unit("u1", { latestMeasurements: [m(TEMPERATURE, 28), m("humidity", 60.04), m("power", 0.68)] })], () => "Home A › Bedroom");
    expect([r?.temp, r?.hum, r?.power, r?.observed, r?.state, r?.place]).toStrictEqual(["28.0", "60.0", "680", "9:12 am MYT", "running", "Home A › Bedroom"]);
    const [stale] = overviewRows([unit("u2", { latestMeasurements: [m(TEMPERATURE, 30, { quality: "stale" })], connection: "offline", effectivePowerState: "unknown", lastSeenAt: "2026-09-12T10:40:00Z" })], () => "");
    expect([stale?.temp, stale?.hum, stale?.power, stale?.observed, stale?.state, stale?.conn]).toStrictEqual([null, null, null, "last seen 12 Sept 2026, 6:40 pm MYT", "unknown", "offline"]);
    // the user's language and display time zone (IR260)
    expect(overviewRows([unit("u3", { connection: "offline", lastSeenAt: "2026-09-12T10:40:00Z" })], () => "", MS_TOKYO)[0]?.observed).toBe("kali terakhir dilihat 12 Sep 2026, 7:40 PTG GMT+9");
    expect(overviewRows([unit("u4", { connection: "offline", lastSeenAt: null })], () => "", MS_TOKYO)[0]?.observed).toBe("kali terakhir dilihat tidak pernah");
  });

  it("compares the energy of the period and keeps missing values apart from 0", () => {
    const e = energyCard("7d", sum(18.6), sum(20.2), [{ label: "Mon", kWh: 3 }, { label: "Tue", kWh: null }], [2, 2.5], "all 5 units");
    expect([e.sub, e.total, e.delta, e.labels, e.series, e.gaps]).toStrictEqual(["Last 7 days · all 5 units", "18.6", { text: "8% vs the 7 days before (20.2 kWh)", icon: "arrowDown", tone: "ok" }, ["Mon", "Tue"], [[2, 2.5], [3, 0]], true]);
    expect(energyCard("today", sum(null), sum(1), [], [], "AC u1")).toMatchObject({ total: null, delta: null, sub: "Today · AC u1" });
    expect(energyCard("today", sum(2), sum(1), [], [], "x").delta).toStrictEqual({ text: "100% vs yesterday at this time (1.0 kWh)", icon: "arrowUp", tone: "warn" });
    expect(energyCard("7d", sum(18.6), sum(20.2), [], [], "semua 5 unit", MS_TOKYO)).toMatchObject({ sub: "7 hari lepas · semua 5 unit", delta: { text: "8% berbanding 7 hari sebelumnya (20.2 kWh)", icon: "arrowDown", tone: "ok" } });
    expect(emissionsCard(sum(10, { emissionsKg: 13.2, savedEmissionsKg: 2.1 }), "demo_fixed")).toStrictEqual({ total: "13.2", factor: "MY 2026 · 0.7 kgCO₂e/kWh (demo factor)", baseline: "15.3 kgCO₂e (demo_fixed)", saved: "2.1 kgCO₂e" });
    expect(emissionsCard(sum(null, {}, false), null)).toStrictEqual({ total: null, factor: null, baseline: null, saved: null });
  });

  it("lists the unresolved warnings and counts reminders separately (IR51)", () => {
    const us = [unit("u1", { displayName: "Bedroom AC" }), unit("u2", { displayName: "Kitchen AC" })];
    const card = attentionCard([
      alert("a1", "u1", "warning", "open", TEXT_2026_09_14T01_12_00Z, { causeCode: "window_open" }), alert("a2", "u2", "critical", "acknowledged", "2026-09-13T14:40:00Z"),
      alert("a3", "u1", "normal", "open", "2026-09-10T01:00:00Z", { type: "maintenance" }), alert("a4", "u1", "warning", "resolved", "2026-09-14T00:00:00Z"), alert("a5", "x", "critical", "open", "2026-09-14T00:00:00Z"),
    ], us, NOW);
    expect(card.items).toStrictEqual([{ id: "a2", title: "Sensor alert", severity: "critical", where: "Kitchen AC · yesterday 10:40 pm MYT" }, { id: "a1", title: "Possible open window", severity: "warning", where: "Bedroom AC · today 9:12 am MYT" }]);
    expect([card.more, card.info]).toStrictEqual([0, { count: 1, text: "filter cleaning reminder" }]);
    expect([whenText("2026-09-10T01:00:00Z", NOW), nextRunText("2026-09-14T10:00:00Z", NOW), nextRunText("2026-09-14T23:30:00Z", NOW), nextRunText("2026-09-20T10:00:00Z", NOW)])
      .toStrictEqual(["10 Sept 2026, 9:00 am MYT", "today 6:00 pm MYT", "tomorrow 7:30 am MYT", "20 Sept 2026, 6:00 pm MYT"]);
    // in Tokyo (UTC+9) the alert of 22:40 KL is 23:40 yesterday, and 23:30 UTC is 08:30 tomorrow
    const ms = attentionCard([alert("a2", "u2", "critical", "open", "2026-09-13T14:40:00Z")], us, NOW, 3, MS_TOKYO);
    expect(ms.items[0]).toMatchObject({ title: "Amaran penderia", where: "Kitchen AC · semalam 11:40 PTG GMT+9" });
    expect([nextRunText("2026-09-14T10:00:00Z", NOW, MS_TOKYO), nextRunText("2026-09-14T23:30:00Z", NOW, MS_TOKYO)]).toStrictEqual(["hari ini 7:00 PTG GMT+9", "esok 8:30 PG GMT+9"]);
  });

  it("plots the hourly averages of the last 24 h, rounded, an hour without a valid reading empty (IR607)", () => {
    const sample = new Map([[23, 1000.4], [5, 712.6]]); // two hours with a value, the others without
    const hours = Array.from({ length: 24 }, (_, k) => ({ value: sample.get(k) ?? null }));
    const pts = bucketPoints(hours);
    expect([pts.length, pts[23], pts[5], pts.filter((p) => p !== null).length]).toStrictEqual([24, 1000, 713, 2]);
  });
});

// IR689: the hour of each point of the air card heads its tooltip, in the display time zone
describe("air card tooltip headings", () => {
  it("gives each hourly point the hour it averages", () => {
    expect(bucketTimes([{ start: "2026-09-14T01:00:00.000Z" }, { start: "2026-09-14T02:00:00.000Z" }])).toStrictEqual(["Mon, 14 Sept, 9:00 – 10:00 am MYT", "Mon, 14 Sept, 10:00 – 11:00 am MYT"]);
  });
});
