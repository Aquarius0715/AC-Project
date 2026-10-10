import { describe, expect, it } from "vitest";
import { asOfText, axisRows, billingView, forecastView, jobRows, kpisFrom, overviewLinks, todayRange, type AdminSummary, type ApiForecast } from "@ac/web/lib/adminSummary";
import { periodRange } from "@ac/web/lib/clientEnergy";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

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
    const today = periodRange("today", new Date("2026-09-14T01:00:00Z"));
    expect(asOfText("2026-09-14T01:00:00Z", today.label)).toBe(`As of 9:00 am MYT · ${today.label}`);
    expect(today.label).toMatch(/\(1 day\) · Asia\/Kuala_Lumpur$/); // the period stays Kuala Lumpur days, named so
    expect(todayRange(new Date("2026-09-14T01:00:00Z"))).toEqual({ from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:00:00.000Z" });
    const l = overviewLinks({ customerId: "c1", propertyId: null });
    expect([l.units, l.power("off"), l.connection("connecting,error,unknown"), l.jobs("assigned"), l.billing, overviewLinks({ customerId: null, propertyId: null }).units])
      .toEqual(["/admin/units?customerId=c1", "/admin/units?customerId=c1&powerState=off", "/admin/units?customerId=c1&connections=connecting%2Cerror%2Cunknown", "/admin/jobs?customerId=c1&stage=assigned", "/admin/billing?overdueOnly=true", "/admin/units"]);
    const withPeriod = overviewLinks({ customerId: null, propertyId: null, period: { from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:00:00.000Z" } });
    expect([withPeriod.jobs("completed"), withPeriod.units]).toEqual(["/admin/jobs?stage=completed&from=2026-09-13T16%3A00%3A00.000Z&to=2026-09-14T01%3A00%3A00.000Z", "/admin/units"]);
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
    expect([jobs.length, jobs[0], jobs.at(-1)]).toEqual([10, { status: "requested", label: "requested", count: 1, href: "/admin/jobs?stage=requested" }, { status: "cancelled", label: "cancelled", count: 0, href: "/admin/jobs" }]);
    expect(billingView(summary({ amountsByCurrency: [{ amountMinor: 12000, currency: "MYR" }, { amountMinor: 5000, currency: "USD" }] }))).toEqual({ forbidden: false, overdue: 1, rows: [{ currency: "MYR", text: "120.00 MYR" }, { currency: "USD", text: "50.00 USD" }] });
    expect(billingView(summary({ billingVisibility: "forbidden", amountsByCurrency: null, overdueInvoiceCount: null }))).toEqual({ forbidden: true });
    expect(kpisFrom(s)).toMatchObject({ rate: "50.0%", kwh: "9.0 kWh", billing: "120.00 MYR", jobs: 2, kwhSub: "measured · coverage 40% · cost 3.29 MYR", billingSub: "1 overdue invoice · unpaid, per currency" });
  });
});

describe("HQ overview in Malay with the display time zone (IR289)", () => {
  it("words the KPIs, the forecast, the axes and the job statuses", () => {
    const s = summary({ overdueInvoiceCount: 2 });
    expect(asOfText("2026-09-14T01:00:00Z", "P", MS)).toBe("Setakat 10:00 PG GMT+9 · P");
    expect(kpisFrom(s, MS.t)).toMatchObject({ kwhSub: "diukur · liputan 40% · kos 3.29 MYR", billingSub: "2 invois tertunggak · belum dibayar, mengikut mata wang" });
    expect(forecastView(forecast(), MS.t)).toMatchObject({ saved: { text: "Jangkaan pengurangan 9.9 kWh" }, coverage: "1,080 / 2,700 unit-min" });
    expect(forecastView(null, MS.t)).toMatchObject({ text: "Tidak dapat dikira" });
    const a = axisRows(s, overviewLinks({ customerId: null, propertyId: null }), MS.t);
    expect([a.power[0].label, a.connection[1].label]).toEqual(["Berjalan", "Luar talian"]);
    expect(jobRows(s.jobCounts, overviewLinks({ customerId: null, propertyId: null }), MS.t).slice(0, 2).map((j) => j.label)).toEqual(["diminta", "ditawarkan"]);
  });
});

describe("HQ Jobs tab period (IR245)", () => {
  it("passes a valid requested-time period to jobs.list and names it on the chip", async () => {
    const { filtersOf, periodChip, periodOfQuery } = await import("@ac/web/lib/adminJobs");
    const q = { from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:00:00.000Z", customerId: "c1" };
    expect(filtersOf(q, null)).toEqual({ customerId: "c1", from: q.from, to: q.to });
    expect([periodOfQuery({ from: q.to, to: q.from }), periodOfQuery({ from: "x", to: q.to }), periodOfQuery({})]).toEqual([null, null, null]);
    expect(periodChip(periodOfQuery(q)!)).toBe("Requested time 09-14 00:00 – 09-14 09:00");
  });
});
