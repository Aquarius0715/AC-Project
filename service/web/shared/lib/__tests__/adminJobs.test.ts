import { describe, expect, it } from "vitest";
import { agreedSlots, classifyBy, controls, costLineOf, costTotals, delivery, extendDefault, money, newJobErrors, reportCard, returnReason, reviewModeOf, facts, filtersOf, hqRefusal, hqRow, longSlot, offerDefaults, preferredRows, slotText, sortOf, stageOf, stepper, type ApiHqJob, type ApiHqRow, type Names } from "@ac/web/lib/adminJobs";

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
  followUpOfJobId: null, followUpClass: null, assignment: null, offer: null, reportRefs: [], draftReportRef: null, costs: [], ...over,
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

  it("adds up costs per currency and checks a new line", () => {
    const lines = [
      { kind: "estimate", description: "Labour", visibility: "customer", amountMinor: 8000, currency: "MYR" }, { kind: "estimate", description: "Filter", visibility: "customer", amountMinor: 2500, currency: "MYR" },
      { kind: "actual", description: "Labour", visibility: "customer", amountMinor: 8000, currency: "MYR" }, { kind: "actual", description: "Filter (imported)", visibility: "internal", amountMinor: 620, currency: "USD" },
    ] as const;
    expect(costTotals([...lines])).toEqual({ estimate: "105.00 MYR", actual: "80.00 MYR + 6.20 USD" });
    expect(costTotals([])).toEqual({ estimate: "—", actual: "—" });
    expect(money(620, "USD")).toBe("6.20 USD");
    expect(costLineOf({ kind: "actual", description: " Drain pump ", visibility: "customer", amount: "45.5", currency: "MYR" }).line).toEqual({ kind: "actual", description: "Drain pump", visibility: "customer", amountMinor: 4550, currency: "MYR" });
    expect(costLineOf({ kind: "actual", description: "", visibility: "customer", amount: "1", currency: "MYR" }).error).toMatch(/description/);
    expect(costLineOf({ kind: "actual", description: "x", visibility: "customer", amount: "-1", currency: "MYR" }).error).toMatch(/non-negative/);
  });

  it("checks a new job and the review mode", () => {
    const ok = newJobErrors({ unitId: "u1", symptom: "Water dripping from the indoor unit", slots: [slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z")] }, NOW);
    expect(ok).toEqual({});
    const bad = newJobErrors({ unitId: "", symptom: "short", slots: [slot("2026-09-21T06:00:00Z", "2026-09-21T08:00:00Z"), slot("2026-09-22T02:00:00Z", "2026-09-22T08:00:00Z"), slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z"), slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z")] }, NOW);
    expect(bad).toEqual({ unitId: "Choose the unit.", symptom: "Describe the symptom in 10–2000 characters.", slot0: "Times start on a later day than today.", slot1: "Each time is 1–4 hours long.", slot3: "The times must differ." });
    expect([reviewModeOf({ contractorOrgId: null }), reviewModeOf({ contractorOrgId: "org-c" })]).toEqual(["normal", "hq_escalation"]);
    expect(extendDefault("2026-09-23T04:00:00Z")).toBe("2026-09-24T04:00:00.000Z");
  });

  it("shows the submitted report with the HQ review mode", () => {
    const report = {
      id: "r1", version: 1, jobId: "job-1", authorId: "u-tech", reviewAvailability: { allowed: true, reason: null }, measurements: [{ id: "m", metric: "refrigerant_pressure", value: 412, unit: "kPa", quality: "valid", observedAt: "2026-09-15T03:40:00Z" }],
      items: [
        { id: "i1", componentGroup: "indoor", componentKey: "filter", result: "normal", reason: null, evidenceIds: [], authorId: "u-tech", observedAt: "2026-09-15T03:40:00Z" },
        { id: "i2", componentGroup: "indoor", componentKey: "blower_fan", result: "attention", reason: "reduced on Low", evidenceIds: [], authorId: "u-tech", observedAt: "2026-09-15T03:40:00Z" },
        { id: "i3", componentGroup: "outdoor", componentKey: "fan", result: "not_applicable", reason: "no access", evidenceIds: [], authorId: "u-tech", observedAt: "2026-09-15T03:40:00Z" },
      ],
      parts: [], refrigerant: [], signOff: null, workText: "Cleaned the filter", nextAction: { kind: "none" }, attachmentRefs: [{ id: "a", name: "p.jpg", mime: "image/jpeg", size: 1, status: "ready" }],
      submittedAt: "2026-09-15T03:48:00Z", acceptedAt: null, reviewHistory: [{ reviewerUserId: "u-hq", reportVersion: 1, decision: "return", reason: "Re-check", occurredAt: "2026-09-15T05:00:00Z" }],
    } as never;
    const byUser = new Map([["u-tech", "tech-internal-a"], ["u-hq", "hq-operator"]]);
    const card = reportCard(job({ status: "submitted" }), report, byUser);
    expect([card.title, card.sub, card.mode, card.decide, card.tone]).toEqual(["Work report · version 1", "Submitted 09-15 11:48 by tech-internal-a · 3 checks · 1 photo", "normal", true, "primary"]);
    expect(card.rows.map((r) => [r.label, r.result])).toEqual([["Indoor unit — filter condition", "OK"], ["Indoor unit — blower fan", "Attention"], ["Outdoor unit — fan", "Not applicable"]]);
    expect(card.readings).toEqual([["Refrigerant pressure", "412 kPa · recorded"]]);
    expect(card.rework).toEqual([{ label: "Indoor unit — blower fan — attention (reduced on Low)", checked: true }, { label: "Refrigerant pressure — re-measure", checked: false }, { label: "Photos — add more evidence", checked: false }]);
    expect(reportCard(job({ status: "submitted", reportRefs: [{ reportId: "r1", reportVersion: 1 }, { reportId: "r1", reportVersion: 4 }] }), report, byUser).sub).toMatch(/^Resubmitted 09-15 11:48/);
    expect(card.reviews).toEqual([{ text: "v1 returned 09-15 13:00 by hq-operator — “Re-check”", tone: "warn" }]);
    expect([card.parts, card.next, card.signOff]).toEqual([[], "No follow-up needed", "Not signed"]);
    const full = reportCard(job({ status: "submitted" }), { ...(report as object), parts: [{ name: "Drain pump", quantity: 1, source: "hq_stock", replacesComponentKey: "drain_pan" }],
      nextAction: { kind: "follow_up", date: "2026-09-30T00:00:00Z", note: "Check the pump" }, signOff: { signerName: "Aiko", signedAt: "2026-09-15T03:50:00Z", absentReason: null } } as never, byUser);
    expect([full.parts, full.next, full.signOff]).toEqual([[["Drain pump × 1", "hq stock · replaces drain pan"]], "Follow-up 2026-09-30 — Check the pump", "Signed by Aiko 09-15 11:50"]);
    const esc = reportCard(job({ status: "submitted", contractorOrgId: "org-c" }), report, byUser);
    expect([esc.mode, esc.tone, esc.note]).toEqual(["hq_escalation", "warn", "The contractor reviews its own reports first. HQ may accept or return this one only by escalation, with a reason (IR31)."]);
    const own = reportCard(job({ status: "submitted" }), { ...(report as object), reviewAvailability: { allowed: false, reason: "self_authored" } } as never, byUser);
    expect([own.decide, own.note]).toEqual([false, "You submitted this report version, so you cannot accept or return it (self-approval is rejected)."]);
    expect(reportCard(job({ status: "completed" }), report, byUser).note).toMatch(/^Accepted/);
    expect(returnReason(["Airflow — attention"], " Re-check the blower. ")).toBe("Rework: Airflow — attention. Re-check the blower.");
    expect(returnReason([], "Only words")).toBe("Only words");
  });

  it("names the refusals of the review, cost and create actions", () => {
    expect(hqRefusal({ code: "FORBIDDEN", messageKey: "errors.self_review", fieldErrors: {} })).toBe("you contributed to this report version, so another reviewer must decide (self-approval is rejected).");
    expect(hqRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { symptom: "error.length", alternativeSlots: "error.count" } })).toBe("symptom: 10–2000 characters · alternativeSlots: up to two more times");
    expect(hqRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { accessValidUntil: "error.mustExtend" } })).toBe("accessValidUntil: the new end must be later than the current end and in the future");
  });

  it("books a plan's job at its occurrence when it has no preferred times", () => {
    const planJob = job({ origin: "periodic_plan", planId: "plan-1", occurrenceAt: "2026-12-08T02:00:00Z", requestedSlot: slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z"), preferredSlots: [] });
    expect(agreedSlots(planJob)).toEqual([slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z")]);
    expect(preferredRows(planJob, [[{ id: "m-int", displayName: "tech-internal-a" }]], NOW).map((r) => [r.rank, r.text, r.fits, r.plan])).toEqual([[1, "Tue 12-08 · 10:00–11:00", true, true]]);
    expect(agreedSlots(job({ preferredSlots: [] }))).toEqual([]);
    expect(agreedSlots(job({ planId: "plan-1", occurrenceAt: "2026-12-08T02:00:00Z" })).length).toBe(2); // the client asked for other times
    expect(hqRow(row({ origin: "periodic_plan", preferredSlots: [], requestedSlot: slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z") }), NOW, names, unitOrg).line).toBe("plan occurrence 12-08 10:00–11:00 · book it");
  });

  it("shows a completed job's assignment as ended (IR234)", () => {
    const done = delivery(job({ status: "completed", assignment: { technicianMembershipId: "m-int", scheduledStart: "2026-09-22T02:00:00Z", scheduledEnd: "2026-09-22T04:00:00Z", status: "completed", acknowledgement: "accepted", cantMakeReason: null, alternativeSlot: null } }), names, NOW)!;
    expect([done.lines, done.ack, done.cantMake]).toEqual([[["Technician", "tech-internal-a · assignment 09-22 10:00–12:00 · ended at completion"]], { tone: "ok", text: "tech-internal-a completed the job — the assignment ended with it, so tech-internal-a’s time is free again (IR234)." }, false]);
  });
});

