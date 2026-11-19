import { beforeEach, describe, expect, it, vi } from "vitest";

// the team screen reads the own company's technicians through the DAL (FR-P06); another company's id in the URL reads
// as not found before anything else is read (AT-P06-E item 1)
// orWhenRefused as the DAL's: the fallback for a refusal, but UNAVAILABLE and TIMEOUT still fail the page
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code }; }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), coreIdentity: vi.fn(), corePrincipal: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
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

import { loadTeam } from "../app/partner/team/_lib/load";
import type { ApiTeamMember } from "@ac/web/lib/partnerTeam";

const technician: ApiTeamMember = { id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician", organizationId: "org-a", employment: null, validFrom: "2026-01-01T00:00:00Z", validUntil: null, revokedAt: null, qualifications: [] };
const slot = { startAt: "2026-11-20T01:00:00Z", endAt: "2026-11-20T05:00:00Z" };
const job = (id: string, status: string, scheduledSlot: typeof slot | null) => ({
  id, status, scheduledSlot, projection: "summary", version: 1, unitId: "unit-1", type: "maintenance", dueAt: "2026-11-25T00:00:00Z", requestedSlot: slot,
  assignmentId: "as-1", displayStatus: status, technicianMembershipId: "m-1", eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null },
});
const rowsOf = async (sp: { status?: string; membershipId?: string }) => { const live = await loadTeam({ date: "2026-11-18", ...sp }); return live.notFound ? null : live.certificates.rows.length; };
const titleOf = async (date: string) => { const live = await loadTeam({ date }); return live.notFound ? null : [live.week.dayIndex, live.week.title]; };

describe("partner team loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
    dal.corePrincipal.mockResolvedValue({ organizationId: "org-a" });
    dal.coreItems.mockResolvedValue([]);
    dal.coreOp.mockResolvedValue({ items: [], total: 0 });
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "members.list" ? [technician] : []));
  });

  it("refuses another company's id in the URL before reading the team", async () => {
    const live = await loadTeam({ orgId: "org-b" });
    expect(live).toStrictEqual({ notFound: true, company: "Contractor A" });
    expect(dal.coreAll).not.toHaveBeenCalled();
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("reads the team for the own company's id and for no id", async () => {
    for (const orgId of ["org-a", undefined]) {
      const live = await loadTeam({ orgId, date: "2026-11-18" });
      expect(live.notFound).toBe(false);
      expect(live.notFound ? [] : live.technicians).toStrictEqual([{ id: "m-1", name: "Aiman" }]);
    }
  });

  it("still shows the team when an optional read is refused, and fails the page when one is unavailable", async () => {
    const failing = (code: string) => (operation: string) => (operation === "members.list" ? Promise.resolve([technician]) : Promise.reject(new dal.CoreError(code)));
    dal.coreAll.mockImplementation(failing("FORBIDDEN"));
    const live = await loadTeam({ date: "2026-11-18" });
    expect(live.notFound ? [] : live.technicians).toStrictEqual([{ id: "m-1", name: "Aiman" }]);
    dal.coreAll.mockImplementation(failing("UNAVAILABLE"));
    await expect(loadTeam({ date: "2026-11-18" })).rejects.toThrow("UNAVAILABLE");
  });

  it("names the company by default when the session has no organization name", async () => {
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "" });
    await expect(loadTeam({ orgId: "org-b" })).resolves.toStrictEqual({ notFound: true, company: "your company" });
  });

  it("tells apart two technicians of the same name by e-mail, and reads bookings only for current technicians", async () => {
    const second: ApiTeamMember = { ...technician, id: "m-2", userId: "u-2" };
    const former: ApiTeamMember = { ...technician, id: "m-3", userId: "u-3", displayName: "Badrul", validUntil: "2026-10-01T00:00:00Z" };
    const owner: ApiTeamMember = { ...technician, id: "m-4", userId: "u-4", displayName: "Owner", role: "partner_owner" };
    const users = [{ membershipId: "m-2", email: "b@example.com" }, { membershipId: null, email: "invited@example.com" }, { membershipId: "m-1", email: "a@example.com" }];
    const lists = new Map<string, unknown[]>([["members.list", [second, former, owner, technician]], ["partnerUsers.list", users]]);
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
    const live = await loadTeam({ date: "2026-11-18" });
    if (live.notFound) throw new Error("expected the team");
    expect(live.technicians).toStrictEqual([{ id: "m-1", name: "Aiman · a@example.com" }, { id: "m-2", name: "Aiman · b@example.com" }]);
    expect([live.total, live.activeCount]).toStrictEqual([3, 2]); // the owner is not a technician; the former one is counted, not current
    expect(dal.coreOp.mock.calls.map((c: unknown[]) => c[1])).toStrictEqual([{ filters: { membershipId: "m-1" }, limit: 100 }, { filters: { membershipId: "m-2" }, limit: 100 }]);
  });

  it("orders two technicians of the same name without e-mail by their membership", async () => {
    const second: ApiTeamMember = { ...technician, id: "m-2", userId: "u-2" };
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "members.list" ? [second, technician] : []));
    const live = await loadTeam({ date: "2026-11-18" });
    if (live.notFound) throw new Error("expected the team");
    expect(live.technicians).toStrictEqual([{ id: "m-1", name: "Aiman" }, { id: "m-2", name: "Aiman" }]);
  });

  it("needs no qualification for a booking on a unit that is not listed or has no service scope", async () => {
    for (const units of [[], [{ id: "unit-1", displayName: "Living room" }]]) {
      const lists = new Map<string, unknown[]>([["members.list", [technician]], ["units.list", units]]);
      dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
      dal.coreOp.mockResolvedValue({ items: [job("job-1111aaaa", "assigned", slot)], total: 1 });
      const live = await loadTeam({ date: "2026-11-18" });
      if (live.notFound) throw new Error("expected the team");
      expect(live.jobs.map((j) => j.jobId)).toStrictEqual(["job-1111aaaa"]);
      expect([live.certificates.rows, live.certificates.impact]).toStrictEqual([[], null]);
    }
  });

  describe("a booked job whose unit needs a qualification the technician does not hold", () => {
    beforeEach(() => {
      const lists = new Map<string, unknown[]>([["members.list", [technician]], ["units.list", [{ id: "unit-1", displayName: "Living room", serviceScope: ["indoor"] }]]]);
      dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
      // one open booking, plus a completed and a cancelled one (released, IR234) and one without a time
      dal.coreOp.mockResolvedValue({ items: [job("job-1111aaaa", "assigned", slot), job("job-2222bbbb", "completed", slot), job("job-3333cccc", "cancelled", slot), job("job-4444dddd", "accepted", null)], total: 4 });
    });

    it("lists the open booking and shows the qualification as not held, blocking it (IR277)", async () => {
      const live = await loadTeam({ date: "2026-11-18" });
      if (live.notFound) throw new Error("expected the team");
      expect(live.jobs.map((j) => [j.jobId, j.short, j.technicianId])).toStrictEqual([["job-1111aaaa", "job-1111", "m-1"]]);
      expect(live.certificates.rows.map((r) => [r.technicianId, r.code, r.status, r.blocks.map((b) => b.jobId)])).toStrictEqual([["m-1", "demo_indoor", "not_held", ["job-1111aaaa"]]]);
    });

    it("filters the certificate rows by status and technician from the URL", async () => {
      expect(await rowsOf({ status: "not_held" })).toBe(1);
      expect(await rowsOf({ status: "valid" })).toBe(0);
      expect(await rowsOf({ status: "nonsense" })).toBe(1); // an unknown status filters nothing
      expect(await rowsOf({ membershipId: "m-1" })).toBe(1);
    });
  });

  it("titles the week by hours with the chosen day on a weekend, by assigned / available hours on a weekday", async () => {
    expect(await titleOf("2026-11-21")).toStrictEqual([5, "Week of 16 Nov — hours (Sat 21 selected)"]);
    expect(await titleOf("2026-11-20")).toStrictEqual([4, "Week of 16 Nov — assigned / available hours"]);
  });
});
