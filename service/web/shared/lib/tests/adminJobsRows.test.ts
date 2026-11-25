import { describe, expect, it } from "vitest";
import { facts, filtersOf, hqRow, stepper, type ApiHqJob, type ApiHqRow, type Names } from "@ac/web/lib/adminJobs";

// HQ maintenance jobs (FR-A06, DD-A06): what each row of the list says comes next in every status, the overdue mark,
// a pending follow-up, the stepper for internal and contractor delivery, the detail's four facts, and the filters a
// query sends.

const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const NOW = Date.parse("2026-09-21T01:30:00Z");
const names: Names = { units: new Map([["u1", "Bedroom AC"]]), customers: new Map([["org-a", "Tan household"]]), people: new Map([["m-int", "Aiman"]]), orgs: new Map([["org-c", "Cool Partners"]]) };
const unitOrg = new Map([["u1", "org-a"]]);
const row = (over: Partial<ApiHqRow>): ApiHqRow => ({
  projection: "summary", id: "job-1-aaaa-bbbb", version: 3, unitId: "u1", type: "reactive", status: "requested", dueAt: "2026-09-22T04:00:00Z", requestedSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"),
  scheduledSlot: null, assignmentId: null, origin: "client_request", preferredSlots: [], severity: "normal", displayStatus: "requested", assignmentAcknowledgement: null, technicianMembershipId: null,
  accessValidFrom: null, accessValidUntil: null, createdAt: "2026-09-18T01:00:00Z", followUpOfJobId: null, followUpClass: null, ...over,
});
const job = (over: Partial<ApiHqJob>): ApiHqJob => ({
  projection: "detail", id: "job-1", version: 4, unitId: "u1", type: "reactive", status: "requested", symptom: "Water dripping", contactWindow: "9:00–18:00", origin: "client_request", planId: null, occurrenceAt: null,
  requestedSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"), preferredSlots: [], preferenceRound: 1, slotProposal: null, partnerSlotProposal: null, scheduledSlot: null, dueAt: "2026-09-22T04:00:00Z",
  startedAt: null, completedAt: null, contractorOrgId: null, createdAt: "2026-09-18T01:00:00Z", followUpOfJobId: null, followUpClass: null, assignment: null, offer: null, reportRefs: [], draftReportRef: null, costs: [], ...over,
});
const line = (over: Partial<ApiHqRow>) => hqRow(row(over), NOW, names, unitOrg).line;
const past = slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z");

describe("a job row says what comes next", () => {
  it("for a requested job: book a preferred time, the plan occurrence, or wait for the due time", () => {
    expect(line({ preferredSlots: [past] })).toBe("1 preferred time · book one or propose");
    expect(line({ preferredSlots: [past, past] })).toBe("2 preferred times · book one or propose");
    expect(line({ origin: "periodic_plan" })).toMatch(/^plan occurrence .+ · book it$/);
    expect(line({})).toMatch(/^due .+ · unassigned$/);
  });

  it("for an offer, an accepted offer, a proposed time and a submitted report", () => {
    expect(line({ status: "offered" })).toBe("Offer open");
    expect(line({ status: "offered", scheduledSlot: slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z") })).toMatch(/^Offer open · /);
    expect(line({ status: "accepted" })).toBe("Contractor accepted · assigning their technician");
    expect(line({ displayStatus: "time_proposed" })).toBe("Time proposed · waiting for the client");
    expect(line({ status: "submitted" })).toBe("report submitted · awaiting HQ review");
    expect(line({ status: "submitted", accessValidUntil: "2026-09-25T00:00:00Z" })).toBe("report submitted · contractor review first");
  });

  it("for an assigned or started job: the technician, the answer, and a window that ended", () => {
    const tech = { technicianMembershipId: "m-int" };
    const ok = hqRow(row({ status: "assigned", ...tech, assignmentAcknowledgement: "accepted" }), NOW, names, unitOrg);
    expect([ok.line, ok.lineTone ?? "none", ok.badge.overdue]).toStrictEqual(["Aiman · accepted", "none", false]);
    expect(hqRow(row({ status: "assigned", ...tech, assignmentAcknowledgement: "cant_make" }), NOW, names, unitOrg)).toMatchObject({ line: "Aiman · can’t make it", lineTone: "warn" });
    expect(line({ status: "assigned", technicianMembershipId: "m-gone", assignmentAcknowledgement: "pending" })).toBe("technician · awaiting acceptance");
    expect(line({ status: "assigned" })).toBe("technician · awaiting acceptance");
    const late = hqRow(row({ status: "in_progress", ...tech, scheduledSlot: past }), NOW, names, unitOrg);
    expect([late.line, late.badge.overdue]).toStrictEqual(["Aiman on site · window ended", true]);
    expect(line({ status: "in_progress" })).toBe("technician on site");
  });

  it("for a returned, held, completed or cancelled job, and a follow-up HQ must classify", () => {
    expect([line({ status: "rework_requested" }), line({ status: "completed" }), line({ status: "cancelled" })]).toStrictEqual(["returned for rework", "completed", "cancelled"]);
    expect(hqRow(row({ status: "on_hold" }), NOW, names, unitOrg)).toMatchObject({ line: "on hold — scheduling and reports blocked", lineTone: "warn" });
    const followUp = hqRow(row({ followUpClass: "pending", followUpOfJobId: "job-c02-orig" }), NOW, names, unitOrg);
    expect(followUp.line).toMatch(/^Follow-up of job-c02- · classify by /);
    expect(followUp.lineTone).toBe("warn");
    expect(line({ followUpClass: "pending", preferredSlots: [past] })).toBe("1 preferred time · book one or propose"); // no original job: an ordinary request
  });

  it("names an unknown unit and customer by their kind, and shortens the ID", () => {
    expect(hqRow(row({ unitId: "u-gone" }), NOW, names, unitOrg)).toMatchObject({ title: "Unit", customer: "customer", short: "job-1-aa" });
    expect(hqRow(row({ displayStatus: "time_proposed" }), NOW, names, unitOrg)).toMatchObject({ status: "time_proposed", badge: { status: "time_proposed" } });
  });
});

describe("the stepper and the detail's facts", () => {
  it("skips Offered and Accepted for internal delivery, and places a returned, held or cancelled job", () => {
    const states = (status: string, internal: boolean) => stepper(status, internal).map((s) => s.state[0]).join("");
    expect([states("assigned", true), states("rework_requested", true), states("on_hold", false), states("cancelled", false), states("completed", true)]).toStrictEqual(["dcttt", "ddctt", "ddddctt", "ttttttt", "ddddd"]);
    expect(stepper("offered", false).map((s) => s.label)).toStrictEqual(["Requested", "Offered", "Accepted", "Assigned", "In progress", "Submitted", "Completed"]);
  });

  it("says what the requested window, due, schedule and symptom come from", () => {
    const [requested, due, scheduled, symptom] = facts(job({ preferredSlots: [past] }), names);
    expect([requested?.sub, due?.sub, scheduled?.value, scheduled?.sub, symptom?.value, symptom?.sub]).toStrictEqual([
      "the client’s 1st preferred time", "= requested end", "not booked", "—", "Water dripping", "contact 9:00–18:00",
    ]);
    const plan = facts(job({ origin: "periodic_plan", type: "periodic", dueAt: "2026-09-30T00:00:00Z", symptom: "", contactWindow: null }), names);
    expect([plan[0]?.sub, plan[1]?.sub, plan[3]?.value, plan[3]?.sub]).toStrictEqual(["the plan occurrence", "plan occurrence", "—", "no contact window"]);
    expect(facts(job({ dueAt: "2026-09-30T00:00:00Z" }), names)[1]?.sub).toBe("reactive SLA");
    const offer = { id: "o", contractorOrgId: "org-c", candidateSlots: [], offerExpiresAt: "", accessValidFrom: "", accessValidUntil: "", decision: null, decidedAt: null, declineReason: null };
    expect([facts(job({ offer }), names)[2]?.sub, facts(job({ offer: { ...offer, contractorOrgId: "org-x" } }), names)[2]?.sub]).toStrictEqual(["Cool Partners", "contractor"]);
    const assignment = { technicianMembershipId: "m-x", scheduledStart: past.startAt, scheduledEnd: past.endAt, status: "active", acknowledgement: "accepted", cantMakeReason: null, alternativeSlot: null };
    const booked = facts(job({ assignment, scheduledSlot: past }), names)[2];
    expect([booked?.sub, booked?.value === "not booked"]).toStrictEqual(["technician", false]); // an unknown technician by their role
  });
});

describe("the filters a query sends", () => {
  it("sends the property, unit and a client-request origin, and leaves out an unknown origin and an empty delivery", () => {
    expect(filtersOf({ propertyId: "p-1", unitId: "u1", origin: "client_request" }, "org-hq")).toMatchObject({ propertyId: "p-1", unitId: "u1", origin: "client_request" });
    expect(Object.keys(filtersOf({ origin: "email", delivery: "internal" }, null))).toStrictEqual(["statuses"]); // HQ's own organization unknown: no delivery filter
  });
});
