import { describe, expect, it } from "vitest";
import { assignRefusal, candidates, formMode, freeText, scheduleRow, scheduleTeam, sortOf, weekGrid } from "@ac/web/lib/partnerSchedule";
import type { ApiDetail, ApiOffer } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import { EN, i18nOf, translator } from "@ac/web/lib/i18n";
import { must } from "./must";

const TECH_EXTERNAL_A = "tech-external-a";
const TECHNICIAN = "technician";
const DEMO_INDOOR = "demo_indoor";
const DEMO_ELECTRICAL = "demo_electrical";
const TEXT_2026_09_25T00_00_00Z = "2026-09-25T00:00:00Z";
const IN_PROGRESS = "in_progress";
const TEXT_2026_09_22 = "2026-09-22";
const VALIDATION_2 = "VALIDATION";
const ERROR_VALIDATION = "error.validation";

const scheduleRowOf = (...args: Parameters<typeof scheduleRow>) => must(scheduleRow(...args)); // the scheduleRow a test expects

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const visit = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"); // Tue 10:00–12:00 KL
const quals = (codes: string[]) => codes.map((code) => ({ code, validFrom: "2026-09-01T00:00:00Z", validUntil: "2027-03-31T00:00:00Z", revokedAt: null }));
const members: ApiMember[] = [
  { id: "m-a", userId: "u-a", displayName: TECH_EXTERNAL_A, role: TECHNICIAN, qualifications: quals([DEMO_INDOOR, "demo_outdoor", DEMO_ELECTRICAL]) },
  { id: "m-b", userId: "u-b", displayName: "tech-external-a2", role: TECHNICIAN, qualifications: quals([DEMO_INDOOR]) },
  { id: "m-c", userId: "u-c", displayName: "tech-external-a3", role: TECHNICIAN, qualifications: quals([DEMO_INDOOR, "demo_outdoor", DEMO_ELECTRICAL]) },
  { id: "m-d", userId: "u-d", displayName: "coordinator", role: "contractor" },
];
const names = new Map(members.map((m) => [m.id, m.displayName]));
const units = new Map([["u1", "Lobby AC"]]);
type SummaryJob = Extract<ApiPartnerJob, { projection: "summary" }>;
const detail = (over: Partial<ApiDetail> = {}): ApiDetail => ({
  projection: "detail", id: "job-1", version: 4, unitId: "u1", type: "reactive", status: "accepted", symptom: "x", origin: "client_request", alertIds: [], requestedSlot: visit, scheduledSlot: null,
  dueAt: TEXT_2026_09_25T00_00_00Z, startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null,
  offer: { id: "o", termsVersion: "t", candidateSlots: [visit], offeredAt: "x", accessValidFrom: "x", accessValidUntil: "x", decision: "accept", decidedAt: "x" }, ...over,
});
const offered = (slots: { startAt: string; endAt: string }[]) => ({ id: "o", termsVersion: "t", candidateSlots: slots, offeredAt: "x", accessValidFrom: "x", accessValidUntil: "x", decision: "accept", decidedAt: "x" });
const acceptedOffer = (): ApiOffer => ({
  projection: "offer", status: "accepted", jobId: "job-2", jobVersion: 1, offerId: "o-2", type: "reactive", siteAddress: null, requiredQualifications: [], requestedSlot: visit, dueAt: TEXT_2026_09_25T00_00_00Z,
  offerExpiresAt: "2026-09-30T00:00:00Z", termsVersion: "t", origin: "client_request", candidateSlots: [visit], offeredAt: "x", accessValidFrom: "2026-09-22T00:00:00Z", accessValidUntil: TEXT_2026_09_25T00_00_00Z, partnerSlotProposal: null,
});
const summary = (over: Partial<SummaryJob>): SummaryJob => {
  return ({
    projection: "summary", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "accepted", dueAt: TEXT_2026_09_25T00_00_00Z, requestedSlot: visit,
    scheduledSlot: null, assignmentId: null, displayStatus: "accepted", technicianMembershipId: null, assignmentAcknowledgement: null,
    accessValidFrom: "2026-09-20T01:30:00Z", accessValidUntil: "2026-09-22T01:30:00Z", eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null }, ...over,
  });
};
const cap = (id: string, date: string, avail: [string, string][], assigned: [string, string][], am: number, unavailability: string | null = null): ApiCapacity =>
  ({ membershipId: id, availableSlots: avail.map(([s, e]) => slot(s, e)), assignedSlots: assigned.map(([s, e]) => slot(s, e)), availableMinutes: am, assignedMinutes: 0, date, unavailability });
const workday = (d: string): [string, string] => [`${d}T01:00:00Z`, `${d}T09:00:00Z`];

const f = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}) => assignRefusal({ code, messageKey, fieldErrors }, TECH_EXTERNAL_A);

const rows = [
  scheduleRowOf(summary({}), NOW, units, names),
  scheduleRowOf(summary({ status: "assigned", scheduledSlot: visit, technicianMembershipId: "m-a", assignmentAcknowledgement: "cant_make" }), NOW, units, names),
  scheduleRowOf(summary({ status: IN_PROGRESS, scheduledSlot: slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z"), technicianMembershipId: "m-a" }), NOW, units, names),
  scheduleRowOf(summary({ status: "on_hold", accessValidUntil: "2026-09-21T01:00:00Z" }), NOW, units, names),
  scheduleRowOf({ projection: "offer", status: "accepted", jobId: "job-2-bbbb", jobVersion: 2, offerId: "o", type: "reactive", siteAddress: "2 Demo Avenue", requestedSlot: visit, dueAt: "x", offerExpiresAt: "x", candidateSlots: [visit], accessValidFrom: "2026-09-22T00:00:00Z", accessValidUntil: "2026-09-26T00:00:00Z" }, NOW, units, names),
];

const asg = (s: { startAt: string; endAt: string }, ack = "pending", why: string | null = null) => ({ technicianMembershipId: "m-a", scheduledStart: s.startAt, scheduledEnd: s.endAt, status: "active", acknowledgement: ack, cantMakeReason: why });
describe("partner schedule", () => {
  it("lists the jobs with their state and delegation window", () => {
    expect(rows.map((r) => [r.title, r.line, r.tone ?? ""])).toStrictEqual([
      ["Lobby AC", "Accepted · awaiting assignment", ""],
      ["Lobby AC", "Assigned · tech-external-a · can’t make it", "warn"],
      ["Lobby AC", "Work window ended · reassignment required", "crit"],
      ["Lobby AC", "On hold by HQ", "warn"],
      ["2 Demo Avenue", "Accepted · delegation starts 22 Sept 2026, 8:00 am MYT", "muted"],
    ]);
    expect([rows[0]?.windowText, rows[0]?.left, rows[0]?.leftTone, rows[0]?.elapsedPct, rows[0]?.ended]).toStrictEqual(["20 Sept, 9:30 am – 22 Sept, 9:30 am MYT", "1 d left", "primary", 50, false]);
    expect([rows[3]?.left, rows[3]?.leftTone, rows[3]?.ended]).toStrictEqual(["Ended", "crit", true]);
    expect(scheduleRow({ projection: "history", jobId: "h", type: "reactive", status: "completed", completedAt: null, asOf: "2026-09-20T16:00:00Z", ownDecisionEvents: [] }, NOW, units, names)).toBeNull();
    expect(sortOf("dueAt")).toBe("dueAt");
    expect(sortOf("nope")).toBe("status");
  });

  it("decides what the form of a job allows", () => {
    const a = formMode(detail({}), "job-1 · Lobby AC", NOW, names);
    expect(a.kind === "assign" && [a.title, a.slot, a.endEditable, a.reasonRequired, a.button]).toStrictEqual(["Assign — job-1 · Lobby AC", visit, false, false, "Confirm assignment"]);
    const r = formMode(detail({ status: "assigned", scheduledSlot: visit, assignment: asg(visit, "cant_make", "sick") }), "job-1", NOW, names);
    expect(r.kind === "reassign" && [r.currentTech, r.banner?.text.startsWith("tech-external-a can’t make this time: “sick”")]).toStrictEqual([TECH_EXTERNAL_A, true]);
    const past = slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z");
    const gone = formMode(detail({ status: "assigned", scheduledSlot: past, assignment: asg(past), offer: offered([past]) }), "job-1", NOW, names);
    expect([gone.kind, gone.banner?.tone]).toStrictEqual(["blocked", "crit"]);
    // IR636: the booked time is over but the client agreed another one — the contractor reassigns to it
    const moved = formMode(detail({ status: "assigned", scheduledSlot: past, assignment: asg(past), offer: offered([past, visit]) }), "job-1", NOW, names);
    expect(moved.kind === "reassign" && [moved.slot, moved.choices]).toStrictEqual([visit, [visit]]);
    // IR636: an offer with several times: earliest first, `pick` chooses one, a time that is over is not offered
    const later = slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z");
    const several = detail({ offer: offered([later, past, visit]) });
    const first = formMode(several, "job-1", NOW, names);
    expect(first.kind === "assign" && [first.slot, first.choices]).toStrictEqual([visit, [visit, later]]);
    const second = formMode(several, "job-1", NOW, names, EN, 1);
    expect(second.kind === "assign" && second.slot).toStrictEqual(later);
    const none = formMode(detail({ offer: offered([past]) }), "job-1", NOW, names);
    expect([none.kind, none.banner?.tone]).toStrictEqual(["blocked", "crit"]);
    const extension = formMode(detail({ status: IN_PROGRESS, scheduledSlot: past, assignment: asg(past) }), "job-1", NOW, names);
    expect(extension.kind === "extend" && [extension.title, extension.endEditable, extension.reasonRequired, extension.banner?.tone]).toStrictEqual(["Reschedule — job-1", true, true, "crit"]);
    const running = formMode(detail({ status: IN_PROGRESS, scheduledSlot: visit, assignment: asg(visit, "accepted") }), "job-1", NOW, names);
    expect(running.kind === "extend" && [running.title, running.banner]).toStrictEqual(["Reassign — job-1 (in progress)", undefined]);
    const offer = acceptedOffer();
    expect(formMode(offer, "job-2", NOW, names)).toMatchObject({ kind: "blocked", banner: { text: "Accepted — the delegation period starts 22 Sept 2026, 8:00 am MYT; the technician can be assigned from then, for one of the times the client agreed: 22 Sept, 10:00 am – 12:00 pm MYT." } });
    expect(formMode(detail({ status: "completed" }), "job-1", NOW, names).kind).toBe("blocked");
  });

  it("names the free time and the technicians that can take the slot", () => {
    const day = [
      cap("m-a", TEXT_2026_09_22, [workday(TEXT_2026_09_22)], [["2026-09-22T05:00:00Z", "2026-09-22T07:00:00Z"]], 480),
      cap("m-b", TEXT_2026_09_22, [workday(TEXT_2026_09_22)], [], 480),
      cap("m-c", TEXT_2026_09_22, [], [], 0, "annual_leave"),
    ];
    expect(freeText(day[0])).toBe("free 09:00–13:00, 15:00–17:00");
    expect(freeText(day[2])).toBe("no hours");
    expect(freeText(cap("m-a", "d", [workday(TEXT_2026_09_22)], [workday(TEXT_2026_09_22)], 480))).toBe("fully booked");
    const candidateRows = candidates(members, new Set(["m-a"]), [DEMO_INDOOR, DEMO_ELECTRICAL], visit, day, null);
    expect(candidateRows.map((c) => [c.name, c.badge.text, c.eligible])).toStrictEqual([
      [TECH_EXTERNAL_A, "Qualified", true], ["tech-external-a2", "Missing: Electrical work", false], ["tech-external-a3", "Unavailable (annual leave)", false],
    ]);
    expect(candidateRows[0]?.sub).toBe("Indoor unit work · Outdoor unit work · Electrical work · free 09:00–13:00, 15:00–17:00");
    const busy = candidates(members, new Set(), [DEMO_INDOOR], visit, [cap("m-a", TEXT_2026_09_22, [workday(TEXT_2026_09_22)], [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], 480)], null);
    expect(busy[0]?.badge.text).toBe("Busy at this time");
    expect(candidates(members, new Set(["m-a"]), [DEMO_INDOOR], visit, [], "m-a")[0]?.badge.text).toBe("Current");
  });

  it("builds the team's week with jobs, leave and the proposal", () => {
    const dates = ["2026-09-21", TEXT_2026_09_22, "2026-09-23"];
    const week = [
      [cap("m-a", must(dates[0]), [workday(must(dates[0]))], [["2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z"]], 480), cap("m-b", must(dates[0]), [workday(must(dates[0]))], [], 480)],
      [cap("m-a", must(dates[1]), [workday(must(dates[1]))], [], 480), cap("m-b", must(dates[1]), [], [], 0, "annual_leave")],
      [cap("m-a", must(dates[2]), [], [], 0), cap("m-b", must(dates[2]), [workday(must(dates[2]))], [], 480)],
    ];
    const jobsOf = new Map([["m-a", [{ short: "job-9", slot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") }]]]);
    const gridRows = weekGrid(dates, members.slice(0, 2), week, jobsOf, { technicianId: "m-a", slot: visit, short: "job-1" });
    expect(gridRows[0]?.cells).toStrictEqual([{ kind: "booked", text: "10:00–12:00", sub: "job-9" }, { kind: "proposal", text: "10:00–12:00", sub: "job-1" }, { kind: "none", text: "no hours" }]);
    expect(gridRows[1]?.cells).toStrictEqual([{ kind: "free", text: "free" }, { kind: "leave", text: "Leave", sub: "annual leave" }, { kind: "free", text: "free" }]);
    const training = weekGrid(dates.slice(0, 1), members.slice(1, 2), [[cap("m-b", must(dates[0]), [], [], 0, "training")]], new Map(), null);
    expect(training[0]?.cells).toStrictEqual([{ kind: "leave", text: "Unavailable", sub: "training" }]);
    expect([gridRows[0]?.sub, gridRows[1]?.sub]).toStrictEqual(["Indoor, Outdoor, Electrical · 16 h avail.", "Indoor · 16 h avail."]);
  });

  it("states the refusals of the assignment", () => {
    expect(f("CONFLICT", "errors.assignment_overlap")).toBe("Overlaps a confirmed schedule for tech-external-a — choose another technician.");
    expect(f("FORBIDDEN", "errors.access_window")).toMatch(/^Outside the delegation period/);
    expect(f("FORBIDDEN", "errors.qualification_missing")).toMatch(/revoked or expiring, FORBIDDEN/);
    expect(f(VALIDATION_2, ERROR_VALIDATION, { startAt: "errors.slot_not_agreed" })).toMatch(/^Start: the slot must be the agreed visit time/);
    expect(f(VALIDATION_2, ERROR_VALIDATION, { reason: "error.required" })).toBe("Reason: required (1–1000 characters)");
    expect(f(VALIDATION_2, ERROR_VALIDATION, { endAt: "errors.extension_only", other: "error.odd" })).toBe("End: keep the start and set an end at or after the current end · other: error.odd");
    expect(f(VALIDATION_2, ERROR_VALIDATION)).toBe("Check the input.");
    expect(f("UNAVAILABLE", "error.unavailable")).toBe("The service is unavailable — try again shortly"); // IR380: worded, not the raw key
    expect(f("NOT_FOUND", "error.notFound")).toBe("The job is no longer delegated to your company.");
  });

  it("words the schedule in Malay with the job's times in the display time zone (IR275)", () => {
    const row = scheduleRowOf(summary({ status: "assigned", scheduledSlot: visit, technicianMembershipId: "m-a", assignmentAcknowledgement: null }), NOW, units, names, MS_TOKYO);
    expect([row.line, row.windowText, row.left]).toStrictEqual(["Ditugaskan · tech-external-a · menunggu penerimaan", "20 Sep, 10:30 PG – 22 Sep, 10:30 PG GMT+9", "1 hari lagi"]);
    const offer = acceptedOffer();
    expect(formMode(offer, "job-2", NOW, names, MS_TOKYO)).toMatchObject({ title: "Tugaskan — job-2", banner: { text: "Diterima — tempoh delegasi bermula 22 Sep 2026, 9:00 PG GMT+9; juruteknik boleh ditugaskan mulai ketika itu, untuk salah satu masa yang dipersetujui pelanggan: 22 Sep, 11:00 PG – 1:00 PTG GMT+9." } });
    const held = formMode(detail({ status: "on_hold", scheduledSlot: visit, offer: null }), "job-1", NOW, names, MS_TOKYO);
    expect(held.kind === "extend" && [held.title, held.button]).toStrictEqual(["Tugaskan semula — job-1 (ditangguhkan)", "Sahkan penugasan semula"]);
    expect(formMode(detail({ status: "submitted", scheduledSlot: visit, offer: null }), "job-1", NOW, names, MS_TOKYO).banner?.text).toBe("Kerja ini dihantar — tiada apa-apa untuk dijadualkan.");
    const day = [cap("m-a", TEXT_2026_09_22, [workday(TEXT_2026_09_22)], [], 480), cap("m-c", TEXT_2026_09_22, [], [], 0, "sick")];
    expect(candidates(members, new Set(["m-a"]), [DEMO_INDOOR, DEMO_ELECTRICAL], visit, day, null, ms).map((c) => [c.sub, c.badge.text])).toStrictEqual([
      ["Kerja unit dalaman · Kerja unit luaran · Kerja elektrik · lapang 09:00–17:00", "Layak"],
      ["Kerja unit dalaman · tiada jam", "Tiada: Kerja elektrik"],
      ["Kerja unit dalaman · Kerja unit luaran · Kerja elektrik · tiada jam", "Tidak tersedia (cuti sakit)"],
    ]);
    const week = weekGrid([TEXT_2026_09_22], members.slice(0, 1), [[cap("m-a", TEXT_2026_09_22, [], [], 0, "sick")]], new Map(), null, ms);
    expect(week[0]?.sub).toBe("Dalaman, Luaran, Elektrik · 0 j tersedia");
    expect(week[0]?.cells).toStrictEqual([{ kind: "leave", text: "Cuti", sub: "cuti sakit" }]);
    expect(assignRefusal({ code: VALIDATION_2, messageKey: ERROR_VALIDATION, fieldErrors: { reason: "error.required" } }, TECH_EXTERNAL_A, ms)).toBe("Sebab: diperlukan (1–1000 aksara)");
    expect(assignRefusal({ code: "CONFLICT", messageKey: "errors.assignment_overlap", fieldErrors: {} }, TECH_EXTERNAL_A, ms)).toBe("Bertindih dengan jadual yang disahkan untuk tech-external-a — pilih juruteknik lain.");
  });
});

// IR624: a former technician — access ended, or revoked by HQ — can take no job and stays out of the schedule's choice
// and week; the job's current technician stays, so the form can name whom it replaces
describe("the schedule's team", () => {
  it("keeps the technicians who can still work, and the current one", () => {
    const ended: ApiMember = { id: "m-e", userId: "u-e", displayName: "former", role: TECHNICIAN, validUntil: "2026-09-20T00:00:00Z", qualifications: [] };
    const revoked: ApiMember = { id: "m-r", userId: "u-r", displayName: "revoked", role: TECHNICIAN, revokedAt: "2026-09-19T00:00:00Z", qualifications: [] };
    const all = [...members, ended, revoked];
    expect(scheduleTeam(all, NOW, null).map((m) => m.id)).toStrictEqual(["m-a", "m-b", "m-c"]);
    expect(scheduleTeam(all, NOW, "m-e").map((m) => m.id)).toStrictEqual(["m-a", "m-b", "m-c", "m-e"]);
  });
});
