import { describe, expect, it } from "vitest";
import { assignRefusal, candidates, formMode, freeSlots, scheduleRow, weekGrid } from "@ac/web/lib/partnerSchedule";
import type { ApiDetail } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";

// The service partner's schedule (FR-P03, DD-P03, IR123, IR636): a row without a delegation window, an accepted offer
// at an address or without one, a job in progress, on hold or with an unknown unit; the form of a job with no time,
// a reassignment without a reason, the extension of a job in progress; the free spans beside a booking outside the
// day; the candidates without qualifications or with their current booking; the week's booked days of an unknown job
// and a technician without qualifications; and the assignment refusals.

const NOW = Date.parse("2026-09-21T01:30:00Z");
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const visit = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z");
type Summary = Extract<ApiPartnerJob, { projection: "summary" }>;
const summary = (over: Partial<Summary>): Summary => ({
  projection: "summary", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "accepted", dueAt: "2026-09-25T00:00:00Z", requestedSlot: visit, scheduledSlot: null,
  assignmentId: null, displayStatus: "accepted", technicianMembershipId: null, assignmentAcknowledgement: null, eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null }, ...over,
});
const offer = (over: Partial<Extract<ApiPartnerJob, { projection: "offer" }>>): ApiPartnerJob => ({
  projection: "offer", status: "accepted", jobId: "job-2-bbbb", jobVersion: 1, offerId: "o", type: "reactive", siteAddress: null, requestedSlot: visit, dueAt: "2026-09-25T00:00:00Z",
  offerExpiresAt: "2026-09-30T00:00:00Z", candidateSlots: [visit], ...over,
});
const detail = (over: Partial<ApiDetail>): ApiDetail => ({
  projection: "detail", id: "job-1", version: 4, unitId: "u1", type: "reactive", status: "assigned", symptom: "", origin: "client_request", alertIds: [], requestedSlot: visit, scheduledSlot: null,
  dueAt: "2026-09-25T00:00:00Z", startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null, offer: null, ...over,
});
const assignment = (acknowledgement: string, scheduledEnd = visit.endAt): NonNullable<ApiDetail["assignment"]> => ({
  acknowledgement, scheduledEnd, cantMakeReason: null, technicianMembershipId: "m-gone", scheduledStart: visit.startAt, status: "active",
});
const cap = (membershipId: string, over: Partial<ApiCapacity> = {}): ApiCapacity => ({
  membershipId, date: "2026-09-22", availableSlots: [slot("2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z")], assignedSlots: [], availableMinutes: 480, assignedMinutes: 0, unavailability: null, ...over,
});
const tech = (id: string, qualifications?: ApiMember["qualifications"]): ApiMember => ({ id, qualifications, userId: `u-${id}`, displayName: `Tech ${id}`, role: "technician" });
const units = new Map([["u1", "Lobby AC"]]);
const names = new Map([["m-a", "Aiman"]]);
const row = (j: ApiPartnerJob) => scheduleRow(j, NOW, units, names);
const refusal = (messageKey: string) => assignRefusal({ messageKey, code: "CONFLICT", fieldErrors: {} }, "Aiman");

describe("a schedule row", () => {
  it("shows an accepted offer at its address or as an accepted job, its window unknown", () => {
    expect(row(offer({ siteAddress: "Jalan Ampang 1" }))).toMatchObject({ title: "Jalan Ampang 1", line: "Accepted · delegation starts later", window: null, windowText: "—", left: "—", elapsedPct: 0 });
    expect(row(offer({}))?.title).toBe("Accepted job");
  });

  it("names the technician in progress, a held job, and an unknown unit", () => {
    expect(row(summary({ status: "in_progress", technicianMembershipId: "m-a" }))?.line).toBe("In progress · Aiman");
    expect(row(summary({ status: "on_hold", unitId: "u-gone" }))).toMatchObject({ line: "On hold by HQ", tone: "warn", title: "Unit" });
    expect(row(summary({ status: "rework_requested" }))?.line).toBe("Rework · technician");
    expect(row(summary({ accessValidFrom: "2026-09-21T01:30:00Z", accessValidUntil: "2026-09-21T01:30:00Z" }))?.elapsedPct).toBe(0); // an empty window: no bar
  });
});

describe("the form of a job", () => {
  it("cannot book a job with no agreed time", () => {
    expect(formMode(detail({ status: "accepted" }), "job-1", NOW, names)).toMatchObject({ kind: "blocked", slot: null, banner: { tone: "warn" } });
  });

  it("offers a reassignment of an unknown technician who can't make it, without a reason", () => {
    const mode = formMode(detail({ assignment: assignment("cant_make"), offer: { id: "o", termsVersion: "t", candidateSlots: [visit], offeredAt: "", accessValidFrom: "", accessValidUntil: "", decision: null, decidedAt: null } }), "job-1", NOW, names);
    expect(mode.kind === "reassign" ? [mode.currentTech, mode.banner?.text] : []).toStrictEqual(["technician", "technician can’t make this time. Reassign another technician, or pick another time the client agreed — or ask HQ to propose a new time to the client."]);
  });

  it("extends a job in progress, and reschedules a held one whose window ended", () => {
    expect(formMode(detail({ status: "in_progress", assignment: assignment("accepted") }), "job-1", NOW, names)).toMatchObject({ kind: "extend", title: "Reassign — job-1 (in progress)", reasonRequired: true });
    expect(formMode(detail({ status: "rework_requested", scheduledSlot: visit }), "job-1", NOW, names).title).toBe("Reassign — job-1 (rework)");
    const late = formMode(detail({ status: "on_hold", assignment: assignment("accepted", "2026-09-20T00:00:00Z") }), "job-1", NOW, names);
    expect([late.title, late.banner?.tone]).toStrictEqual(["Reschedule — job-1", "crit"]);
  });
});

describe("free time, candidates and the week", () => {
  it("leaves a booking outside an available span out of its free time, and nothing without capacity", () => {
    expect(freeSlots(cap("m-a", { assignedSlots: [slot("2026-09-22T10:00:00Z", "2026-09-22T11:00:00Z"), slot("2026-09-21T00:00:00Z", "2026-09-21T01:00:00Z")] }))).toStrictEqual(["09:00–17:00"]);
    expect(freeSlots(undefined)).toStrictEqual([]);
  });

  it("names a technician without qualifications, and the current one booked at this time", () => {
    const rows = candidates([tech("m-x"), tech("m-cur", [{ code: "demo_indoor" }])], new Set(), [], visit, [cap("m-cur", { assignedSlots: [visit] })], "m-cur");
    expect(rows.map((r) => [r.id, r.sub.split(" · ", 1)[0], r.badge.text])).toStrictEqual([["m-cur", "Indoor unit work", "Current · booked here"], ["m-x", "no qualifications", "Not eligible"]]);
  });

  it("shows a booked day of an unknown job, and a technician without qualifications or hours", () => {
    const [week] = weekGrid(["2026-09-22", "2026-09-23"], [tech("m-x")], [[cap("m-x", { assignedSlots: [visit] })], []], new Map(), null);
    expect(week?.sub).toBe("— · 8 h avail.");
    expect(week?.cells).toStrictEqual([{ kind: "booked", text: "10:00–12:00", sub: "booked" }, { kind: "none", text: "no hours" }]);
  });
});

describe("the assignment refusals", () => {
  it("words each known refusal", () => {
    expect(["errors.qualification_missing", "errors.technician_out_of_scope", "errors.technician_not_allowed", "error.invalidState"].map((k) => refusal(k))).toStrictEqual([
      "Aiman’s qualification is not valid for this slot (revoked or expiring, FORBIDDEN) — choose another technician.", "Aiman cannot be assigned to this unit.", "Aiman cannot be assigned to this unit.",
      "The job can no longer be scheduled in its current state — the latest state is shown.",
    ]);
  });
});
