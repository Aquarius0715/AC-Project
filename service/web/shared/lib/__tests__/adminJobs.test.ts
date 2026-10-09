import { describe, expect, it } from "vitest";
import { classifyBy, controls, delivery, facts, filtersOf, hqRefusal, hqRow, longSlot, offerDefaults, preferredRows, slotText, sortOf, stageOf, stepper, type ApiHqJob, type ApiHqRow, type Names } from "@ac/web/lib/adminJobs";

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const names: Names = {
  units: new Map([["u1", "Bedroom AC"]]), customers: new Map([["org-a", "Demo Customer A"]]),
  people: new Map([["m-int", "tech-internal-a"], ["m-ext", "tech-external-a"]]), orgs: new Map([["org-c", "Demo Contractor A"]]),
};
const unitOrg = new Map([["u1", "org-a"]]);
const row = (over: Record<string, unknown>) => ({
  projection: "summary", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "requested", dueAt: "2026-09-22T04:00:00Z", requestedSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"),
  scheduledSlot: null, assignmentId: null, origin: "client_request", preferredSlots: [slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z")],
  severity: "normal", displayStatus: "requested", assignmentAcknowledgement: null, technicianMembershipId: null, accessValidFrom: null, accessValidUntil: null, ...over,
}) as unknown as ApiHqRow;
const job = (over: Record<string, unknown>) => ({
  projection: "detail", id: "job-1-aaaa", version: 4, unitId: "u1", type: "reactive", status: "requested", symptom: "Water dripping", contactWindow: "9:00–18:00", origin: "client_request", planId: null, occurrenceAt: null,
  requestedSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), preferredSlots: [slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z")], preferenceRound: 1,
  slotProposal: null, partnerSlotProposal: null, scheduledSlot: null, dueAt: "2026-09-22T04:00:00Z", startedAt: null, completedAt: null, contractorOrgId: null, createdAt: "2026-09-18T01:00:00Z",
  followUpOfJobId: null, followUpClass: null, assignment: null, offer: null, ...over,
}) as unknown as ApiHqJob;

describe("HQ maintenance jobs", () => {
  it("builds the filters of the scope and the filter bar", () => {
    expect(filtersOf({ customerId: "c1", origin: "periodic_plan", delivery: "internal", assignee: "m-int", overdue: "1", stage: "time_proposed" }, "org-hq")).toEqual({ customerId: "c1", origin: "periodic_plan", organizationId: "org-hq", membershipId: "m-int", overdueOnly: true, proposalPending: true });
    expect(filtersOf({ delivery: "org-c", stage: "requested" }, "org-hq")).toEqual({ organizationId: "org-c", status: "requested", proposalPending: false });
    expect(filtersOf({ stage: "requested" }, null, false)).toEqual({});
    expect([stageOf("on_hold"), stageOf("x"), sortOf("dueAt"), sortOf("x")]).toEqual(["on_hold", null, "dueAt", "status"]);
  });

  it("describes each row with what is next", () => {
    expect(hqRow(row({}), NOW, names, unitOrg)).toMatchObject({ title: "Bedroom AC", customer: "Demo Customer A", type: "Repair", status: "requested", line: "2 preferred times · book one or propose" });
    expect(hqRow(row({ displayStatus: "time_proposed" }), NOW, names, unitOrg)).toMatchObject({ status: "time_proposed", line: "Time proposed · waiting for the client" });
    const late = hqRow(row({ status: "assigned", displayStatus: "assigned", scheduledSlot: slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z"), technicianMembershipId: "m-int", assignmentAcknowledgement: "cant_make" }), NOW, names, unitOrg);
    expect([late.badge, late.line, late.lineTone]).toEqual([{ status: "assigned", overdue: true }, "tech-internal-a · can’t make it ⚠ · window ended", "warn"]);
  });

  it("shows the stepper, the facts and the delivery", () => {
    expect(stepper("in_progress", true).map((s) => s.state)).toEqual(["done", "done", "current", "todo", "todo"]);
    expect(stepper("accepted", false).map((s) => s.label)).toEqual(["Requested", "Offered", "Accepted", "Assigned", "In progress", "Submitted", "Completed"]);
    expect(stepper("completed", true).every((s) => s.state === "done")).toBe(true);
    expect(facts(job({}), names).map((f) => [f.label, f.value, f.sub])).toEqual([
      ["Requested window", "09-22 10:00–12:00", "Asia/Kuala_Lumpur"], ["Due", "09-22 12:00", "= requested end"], ["Scheduled", "not booked", "—"], ["Symptom", "Water dripping", "contact 9:00–18:00"],
    ]);
    const internal = delivery(job({ status: "assigned", assignment: { technicianMembershipId: "m-int", scheduledStart: "2026-09-22T02:00:00Z", scheduledEnd: "2026-09-22T04:00:00Z", status: "active", acknowledgement: "cant_make", cantMakeReason: "sick", alternativeSlot: null } }), names, NOW)!;
    expect([internal.kind, internal.lines, internal.ack?.text, internal.cantMake]).toEqual(["internal", [["Technician", "tech-internal-a · assignment 09-22 10:00–12:00"]], "tech-internal-a can’t make this time: “sick”. Any new time needs the client’s approval.", true]);
    const offered = delivery(job({ status: "offered", contractorOrgId: "org-c", offer: { id: "o", contractorOrgId: "org-c", visitSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), offerExpiresAt: "2026-09-21T10:00:00Z", accessValidFrom: "2026-09-21T01:00:00Z", accessValidUntil: "2026-09-23T04:00:00Z", decision: null, decidedAt: null, declineReason: null } }), names, NOW)!;
    expect([offered.title, offered.lines]).toEqual(["Delivery · contractor Demo Contractor A", [["Visit", "09-22 10:00–12:00 · offer open · expires 09-21 18:00"], ["Access", "09-21 09:00 → 09-23 12:00"], ["Technician", "not assigned by the contractor yet"]]]);
    expect(delivery(job({}), names, NOW)).toBeNull();
  });

  it("checks the preferred times, the offer defaults and the controls", () => {
    const rows = preferredRows(job({}), [[{ id: "m-int", displayName: "tech-internal-a" }], [{ id: "m-int", displayName: "tech-internal-a" }]], NOW);
    expect(rows.map((r) => [r.rank, r.text, r.hq, r.fits])).toEqual([[1, "Tue 09-22 · 10:00–12:00", "tech-internal-a", true], [2, "Sun 09-20 · 10:00–12:00", "in the past", false]]);
    expect(longSlot(slot("2026-09-28T06:00:00Z", "2026-09-28T08:00:00Z"))).toBe("Mon 09-28 · 14:00–16:00");
    expect(slotText(slot("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z"))).toBe("09-14 08:00 → 09-20 08:00");
    expect(offerDefaults(slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), NOW)).toEqual({ offerExpiresAt: "2026-09-22T01:30:00.000Z", accessValidFrom: "2026-09-21T01:30:00.000Z", accessValidUntil: "2026-09-23T04:00:00.000Z" });
    expect(offerDefaults(slot("2026-09-21T05:00:00Z", "2026-09-21T07:00:00Z"), NOW).offerExpiresAt).toBe("2026-09-21T05:00:00.000Z");
    expect(controls("in_progress")).toEqual({ hold: true, resume: false, cancel: false, cancelWhy: "Put the job on hold first (IR56)." });
    expect(controls("on_hold")).toEqual({ hold: false, resume: true, cancel: true, cancelWhy: null });
    expect(controls("completed").cancelWhy).toBe("Completed and cancelled jobs cannot be cancelled.");
    expect(classifyBy("2026-09-18T03:05:00Z")).toBe("09-21 11:05"); // Fri → Mon
  });

  it("names the refusals", () => {
    expect(hqRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { startAt: "errors.slot_not_agreed" } })).toBe("startAt: only one of the client’s preferred times (or an accepted proposal) can be booked");
    expect(hqRefusal({ code: "CONFLICT", messageKey: "errors.proposal_pending", fieldErrors: {} })).toBe("Not saved (CONFLICT): a proposal is already waiting for the client — withdraw it first.");
    expect(hqRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })).toBe("The job no longer exists in your scope.");
  });
});
