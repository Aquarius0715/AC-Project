import { describe, expect, it } from "vitest";
import {
  clientJobRefusal, clientRows, contactError, declinedNotice, detailFacts, feedback, followUpText, notesOf, planVisit, preferredError, preferredLines, proposalCard,
  type ApiClientJob, type ApiClientRow,
} from "@ac/web/lib/customerMaintenance";

// The client's maintenance requests (FR-C09, FR-C17, IR110, IR113): what each list row says is next for a plan visit,
// a booked partner, a completed or confirmed job; the detail's facts for a plan visit, a follow-up with photos and a
// technician who accepted; the preferred times past the third; a contractor's proposal; a declined notice with an
// unknown reason; a plan visit without a time; a rated job past its edit time; and the refusals in words.

const NOW = Date.parse("2026-09-14T01:00:00Z");
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const visit = slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z");
const row = (id: string, over: Partial<ApiClientRow>): ApiClientRow => ({
  projection: "summary", id, version: 1, unitId: "u1", type: "reactive", status: "requested", displayStatus: "requested", origin: "client_request", dueAt: "2026-09-16T04:00:00Z",
  requestedSlot: visit, scheduledSlot: null, preferredSlots: [], assignmentAcknowledgement: null, ...over,
});
const job = (over: Partial<ApiClientJob>): ApiClientJob => ({
  projection: "detail", id: "job-1", version: 3, unitId: "u1", type: "reactive", status: "requested", symptom: "", contactWindow: null, origin: "client_request", planId: null, occurrenceAt: null,
  requestedSlot: visit, preferredSlots: [], preferenceRound: 1, slotProposal: null, scheduledSlot: null, dueAt: "2026-09-16T04:00:00Z", completedAt: null, contractorOrgId: null, followUpOfJobId: null,
  followUpClass: null, rating: null, customerConfirmedAt: null, reportRefs: [], assignment: null, ...over,
});
const unitOf = () => ({ name: "Bedroom AC", place: "" });
const lines = (rows: ApiClientRow[]) => clientRows(rows, unitOf).map((r) => r.line);
const proposal = { id: "p-1", source: "contractor" as const, slot: visit, hold: { kind: "contractor" as const }, message: "Parts late", replyBy: "2026-09-15T00:00:00Z", status: "pending", decidedAt: null, declineReason: null, declineComment: null };

const note = (id: string, actorUserId: string | null) => ({ id, actorUserId, action: "note", occurredAt: `2026-09-1${id}T00:00:00Z`, note: { visibility: "customer", message: id } });

describe("what a request row says is next", () => {
  it("for a requested job without preferred times, and a booked partner on a plan visit or a confirmed time", () => {
    expect(lines([row("a", { origin: "periodic_plan" }), row("b", {})])).toStrictEqual([expect.stringMatching(/^Plan visit .+ · HQ is booking it$/), "Waiting for HQ"]);
    expect(lines([row("c", { displayStatus: "offered", origin: "periodic_plan" }), row("d", { displayStatus: "accepted", scheduledSlot: visit })])).toStrictEqual([
      expect.stringMatching(/^Scheduled .+ · booking the technician$/), expect.stringMatching(/^Confirmed .+ · booking the technician$/),
    ]);
  });

  it("for a completed job without a visit time, a cancelled one, and an assigned job with or without a time", () => {
    const rows = clientRows([row("e", { displayStatus: "completed" }), row("f", { displayStatus: "cancelled" }), row("g", { displayStatus: "assigned", origin: "periodic_plan" }), row("h", { displayStatus: "in_progress", type: "commissioning" })], unitOf);
    expect(rows.map((r) => [r.id, r.line, r.icon])).toStrictEqual([["h", "Confirmed ", "time"], ["g", "Scheduled ", "time"], ["e", "Completed · report available", "success"], ["f", "Cancelled", "cancelled"]]); // in the business order
    expect([rows[0]?.type, rows[1]?.type]).toStrictEqual(["commissioning", "Periodic inspection"]); // an unknown type as stored
  });

  it("orders the rows by status and then by due time", () => {
    expect(clientRows([row("late", { dueAt: "2026-09-20T00:00:00Z" }), row("soon", { dueAt: "2026-09-15T00:00:00Z" }), row("reply", { displayStatus: "time_proposed" })], unitOf).map((r) => r.id)).toStrictEqual(["reply", "soon", "late"]);
  });
});

describe("the detail's facts", () => {
  it("for a plan visit: the visit day and the time HQ set", () => {
    const facts = detailFacts(job({ origin: "periodic_plan", occurrenceAt: "2026-09-16T00:00:00Z", scheduledSlot: visit, status: "assigned", symptom: "ignored" }), { name: "Bedroom AC", place: "Home A" });
    expect(facts.map(([k]) => k)).toStrictEqual(["Unit", "Origin · type", "Scheduled", "Technician"]); // no request text on a plan visit
    expect([facts[0]?.[1], facts[1]?.[1].startsWith("Periodic plan · visit "), facts[2]?.[1].endsWith("· set by HQ from your plan")]).toStrictEqual(["Bedroom AC · Home A", true, true]);
    expect(detailFacts(job({ origin: "periodic_plan" }), unitOf())[1]?.[1]).toBe("Periodic plan · visit ");
  });

  it("for a follow-up with photos, a contact window and a technician who accepted", () => {
    const facts = detailFacts(job({
      followUpOfJobId: "job-c02-orig", followUpClass: "rework", symptom: "Leaking again", contactWindow: "Weekdays 09:00-18:00", scheduledSlot: visit, status: "assigned", contractorOrgId: "org-c", type: "survey",
      problemPhotos: [{ id: "p1", name: "a.jpg", status: "ready" }, { id: "p2", name: "b.jpg", status: "ready" }, { id: "p3", name: "c.jpg", status: "scanning" }],
      assignment: { acknowledgement: "accepted", technicianName: null, scheduledStart: visit.startAt, scheduledEnd: visit.endAt, status: "active" },
    }), unitOf());
    expect(facts.map(([k, v]) => `${k}=${v}`).filter((x) => !x.startsWith("Confirmed time"))).toStrictEqual([
      "Unit=Bedroom AC", "Origin · type=Client request · survey", "Follow-up of=job-c02- · Rework (free)", "Request=“Leaking again”", "Your photos=2 photos sent with your report",
      "Contact window=Weekdays 09:00-18:00", "Technician=your technician · service partner",
    ]);
    expect(detailFacts(job({ problemPhotos: [{ id: "p1", name: "a.jpg", status: "ready" }] }), unitOf()).find(([k]) => k === "Your photos")?.[1]).toBe("1 photo sent with your report");
    expect(detailFacts(job({ scheduledSlot: visit, slotProposal: proposal }), unitOf()).some(([k]) => k === "Confirmed time")).toBe(false); // a pending proposal hides the old time
    expect([followUpText("new_request"), followUpText(null)]).toStrictEqual(["New request", "Under HQ review"]);
  });

  it("numbers preferred times past the third, and marks none booked without a booking", () => {
    const four = [visit, slot("2026-09-17T02:00:00Z", "2026-09-17T04:00:00Z"), slot("2026-09-18T02:00:00Z", "2026-09-18T04:00:00Z"), slot("2026-09-19T02:00:00Z", "2026-09-19T04:00:00Z")];
    const rows = preferredLines(job({ preferredSlots: four }));
    expect(rows.map((r) => [r.text.split(" · ", 1)[0], r.note])).toStrictEqual([["1st", null], ["2nd", null], ["3rd", null], ["4", null]]);
    expect(preferredLines(job({ preferredSlots: four, status: "cancelled" }))).toStrictEqual([]);
  });
});

describe("proposals, plan visits and feedback", () => {
  it("names a contractor's proposal and its technician, and words a decline of an unknown reason", () => {
    expect(proposalCard(job({ slotProposal: proposal }), NOW)).toMatchObject({ title: "Your service partner asks for another time (via HQ)", who: "A service partner technician (name shown once assigned)", late: false });
    const declined = (declineReason: string | null) => declinedNotice(job({ slotProposal: { ...proposal, status: "declined", declineReason } }));
    expect([declined("not_home"), declined("bad_weather"), declined(null)]).toStrictEqual([
      "You declined the proposed time (not at home). HQ is looking at your new times.", "You declined the proposed time (bad weather). HQ is looking at your new times.",
      "You declined the proposed time (other). HQ is looking at your new times.",
    ]);
    expect(declinedNotice(job({ status: "offered", slotProposal: { ...proposal, status: "declined" } }))).toBeNull();
  });

  it("lets the client move a plan visit with no time yet", () => {
    expect(planVisit(job({ origin: "periodic_plan", status: "offered" }), NOW)).toStrictEqual({ can: true, why: null });
    expect(planVisit(job({ origin: "periodic_plan", status: "in_progress" }), NOW)).toBeNull();
  });

  it("offers nothing on an open job, and shows a rating past its edit time with its tags", () => {
    expect(feedback(job({ status: "assigned" }), NOW)).toStrictEqual({ kind: "none", text: "", report: false });
    const rated = feedback(job({ status: "completed", completedAt: "2026-09-01T00:00:00Z", rating: { stars: 4, tags: ["On time", "Polite"], comment: null, ratedAt: "2026-09-02T00:00:00Z", editableUntil: "2026-09-09T00:00:00Z" } }), NOW);
    expect(rated).toStrictEqual({ kind: "rated", stars: 4, report: false, text: " · On time, Polite" });
    expect(feedback(job({ status: "completed", completedAt: null }), NOW)).toMatchObject({ kind: "rate", report: true }); // no completion time: still open
  });
});

describe("the form checks and the refusals", () => {
  it("asks for different times in the words of the count, and caps the contact window", () => {
    const later = slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z");
    expect([preferredError([later, later], NOW, 2), preferredError([later, later, later], NOW)]).toStrictEqual(["The times must be different.", "The 3 times must be different."]);
    expect(contactError("x".repeat(201))).toBe("Up to 200 characters.");
  });

  it("names the client's own notes and the coordinator's", () => {
    expect(notesOf([note("3", "me"), note("2", "hq-1"), note("4", null)], "me").map((n) => n.who)).toStrictEqual(["Coordinator", "You", "Coordinator"]);
  });

  it("words field errors, a known refusal, a missing request and the rest", () => {
    expect(clientJobRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { contactWindow: "errors.contact_details_forbidden", symptom: "error.tooShort" } })).toBe(
      "contactWindow: no e-mail addresses or phone numbers in the contact window · symptom: error.tooShort");
    expect(clientJobRefusal({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {} })).toBe("Not saved: the request changed meanwhile — the latest state is shown.");
    expect(clientJobRefusal({ code: "VALIDATION", messageKey: "errors.reschedule_too_late", fieldErrors: {} })).toBe("less than 48 h before the visit — contact HQ.");
    expect(clientJobRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })).toBe("The request no longer exists.");
    expect(clientJobRefusal({ code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 30 })).toMatch(/30/);
  });
});
