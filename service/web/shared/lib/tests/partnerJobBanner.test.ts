import { describe, expect, it } from "vitest";
import {
  decisionRefusal, detailBanner, eventRows, fits, ordinal, originLabel, qualificationLabel, qualified, statusWord, timeline, typeLabel, type ApiDetail,
} from "@ac/web/lib/partnerJobDetail";
import type { ApiMember } from "@ac/web/lib/partnerOverview";

// The service partner's delegated job (FR-P02, DD-P02, Figma 02-7…02-13): the banner of every state, the technician's
// answer, the timeline of a held, returned or cancelled job, the labels of unknown codes, the ordinals, a technician
// without qualifications in the fit list, and the refusals of the offer decisions.

const NOW = Date.parse("2026-09-21T01:30:00Z");
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const visit = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z");
const job = (over: Partial<ApiDetail>): ApiDetail => ({
  projection: "detail", id: "j", version: 4, unitId: "u", type: "reactive", status: "accepted", symptom: "", origin: "client_request", alertIds: [], requestedSlot: visit, scheduledSlot: null,
  dueAt: "2026-09-25T00:00:00Z", startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null, offer: null, ...over,
});
const assignment = (acknowledgement: string): NonNullable<ApiDetail["assignment"]> => ({
  acknowledgement, cantMakeReason: null, technicianMembershipId: "m-a", scheduledStart: visit.startAt, scheduledEnd: visit.endAt, status: "active",
});
const banner = (over: Partial<ApiDetail>, name: string | null = "Kamal") => detailBanner(job(over), name, NOW);
const event = (action: string) => ({ action, id: action, jobId: "j", actorUserId: null, occurredAt: "2026-09-20T05:00:00Z" });
const refusal = (messageKey: string, code = "CONFLICT", fieldErrors: Record<string, string> = {}) => decisionRefusal({ code, messageKey, fieldErrors });

describe("the delegated job's banner", () => {
  it("says the offer was accepted without its decision time", () => {
    expect(banner({})?.text).toBe("Accepted (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.");
  });

  it("names the technician's answer: accepted, waiting, or can't make it without a reason", () => {
    const accepted = banner({ status: "in_progress", assignment: assignment("accepted") });
    expect(accepted?.tone).toBe("ok");
    expect(accepted?.text).toMatch(/^In progress — Kamal · .+ · accepted by the technician\.$/);
    expect(banner({ status: "assigned", assignment: assignment("pending") }, null)?.text).toMatch(/^Assigned — technician · .+ · awaiting the technician’s acceptance\.$/);
    const cantMake = banner({ status: "assigned", assignment: assignment("cant_make") });
    expect(cantMake?.tone).toBe("warn");
    expect(cantMake?.text).toMatch(/the technician can’t make it — reassign in Schedule\.$/);
    expect(banner({ status: "assigned" }, null)?.text).toBe("Assigned — technician ·  · awaiting the technician’s acceptance."); // nobody assigned yet: no time
  });

  it("states a submitted, returned, held, completed or cancelled job, and nothing for the others", () => {
    expect([banner({ status: "submitted" })?.tone, banner({ status: "rework_requested" })?.tone, banner({ status: "on_hold" })?.tone, banner({ status: "cancelled" })?.tone]).toStrictEqual(["primary", "warn", "warn", "crit"]);
    expect(banner({ status: "completed" }, null)?.text).toBe("Completed — accepted in the quality review. The technician’s assignment ended with it, so the time is free for other visits.");
    expect(banner({ status: "completed", completedAt: "2026-09-20T05:00:00Z" })?.text).toMatch(/^Completed .+ — accepted in the quality review\. Kamal’s assignment ended/);
    expect(banner({ status: "requested" })).toBeNull();
  });
});

describe("the timeline", () => {
  it("places a held or returned job at In progress with a technician and at Accepted without, and a cancelled one at the start", () => {
    const current = (status: string, hasTechnician: boolean) => timeline(status, [], hasTechnician).findIndex((s) => s.state === "current");
    expect([current("on_hold", true), current("rework_requested", false), current("cancelled", false)]).toStrictEqual([4, 2, 0]);
    expect(timeline("requested", [], false, "client_request")[0]?.sub).toBe("client request");
    expect(timeline("requested", [], false)[0]?.sub).toBe("by the client or the plan");
    expect(timeline("completed", [event("job.reviewed")], true).at(-1)).toMatchObject({ state: "done", at: "2026-09-20T05:00:00Z" });
  });
});

describe("labels and the fit list", () => {
  it("words unknown codes as stored, and every English ordinal", () => {
    expect([qualificationLabel("demo_gas_handling"), typeLabel("survey"), originLabel("hq_request"), originLabel("client_request"), statusWord("paused_by_hq")]).toStrictEqual([
      "gas handling", "survey", "requested by HQ", "agreed with the client", "paused by hq",
    ]);
    expect([1, 2, 3, 4, 11, 12, 13, 21, 22, 23, 101, 111].map((n) => ordinal(n))).toStrictEqual(["1st", "2nd", "3rd", "4th", "11th", "12th", "13th", "21st", "22nd", "23rd", "101st", "111th"]);
    expect([ordinal(2, "ms"), ordinal(2, "id"), ordinal(2, "ja")]).toStrictEqual(["ke-2", "ke-2", "第2"]);
  });

  it("treats a technician without qualifications as not qualified, and one without a start or end as always valid", () => {
    const bare: ApiMember = { id: "m-x", userId: "u-x", displayName: "Zed", role: "technician" };
    const open: ApiMember = { id: "m-y", userId: "u-y", displayName: "Amir", role: "technician", qualifications: [{ code: "demo_indoor" }] };
    expect([qualified(bare, ["demo_indoor"], visit.startAt), qualified(open, ["demo_indoor"], visit.startAt), qualified(bare, [], visit.startAt)]).toStrictEqual([false, true, true]);
    const rows = fits([bare, open], [], visit, []);
    expect(rows.map((r) => [r.name, r.sub])).toStrictEqual([["Amir", "Indoor unit work"], ["Zed", "qualified"]]); // no visit day: both busy, then by name
    expect(fits([bare], [], visit, [], "detail")[0]?.sub).toBe("Qualified · 0 h free "); // no days: no span
  });
});

describe("the refusals of the offer decisions", () => {
  it("words each known refusal, a field error by its key or alone, and an empty validation", () => {
    expect([refusal("errors.terms_changed"), refusal("errors.partner_proposal_pending"), refusal("errors.technician_out_of_scope"), refusal("errors.technician_not_allowed"), refusal("error.versionConflict")]).toStrictEqual([
      "The delegation terms changed — reload to read the new terms before deciding.", "Your time change is still waiting for the client — accept after it is answered or withdraw it.",
      "That technician cannot be booked for this unit — choose another.", "That technician cannot be booked for this unit — choose another.", "The offer changed elsewhere — the page shows the latest version.",
    ]);
    expect(refusal("error.validation", "VALIDATION", { _: "error.slotRules", reason: "error.tooLong" })).toBe("pick a time on a later day, 1–4 hours long · reason: error.tooLong");
    expect(refusal("error.validation", "VALIDATION")).toBe("Check the input.");
  });

  it("words an unknown event by its action", () => {
    expect(eventRows([event("job.teleported")])[0]?.text).toBe("job teleported");
  });
});
