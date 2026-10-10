import { describe, expect, it } from "vitest";
import { agreedSlots, classifyBy, controls, costLineOf, costTotals, delivery, extendDefault, fromZonedInput, LISTED, money, newJobErrors, periodChip, reportCard, returnReason, reviewModeOf, facts, filtersOf, hqRefusal, hqRow, longSlot, offerDefaults, preferredRows, slotText, sortOf, stageOf, stepper, tomorrowIn, zonedInput, type ApiHqJob, type ApiHqRow, type Names } from "@ac/web/lib/adminJobs";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

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
    // the type and "not cancelled" go to jobs.list, so its total counts what the list shows (IR290)
    expect(filtersOf({ type: "periodic" }, null)).toEqual({ type: "periodic", statuses: LISTED });
    expect([filtersOf({ type: "repair" }, null).type, LISTED.includes("cancelled"), filtersOf({ stage: "on_hold" }, null).statuses]).toEqual([undefined, false, undefined]);
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
      ["Requested window", "22 Sept, 10:00 am – 12:00 pm MYT", "the client’s 1st preferred time"], ["Due", "22 Sept 2026, 12:00 pm MYT", "= requested end"], ["Scheduled", "not booked", "—"], ["Symptom", "Water dripping", "contact 9:00–18:00"],
    ]);
    const internal = delivery(job({ status: "assigned", assignment: { technicianMembershipId: "m-int", scheduledStart: "2026-09-22T02:00:00Z", scheduledEnd: "2026-09-22T04:00:00Z", status: "active", acknowledgement: "cant_make", cantMakeReason: "sick", alternativeSlot: null } }), names, NOW)!;
    expect([internal.kind, internal.lines, internal.ack?.text, internal.cantMake]).toEqual(["internal", [["Technician", "tech-internal-a · assignment 22 Sept, 10:00 am – 12:00 pm MYT"]], "tech-internal-a can’t make this time: “sick”. Any new time needs the client’s approval.", true]);
    const offered = delivery(job({ status: "offered", contractorOrgId: "org-c", offer: { id: "o", contractorOrgId: "org-c", visitSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), offerExpiresAt: "2026-09-21T10:00:00Z", accessValidFrom: "2026-09-21T01:00:00Z", accessValidUntil: "2026-09-23T04:00:00Z", decision: null, decidedAt: null, declineReason: null } }), names, NOW)!;
    expect([offered.title, offered.lines]).toEqual(["Delivery · contractor Demo Contractor A", [["Visit", "22 Sept, 10:00 am – 12:00 pm MYT · offer open · expires 21 Sept 2026, 6:00 pm MYT"], ["Access", "21 Sept, 9:00 am – 23 Sept, 12:00 pm MYT"], ["Technician", "not assigned by the contractor yet"]]]);
    expect(delivery(job({}), names, NOW)).toBeNull();
  });

  it("checks the preferred times, the offer defaults and the controls", () => {
    const rows = preferredRows(job({}), [[{ id: "m-int", displayName: "tech-internal-a" }], [{ id: "m-int", displayName: "tech-internal-a" }]], NOW);
    expect(rows.map((r) => [r.rank, r.text, r.hq, r.fits])).toEqual([[1, "Tue, 22 Sept, 10:00 am – 12:00 pm MYT", "tech-internal-a", true], [2, "Sun, 20 Sept, 10:00 am – 12:00 pm MYT", "in the past", false]]);
    expect(longSlot(slot("2026-09-28T06:00:00Z", "2026-09-28T08:00:00Z"))).toBe("Mon, 28 Sept, 2:00 – 4:00 pm MYT");
    expect(slotText(slot("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z"))).toBe("14 Sept, 8:00 am – 20 Sept, 8:00 am MYT");
    expect(offerDefaults(slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), NOW)).toEqual({ offerExpiresAt: "2026-09-22T01:30:00.000Z", accessValidFrom: "2026-09-21T01:30:00.000Z", accessValidUntil: "2026-09-23T04:00:00.000Z" });
    expect(offerDefaults(slot("2026-09-21T05:00:00Z", "2026-09-21T07:00:00Z"), NOW).offerExpiresAt).toBe("2026-09-21T05:00:00.000Z");
    expect(controls("in_progress")).toEqual({ hold: true, resume: false, cancel: false, cancelWhy: "Put the job on hold first (IR56)." });
    expect(controls("on_hold")).toEqual({ hold: false, resume: true, cancel: true, cancelWhy: null });
    expect(controls("completed").cancelWhy).toBe("Completed and cancelled jobs cannot be cancelled.");
    expect(classifyBy("2026-09-18T03:05:00Z")).toBe("21 Sept 2026, 11:05 am MYT"); // Fri → Mon, a Kuala Lumpur business day
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
    expect(bad).toEqual({ unitId: "Choose the unit.", symptom: "Describe the symptom in 10–2000 characters.", slot0: "Times start on a later day than today (Kuala Lumpur).", slot1: "Each time is 1–4 hours long.", slot3: "The times must differ." });
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
    expect([card.title, card.sub, card.mode, card.decide, card.tone]).toEqual(["Work report · version 1", "Submitted 15 Sept 2026, 11:48 am MYT by tech-internal-a · 3 checks · 1 photo", "normal", true, "primary"]);
    expect(card.rows.map((r) => [r.label, r.result])).toEqual([["Indoor unit — filter condition", "OK"], ["Indoor unit — blower fan", "Attention"], ["Outdoor unit — fan", "Not applicable"]]);
    expect(card.readings).toEqual([["Refrigerant pressure", "412 kPa · recorded"]]);
    expect(card.rework).toEqual([{ label: "Indoor unit — blower fan — attention (reduced on Low)", checked: true }, { label: "Refrigerant pressure — re-measure", checked: false }, { label: "Photos — add more evidence", checked: false }]);
    expect(reportCard(job({ status: "submitted", reportRefs: [{ reportId: "r1", reportVersion: 1 }, { reportId: "r1", reportVersion: 4 }] }), report, byUser).sub).toMatch(/^Resubmitted 15 Sept 2026, 11:48 am MYT/);
    expect(card.reviews).toEqual([{ text: "v1 returned 15 Sept 2026, 1:00 pm MYT by hq-operator — “Re-check”", tone: "warn" }]);
    expect([card.parts, card.next, card.signOff]).toEqual([[], "No follow-up needed", "Not signed"]);
    const full = reportCard(job({ status: "submitted" }), { ...(report as object), parts: [{ name: "Drain pump", quantity: 1, source: "hq_stock", replacesComponentKey: "drain_pan" }],
      nextAction: { kind: "follow_up", date: "2026-09-30T00:00:00Z", note: "Check the pump" }, signOff: { signerName: "Aiko", signedAt: "2026-09-15T03:50:00Z", absentReason: null } } as never, byUser);
    expect([full.parts, full.next, full.signOff]).toEqual([[["Drain pump × 1", "hq stock · replaces drain pan"]], "Follow-up 2026-09-30 — Check the pump", "Signed by Aiko 15 Sept 2026, 11:50 am MYT"]);
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
    expect(preferredRows(planJob, [[{ id: "m-int", displayName: "tech-internal-a" }]], NOW).map((r) => [r.rank, r.text, r.fits, r.plan])).toEqual([[1, "Tue, 8 Dec, 10:00 – 11:00 am MYT", true, true]]);
    expect(agreedSlots(job({ preferredSlots: [] }))).toEqual([]);
    expect(agreedSlots(job({ planId: "plan-1", occurrenceAt: "2026-12-08T02:00:00Z" })).length).toBe(2); // the client asked for other times
    expect(hqRow(row({ origin: "periodic_plan", preferredSlots: [], requestedSlot: slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z") }), NOW, names, unitOrg).line).toBe("plan occurrence 8 Dec, 10:00 – 11:00 am MYT · book it");
  });

  it("shows a completed job's assignment as ended (IR234)", () => {
    const done = delivery(job({ status: "completed", assignment: { technicianMembershipId: "m-int", scheduledStart: "2026-09-22T02:00:00Z", scheduledEnd: "2026-09-22T04:00:00Z", status: "completed", acknowledgement: "accepted", cantMakeReason: null, alternativeSlot: null } }), names, NOW)!;
    expect([done.lines, done.ack, done.cantMake]).toEqual([[["Technician", "tech-internal-a · assignment 22 Sept, 10:00 am – 12:00 pm MYT · ended at completion"]], { tone: "ok", text: "tech-internal-a completed the job — the assignment ended with it, so tech-internal-a’s time is free again (IR234)." }, false]);
  });
});

describe("HQ Jobs tab in Malay with the display time zone (IR290)", () => {
  it("words the rows, the facts, the delivery and the refusals, with the times in GMT+9", () => {
    expect(hqRow(row({}), NOW, names, unitOrg, MS)).toMatchObject({ type: "Pembaikan", line: "2 masa pilihan · tempah satu atau cadangkan" });
    expect(periodChip({ from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:00:00.000Z" }, MS)).toMatch(/^Masa diminta 14 Sep.* GMT\+9$/);
    expect(stepper("accepted", false, MS.t).map((s) => s.label).slice(0, 3)).toEqual(["Diminta", "Ditawarkan", "Diterima"]);
    const f = facts(job({}), names, MS);
    expect([f[0].label, f[0].sub, f[2].value]).toEqual(["Tetingkap yang diminta", "masa pilihan pertama pelanggan", "belum ditempah"]);
    expect(f[0].value).toMatch(/GMT\+9$/); // 11:00 – 1:00 in Tokyo, the client's 10:00–12:00 in Kuala Lumpur
    expect(controls("in_progress", MS.t).cancelWhy).toBe("Tangguhkan kerja dahulu (IR56).");
    expect(newJobErrors({ unitId: "", symptom: "x", slots: [slot("2026-09-21T06:00:00Z", "2026-09-21T08:00:00Z")] }, NOW, MS.t)).toEqual({ unitId: "Pilih unit.", symptom: "Terangkan simptom dalam 10–2000 aksara.", slot0: "Masa bermula pada hari selepas hari ini (Kuala Lumpur)." });
    expect(hqRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} }, MS.t)).toBe("Kerja ini tidak lagi wujud dalam skop anda.");
    expect(returnReason(["Airflow — attention"], "Re-check", MS.t)).toBe("Kerja semula: Airflow — attention. Re-check");
  });

  it("reads the times HQ types in the display time zone and starts the dialogs tomorrow there (NFR-08)", () => {
    expect(zonedInput("2026-09-22T01:30:00.000Z", "Asia/Tokyo")).toBe("2026-09-22T10:30");
    expect(fromZonedInput("2026-09-22T10:30", "Asia/Tokyo")).toBe("2026-09-22T01:30:00.000Z");
    expect(fromZonedInput(zonedInput("2026-09-22T01:30:00.000Z", "Asia/Kuala_Lumpur"), "Asia/Kuala_Lumpur")).toBe("2026-09-22T01:30:00.000Z");
    expect([tomorrowIn(Date.parse("2026-09-21T15:30:00Z"), "Asia/Tokyo"), tomorrowIn(Date.parse("2026-09-21T15:30:00Z"), "Asia/Kuala_Lumpur")]).toEqual(["2026-09-23", "2026-09-22"]); // 00:30 in Tokyo is already the 22nd
  });
});
