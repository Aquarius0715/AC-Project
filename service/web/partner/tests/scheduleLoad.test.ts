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
import type { ApiDetail } from "@ac/web/lib/partnerJobDetail";

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
});
