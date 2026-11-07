import { describe, expect, it } from "vitest";
import { jobRow, pages, sortOf, sortSpec, tabFilters, tabOf } from "@ac/web/lib/partnerJobs";
import { period, periodChoice, type ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import { i18nOf } from "@ac/web/lib/i18n";

const TECH_EXTERNAL_A = "tech-external-a";
const TEXT_2026_09_21T02_00_00Z = "2026-09-21T02:00:00Z";
const TEXT_2026_09_21T04_00_00Z = "2026-09-21T04:00:00Z";
const IN_PROGRESS = "in_progress";
const TEXT_22222222_BBBB = "22222222-bbbb";
const TEXT_2026_09_22T01_00_00Z = "2026-09-22T01:00:00Z";
const TEXT_2026_09_22T03_00_00Z = "2026-09-22T03:00:00Z";
const TEXT_33333333_CCCC = "33333333-cccc";

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 in Kuala Lumpur
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const units = new Map([["u-1", "Bedroom AC"]]);
const names = new Map([["m-a", TECH_EXTERNAL_A]]);
const summary = (status: string, over: Partial<Extract<ApiPartnerJob, { projection: "summary" }>> = {}): ApiPartnerJob => ({ projection: "summary", id: "11111111-aaaa", version: 3, unitId: "u-1", type: "periodic", status, dueAt: "2026-09-25T10:00:00Z", requestedSlot: slot(TEXT_2026_09_21T02_00_00Z, TEXT_2026_09_21T04_00_00Z), scheduledSlot: null, assignmentId: null, displayStatus: status, technicianMembershipId: null, eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null }, ...over });

describe("partner job list", () => {
  it("maps tabs, sorts and pages", () => {
    expect(tabOf("review")).toBe("review");
    expect(tabOf("nope")).toBe("all");
    expect(tabFilters("all", "F", "T")).toEqual({ from: "F", to: "T" });
    expect(tabFilters("active", "F", "T")).toEqual({ from: "F", to: "T", statuses: ["accepted", "assigned", IN_PROGRESS, "on_hold", "rework_requested"] });
    expect(sortOf("dueAt:desc")).toBe("dueAt:desc");
    expect(sortOf("x")).toBe("status:asc");
    expect(sortSpec("severity:desc")).toEqual({ field: "severity", direction: "desc" });
    expect(pages(0, 1)).toEqual({ page: 1, of: 1 });
    expect(pages(26, 2)).toEqual({ page: 2, of: 2 });
  });

  it("shows an offer with its answer deadline and an accepted offer before the window", () => {
    const offered: ApiPartnerJob = { projection: "offer", status: "offered", jobId: TEXT_22222222_BBBB, jobVersion: 2, offerId: "o", type: "reactive", siteAddress: "2 Demo Avenue", requestedSlot: slot(TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T03_00_00Z), dueAt: "2026-09-24T10:00:00Z", offerExpiresAt: "2026-09-21T16:30:00Z", candidateSlots: [slot(TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T03_00_00Z)] };
    expect(jobRow(offered, NOW, units, names)).toEqual({
      id: TEXT_22222222_BBBB, title: "22222222 · 2 Demo Avenue", origin: null, href: "/partner/jobs/22222222-bbbb",
      line: "Offered by HQ · 22 Sept, 9:00 – 11:00 am MYT", tech: { name: "—", sub: "not assignable before acceptance" },
      slot: { main: "Answer by 22 Sept 2026, 12:30 am MYT", sub: "15 h left" }, badge: { text: "Offered", tone: "primary" },
    });
    const proposed = jobRow({ ...offered, partnerSlotProposal: { status: "sent_to_client" } }, NOW, units, names);
    expect(proposed.badge).toEqual({ text: "Time proposed", tone: "warn" });
    const accepted = jobRow({ ...offered, status: "accepted" }, NOW, units, names);
    expect(accepted).toMatchObject({ href: "/partner/schedule?jobId=22222222-bbbb", tech: { name: "Unassigned", tone: "warn" }, badge: { text: "Accepted" } });
  });

  it("follows a delegated job from unassigned to completed", () => {
    expect(jobRow(summary("accepted"), NOW, units, names)).toMatchObject({ title: "11111111 · Bedroom AC", href: "/partner/schedule?jobId=11111111-aaaa", tech: { name: "Unassigned" }, slot: { main: "Due 25 Sept 2026, 6:00 pm MYT" }, badge: { text: "Accepted" } });
    const assigned = { assignmentId: "as", technicianMembershipId: "m-a", assignmentAcknowledgement: "pending" as const, scheduledSlot: slot(TEXT_2026_09_21T02_00_00Z, TEXT_2026_09_21T04_00_00Z) };
    expect(jobRow(summary("assigned", assigned), NOW, units, names)).toMatchObject({
      line: "Assigned tech-external-a · 21 Sept, 10:00 am – 12:00 pm MYT", tech: { name: TECH_EXTERNAL_A, sub: "awaiting the technician’s acceptance" },
      slot: { main: "21 Sept, 10:00 am – 12:00 pm MYT", sub: "Due 25 Sept 2026, 6:00 pm MYT" }, badge: { text: "Assigned", tone: "primary" },
    });
    expect(jobRow(summary("assigned", { ...assigned, assignmentAcknowledgement: "cant_make" }), NOW, units, names).tech).toEqual({ name: TECH_EXTERNAL_A, sub: "can’t make it — reassign", tone: "warn", subTone: "warn" });
    expect(jobRow(summary(IN_PROGRESS, { ...assigned, scheduledSlot: slot("2026-09-20T01:00:00Z", "2026-09-20T09:00:00Z") }), NOW, units, names)).toMatchObject({ line: "Work window ended 20 Sept 2026, 5:00 pm MYT · report not submitted", slot: { main: "Ended 20 Sept 2026, 5:00 pm MYT", tone: "crit" }, badge: { text: "Overdue", tone: "crit" } });
    expect(jobRow(summary(IN_PROGRESS, assigned), NOW, units, names)).toMatchObject({ line: "In progress · tech-external-a on site", badge: { text: "In progress" } });
    expect(jobRow(summary("submitted", assigned), NOW, units, names)).toMatchObject({ href: "/partner/jobs/11111111-aaaa/review", line: "Submitted by tech-external-a · awaiting your review", badge: { text: "Submitted" } });
    expect(jobRow(summary("rework_requested", assigned), NOW, units, names).badge).toEqual({ text: "Rework", tone: "warn" });
    expect(jobRow(summary("on_hold", assigned), NOW, units, names).badge).toEqual({ text: "On hold", tone: "warn" });
    expect(jobRow(summary("completed", assigned), NOW, units, names)).toMatchObject({ badge: { text: "Completed", tone: "ok" }, tech: { name: TECH_EXTERNAL_A, sub: "report author" } });
    expect(jobRow(summary("cancelled"), NOW, units, names).badge).toEqual({ text: "Cancelled", tone: "muted" });
  });

  it("keeps only the history after the delegation", () => {
    expect(jobRow({ projection: "history", jobId: TEXT_33333333_CCCC, type: "reactive", status: "completed", completedAt: "2026-09-10T06:00:00Z", asOf: "2026-09-20T16:00:00Z", ownDecisionEvents: [] }, NOW, units, names)).toEqual({
      id: TEXT_33333333_CCCC, title: "33333333 · Repair", origin: null, href: "/partner/history?jobId=33333333-cccc", line: "Completed · delegation ended, history only",
      tech: { name: "—", sub: "history only" }, slot: { main: "Completed 10 Sept 2026, 2:00 pm MYT", sub: "access closed" }, badge: { text: "Completed", tone: "ok" },
    });
  });

  it("words the rows in Malay with the slots and deadlines in the display time zone (IR271)", () => {
    const offered: ApiPartnerJob = { projection: "offer", status: "offered", jobId: TEXT_22222222_BBBB, jobVersion: 2, offerId: "o", type: "reactive", siteAddress: "2 Demo Avenue", requestedSlot: slot(TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T03_00_00Z), dueAt: "2026-09-24T10:00:00Z", offerExpiresAt: "2026-09-21T16:30:00Z", candidateSlots: [slot(TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T03_00_00Z)] };
    expect(jobRow(offered, NOW, units, names, MS_TOKYO)).toMatchObject({
      line: "Ditawarkan oleh HQ · 22 Sep, 10:00 PG – 12:00 PTG GMT+9", tech: { sub: "tidak boleh ditugaskan sebelum diterima" },
      slot: { main: "Jawab sebelum 22 Sep 2026, 1:30 PG GMT+9", sub: "15 j lagi" }, badge: { text: "Ditawarkan" },
    });
    const cantMake = { assignmentId: "as", technicianMembershipId: "m-a", assignmentAcknowledgement: "cant_make" as const, scheduledSlot: slot(TEXT_2026_09_21T02_00_00Z, TEXT_2026_09_21T04_00_00Z) };
    expect(jobRow(summary("assigned", cantMake), NOW, units, names, MS_TOKYO)).toMatchObject({
      line: "Ditugaskan tech-external-a · 21 Sep, 11:00 PG – 1:00 PTG GMT+9", tech: { sub: "tidak dapat hadir — tugaskan semula", subTone: "warn" },
      slot: { sub: "Tarikh akhir 25 Sep 2026, 7:00 PTG GMT+9" }, badge: { text: "Ditugaskan" },
    });
    expect(jobRow({ projection: "history", jobId: TEXT_33333333_CCCC, type: "reactive", status: "completed", completedAt: "2026-09-10T06:00:00Z", asOf: "2026-09-20T16:00:00Z", ownDecisionEvents: [] }, NOW, units, names, MS_TOKYO))
      .toMatchObject({ title: "33333333 · Pembaikan", line: "Selesai · delegasi tamat, sejarah sahaja", slot: { main: "Selesai 10 Sep 2026, 3:00 PTG GMT+9", sub: "akses ditutup" } });
    expect(jobRow(summary("accepted"), NOW, units, names, MS_TOKYO).tech).toEqual({ name: "Belum ditugaskan", sub: "tugaskan sebelum delegasi tamat", tone: "warn" }); // no subTone: only the name is coloured
  });

  it("offers the three weeks around now with Kuala Lumpur dates in the user's language", () => {
    const now = "2026-09-21T01:30:00Z";
    expect(periodChoice(period(now), now)).toEqual({
      period: { value: "2026-09-21|2026-09-27", custom: false, text: "this week", label: "21 Sept – 27 Sept" },
      periods: [
        { value: "2026-09-14|2026-09-20", label: "Last week (14 Sept – 20 Sept)" }, { value: "2026-09-21|2026-09-27", label: "This week (21 Sept – 27 Sept)" },
        { value: "2026-09-28|2026-10-04", label: "Next week (28 Sept – 4 Oct)" },
      ],
    });
    const custom = periodChoice(period(now, "2026-09-01", "2026-09-10"), now, MS_TOKYO);
    expect([custom.period, custom.periods.map((p) => p.label)]).toEqual([
      { value: "2026-09-01|2026-09-10", custom: true, text: "1 Sep – 10 Sep", label: "1 Sep – 10 Sep" },
      ["Minggu lepas (14 Sep – 20 Sep)", "Minggu ini (21 Sep – 27 Sep)", "Minggu depan (28 Sep – 4 Okt)"],
    ]);
  });
});
