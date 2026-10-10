import { describe, expect, it } from "vitest";
import { compareRow, dayRanges, periodRange, pickUnits, selectionQuery } from "@ac/web/lib/clientEnergy";
import { amount, one, roundAway, saving, summaryView, warning, type ApiEnergySummary } from "@ac/web/lib/energy";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const NOW = new Date("2026-09-14T01:30:20Z"); // Mon 09:30 KL
const summary: ApiEnergySummary = {
  period: { from: "2026-09-07T16:00:00Z", to: "2026-09-14T01:30:00Z" }, unitIds: ["u1", "u2"],
  totals: { kWh: 120.25, amountMinor: 6012, savedKWh: 14.75, deltaKWh: -14.75, savingPercentage: 10.93, savedAmountMinor: 738, emissionsKg: 70.95, savedEmissionsKg: 8.7 },
  currency: "MYR", baselineRef: { id: "b1", version: 2 }, factorRef: { id: "f1", version: 1 },
  baselineSnapshot: {
    id: "b1", version: 2, createdAt: "2026-09-01T00:00:00Z", unitIds: ["u1", "u2"], period: { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z" },
    method: "demo_period_comparison", baselineKWh: 135, quality: { kind: "measured", coverage: 1 }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "Same week", source: "demo",
  },
  factorSnapshot: { id: "f1", version: 1, region: "MY-Peninsular", year: 2025, kgCO2ePerKWh: 0.59, source: "demo factor" },
  tariffVersion: "TNB-C1-2026", boundaryId: "ac_input_electricity", boundary: "AC input", coverage: 0.9875, qualityWarnings: ["partial_coverage", "modeled_baseline"],
};
const empty: ApiEnergySummary = {
  ...summary, baselineRef: null, factorRef: null, baselineSnapshot: null, factorSnapshot: null, coverage: null, qualityWarnings: ["factor_missing"],
  totals: { kWh: null, amountMinor: null, savedKWh: null, deltaKWh: null, savingPercentage: null, savedAmountMinor: null, emissionsKg: null, savedEmissionsKg: null },
};

describe("customer energy & cost (FR-C06, FR-C13)", () => {
  it("rounds half away from zero and formats energy and money in en-MY (IR44)", () => {
    expect([roundAway(2.25, 1), roundAway(-2.25, 1), roundAway(1.005, 2), roundAway(0.285, 2), roundAway(0.1 + 0.2, 6), roundAway(1e-7, 1)]).toEqual([2.3, -2.3, 1.01, 0.29, 0.3, 0]);
    expect([Object.is(roundAway(-0.04, 1), 0), roundAway(Infinity, 1)]).toEqual([true, Infinity]);
    expect([one(120.25), one(1234.56), one(-0.04), amount(6012, "MYR"), amount(123456789, "MYR")]).toEqual(["120.3", "1,234.6", "0.0", "60.12 MYR", "1,234,567.89 MYR"]);
  });

  it("words a saving as a reduction or an increase, never a negative saving (IR68)", () => {
    expect([saving(null, " kWh"), saving(0.04, " kWh"), saving(12.34, " kWh"), saving(-3.25, "%")]).toEqual(["Cannot calculate", "No change 0.0 kWh", "Reduction 12.3 kWh", "Increase 3.3%"]);
    const t = translator("ms");
    expect([saving(null, "", undefined, t), saving(0, " kWh", undefined, t), saving(12.34, " kWh", undefined, t), saving(-3.25, "%", undefined, t)])
      .toEqual(["Tidak dapat dikira", "Tiada perubahan 0.0 kWh", "Pengurangan 12.3 kWh", "Peningkatan 3.3%"]);
    expect([warning("partial_coverage"), warning("partial_coverage", t), warning("a_new_code", t)])
      .toEqual(["Partial coverage — some expected readings are missing or invalid", "Liputan separa — sesetengah bacaan yang dijangka tiada atau tidak sah", "a_new_code"]);
  });

  it("shows the figures and the calculation conditions of a summary", () => {
    const v = summaryView(summary);
    expect([v.actual, v.baseline, v.difference, v.percent, v.cost, v.savedCost, v.emissions, v.savedEmissions, v.coverage, v.comparable, v.increase])
      .toEqual(["120.3 kWh", "135.0 kWh", "Reduction 14.8 kWh", "Reduction 10.9%", "60.12 MYR", "Reduction 7.38 MYR", "71.0 kgCO₂e", "Reduction 8.7 kgCO₂e", "98.8%", true, false]);
    expect(v.conditions).toEqual([
      ["Period", "2026-09-08 00:00 → 2026-09-14 09:30 (Asia/Kuala_Lumpur)"], ["Boundary", "ac_input_electricity — AC input"],
      ["Baseline", "v2 · demo_period_comparison · 2026-09-01 00:00 → 2026-09-08 00:00 · 2 units"], ["Baseline assumptions", "Same week"],
      ["Emission factor", "MY-Peninsular 2025 v1 · 0.59 kgCO₂e/kWh · demo factor"], ["Tariff", "TNB-C1-2026"], ["Coverage", "98.8% of expected readings"],
    ]);
    expect(v.warnings).toEqual(["Partial coverage — some expected readings are missing or invalid", "Modeled baseline — a demo assumption, not a measurement"]);
  });

  it("words a summary in Malay and keeps the Kuala Lumpur business period named (REV18-035, IR265)", () => {
    const i = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
    const v = summaryView(summary, i);
    expect([v.difference, v.savedCost, v.conditions.map(([k]) => k)]).toEqual([
      "Pengurangan 14.8 kWh", "Pengurangan 7.38 MYR", ["Tempoh", "Sempadan", "Garis dasar", "Andaian garis dasar", "Faktor pelepasan", "Tarif", "Liputan"],
    ]);
    expect([v.conditions[0][1], v.conditions[2][1], v.conditions[6][1]]).toEqual([
      "2026-09-08 00:00 → 2026-09-14 09:30 (Asia/Kuala_Lumpur)", "v2 · demo_period_comparison · 2026-09-01 00:00 → 2026-09-08 00:00 · 2 unit", "98.8% daripada bacaan yang dijangka",
    ]);
    const none = summaryView(empty, i);
    expect([none.actual, none.baseline, none.difference, none.cost, none.savedCost, none.coverage, none.comparable]).toEqual(["Tiada bacaan sah", "Tiada garis dasar", "Tidak dapat dikira", "—", "Tidak dapat dikira", "—", false]);
    expect([none.conditions[2][1], none.conditions[3], none.warnings]).toEqual(["tiada yang dipilih", ["Faktor pelepasan", "tiada"], ["Faktor pelepasan tiada — pelepasan tidak dapat dikira"]]);
  });

  it("takes whole Kuala Lumpur days up to the current minute and labels them in the display language (DD-C06)", () => {
    expect(periodRange("today", NOW)).toEqual({ kind: "today", from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:30:00.000Z", days: 1, error: undefined, label: "14 Sept, 00:00 – 14 Sept, 09:30 (1 day) · Asia/Kuala_Lumpur" });
    const i = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
    const week = periodRange("7d", NOW, {}, i);
    expect([week.from, week.days, week.label]).toEqual(["2026-09-07T16:00:00.000Z", 7, "8 Sep, 00:00 – 14 Sep, 09:30 (7 hari) · Asia/Kuala_Lumpur"]);
    expect(periodRange("30d", NOW).from).toBe("2026-08-15T16:00:00.000Z");
    expect([periodRange("custom", NOW, { from: "2026-09-10T00:00", to: "2026-09-09T00:00" }, i).error, periodRange("custom", NOW, { from: "2025-01-01T00:00", to: "2026-09-01T00:00" }, i).error])
      .toEqual(["Tamat mesti selepas mula", "Paling lama 366 hari"]);
    const custom = periodRange("custom", NOW, { from: "2026-09-01T00:00" });
    expect([custom.from, custom.to, custom.days, custom.error]).toEqual(["2026-08-31T16:00:00.000Z", "2026-09-14T01:30:00.000Z", 14, undefined]);
  });

  it("splits the period into Kuala Lumpur days named in the display language (Figma Client 04a)", () => {
    const days = dayRanges("2026-09-12T16:00:00Z", "2026-09-14T01:30:00Z");
    expect(days).toEqual([{ label: "Sun 13", from: "2026-09-12T16:00:00Z", to: "2026-09-13T16:00:00.000Z" }, { label: "Mon 14", from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:30:00Z" }]);
    expect(dayRanges("2026-09-12T16:00:00Z", "2026-09-14T01:30:00Z", "ms").map((d) => d.label)).toEqual(["Ahd 13", "Isn 14"]);
    expect(dayRanges("2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z")).toHaveLength(31);
  });

  it("compares a unit with its baseline, or says why it cannot (Figma Client 04b)", () => {
    expect(compareRow("u1", "Lobby", summary)).toEqual({ unitId: "u1", name: "Lobby", actual: "120.3", baseline: "135.0", difference: "Reduction 14.8 kWh · Reduction 10.9%", tone: "ok", coverage: "98.8%" });
    const t = translator("ms");
    const up = compareRow("u1", "Lobby", { ...summary, totals: { ...summary.totals, savedKWh: -2, savingPercentage: -1.5 } }, t);
    expect([up.difference, up.tone]).toEqual(["Peningkatan 2.0 kWh · Peningkatan 1.5%", "warn"]);
    const zero = { ...summary, baselineSnapshot: { ...summary.baselineSnapshot!, baselineKWh: 0 } };
    expect([
      compareRow("u1", "Lobby", empty, t).difference, compareRow("u1", "Lobby", { ...summary, totals: { ...summary.totals, savedKWh: null } }, t).difference, compareRow("u1", "Lobby", zero, t).difference,
    ]).toEqual(["Tiada garis dasar untuk unit ini", "Tidak dapat dikira — garis dasar tidak setanding", "Tidak dapat dikira — garis dasar ialah 0"]);
    expect([compareRow("u1", "Lobby", empty).actual, compareRow("u1", "Lobby", empty).tone]).toEqual(["—", "muted"]);
  });

  it("takes the URL's units, else the first by name on both Energy & cost and Offsets", () => {
    const units = [{ id: "u3", displayName: "Meeting room AC" }, { id: "u1", displayName: "Bedroom AC" }, { id: "u2", displayName: "Lobby AC" }];
    expect([pickUnits(units, undefined), pickUnits(units, "u2,u9,u2,u3"), pickUnits(units, "u9"), pickUnits([], undefined)]).toEqual([["u1"], ["u2", "u3"], ["u1"], []]);
    expect(pickUnits([...units, { id: "u4", displayName: "A" }, { id: "u5", displayName: "B" }], "u1,u2,u3,u4,u5")).toEqual(["u1", "u2", "u3", "u4"]);
  });

  it("carries the selection to the offsets page as URL parameters (FR-C13)", () => {
    expect(selectionQuery({ unitIds: ["u1", "u2"], period: "7d", baselineId: "b1" })).toBe("unitIds=u1%2Cu2&period=7d&baselineId=b1");
    expect(selectionQuery({ unitIds: ["u1"], period: "custom", from: "2026-09-01T00:00", to: "2026-09-08T00:00" })).toBe("unitIds=u1&period=custom&from=2026-09-01T00%3A00&to=2026-09-08T00%3A00");
  });
});
