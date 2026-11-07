import { describe, expect, it } from "vitest";
import { decisionRefusal, delegationLeft, detailBanner, eventRows, fits, freeHours, qualificationLabel, qualified, timeline, type ApiDetail } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiMember } from "@ac/web/lib/partnerOverview";
import { i18nOf } from "@ac/web/lib/i18n";
import { must } from "./must";

const TECH_EXTERNAL_A = "tech-external-a";
const DEMO_INDOOR = "demo_indoor";
const DEMO_ELECTRICAL = "demo_electrical";
const TEXT_2026_09_22T01_00_00Z = "2026-09-22T01:00:00Z";
const TEXT_2026_09_22T02_00_00Z = "2026-09-22T02:00:00Z";
const TEXT_2026_09_22T04_00_00Z = "2026-09-22T04:00:00Z";
const TEXT_2026_09_22 = "2026-09-22";
const TEXT_2026_09_22T09_00_00Z = "2026-09-22T09:00:00Z";
const TEXT_2026_09_13T10_20_00Z = "2026-09-13T10:20:00Z";
const TEXT_2026_09_14T02_02_00Z = "2026-09-14T02:02:00Z";
const TEXT_2026_09_28T16_00_00Z = "2026-09-28T16:00:00Z";
const VALIDATION_2 = "VALIDATION";
const ERROR_VALIDATION = "error.validation";

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const quals = (codes: string[], over: Record<string, unknown> = {}) => codes.map((code) => ({ code, validFrom: "2026-09-01T00:00:00Z", validUntil: "2027-03-31T00:00:00Z", revokedAt: null, ...over }));
const members: ApiMember[] = [
  { id: "m-a", userId: "u-a", displayName: TECH_EXTERNAL_A, role: "technician", qualifications: quals([DEMO_INDOOR, "demo_outdoor", DEMO_ELECTRICAL]) },
  { id: "m-b", userId: "u-b", displayName: "tech-external-a2", role: "technician", qualifications: quals([DEMO_INDOOR]) },
  { id: "m-c", userId: "u-c", displayName: "coordinator", role: "contractor" },
];
const cap = (id: string, date: string, avail: [string, string][], assigned: [string, string][], am: number | null, as = 0): ApiCapacity =>
  ({ membershipId: id, availableSlots: avail.map(([s, e]) => slot(s, e)), assignedSlots: assigned.map(([s, e]) => slot(s, e)), availableMinutes: am, assignedMinutes: as, unavailability: null, date });

const event = (action: string, at: string) => ({ id: action, jobId: "j", actorUserId: null, occurredAt: at, action });

describe("partner job detail", () => {
  it("checks qualifications at the visit and labels them", () => {
    expect(qualificationLabel("demo_outdoor")).toBe("Outdoor unit work");
    expect(qualified(must(members[0]), [DEMO_INDOOR, DEMO_ELECTRICAL], TEXT_2026_09_22T01_00_00Z)).toBe(true);
    expect(qualified(must(members[1]), [DEMO_INDOOR, DEMO_ELECTRICAL], TEXT_2026_09_22T01_00_00Z)).toBe(false);
    expect(qualified({ ...must(members[0]), qualifications: quals([DEMO_INDOOR], { revokedAt: "2026-09-10T00:00:00Z" }) }, [DEMO_INDOOR], TEXT_2026_09_22T01_00_00Z)).toBe(false);
    expect(qualified(must(members[0]), [DEMO_INDOOR], "2027-04-01T00:00:00Z")).toBe(false); // expired by the visit
    expect(freeHours(cap("m-a", "d", [], [], 480, 120))).toBe(6);
    expect(freeHours(cap("m-a", "d", [], [], null))).toBeNull();
  });

  it("tells which technicians can take the visit", () => {
    const visit = slot(TEXT_2026_09_22T02_00_00Z, TEXT_2026_09_22T04_00_00Z); // 10:00–12:00 KL
    const days = [
      { date: TEXT_2026_09_22, capacity: [cap("m-a", TEXT_2026_09_22, [[TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T09_00_00Z]], [], 480), cap("m-b", TEXT_2026_09_22, [[TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T09_00_00Z]], [], 480)] },
      { date: "2026-09-23", capacity: [cap("m-a", "2026-09-23", [], [], 480, 480)] },
    ];
    const rows = fits(members, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days);
    expect(rows.map((r) => [r.name, r.badge.text, r.days.map((d) => d.text).join(",")])).toStrictEqual([
      [TECH_EXTERNAL_A, "Fits", "8 h,0 h"],
      ["tech-external-a2", "Not qualified", "8 h,—"],
    ]);
    expect(rows[1]?.sub).toBe("missing Electrical work");
    expect(rows[0]?.days[0]?.label).toBe("Tue");
    const busy = fits(members, [DEMO_INDOOR], visit, [{ date: TEXT_2026_09_22, capacity: [cap("m-a", TEXT_2026_09_22, [[TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T09_00_00Z]], [["2026-09-22T03:00:00Z", "2026-09-22T05:00:00Z"]], 480, 120)] }]);
    expect(busy[0]?.badge).toStrictEqual({ text: "Busy at the visit", tone: "warn" });
    const detail = fits(members, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days, "detail");
    expect(detail.map((r) => [r.sub, r.badge.text, r.days.length])).toStrictEqual([["Qualified · 8 h free 22 Sept – 23 Sept", "Best fit", 0], ["Missing Electrical work", "Not eligible", 0]]);
    expect(fits(members, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days, "offer", undefined, MS_TOKYO).map((r) => [r.sub, r.badge.text, r.days.map((d) => `${d.label} ${d.text}`)])).toStrictEqual([
      ["Kerja unit dalaman · Kerja unit luaran · Kerja elektrik", "Sesuai", ["Sel 8 j", "Rab 0 j"]], ["tiada Kerja elektrik", "Tidak layak", ["Sel 8 j", "Rab —"]],
    ]); // Kuala Lumpur days, in Malay
    expect(fits(members, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days, "detail", undefined, MS_TOKYO)[0]?.sub).toBe("Layak · 8 j lapang 22 Sep – 23 Sep");
    const own = fits(members, [DEMO_INDOOR], visit, [{ date: TEXT_2026_09_22, capacity: [cap("m-a", TEXT_2026_09_22, [[TEXT_2026_09_22T01_00_00Z, TEXT_2026_09_22T09_00_00Z]], [[TEXT_2026_09_22T02_00_00Z, TEXT_2026_09_22T04_00_00Z]], 480, 120)] }], "detail", "m-a");
    expect([own[0]?.badge.text, own[0]?.sub]).toStrictEqual(["Assigned", "Qualified · assigned to this job"]);
    // whoever can take the visit comes first, whatever order the API lists the team in (IR569)
    const reversed = members.toReversed();
    expect(fits(reversed, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days).map((r) => r.badge.text)).toStrictEqual(["Fits", "Not qualified"]);
    expect(fits(reversed, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days, "detail").map((r) => r.badge.text)).toStrictEqual(["Best fit", "Not eligible"]);
    expect(fits(members, [DEMO_INDOOR, DEMO_ELECTRICAL], visit, days, "detail", "m-b").map((r) => r.badge.text)).toStrictEqual(["Assigned", "Best fit"]); // the assigned one first, even when not eligible
  });

  it("builds the status timeline from the events", () => {
    const events = [event("job.created", "2026-09-13T01:00:00Z"), event("job.offered", TEXT_2026_09_13T10_20_00Z), event("offer.accepted", TEXT_2026_09_14T02_02_00Z)];
    const t = timeline("accepted", events, false, "periodic_plan");
    expect(t[0]?.sub).toBe("periodic plan");
    expect(t.map((s) => [s.label, s.state, s.at])).toStrictEqual([
      ["Requested", "done", "2026-09-13T01:00:00Z"], ["Offered", "done", TEXT_2026_09_13T10_20_00Z], ["Accepted", "current", TEXT_2026_09_14T02_02_00Z],
      ["Assigned", "todo", null], ["In progress", "todo", null], ["Submitted", "todo", null], ["Completed", "todo", null],
    ]);
    expect(timeline("completed", events, true).every((s) => s.state === "done")).toBe(true);
    expect(timeline("rework_requested", events, true).find((s) => s.state === "current")?.label).toBe("In progress");
    expect(eventRows(events).map((r) => r.text)).toStrictEqual(["Job accepted", "Offer received from HQ", "Job created"]);
    expect(timeline("accepted", events, false, "periodic_plan", MS_TOKYO).slice(0, 3).map((s) => [s.label, s.sub, s.atText])).toStrictEqual([
      ["Diminta", "pelan berkala", "13 Sep 2026, 10:00 PG GMT+9"], ["Ditawarkan", "HQ menawarkan kepada syarikat anda", "13 Sep 2026, 7:20 PTG GMT+9"], ["Diterima", "anda", "14 Sep 2026, 11:02 PG GMT+9"],
    ]); // IR272: each step's time in the display time zone
    expect(eventRows(events, MS_TOKYO)[0]).toStrictEqual({ id: "offer.accepted", at: "14 Sep 2026, 11:02 PG GMT+9", text: "Kerja diterima" });
  });

  it("states the delegated job's banner, countdown and refusals", () => {
    const base = { projection: "detail", id: "j", version: 4, unitId: "u", type: "reactive", status: "accepted", symptom: "", origin: "client_request", alertIds: [], requestedSlot: slot(TEXT_2026_09_22T02_00_00Z, TEXT_2026_09_22T04_00_00Z), scheduledSlot: null, dueAt: "2026-09-25T00:00:00Z", startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null, offer: { id: "o", termsVersion: "t", candidateSlots: [slot(TEXT_2026_09_22T02_00_00Z, TEXT_2026_09_22T04_00_00Z)], offeredAt: TEXT_2026_09_13T10_20_00Z, accessValidFrom: "2026-09-14T00:00:00Z", accessValidUntil: TEXT_2026_09_28T16_00_00Z, decision: "accept", decidedAt: TEXT_2026_09_14T02_02_00Z } } as ApiDetail;
    expect(detailBanner(base, null, NOW)?.text).toBe("Accepted · decided 14 Sept 2026, 10:02 am MYT (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.");
    const assigned = { ...base, status: "assigned", assignment: { technicianMembershipId: "m-a", scheduledStart: TEXT_2026_09_22T02_00_00Z, scheduledEnd: TEXT_2026_09_22T04_00_00Z, status: "active", acknowledgement: "cant_make", cantMakeReason: "sick" } };
    expect(detailBanner(assigned, TECH_EXTERNAL_A, NOW)).toStrictEqual({ tone: "warn", text: "Assigned — tech-external-a · 22 Sept, 10:00 am – 12:00 pm MYT · the technician can’t make it (“sick”) — reassign in Schedule." });
    expect(detailBanner(assigned, TECH_EXTERNAL_A, NOW, MS_TOKYO)?.text).toBe("Ditugaskan — tech-external-a · 22 Sep, 11:00 PG – 1:00 PTG GMT+9 · juruteknik tidak dapat hadir (“sick”) — tugaskan semula dalam Jadual.");
    expect(detailBanner({ ...assigned, assignment: { ...assigned.assignment, scheduledEnd: "2026-09-20T09:00:00Z" } }, TECH_EXTERNAL_A, NOW)?.tone).toBe("crit");
    expect(delegationLeft(TEXT_2026_09_28T16_00_00Z, NOW)).toBe("Delegation ends 29 Sept 2026, 12:00 am MYT — 7 d 14 h left.");
    expect(delegationLeft(TEXT_2026_09_28T16_00_00Z, NOW, true)).toBe("Delegation ends 29 Sept 2026, 12:00 am MYT — 7 d 14 h to schedule the work.");
    expect(delegationLeft(TEXT_2026_09_28T16_00_00Z, NOW, true, MS_TOKYO)).toBe("Delegasi tamat 29 Sep 2026, 1:00 PG GMT+9 — 7 hari 14 j lagi untuk menjadualkan kerja.");
    expect(delegationLeft("2026-09-20T16:00:00Z", NOW)).toBe("The delegation period has ended.");
    expect(decisionRefusal({ code: "CONFLICT", messageKey: "errors.offer_expired", fieldErrors: {} })).toMatch(/expired or was cancelled/);
    expect(decisionRefusal({ code: VALIDATION_2, messageKey: ERROR_VALIDATION, fieldErrors: { reason: "error.length" } })).toBe("reason: required (1–1000 characters)");
    expect(decisionRefusal({ code: VALIDATION_2, messageKey: ERROR_VALIDATION, fieldErrors: { slot: "error.slotRules" } })).toBe("slot: pick a time on a later day, 1–4 hours long");
    expect(decisionRefusal({ code: VALIDATION_2, messageKey: ERROR_VALIDATION, fieldErrors: { _: "error.malformedInput" } })).toMatch(/^the request was not understood/);
    expect(decisionRefusal({ code: "FORBIDDEN", messageKey: "errors.qualification_missing", fieldErrors: {} })).toMatch(/lacks a required qualification/);
    expect(decisionRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })).toBe("This offer is no longer available to your company.");
    expect(decisionRefusal({ code: VALIDATION_2, messageKey: ERROR_VALIDATION, fieldErrors: { slot: "error.slotRules" } }, MS_TOKYO.t)).toBe("slot: pilih masa pada hari kemudian, 1–4 jam"); // the field keeps its API key
  });
});
