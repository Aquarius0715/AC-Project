import { beforeEach, describe, expect, it, vi } from "vitest";

// the schedule reads the company's open jobs and the selected job through the DAL (FR-P03): `?jobId=` picks the row,
// `?slot=` one of the offer's open candidate times (IR636), and a job whose delegation ended meanwhile shows no form
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code }; }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
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

import { loadSchedule } from "../app/partner/schedule/_lib/load";
import type { ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import type { ApiDetail, ApiOffer } from "@ac/web/lib/partnerJobDetail";

const slotA = { startAt: "2026-11-20T01:00:00Z", endAt: "2026-11-20T05:00:00Z" };
const slotB = { startAt: "2026-11-21T01:00:00Z", endAt: "2026-11-21T05:00:00Z" };
const past = { startAt: "2026-11-10T01:00:00Z", endAt: "2026-11-10T05:00:00Z" };
type Summary = Extract<ApiPartnerJob, { projection: "summary" }>;
const summary = (id: string): Summary => ({
  id, projection: "summary", version: 3, unitId: "unit-1", type: "maintenance", status: "accepted", dueAt: "2026-11-25T00:00:00Z", requestedSlot: slotA,
  scheduledSlot: null, assignmentId: null, displayStatus: "accepted", technicianMembershipId: null, eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null },
});
// an accepted job the client agreed three times for: one passed, then A and B (B listed first, the form sorts them)
const detail = (id: string): ApiDetail => ({
  id, projection: "detail", version: 3, unitId: "unit-1", type: "maintenance", status: "accepted", symptom: "", origin: "hq", alertIds: [], requestedSlot: slotA,
  scheduledSlot: null, dueAt: "2026-11-25T00:00:00Z", startedAt: null, completedAt: null, assignmentId: null, partnerSlotProposal: null, reportRefs: [], assignment: null,
  offer: { id: "offer-1", termsVersion: "v1", candidateSlots: [past, slotB, slotA], offeredAt: "2026-11-01T00:00:00Z", accessValidFrom: "2026-11-01T00:00:00Z", accessValidUntil: "2026-11-30T00:00:00Z", decision: "accepted", decidedAt: "2026-11-02T00:00:00Z" },
});
// an assigned job whose technician can't make the booked time A and offered B instead
const assigned = (id: string): ApiDetail => ({
  ...detail(id), status: "assigned", assignmentId: "as-1",
  assignment: { technicianMembershipId: "m-1", scheduledStart: slotA.startAt, scheduledEnd: slotA.endAt, status: "active", acknowledgement: "cant_make", cantMakeReason: "car broke down", alternativeSlot: slotB },
});
// an accepted offer whose delegation period has not started yet
const offer = (id: string): ApiOffer => ({
  projection: "offer", status: "accepted", jobId: id, jobVersion: 7, offerId: "offer-1", type: "maintenance", siteAddress: null, requiredQualifications: ["demo_indoor"],
  requestedSlot: slotA, dueAt: "2026-11-25T00:00:00Z", offerExpiresAt: "2026-11-19T00:00:00Z", termsVersion: "v1", origin: "hq", candidateSlots: [slotA],
  offeredAt: "2026-11-17T00:00:00Z", accessValidFrom: "2026-11-19T00:00:00Z", accessValidUntil: "2026-11-30T00:00:00Z", partnerSlotProposal: null,
});
const noScope = () => Promise.resolve({ serviceScope: [] });
const requiredOf = async (sp: { slot?: string }) => { const live = await loadSchedule(sp); return live.selected?.required; };

let jobs: ApiPartnerJob[] = [];
let get: (id: string) => Promise<unknown> = (id) => Promise.resolve(detail(id));
const eligibleSlots: unknown[] = [];

describe("partner schedule loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    eligibleSlots.length = 0;
    jobs = [summary("job-1111aaaa"), summary("job-2222bbbb")];
    get = (id) => Promise.resolve(detail(id));
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreItems.mockResolvedValue([]);
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? jobs : []));
    dal.coreOp.mockImplementation((operation: string, input: { jobId: string; startAt?: string; endAt?: string }) => {
      if (operation === "jobs.get") return get(input.jobId);
      if (operation === "members.eligible") eligibleSlots.push({ startAt: input.startAt, endAt: input.endAt });
      return Promise.resolve({ items: [], total: 0 });
    });
  });

  it("shows no form and reads no job when nothing is open", async () => {
    jobs = [];
    const live = await loadSchedule({});
    expect([live.rows, live.selected, live.week]).toStrictEqual([[], null, null]);
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("selects the job in the URL, and the first row for an id that is not listed", async () => {
    const picked = await loadSchedule({ jobId: "job-2222bbbb" });
    expect(picked.selected?.id).toBe("job-2222bbbb");
    const unknown = await loadSchedule({ jobId: "job-9999" });
    expect(unknown.selected?.id).toBe("job-1111aaaa");
  });

  it("picks the open candidate time the URL names, and the earliest open one otherwise", async () => {
    const second = await loadSchedule({ slot: "1" });
    expect(second.selected?.mode.slot).toStrictEqual(slotB);
    expect(second.selected?.times.map((x) => x.on)).toStrictEqual([false, true]); // the passed time is not offered
    expect(eligibleSlots).toStrictEqual([slotB]); // the candidates are searched for the picked time
    for (const slot of [undefined, "0", "9", "-1", "1x", ""]) {
      const live = await loadSchedule({ slot });
      expect(live.selected?.mode.slot).toStrictEqual(slotA);
    }
  });

  it("shows no form for a job whose delegation ended meanwhile", async () => {
    for (const code of ["NOT_FOUND", "FORBIDDEN"]) {
      get = () => Promise.reject(new dal.CoreError(code));
      const live = await loadSchedule({});
      expect([live.rows.length, live.selected, live.week]).toStrictEqual([2, null, null]);
    }
    get = (id) => Promise.resolve({ projection: "history", jobId: id, type: "maintenance", status: "accepted", asOf: "2026-11-18T00:00:00Z", completedAt: null, ownDecisionEvents: [] });
    await expect(loadSchedule({})).resolves.toMatchObject({ selected: null, week: null });
  });

  it("shows a technician's booked open jobs in the week, not the completed or cancelled ones", async () => {
    const member = { id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", revokedAt: null, validUntil: null };
    const booked = (id: string, status: string): Summary => ({ ...summary(id), status, scheduledSlot: slotA });
    const lists = new Map<string, unknown[]>([["jobs.list", jobs], ["members.list", [member]]]);
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
    dal.coreOp.mockImplementation((operation: string, input: { jobId: string; filters?: { membershipId?: string } }) => {
      if (operation === "jobs.get") return get(input.jobId);
      if (operation === "jobs.list" && input.filters?.membershipId === "m-1") {
        return Promise.resolve({ items: [booked("job-3333cccc", "assigned"), booked("job-4444dddd", "completed"), booked("job-5555eeee", "cancelled"), summary("job-6666ffff")], total: 4 });
      }
      return Promise.resolve({ items: [], total: 0 });
    });
    const live = await loadSchedule({});
    expect(live.week?.jobsOf).toStrictEqual({ "m-1": [{ short: "job-3333", slot: slotA }] });
  });

  it("fails the page when the job cannot be read now", async () => {
    get = () => Promise.reject(new dal.CoreError("UNAVAILABLE"));
    await expect(loadSchedule({})).rejects.toThrow("UNAVAILABLE");
  });

  describe("the selected job's form", () => {
    const member = { id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", revokedAt: null, validUntil: null };
    let unit: () => Promise<unknown> = noScope;
    let eligible: string[] = [];
    const capacityDates: string[] = [];
    beforeEach(() => {
      unit = noScope;
      eligible = [];
      capacityDates.length = 0;
      const lists = new Map<string, unknown[]>([["jobs.list", jobs], ["members.list", [member]]]);
      dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
      dal.coreItems.mockImplementation((_operation: string, input: { date: string }) => { capacityDates.push(input.date); return Promise.resolve([]); });
      dal.coreOp.mockImplementation((operation: string, input: { jobId: string; startAt?: string; endAt?: string }) => {
        if (operation === "jobs.get") return get(input.jobId);
        if (operation === "units.get") return unit();
        if (operation === "members.eligible") {
          eligibleSlots.push({ startAt: input.startAt, endAt: input.endAt });
          return Promise.resolve({ items: eligible.map((id) => ({ ...member, id })), total: eligible.length });
        }
        return Promise.resolve({ items: [], total: 0 });
      });
    });

    it("shows the current assignment, the technician's answer and the booking on the booked time's day", async () => {
      get = (id) => Promise.resolve(assigned(id));
      // the booked time's day (Friday 20 Nov in Kuala Lumpur) has the technician busy then; the other days are empty
      dal.coreItems.mockImplementation((_operation: string, input: { date: string }) => {
        capacityDates.push(input.date);
        return Promise.resolve(input.date === "2026-11-20"
          ? [{ membershipId: "m-1", date: input.date, availableSlots: [slotA], assignedSlots: [slotA], availableMinutes: 240, assignedMinutes: 240, unavailability: null }] : []);
      });
      const live = await loadSchedule({});
      expect(capacityDates).toStrictEqual(["2026-11-16", "2026-11-17", "2026-11-18", "2026-11-19", "2026-11-20", "2026-11-21", "2026-11-22"]);
      expect(live.selected).toMatchObject({ currentId: "m-1", moved: false, ack: { status: "cant_make", reason: "car broke down", alternative: slotB }, mode: { kind: "reassign", slot: slotA } });
      expect(live.selected?.text.alternative).not.toBeNull();
      expect(live.selected?.candidates.map((c) => [c.id, c.badge.text, c.eligible])).toStrictEqual([["m-1", "Current · booked here", false]]);
      expect(live.week?.from).toBe("2026-11-16");
    });

    it("marks a picked time other than the booked one as moved, and badges the eligible current technician", async () => {
      get = (id) => Promise.resolve(assigned(id));
      eligible = ["m-1"];
      const live = await loadSchedule({ slot: "1" });
      expect(live.selected).toMatchObject({ moved: true, mode: { slot: slotB } });
      expect(live.selected?.times.map((x) => x.on)).toStrictEqual([false, true]);
      expect(live.selected?.candidates.map((c) => [c.badge.text, c.eligible])).toStrictEqual([["Current", true]]);
      expect(eligibleSlots).toStrictEqual([slotB]);
    });

    it("takes an accepted offer's version and required qualifications, searches no candidates before the delegation starts, and reads no unit", async () => {
      get = (id) => Promise.resolve(offer(id));
      const live = await loadSchedule({});
      expect(live.selected).toMatchObject({ version: 7, required: ["demo_indoor"], ack: null, moved: false, currentId: null, times: [], mode: { kind: "blocked" } });
      expect(eligibleSlots).toStrictEqual([]);
      expect(dal.coreOp).not.toHaveBeenCalledWith("units.get", expect.anything());
    });

    it("requires a qualification per entry of the unit's scope, and none when the unit cannot be read", async () => {
      unit = () => Promise.resolve({ serviceScope: ["indoor", "electrical"] });
      expect(await requiredOf({})).toStrictEqual(["demo_indoor", "demo_electrical"]);
      unit = () => Promise.reject(new dal.CoreError("FORBIDDEN"));
      expect(await requiredOf({})).toStrictEqual([]);
    });

    it("gives the end time in the user's zone and flags a zone other than Kuala Lumpur", async () => {
      const inKl = await loadSchedule({});
      expect(inKl.otherZone).toBe(false);
      dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Tokyo" });
      const live = await loadSchedule({});
      expect(live.otherZone).toBe(true);
      expect(live.selected?.text.end).toBe("2026-11-20T14:00"); // 05:00 UTC
    });
  });
});
