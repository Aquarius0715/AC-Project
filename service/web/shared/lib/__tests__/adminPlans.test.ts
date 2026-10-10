import { describe, expect, it } from "vitest";
import { anchorOf, cadence, everyText, generateConflict, nextBox, nextOccurrence, planDate, planErrors, planRefusal, planRows, type ApiPlan } from "@ac/web/lib/adminPlans";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-15T00:30:00Z"); // Tue 08:30 KL
const plan = (over: Partial<ApiPlan>): ApiPlan => ({
  id: "plan-living-a-0001", version: 3, unitId: "u-living", recurrence: { kind: "monthly", intervalMonths: 3 }, timezone: "UTC", anchorDay: 8, nextDueAt: "2026-12-08T02:00:00Z",
  generatedOccurrences: [{ occurrenceAt: "2026-06-08T02:00:00Z", jobId: "job-a06-0000" }, { occurrenceAt: "2026-09-08T02:00:00Z", jobId: "job-c02-0000" }], createdAt: "2026-05-01T00:00:00Z", updatedAt: "2026-09-08T00:00:00Z", ...over,
});

describe("HQ maintenance plans", () => {
  it("lists the plans next due first", () => {
    const rows = planRows([plan({}), plan({ id: "plan-lobby-b-0001", unitId: "u-lobby", anchorDay: 1, nextDueAt: "2026-11-01T02:00:00Z" })],
      new Map([["u-living", "Living room AC"], ["u-lobby", "Lobby AC"]]), new Map([["u-living", "customer-a"], ["u-lobby", "customer-b"]]));
    expect(rows.map((r) => [r.short, r.unit, r.customer, r.line, r.next])).toEqual([
      ["plan-lob", "Lobby AC", "customer-b", "Every 3 months · day 1", "Next 1 Nov 2026"], ["plan-liv", "Living room AC", "customer-a", "Every 3 months · day 8", "Next 8 Dec 2026"],
    ]);
    expect([everyText(1), cadence(3), cadence(6), cadence(12), cadence(2), planDate("2026-12-07T23:00:00Z")]).toEqual(["Every month", "quarterly", "half-yearly", "yearly", "every 2 months", "8 Dec 2026"]);
  });

  it("computes the next occurrence like D16 and the stored anchor day", () => {
    expect(nextOccurrence("2027-01-31T10:00:00.000Z", 1, 31)).toBe("2027-02-28T10:00:00.000Z");
    expect(nextOccurrence("2027-02-28T10:00:00.000Z", 1, 31)).toBe("2027-03-31T10:00:00.000Z");
    expect(nextOccurrence("2026-12-08T02:00:00Z", 3, 8)).toBe("2027-03-08T02:00:00.000Z");
    expect(nextOccurrence("2026-11-30T02:00:00Z", 3, 30)).toBe("2027-02-28T02:00:00.000Z");
    expect(anchorOf(plan({}), "2026-12-08T02:00:00Z")).toBe(8);
    expect(anchorOf(plan({}), "2026-12-15T02:00:00.000Z")).toBe(15);
    expect(anchorOf(null, "2026-12-07T23:00:00.000Z")).toBe(7); // 2026-12-08 07:00 KL is the 7th in UTC
  });

  it("checks the plan form and the next-occurrence box", () => {
    expect(planErrors({ unitId: "u", every: 3, nextDueAt: "2026-12-08T02:00:00.000Z" }, NOW)).toEqual({});
    expect(planErrors({ unitId: "", every: 13, nextDueAt: "2026-09-14T02:00:00.000Z" }, NOW)).toEqual({ unitId: "Choose the unit.", every: "Every 1–12 months.", nextDueAt: "The next date must be in the future." });
    expect(planErrors({ unitId: "u", every: 3, nextDueAt: null }, NOW).nextDueAt).toBe("Enter the next date and time.");
    expect(nextBox(plan({}), NOW, false, null)).toEqual({ title: "Next occurrence · 8 Dec 2026", text: "Not generated yet. Generating creates one periodic job (requested) for this date — generating the same date again is rejected.", generate: true, why: null });
    expect(nextBox(plan({}), NOW, true, null)).toMatchObject({ generate: false, why: "Save the plan first." });
    expect(nextBox(plan({ nextDueAt: "2026-09-14T02:00:00Z" }), NOW, false, null)).toMatchObject({ generate: false, why: "The next date has passed." });
    expect(nextBox(plan({ nextDueAt: "2027-03-08T02:00:00Z" }), NOW, false, { occurrenceAt: "2026-12-08T02:00:00Z", jobId: "job-c08-0000" }).text).toBe("8 Dec 2026 was just generated as job-c08-. Next due advanced to 8 Mar 2027.");
  });

  it("explains a refused Generate job and the other refusals", () => {
    const after = plan({ nextDueAt: "2027-03-08T02:00:00Z", generatedOccurrences: [...plan({}).generatedOccurrences, { occurrenceAt: "2026-12-08T02:00:00Z", jobId: "job-c08-0000" }] });
    expect(generateConflict(after, "2026-12-08T02:00:00Z")).toEqual({ title: "CONFLICT · 8 Dec 2026 already has a job", text: "A second “Generate job” for the same plan and date (another tab / double submit) was rejected. Still one job: job-c08-." });
    expect(generateConflict(plan({}), "2026-11-08T02:00:00Z").title).toBe("CONFLICT · the plan changed meanwhile");
    expect(planRefusal({ code: "CONFLICT", messageKey: "errors.next_date_past", fieldErrors: {} })).toBe("Not saved (CONFLICT): the next date has passed — move it forward and save the plan first.");
    expect(planRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { nextDueAt: "error.past" } })).toBe("nextDueAt: must be in the future");
    expect(planRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })).toBe("The plan or its unit no longer exists in your scope.");
  });
});

describe("HQ maintenance plans in Malay with the display time zone (IR291)", () => {
  it("words the rows, the box and the refusals, with the dates in the display zone", () => {
    const rows = planRows([plan({ nextDueAt: "2026-12-07T16:30:00Z" })], new Map([["u-living", "Living room AC"]]), new Map(), MS);
    expect([rows[0].line, rows[0].next, rows[0].customer]).toEqual(["Setiap 3 bulan · hari 8", "Seterusnya 8 Dis 2026", "pelanggan"]); // 01:30 on the 8th in Tokyo, the 7th in UTC
    expect([cadence(3, MS.t), cadence(2, MS.t), everyText(1, MS.t)]).toEqual(["suku tahunan", "setiap 2 bulan", "Setiap bulan"]);
    expect(nextBox(plan({}), NOW, true, null, MS)).toMatchObject({ title: "Kejadian seterusnya · 8 Dis 2026", why: "Simpan pelan dahulu." });
    expect(nextBox(plan({}), NOW, false, null, MS, "server text").title).toBe("Kejadian seterusnya · server text"); // the page passes the server's date
    expect(planErrors({ unitId: "", every: 3, nextDueAt: "2026-12-08T02:00:00.000Z" }, NOW, MS.t)).toEqual({ unitId: "Pilih unit." });
    expect(planRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} }, MS.t)).toBe("Pelan atau unitnya tidak lagi wujud dalam skop anda.");
  });
});
