// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

// the technician overview reads the user's jobs through the DAL (FR-T01); the jobs list is paged (limit 100, SR14)
const dal = vi.hoisted(() => ({
  coreNow: vi.fn(), coreDisplay: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
  orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => read.catch(() => fallback),
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadOverview } from "../../../technician/app/technician/(overview)/_lib/load";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";

const now = new Date("2026-11-10T02:00:00Z");
const ended = (n: number): ApiTechJobRow => ({ projection: "history", jobId: `old-${String(n)}`, type: "maintenance", status: "completed", completedAt: "2026-10-01T03:00:00Z" });
const todayJob: ApiTechJobRow = {
  projection: "summary", id: "job-today", version: 1, unitId: "unit-1", type: "maintenance", status: "assigned", severity: "normal", dueAt: "2026-11-10T10:00:00Z",
  requestedSlot: { startAt: "2026-11-10T01:00:00Z", endAt: "2026-11-10T05:00:00Z" }, scheduledSlot: { startAt: "2026-11-10T01:00:00Z", endAt: "2026-11-10T05:00:00Z" },
  assignmentAcknowledgement: "accepted",
};
const firstPage = Array.from({ length: 100 }, (_, n) => ended(n));

describe("technician overview", () => {
  it("keeps today's job that the list returns after its first page (IR702)", async () => {
    dal.coreNow.mockResolvedValue(now);
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreItems.mockResolvedValue([]);
    // one page of 100 ended jobs, sorted by deadline (oldest first), and today's job on the next page
    dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list"
      ? { items: firstPage, total: 101, nextCursor: "page-2" }
      : { counts: { total: 101, scheduledCount: 1, inProgressCount: 0, overdueCount: 0, assignedCount: 1 } }));
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? [...firstPage, todayJob] : []));
    const live = await loadOverview({ tab: "today", sort: "dueAt" });
    expect(live.rows.map((r) => r.id)).toStrictEqual(["job-today"]);
    expect(live.tabs.map((x) => x.count)).toStrictEqual([1, 1]); // the completed ones are not part of the work (IR642)
  });
});
