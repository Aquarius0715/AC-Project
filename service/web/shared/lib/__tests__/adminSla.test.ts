import { describe, expect, it } from "vitest";
import { breachRows, breachText, periodOf, slaCsv, slaRefusal, slaRows, slaTiles, targetErrors, targetsByPlan, type ApiScorecard, type ApiTargetView } from "@ac/web/lib/adminSla";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-14T01:00:00Z");
const target = (planType: ApiTargetView["planType"], over: Partial<ApiTargetView> = {}): ApiTargetView =>
  ({ planType, responseHours: 4, arrivalInWindowPercent: 90, firstTimeFixPercent: 85, version: 0, effectiveFrom: null, state: "default", ...over });
const metrics = { responseWithinTarget: 94, arrivalInWindow: 96, firstTimeFix: 86, averageRating: 4.5, ratingCount: 27, openOverdue: 1 };
const sc = (over: Partial<ApiScorecard> = {}): ApiScorecard => ({
  period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, contractorOrgId: null, totals: metrics,
  customers: [{ ...metrics, customerId: "c-a", planType: "rto", jobCount: 18, status: "on_track" }, { ...metrics, firstTimeFix: 70, customerId: "c-b", planType: "general", jobCount: 9, status: "breached" }],
  breaches: [{ jobId: "job-a11-0000", customerId: "c-b", kind: "response", detail: "response 6 h 10 min vs 4 h" }, { jobId: "job-x-000000", customerId: "c-a", kind: "overdue", detail: "open past its due time" }],
  targets: [target("rto", { arrivalInWindowPercent: 95, version: 2, effectiveFrom: "2026-08-01T00:00:00Z", state: "in_effect" }), target("general"), target("energy"), target("environment"),
    target("general", { responseHours: 2, version: 1, effectiveFrom: "2026-10-01T00:00:00Z", state: "scheduled" })],
  ...over,
});

describe("HQ SLA by customer", () => {
  it("reads the period from the URL key", () => {
    expect(periodOf(undefined, NOW)).toEqual({ id: "90", from: "2026-06-16T01:00:00.000Z", to: "2026-09-14T01:00:00.000Z" });
    expect(periodOf("30", NOW).from).toBe("2026-08-15T01:00:00.000Z");
    expect(periodOf("bogus", NOW).id).toBe("90");
  });

  it("shows the tiles with the targets of the customers' plans", () => {
    const tiles = slaTiles(sc());
    expect(tiles.map((t) => [t.label, t.value, t.sub, t.tone])).toEqual([
      ["Response ≤ 4 h", "94 %", "target 100 % of jobs", "warn"], ["Arrival in window", "96 %", "targets 90–95 % by plan", undefined], ["First-time fix", "86 %", "target 85 %", undefined],
      ["Avg. customer rating", "4.5 ★", "27 ratings", undefined], ["Open & overdue", "1", "2 breaches in the period", "crit"],
    ]);
    const one = slaTiles(sc({ customers: [{ ...metrics, customerId: "c-a", planType: "rto", jobCount: 1, status: "on_track" }], totals: { ...metrics, arrivalInWindow: null, openOverdue: 0 }, breaches: [] }));
    expect([one[1].value, one[1].sub, one[4].sub, one[4].tone]).toEqual(["—", "target 95 %", "0 breaches in the period", undefined]);
  });

  it("lists customers and breaches", () => {
    const rows = slaRows(sc(), [{ id: "c-a", name: "Demo Customer A", organizationId: "o-a" }, { id: "c-b", name: "Demo Customer B", organizationId: "o-b" }],
      [{ name: "Home A", customerOrgId: "o-a" }, { name: "Office A", customerOrgId: "o-a" }, { name: "Home B", customerOrgId: "o-b" }], [{ customerOrgId: "o-a" }, { customerOrgId: "o-a" }, { customerOrgId: "o-b" }]);
    expect(rows.map((r) => [r.name, r.sub, r.plan, r.jobs, r.ftf, r.status.label])).toEqual([["Demo Customer A", "Home A, Office A · 2 units", "RTO", 18, "86 %", "On track"], ["Demo Customer B", "Home B · 1 unit", "General", 9, "70 %", "Breached"]]);
    expect(breachRows(sc(), new Map([["c-b", "Demo Customer B"]])).map((b) => [b.short, b.text, b.kind, b.tone])).toEqual([["job-a11-", "Demo Customer B · response 6 h 10 min vs 4 h", "Response", "warn"], ["job-x-00", "customer · open past its due time", "Overdue", "crit"]]);
    expect(slaCsv(rows.slice(0, 1), sc().period)).toBe('"Customer","Plan","Jobs","Response","Arrival","First-time fix","Rating","Overdue","Status","Period from","Period to"\n"Demo Customer A","RTO","18","94 %","96 %","86 %","4.5 ★","1","On track","2026-06-16","2026-09-14"\n');
  });

  it("shows the targets per plan and checks the edit form", () => {
    const by = targetsByPlan(sc().targets);
    expect(by.map((p) => [p.label, p.line, p.scheduled])).toEqual([
      ["RTO", "v2 since 1 Aug 2026: ≤ 4 h · arrival 95 % · first-time fix 85 %", []], ["General", "default: ≤ 4 h · arrival 90 % · first-time fix 85 %", ["v1 from 1 Oct 2026, 8:00 am MYT: ≤ 2 h · 90 % · 85 %"]],
      ["Energy", "default: ≤ 4 h · arrival 90 % · first-time fix 85 %", []], ["Environment", "default: ≤ 4 h · arrival 90 % · first-time fix 85 %", []],
    ]);
    expect(targetErrors({ responseHours: "4", arrival: "95", ftf: "85.5", effectiveFrom: new Date(NOW).toISOString() }, NOW)).toEqual({});
    expect(targetErrors({ responseHours: "0", arrival: "120", ftf: "x", effectiveFrom: "2026-09-01T00:00:00Z" }, NOW)).toEqual({ responseHours: "1–168 whole hours.", arrival: "0–100 %.", ftf: "0–100 %.", effectiveFrom: "Now or later." });
    expect(slaRefusal({ code: "CONFLICT", messageKey: "errors.targets_exist", fieldErrors: {} })).toBe("Not saved (CONFLICT): targets for this plan already start at that time.");
  });
});

describe("HQ SLA by customer in Malay with the display time zone (IR291)", () => {
  it("phrases a breach from its kind and minutes, and words the tiles, rows, targets and CSV", () => {
    const took = { jobId: "j", customerId: "c-b", kind: "response" as const, detail: "response 6 h 10 min vs 4 h", tookMinutes: 370, limitMinutes: 240 };
    expect([breachText(took), breachText(took, MS.t), breachText({ ...took, tookMinutes: null, detail: "no response within 4 h" }, MS.t)]).toEqual(["response 6 h 10 min vs 4 h", "respons 6 j 10 min berbanding 4 j", "tiada respons dalam 4 j"]);
    expect(breachText({ ...took, kind: "overdue", tookMinutes: null, limitMinutes: null }, MS.t)).toBe("terbuka melepasi masa akhirnya");
    expect(slaTiles(sc(), MS.t).map((x) => x.label)).toEqual(["Respons ≤ 4 j", "Ketibaan dalam tetingkap", "Pembaikan kali pertama", "Purata penilaian pelanggan", "Terbuka & tertunggak"]);
    const rows = slaRows(sc(), [{ id: "c-a", name: "A", organizationId: "o-a" }], [], [], MS.t);
    expect([rows[0].sub, rows[0].status.label, rows[1].name]).toEqual(["tiada premis · 0 unit", "Mengikut sasaran", "pelanggan"]);
    expect(targetsByPlan(sc().targets, MS)[1].scheduled[0]).toMatch(/^v1 dari 1 Okt 2026, 9:00 PG GMT\+9: ≤ 2 j/); // the start in the display zone
    expect(slaCsv(rows.slice(0, 1), sc().period, MS).split("\n")[0]).toBe('"Pelanggan","Pelan","Kerja","Respons","Ketibaan","Pembaikan kali pertama","Penilaian","Tertunggak","Status","Tempoh dari","Tempoh hingga"');
  });
});
