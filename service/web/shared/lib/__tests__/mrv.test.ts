import { describe, expect, it } from "vitest";
import type { ApiEnergySummary } from "@ac/web/lib/energy";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import {
  factorErrors, factorInput, mrvConditions, mrvDraftErrors, mrvDraftOf, mrvView, reportRows, reviewItems, statusWord, versionKey,
  type ApiMRVReport, type FactorDraft, type MRVDraft,
} from "@ac/web/lib/mrv";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const summary: ApiEnergySummary = {
  period: { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z" }, unitIds: ["u1", "u2"],
  totals: { kWh: 120.25, amountMinor: 6012, savedKWh: -14.75, deltaKWh: 14.75, savingPercentage: -10.93, savedAmountMinor: -738, emissionsKg: 70.95, savedEmissionsKg: -8.7 },
  currency: "MYR", baselineRef: { id: "b1", version: 2 }, factorRef: { id: "f1", version: 1 },
  baselineSnapshot: {
    id: "b1", version: 2, createdAt: "2026-08-01T00:00:00Z", unitIds: ["u1", "u2"], period: { from: "2026-08-24T16:00:00Z", to: "2026-08-31T16:00:00Z" },
    method: "demo_fixed", baselineKWh: 105.5, quality: { kind: "modeled", coverage: null }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "Same week", source: "demo",
  },
  factorSnapshot: { id: "f1", version: 1, region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo (fictional)" },
  tariffVersion: "tariff-demo-1", boundaryId: "ac_input_electricity", boundary: "AC input", coverage: 1, qualityWarnings: ["modeled_baseline"],
};
const report: ApiMRVReport = {
  id: "r1234567-aaaa", version: 2, createdAt: "2026-09-08T01:00:00Z", status: "draft", evidenceIds: [], scope: "scope_2", isDemo: true, incomplete: false, summary,
  conditions: { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z", unitIds: ["u1", "u2"], baselineId: "b1", baselineVersion: 2, factorId: "f1", factorVersion: 1, boundaryId: "ac_input_electricity", boundary: "AC input", organizationId: "org-a" },
  reviewHistory: [{ userId: "user-hq-0001", reportVersion: 1, comment: "Looks consistent", at: "2026-09-14T01:30:00Z" }],
};
const orgs = [{ id: "org-a", name: "Demo Customer A" }];
const units = [{ id: "u1", label: "Lobby AC" }, { id: "u2", label: "Bedroom AC" }];
const draft = (over: Partial<MRVDraft>): MRVDraft => ({
  organizationId: "org-a", unitIds: ["u1"], from: "2026-09-01T00:00", to: "2026-09-08T00:00", baseline: versionKey("b1", 2), factor: versionKey("f1", 1), boundaryId: "ac_input_electricity", boundary: " AC input ", ...over,
});

describe("HQ digital MRV (FR-A14, DD-A14)", () => {
  it("lists reports with their Kuala Lumpur period and result, or says the calculation is incomplete", () => {
    const rows = reportRows([report, { ...report, id: "r2", conditions: { ...report.conditions, organizationId: "org-x", unitIds: ["u1"] }, incomplete: true }], orgs);
    expect(rows.map((r) => [r.org, r.units, r.period, r.result])).toEqual([
      ["Demo Customer A", "2 units", "2026-09-01 00:00 → 2026-09-08 00:00", "71.0 kgCO₂e"], ["customer", "1 unit", "2026-09-01 00:00 → 2026-09-08 00:00", "Calculation incomplete"],
    ]);
  });

  it("keeps energy and emissions vs the baseline apart with the IR68 wording, and shows the stored conditions", () => {
    const v = mrvView(report, orgs, units);
    expect([v.incomplete, v.electricity, v.emissions, v.energyVsBaseline, v.energyPercent, v.emissionsVsBaseline]).toEqual([false, "120.3 kWh", "71.0 kgCO₂e", "Increase 14.8 kWh", "Increase 10.9%", "Increase 8.7 kgCO₂e"]);
    expect(v.conditions).toEqual([
      ["Organization", "Demo Customer A"], ["Units", "Lobby AC, Bedroom AC"], ["Period", "2026-09-01 00:00 → 2026-09-08 00:00 (Asia/Kuala_Lumpur)"], ["Boundary", "ac_input_electricity — AC input"],
      ["Baseline", "v2 · demo_fixed · 105.5 kWh"], ["Emission factor", "v1 · Demo region · 2026 · 0.59 kgCO₂e/kWh"], ["Factor source", "Demo (fictional)"], ["Coverage", "100.0%"],
    ]);
    const missing = mrvView({ ...report, summary: { ...summary, factorSnapshot: null, totals: { ...summary.totals, emissionsKg: null } } }, orgs, units);
    expect([missing.incomplete, missing.emissions, missing.conditions[5]]).toEqual([true, "Calculation incomplete", ["Emission factor", "Calculation incomplete — factor missing"]]);
    expect(reviewItems(report)).toEqual([{ time: "14 Sept 2026, 9:30 am MYT", title: "“Looks consistent”", detail: "version 1 · user-hq-" }]);
  });

  it("checks a new report and turns its Kuala Lumpur period into instants", () => {
    expect(mrvDraftErrors(draft({}))).toEqual({});
    expect(mrvDraftErrors(draft({ organizationId: "", unitIds: [], to: "2026-09-01T00:00", baseline: "", factor: "", boundary: " " }))).toEqual({
      organizationId: "Choose a customer organization", unitIds: "Choose 1–100 units", period: "The end must be after the start",
      baseline: "Choose a baseline version", factor: "Choose an emission factor version", boundary: "1–500 characters",
    });
    const c = mrvConditions(draft({}));
    expect([c.from, c.to, c.baselineId, c.baselineVersion, c.factorId, c.factorVersion, c.boundary]).toEqual(["2026-08-31T16:00:00.000Z", "2026-09-07T16:00:00.000Z", "b1", 2, "f1", 1, "AC input"]);
    expect(mrvDraftOf(report.conditions)).toEqual(draft({ boundary: "AC input", unitIds: ["u1", "u2"] }));
    expect(mrvDraftOf().organizationId).toBe("");
  });

  it("checks a demo emission factor (IR102: the unit is fixed)", () => {
    const f: FactorDraft = { region: " Demo region ", year: "2026", kgCO2ePerKWh: "0.59", source: " Demo (fictional) " };
    expect(factorErrors(f)).toEqual({});
    expect(factorInput(f, "f1")).toEqual({ id: "f1", region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo (fictional)", isDemo: true });
    expect(factorErrors({ region: "", year: "1999", kgCO2ePerKWh: "0", source: "" })).toEqual({
      region: "1–120 characters", year: "A year 2000–2100", kgCO2ePerKWh: "Above 0 and at most 10 kgCO₂e/kWh", source: "1–500 characters, stating the demo source",
    });
  });
});

describe("HQ digital MRV in Malay with the display time zone (IR297)", () => {
  it("words the rows, figures, conditions, statuses and checks; review times are in the display zone", () => {
    const t = translator("ms");
    expect(reportRows([{ ...report, incomplete: true }], [], t).map((r) => [r.org, r.units, r.result])).toEqual([["pelanggan", "2 unit", "Pengiraan tidak lengkap"]]);
    const v = mrvView(report, orgs, units, MS);
    expect([v.energyVsBaseline, v.emissionsVsBaseline, v.conditions.map(([k]) => k)]).toEqual([
      "Peningkatan 14.8 kWh", "Peningkatan 8.7 kgCO₂e", ["Organisasi", "Unit", "Tempoh", "Sempadan", "Garis dasar", "Faktor pelepasan", "Sumber faktor", "Liputan"],
    ]);
    expect(v.conditions[2][1]).toBe("2026-09-01 00:00 → 2026-09-08 00:00 (Asia/Kuala_Lumpur)"); // the period stays Kuala Lumpur time, named so
    expect([statusWord("draft", t), statusWord("demo_reviewed", t), statusWord("other", t)]).toEqual(["draf", "disemak (demo)", "other"]);
    expect(reviewItems(report, MS)).toEqual([{ time: "14 Sep 2026, 10:30 PG GMT+9", title: "“Looks consistent”", detail: "versi 1 · user-hq-" }]);
    expect(mrvDraftErrors(draft({ factor: "" }), t)).toEqual({ factor: "Pilih versi faktor pelepasan" });
    expect(factorErrors({ region: "x", year: "2026", kgCO2ePerKWh: "11", source: "demo" }, t)).toEqual({ kgCO2ePerKWh: "Melebihi 0 dan paling banyak 10 kgCO₂e/kWh" });
  });
});
