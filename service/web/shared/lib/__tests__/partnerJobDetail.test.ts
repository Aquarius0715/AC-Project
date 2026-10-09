import { describe, expect, it } from "vitest";
import { decisionRefusal, delegationLeft, detailBanner, eventRows, fits, freeHours, qualificationLabel, qualified, timeline, type ApiDetail } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiMember } from "@ac/web/lib/partnerOverview";

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const quals = (codes: string[], over: Record<string, unknown> = {}) => codes.map((code) => ({ code, validFrom: "2026-09-01T00:00:00Z", validUntil: "2027-03-31T00:00:00Z", revokedAt: null, ...over }));
const members = [
  { id: "m-a", userId: "u-a", displayName: "tech-external-a", role: "technician", qualifications: quals(["demo_indoor", "demo_outdoor", "demo_electrical"]) },
  { id: "m-b", userId: "u-b", displayName: "tech-external-a2", role: "technician", qualifications: quals(["demo_indoor"]) },
  { id: "m-c", userId: "u-c", displayName: "coordinator", role: "contractor" },
] as unknown as ApiMember[];
const cap = (id: string, date: string, avail: [string, string][], assigned: [string, string][], am: number | null, as = 0): ApiCapacity =>
  ({ membershipId: id, date, availableSlots: avail.map(([s, e]) => slot(s, e)), assignedSlots: assigned.map(([s, e]) => slot(s, e)), availableMinutes: am, assignedMinutes: as, unavailability: null });

describe("partner job detail", () => {
  it("checks qualifications at the visit and labels them", () => {
    expect(qualificationLabel("demo_outdoor")).toBe("Outdoor unit work");
    expect(qualified(members[0], ["demo_indoor", "demo_electrical"], "2026-09-22T01:00:00Z")).toBe(true);
    expect(qualified(members[1], ["demo_indoor", "demo_electrical"], "2026-09-22T01:00:00Z")).toBe(false);
    expect(qualified({ ...members[0], qualifications: quals(["demo_indoor"], { revokedAt: "2026-09-10T00:00:00Z" }) } as unknown as ApiMember, ["demo_indoor"], "2026-09-22T01:00:00Z")).toBe(false);
    expect(qualified(members[0], ["demo_indoor"], "2027-04-01T00:00:00Z")).toBe(false); // expired by the visit
    expect(freeHours(cap("m-a", "d", [], [], 480, 120))).toBe(6);
    expect(freeHours(cap("m-a", "d", [], [], null))).toBeNull();
  });

  it("tells which technicians can take the visit", () => {
    const visit = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"); // 10:00–12:00 KL
    const days = [
      { date: "2026-09-22", capacity: [cap("m-a", "2026-09-22", [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], [], 480), cap("m-b", "2026-09-22", [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], [], 480)] },
      { date: "2026-09-23", capacity: [cap("m-a", "2026-09-23", [], [], 480, 480)] },
    ];
    const rows = fits(members, ["demo_indoor", "demo_electrical"], visit, days);
    expect(rows.map((r) => [r.name, r.badge.text, r.days.map((d) => d.text).join(",")])).toEqual([
      ["tech-external-a", "Fits", "8 h,0 h"],
      ["tech-external-a2", "Not qualified", "8 h,—"],
    ]);
    expect(rows[1].sub).toBe("missing Electrical work");
    expect(rows[0].days[0].label).toBe("Tue");
    const busy = fits(members, ["demo_indoor"], visit, [{ date: "2026-09-22", capacity: [cap("m-a", "2026-09-22", [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], [["2026-09-22T03:00:00Z", "2026-09-22T05:00:00Z"]], 480, 120)] }]);
    expect(busy[0].badge).toEqual({ text: "Busy at the visit", tone: "warn" });
    const detail = fits(members, ["demo_indoor", "demo_electrical"], visit, days, "detail");
    expect(detail.map((r) => [r.sub, r.badge.text, r.days.length])).toEqual([["Qualified · 8 h free 09-22–09-23", "Best fit", 0], ["Missing Electrical work", "Not eligible", 0]]);
    const own = fits(members, ["demo_indoor"], visit, [{ date: "2026-09-22", capacity: [cap("m-a", "2026-09-22", [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], [["2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"]], 480, 120)] }], "detail", "m-a");
    expect([own[0].badge.text, own[0].sub]).toEqual(["Assigned", "Qualified · assigned to this job"]);
  });

  it("builds the status timeline from the events", () => {
    const ev = (action: string, at: string) => ({ id: action, jobId: "j", actorUserId: null, action, occurredAt: at });
    const events = [ev("job.created", "2026-09-13T01:00:00Z"), ev("job.offered", "2026-09-13T10:20:00Z"), ev("offer.accepted", "2026-09-14T02:02:00Z")];
    const t = timeline("accepted", events, false, "periodic_plan");
    expect(t[0].sub).toBe("periodic plan");
    expect(t.map((s) => [s.label, s.state, s.at])).toEqual([
      ["Requested", "done", "2026-09-13T01:00:00Z"], ["Offered", "done", "2026-09-13T10:20:00Z"], ["Accepted", "current", "2026-09-14T02:02:00Z"],
      ["Assigned", "todo", null], ["In progress", "todo", null], ["Submitted", "todo", null], ["Completed", "todo", null],
    ]);
    expect(timeline("completed", events, true).every((s) => s.state === "done")).toBe(true);
    expect(timeline("rework_requested", events, true).find((s) => s.state === "current")?.label).toBe("In progress");
    expect(eventRows(events).map((r) => r.text)).toEqual(["Job accepted", "Offer received from HQ", "Job created"]);
  });

  it("states the delegated job's banner, countdown and refusals", () => {
    const base = { projection: "detail", id: "j", version: 4, unitId: "u", type: "reactive", status: "accepted", symptom: "", origin: "client_request", alertIds: [], requestedSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), scheduledSlot: null, dueAt: "2026-09-25T00:00:00Z", startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null, offer: { id: "o", termsVersion: "t", visitSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), offeredAt: "2026-09-13T10:20:00Z", accessValidFrom: "2026-09-14T00:00:00Z", accessValidUntil: "2026-09-28T16:00:00Z", decision: "accept", decidedAt: "2026-09-14T02:02:00Z" } } as ApiDetail;
    expect(detailBanner(base, null, NOW)?.text).toBe("Accepted · decided 2026-09-14 10:02 (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.");
    const assigned = { ...base, status: "assigned", assignment: { technicianMembershipId: "m-a", scheduledStart: "2026-09-22T02:00:00Z", scheduledEnd: "2026-09-22T04:00:00Z", status: "active", acknowledgement: "cant_make", cantMakeReason: "sick" } };
    expect(detailBanner(assigned, "tech-external-a", NOW)).toEqual({ tone: "warn", text: "Assigned — tech-external-a · 09-22 10:00–12:00 · the technician can’t make it (“sick”) — reassign in Schedule." });
    expect(detailBanner({ ...assigned, assignment: { ...assigned.assignment!, scheduledEnd: "2026-09-20T09:00:00Z" } }, "tech-external-a", NOW)?.tone).toBe("crit");
    expect(delegationLeft("2026-09-28T16:00:00Z", NOW)).toBe("Delegation ends 2026-09-29 00:00 — 7 d 14 h left.");
    expect(delegationLeft("2026-09-28T16:00:00Z", NOW, true)).toBe("Delegation ends 2026-09-29 00:00 — 7 d 14 h to schedule the work.");
    expect(delegationLeft("2026-09-20T16:00:00Z", NOW)).toBe("The delegation period has ended.");
    expect(decisionRefusal({ code: "CONFLICT", messageKey: "errors.offer_expired", fieldErrors: {} })).toMatch(/expired or was cancelled/);
    expect(decisionRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { reason: "error.length" } })).toBe("reason: required (1–1000 characters)");
    expect(decisionRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { slot: "error.slotRules" } })).toBe("slot: pick a time on a later day, 1–4 hours long");
    expect(decisionRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { _: "error.malformedInput" } })).toMatch(/^the request was not understood/);
    expect(decisionRefusal({ code: "FORBIDDEN", messageKey: "errors.qualification_missing", fieldErrors: {} })).toMatch(/lacks a required qualification/);
    expect(decisionRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })).toBe("This offer is no longer available to your company.");
  });
});
