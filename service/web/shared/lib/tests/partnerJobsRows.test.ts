import { describe, expect, it } from "vitest";
import { jobRow } from "@ac/web/lib/partnerJobs";
import type { ApiPartnerJob } from "@ac/web/lib/partnerOverview";

// The service partner's job list (FR-P01, DD-P01): each row of an offer without candidate times or past its answer
// time, an accepted offer at an address, a history row of an unknown status or without a completion, and each status
// of a summary without a technician or with an unknown one — what the line, technician, time and badge say.

const NOW = Date.parse("2026-09-21T01:30:00Z");
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const past = slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z");
const units = new Map([["u-1", "Bedroom AC"]]);
const names = new Map([["m-a", "Aiman"]]);
type Summary = Extract<ApiPartnerJob, { projection: "summary" }>;
const summary = (status: string, over: Partial<Summary> = {}): ApiPartnerJob => ({
  status, projection: "summary", id: "11111111-aaaa", version: 3, unitId: "u-1", type: "periodic", dueAt: "2026-09-25T10:00:00Z", requestedSlot: past, scheduledSlot: null, assignmentId: "as-1",
  displayStatus: status, technicianMembershipId: null, accessValidFrom: "2026-09-19T00:00:00Z", accessValidUntil: "2026-09-30T00:00:00Z", eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null }, ...over,
});
const offer = (over: Partial<Extract<ApiPartnerJob, { projection: "offer" }>>): ApiPartnerJob => ({
  projection: "offer", status: "offered", jobId: "22222222-bbbb", jobVersion: 1, offerId: "o", type: "reactive", siteAddress: null, requestedSlot: past, dueAt: "2026-09-25T00:00:00Z",
  offerExpiresAt: "2026-09-22T00:00:00Z", candidateSlots: [], ...over,
});
const row = (j: ApiPartnerJob) => jobRow(j, NOW, units, names);

describe("offers and history rows", () => {
  it("shows an offer without candidate times, and one whose answer time has passed", () => {
    expect(row(offer({})).line).toBe("Offered by HQ · —");
    const late = row(offer({ offerExpiresAt: "2026-09-21T00:00:00Z" }));
    expect([late.slot.sub.endsWith(" left"), late.slot.tone]).toStrictEqual([false, "warn"]);
  });

  it("titles an accepted offer by its site address", () => {
    expect(row(offer({ status: "accepted", siteAddress: "Jalan Ampang 1", candidateSlots: [past] })).title).toBe("22222222 · Jalan Ampang 1");
  });

  it("words a history row of an unknown status, and one never completed", () => {
    const r = row({ projection: "history", jobId: "33333333-cccc", type: "reactive", status: "closed_by_hq", completedAt: null, asOf: "2026-09-20T00:00:00Z", ownDecisionEvents: [] });
    expect([r.line, r.slot.main, r.badge, r.origin]).toStrictEqual(["closed by hq · delegation ended, history only", "—", { text: "closed by hq", tone: "muted" }, null]);
  });
});

describe("a summary row without a technician", () => {
  it("words each status without a technician", () => {
    const lines = ["in_progress", "rework_requested", "assigned", "submitted"].map((s) => row(summary(s)).line);
    expect(lines).toStrictEqual(["In progress", "Returned for rework", "Assigned", "Submitted · awaiting your review"]);
    expect([row(summary("on_hold")).tech, row(summary("completed")).tech]).toStrictEqual([{ name: "—", sub: "" }, { name: "—", sub: "" }]);
  });

  it("shows a job with no due time, and an overdue job in progress", () => {
    expect(row(summary("in_progress", { dueAt: "" })).slot).toStrictEqual({ main: "—", sub: "" });
    const late = row(summary("in_progress", { scheduledSlot: past, technicianMembershipId: "m-a" }));
    expect([late.badge.text, late.slot.sub, late.tech.name]).toStrictEqual(["Overdue", "Extend or reassign", "Aiman"]);
  });
});

describe("a summary row with a technician", () => {
  it("names an unknown technician by their role, and one with no answer as assigned", () => {
    const r = row(summary("assigned", { technicianMembershipId: "m-gone", scheduledSlot: slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z") }));
    expect([r.tech, r.line.startsWith("Assigned Technician · ")]).toStrictEqual([{ name: "Technician", sub: "assigned" }, true]);
    expect(row(summary("completed", { technicianMembershipId: "m-a" })).tech).toStrictEqual({ name: "Aiman", sub: "report author" });
  });
});

