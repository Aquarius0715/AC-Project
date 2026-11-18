import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's offer / job detail (FR-P02, FR-P08): the offer before the delegation period (capacity of the first
// candidate day and the next four, qualified technicians, which offer this is), the detail inside it (visit, unit,
// linked alerts) and the history after it; another company's job reads as absent.
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code }; }
  }
  return {
    CoreError, coreAll: vi.fn(), coreOp: vi.fn(), coreItems: vi.fn(), coreNow: vi.fn(), coreDisplay: vi.fn(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
      try { return await read; } catch (error) {
        if (error instanceof CoreError && error.error.code !== "UNAVAILABLE" && error.error.code !== "TIMEOUT") return fallback;
        throw error;
      }
    },
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadJobDetail } from "../app/partner/jobs/[id]/(detail)/_lib/load";

const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
// 01:00 on 15 Sept in Kuala Lumpur is still 14 Sept in UTC: the capacity days are Kuala Lumpur dates
const late = slot("2026-09-14T17:00:00Z", "2026-09-14T19:00:00Z");
const later = slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z");
const offer = {
  projection: "offer", status: "offered", jobId: "job-p02-aaaa", jobVersion: 1, offerId: "off-1", type: "reactive", siteAddress: null, requiredQualifications: ["demo_indoor"],
  requestedSlot: slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z"), dueAt: "2026-09-21T00:00:00Z", offerExpiresAt: "2026-09-14T06:00:00Z", termsVersion: "t1", origin: "hq_request",
  candidateSlots: [later, late], offeredAt: "2026-09-14T00:00:00Z", accessValidFrom: "2026-09-14T16:00:00Z", accessValidUntil: "2026-09-17T16:00:00Z", partnerSlotProposal: null,
};
const detail = {
  projection: "detail", id: "job-p02-aaaa", version: 3, unitId: "unit-p-01", type: "reactive", status: "assigned", symptom: "noise", origin: "hq_request", alertIds: ["al-1", "al-2"],
  requestedSlot: slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z"), scheduledSlot: null, dueAt: "2026-09-21T00:00:00Z", startedAt: null, completedAt: null, assignmentId: "as-1",
  partnerSlotProposal: null, reportRefs: [],
  assignment: { technicianMembershipId: "mem-tech-1", scheduledStart: "2026-09-15T02:00:00Z", scheduledEnd: "2026-09-15T04:00:00Z", status: "assigned", acknowledgement: "pending", cantMakeReason: null },
  offer: { id: "off-1", termsVersion: "t1", candidateSlots: [later, late], offeredAt: "2026-09-14T00:00:00Z", accessValidFrom: "2026-09-14T16:00:00Z", accessValidUntil: "2026-09-17T16:00:00Z", decision: "accept", decidedAt: "2026-09-14T01:00:00Z" },
};
const members = [
  { id: "mem-tech-1", userId: "u1", displayName: "Tech One", role: "technician", qualifications: [{ code: "demo_indoor" }] },
  { id: "mem-tech-2", userId: "u2", displayName: "Tech Two", role: "technician", qualifications: [] },
  { id: "mem-own-1", userId: "u3", displayName: "Owner", role: "owner", qualifications: [{ code: "demo_indoor" }] },
];
const dateOf = (input: unknown) => (typeof input === "object" && input !== null && "date" in input && typeof input.date === "string" ? input.date : null);
/** the days members.capacity was read for, in order */
const capacityDates = () => dal.coreItems.mock.calls.filter(([operation]) => operation === "members.capacity").map(([, input]) => dateOf(input));
const offered = (id: string) => ({ id, jobId: "job-p02-aaaa", actorUserId: null, action: "job.offered", occurredAt: "2026-09-14T00:00:00Z" });

describe("partner job detail loader", () => {
  let job: object;
  beforeEach(() => {
    vi.clearAllMocks();
    job = offer;
    dal.coreNow.mockResolvedValue(new Date("2026-09-14T01:35:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreAll.mockResolvedValue(members);
    dal.coreItems.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.events" ? [offered("ev-1"), offered("ev-2")] : []));
    dal.coreOp.mockImplementation((operation: string, input: { id?: string }) => {
      if (operation === "jobs.get") return Promise.resolve(job);
      if (operation === "units.get") return Promise.resolve({ id: "unit-p-01", serviceScope: ["indoor", "electrical"], lastSeenAt: null });
      if (operation === "alerts.get" && input.id === "al-1") return Promise.resolve({ id: "al-1", detectedAt: "2026-09-14T00:30:00Z" });
      return Promise.reject(new dal.CoreError("NOT_FOUND"));
    });
  });

  it("reads another company's job as absent and fails on any other error", async () => {
    for (const code of ["NOT_FOUND", "FORBIDDEN"]) {
      dal.coreOp.mockRejectedValue(new dal.CoreError(code));
      expect(await loadJobDetail("job-p02-aaaa")).toBeNull();
    }
    dal.coreOp.mockRejectedValue(new dal.CoreError("UNAVAILABLE"));
    await expect(loadJobDetail("job-p02-aaaa")).rejects.toThrow("UNAVAILABLE");
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("shows the history snapshot after the period, without reading members or capacity", async () => {
    job = { projection: "history", jobId: "job-p02-aaaa", type: "reactive", status: "completed", asOf: "2026-09-21T16:00:00Z", completedAt: "2026-09-21T08:00:00Z", ownDecisionEvents: [] };
    const h = await loadJobDetail("job-p02-aaaa");
    expect(h?.kind === "history" ? [h.text.completed, h.text.snapshot] : null).toStrictEqual(["21 Sept 2026, 4:00 pm MYT", "22 Sept 2026, 12:00 am MYT"]);
    job = { ...job, completedAt: null };
    const open = await loadJobDetail("job-p02-aaaa");
    expect(open?.kind === "history" ? open.text.completed : null).toBe("—");
    expect(dal.coreAll).not.toHaveBeenCalled();
    expect(dal.coreItems).not.toHaveBeenCalled();
  });

  it("reads the offer's capacity from the earliest candidate day in Kuala Lumpur, and counts the offers", async () => {
    const o = await loadJobDetail("job-p02-aaaa");
    if (o?.kind !== "offer") throw new Error("expected the offer");
    const dates = capacityDates();
    expect(dates).toStrictEqual(["2026-09-15", "2026-09-16", "2026-09-17", "2026-09-18", "2026-09-19"]);
    expect(o.technicians).toStrictEqual([{ id: "mem-tech-1", name: "Tech One" }]);
    expect([o.text.count, o.text.offers, o.text.nth, o.text.proposal]).toStrictEqual([2, 2, "2nd", null]);
    expect(o.text.propose).toStrictEqual({ zone: "Asia/Kuala_Lumpur", date: "2026-09-16", from: "01:00", to: "03:00" });
    expect(o.names).toStrictEqual({ "mem-tech-1": "Tech One", "mem-tech-2": "Tech Two", "mem-own-1": "Owner" });
  });

  it("falls back to the requested time without candidate times, and to the first offer without events", async () => {
    job = { ...offer, candidateSlots: [], partnerSlotProposal: { id: "pr-1", offerId: "off-1", slot: later, technicianMembershipId: "mem-tech-1", reason: "busy", sentAt: "2026-09-14T01:00:00Z", status: "pending" } };
    dal.coreItems.mockRejectedValue(new dal.CoreError("FORBIDDEN"));
    dal.coreAll.mockRejectedValue(new dal.CoreError("FORBIDDEN"));
    const o = await loadJobDetail("job-p02-aaaa");
    if (o?.kind !== "offer") throw new Error("expected the offer");
    const dates = capacityDates();
    expect(dates[0]).toBe("2026-09-20");
    expect([o.text.offers, o.text.nth, o.technicians, o.events]).toStrictEqual([1, "1st", [], []]);
    expect(o.text.proposal).not.toBeNull();
  });

  it("shows the detail at the assigned time with the unit's scope and only the linked alerts it may read", async () => {
    job = detail;
    const d = await loadJobDetail("job-p02-aaaa");
    if (d?.kind !== "detail") throw new Error("expected the detail");
    const dates = capacityDates();
    expect(dates[0]).toBe("2026-09-15");
    expect(d.text.tech).toBe("Tech One");
    expect(d.required).toStrictEqual(["demo_indoor", "demo_electrical"]);
    expect(d.alerts.map((a) => a.id)).toStrictEqual(["al-1"]);
    expect(Object.keys(d.text.raised)).toStrictEqual(["al-1"]);
    expect([d.text.visit, d.text.scheduled]).toStrictEqual([d.text.scheduled, "15 Sept, 10:00 am – 12:00 pm MYT"]);
    expect(d.text.access).not.toBeNull();
    expect(d.text.delegation).not.toBeNull();
    expect(d.text.seen).toBeNull();
  });

  it("names an unknown technician, and shows the candidate times until a visit is fixed", async () => {
    job = { ...detail, assignment: { ...detail.assignment, technicianMembershipId: "mem-gone" } };
    const unknown = await loadJobDetail("job-p02-aaaa");
    expect(unknown?.kind === "detail" ? unknown.text.tech : null).toBe("Technician");
    job = { ...detail, assignment: null, alertIds: [] };
    dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.get" ? Promise.resolve(job) : Promise.resolve({ id: "unit-p-01", serviceScope: ["indoor"], lastSeenAt: "2026-09-14T01:00:00Z" })));
    const open = await loadJobDetail("job-p02-aaaa");
    if (open?.kind !== "detail") throw new Error("expected the detail");
    expect(open.text.tech).toBeNull();
    expect(open.text.scheduled).toBeNull();
    expect(open.text.visit.split(" / ")).toHaveLength(2);
    expect(open.text.seen).not.toBeNull();
    const dates = capacityDates();
    expect(dates.at(-5)).toBe("2026-09-15");
  });

  it("uses the scheduled time, or the requested time without an offer, and no unit scope when the unit is refused", async () => {
    job = { ...detail, assignment: null, scheduledSlot: later };
    const scheduled = await loadJobDetail("job-p02-aaaa");
    expect(scheduled?.kind === "detail" ? scheduled.text.visit : null).toBe("16 Sept, 10:00 am – 12:00 pm MYT");
    job = { ...detail, assignment: null, offer: null, alertIds: [] };
    dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.get" ? Promise.resolve(job) : Promise.reject(new dal.CoreError("FORBIDDEN"))));
    const bare = await loadJobDetail("job-p02-aaaa");
    if (bare?.kind !== "detail") throw new Error("expected the detail");
    expect([bare.text.visit, bare.text.access, bare.text.delegation, bare.unit, bare.required]).toStrictEqual(["20 Sept, 10:00 am – 12:00 pm MYT", null, null, null, []]);
  });
});
