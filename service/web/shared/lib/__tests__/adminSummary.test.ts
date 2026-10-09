import { describe, expect, it } from "vitest";
import { asOfText, axisRows, billingView, forecastView, jobRows, kpisFrom, overviewLinks, todayRange, type AdminSummary, type ApiForecast } from "@ac/web/lib/adminSummary";

const summary = (over: Partial<AdminSummary> = {}): AdminSummary => ({
  customerCount: 2, total: 5, online: 5, offline: 0, unknown: 0, powerOn: 2, powerOff: 2, powerUnknown: 1, operatingRate: 50, alertCount: 1,
  jobCounts: { requested: 1, assigned: 1 }, overdueInvoiceCount: 1, amountsByCurrency: [{ amountMinor: 12000, currency: "MYR" }], billingVisibility: "allowed",
  energySummary: { totals: { kWh: 9, amountMinor: 329 }, coverage: 0.4 }, energyForecast: null, asOf: "2026-09-14T01:00:00Z", ...over,
});
const forecast = (over: Partial<ApiForecast> = {}): ApiForecast => ({
  period: { from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:00:00Z" }, unitIds: ["u1"], baselineRef: { id: "baseline-demo-tenant-a", version: 1 },
  baselineSnapshot: { id: "baseline-demo-tenant-a", version: 1, method: "demo_fixed", baselineKWh: 2592, period: { from: "", to: "" } },
  expectedUnitMinutes: 2700, validUnitMinutes: 1080, actualKWhOnValidSlots: 9, predictedBaselineKWh: 32.4, predictedActualKWh: 22.5,
  forecastSavedKWh: 9.9, forecastSavingPercentage: 30.6, qualityWarnings: ["modeled_baseline", "partial_coverage", "prorated_forecast"], ...over,
});

describe("HQ overview sections (DD-A01, IR244)", () => {
  it("states the time and period, and links lists with the scope but not the period", () => {
    expect(asOfText("2026-09-14T01:00:00Z", todayRange(new Date("2026-09-14T01:00:00Z")))).toBe("As of 09:00 MYT · 2026-09-14 00:00–09:00 (Asia/Kuala_Lumpur)");
    expect(asOfText("2026-09-14T01:00:00Z", { from: "2026-09-07T16:00:00Z", to: "2026-09-14T01:00:00Z" })).toBe("As of 09:00 MYT · 2026-09-08 00:00 – 2026-09-14 09:00 (Asia/Kuala_Lumpur)");
    const l = overviewLinks({ customerId: "c1", propertyId: null });
    expect([l.units, l.power("off"), l.connection("connecting,error,unknown"), l.jobs("assigned"), l.billing, overviewLinks({ customerId: null, propertyId: null }).units])
      .toEqual(["/admin/units?customerId=c1", "/admin/units?customerId=c1&powerState=off", "/admin/units?customerId=c1&connections=connecting%2Cerror%2Cunknown", "/admin/jobs?customerId=c1&stage=assigned", "/admin/billing?overdueOnly=true", "/admin/units"]);
  });

  it("shows the forecast with its direction, or why there is none (IR78)", () => {
    const f = forecastView(forecast());
    expect(f).toEqual({
      state: "ok", saved: { text: "Expected reduction 9.9 kWh", tone: "ok" }, rate: { text: "Expected reduction 30.6%", tone: "ok" },
      baseline: { value: "32.4 kWh", sub: "baseline v1 · demo_fixed · 2,592 kWh" }, actual: { value: "22.5 kWh", sub: "9.0 kWh on valid slots ÷ 1,080 × 2,700 unit-min" },
      coverage: "1,080 / 2,700 unit-min", warnings: ["modeled_baseline", "partial_coverage", "prorated_forecast"],
    });
    expect(forecastView(forecast({ forecastSavedKWh: -2.5, forecastSavingPercentage: -8.1 }))).toMatchObject({ saved: { text: "Expected increase 2.5 kWh", tone: "crit" }, rate: { text: "Expected increase 8.1%" } });
    expect(forecastView(forecast({ forecastSavedKWh: 0, forecastSavingPercentage: 0 }))).toMatchObject({ saved: { text: "No change 0.0", tone: "muted" } });
    expect(forecastView(forecast({ forecastSavedKWh: null, predictedBaselineKWh: null, qualityWarnings: ["baseline_unavailable"] }))).toEqual({ state: "none", text: "Baseline not set", warnings: ["baseline_unavailable"] });
    expect(forecastView(forecast({ forecastSavedKWh: null, qualityWarnings: ["no_units"] }))).toMatchObject({ text: "No target equipment" });
    expect(forecastView(null)).toEqual({ state: "none", text: "Cannot calculate", warnings: [] });
  });

  it("splits the power and connection axes, lists all job statuses and keeps currencies apart", () => {
    const s = summary();
    const a = axisRows(s, overviewLinks({ customerId: null, propertyId: null }));
    expect(a.power.map((r) => [r.label.split(" ")[0], r.count, r.pct])).toEqual([["Running", 2, 40], ["Stopped", 2, 40], ["Unknown", 1, 20]]);
    expect(a.connection.map((r) => r.count)).toEqual([5, 0, 0]);
    expect(a.rateNote).toMatch(/= 2 ÷ 4 = 50\.0%/);
    const jobs = jobRows(s.jobCounts, overviewLinks({ customerId: null, propertyId: null }));
    expect([jobs.length, jobs[0], jobs.at(-1)]).toEqual([10, { status: "requested", count: 1, href: "/admin/jobs?stage=requested" }, { status: "cancelled", count: 0, href: "/admin/jobs" }]);
    expect(billingView(summary({ amountsByCurrency: [{ amountMinor: 12000, currency: "MYR" }, { amountMinor: 5000, currency: "USD" }] }))).toEqual({ forbidden: false, overdue: 1, rows: [{ currency: "MYR", text: "120.00 MYR" }, { currency: "USD", text: "50.00 USD" }] });
    expect(billingView(summary({ billingVisibility: "forbidden", amountsByCurrency: null, overdueInvoiceCount: null }))).toEqual({ forbidden: true });
    expect(kpisFrom(s)).toMatchObject({ rate: "50.0%", kwh: "9.0 kWh", billing: "120.00 MYR", jobs: 2 });
  });
});
