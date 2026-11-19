import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor job list (FR-P01, SCR-P09) pages by cursor; the page number in the URL only labels the page. It is a
// whole number ≥ 2 that goes with a cursor, at most the last page; without a cursor the list shows the first page.
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; fieldErrors: Record<string, string>; messageKey: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code, fieldErrors: {}, messageKey: `error.${code}` }; }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(),
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

import { loadJobs } from "../app/partner/jobs/(list)/_lib/load";

const pageOf = async (sp: { cursor?: string; page?: string }) => { const live = await loadJobs(sp); return live.paging; };
const staleCursor = () => { const error = new dal.CoreError("VALIDATION"); error.error.fieldErrors.cursor = "error.invalid"; return error; };

describe("partner jobs list loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreAll.mockResolvedValue([]);
    dal.coreOp.mockResolvedValue({ items: [], nextCursor: null, total: 60 }); // 3 pages of 25
  });

  it("labels a cursor's page by the URL's page number, a whole number from 2 to the last page", async () => {
    await expect(pageOf({})).resolves.toStrictEqual({ page: 1, of: 3 });
    await expect(pageOf({ cursor: "c-2" })).resolves.toStrictEqual({ page: 2, of: 3 });
    await expect(pageOf({ cursor: "c-3", page: "3" })).resolves.toStrictEqual({ page: 3, of: 3 });
    await expect(pageOf({ cursor: "c-2", page: "1.5" })).resolves.toStrictEqual({ page: 2, of: 3 });
    await expect(pageOf({ cursor: "c-2", page: "1" })).resolves.toStrictEqual({ page: 2, of: 3 });
    await expect(pageOf({ cursor: "c-3", page: "9" })).resolves.toStrictEqual({ page: 3, of: 3 });
  });

  it("shows the first page for a page number without a cursor", async () => {
    await expect(pageOf({ page: "3" })).resolves.toStrictEqual({ page: 1, of: 3 });
  });

  it("starts over at the first page when the cursor no longer fits the list, and fails on another refusal", async () => {
    dal.coreOp.mockImplementation((operation: string, input: { cursor?: string }) => (operation === "jobs.list" && input.cursor
      ? Promise.reject(staleCursor())
      : Promise.resolve({ items: [], nextCursor: null, total: 60 })));
    const live = await loadJobs({ cursor: "c-old", page: "3" });
    expect([live.restarted, live.paging]).toStrictEqual([true, { page: 1, of: 3 }]);
    dal.coreOp.mockRejectedValue(new dal.CoreError("UNAVAILABLE"));
    await expect(loadJobs({ cursor: "c-old" })).rejects.toThrow("UNAVAILABLE");
  });

  it("names each row's unit and technician, counts every tab, and leaves unknown ones generic", async () => {
    const slot = { startAt: "2026-11-20T01:00:00Z", endAt: "2026-11-20T03:00:00Z" };
    const row = (id: string, unitId: string, technicianMembershipId: string) => ({
      id, unitId, technicianMembershipId, status: "assigned", dueAt: "2026-11-20T08:00:00Z", projection: "summary", version: 1, type: "reactive", requestedSlot: slot, scheduledSlot: slot,
      assignmentId: `a-${id}`, displayStatus: "assigned", eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null },
    });
    dal.coreOp.mockImplementation((_operation: string, input: { limit: number }) => Promise.resolve(input.limit === 1
      ? { items: [], nextCursor: null, total: 4 }
      : { items: [row("job-aaaa-1", "unit-1", "m-1"), row("job-bbbb-2", "unit-x", "m-x")], nextCursor: "c-2", total: 2 }));
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "units.list"
      ? [{ id: "unit-1", displayName: "Lobby AC" }]
      : [{ id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician" }]));
    const live = await loadJobs({});
    expect(live.rows.map((r) => [r.title, r.tech.name])).toStrictEqual([["job-aaaa · Lobby AC", "Aiman"], ["job-bbbb · Repair", "Technician"]]);
    expect(live.tabs.every((tb) => tb.count === 4)).toBe(true);
    expect([live.nextCursor, live.total]).toStrictEqual(["c-2", 2]);
  });
});
