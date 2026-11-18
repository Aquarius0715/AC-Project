import { describe, expect, it, vi } from "vitest";

// the technician overview reads the user's jobs through the DAL (FR-T01); the jobs list is paged (limit 100, SR14)
const dal = vi.hoisted(() => ({
  coreNow: vi.fn(), coreDisplay: vi.fn(), coreOp: vi.fn(), coreAll: vi.fn(), coreItems: vi.fn(),
  orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => read.catch(() => fallback),
}));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadOverview } from "../app/technician/(overview)/_lib/load";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiAlert } from "@ac/web/lib/alerts";

const now = new Date("2026-11-10T02:00:00Z");
const ended = (n: number): ApiTechJobRow => ({ projection: "history", jobId: `old-${String(n)}`, type: "maintenance", status: "completed", completedAt: "2026-10-01T03:00:00Z" });
const todayJob: ApiTechJobRow = {
  projection: "summary", id: "job-today", version: 1, unitId: "unit-1", type: "maintenance", status: "assigned", severity: "normal", dueAt: "2026-11-10T10:00:00Z",
  requestedSlot: { startAt: "2026-11-10T01:00:00Z", endAt: "2026-11-10T05:00:00Z" }, scheduledSlot: { startAt: "2026-11-10T01:00:00Z", endAt: "2026-11-10T05:00:00Z" },
  assignmentAcknowledgement: "accepted",
};
const firstPage = Array.from({ length: 100 }, (_, n) => ended(n));
const alert = (id: string, status: string, severity: ApiAlert["severity"]): ApiAlert => ({
  id, status, severity, unitId: "unit-1", type: "high_temperature", causeCode: "threshold", evidenceKind: "measurement", evidenceText: "", detectedAt: "2026-11-10T01:00:00Z",
});
// the server's alerts, most severe first across the statuses: 50 resolved critical ones ahead of an open warning
const server = [...Array.from({ length: 50 }, (_, n) => alert(`resolved-${String(n)}`, "resolved", "critical")), alert("open-1", "open", "warning")];
interface Query { limit?: number; filters?: { status?: string } }
const matching = (q: Query = {}) => server.filter((a) => !q.filters?.status || a.status === q.filters.status);
// the alerts as above, and today's job, so unit-1 is one of the current assignments
const withAssignment = (operation: string, q: Query) => {
  if (operation === "alerts.list") return matching(q);
  return operation === "jobs.list" ? [todayJob] : [];
};

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

  it("keeps an open alert that 50 resolved alerts of a higher severity sort ahead of (IR711)", async () => {
    dal.coreNow.mockResolvedValue(now);
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreOp.mockResolvedValue({ counts: { total: 0, scheduledCount: 0, inProgressCount: 0, overdueCount: 0, assignedCount: 0 } });
    dal.coreItems.mockImplementation((operation: string, q: Query) => Promise.resolve(operation === "alerts.list" ? matching(q).slice(0, q.limit ?? 25) : []));
    // unit-1 is assigned (todayJob): the card lists only the alerts of the current assignments (UI/UX loop R49)
    dal.coreAll.mockImplementation((operation: string, q: Query) => Promise.resolve(withAssignment(operation, q)));
    const live = await loadOverview({});
    expect(live.alerts.map((a) => a.id)).toStrictEqual(["open-1"]);
    dal.coreAll.mockImplementation((operation: string, q: Query) => Promise.resolve(operation === "alerts.list" ? matching(q) : []));
    const unassigned = await loadOverview({});
    expect(unassigned.alerts).toStrictEqual([]); // readable, but no unit of an assignment
  });

  // later today, not accepted yet: read-only until its window opens (IR49) and waiting for the acknowledgement
  const laterJob: ApiTechJobRow = {
    ...todayJob, id: "job-later", unitId: "unit-2", assignmentAcknowledgement: "pending",
    requestedSlot: { startAt: "2026-11-10T06:00:00Z", endAt: "2026-11-10T08:00:00Z" }, scheduledSlot: { startAt: "2026-11-10T06:00:00Z", endAt: "2026-11-10T08:00:00Z" },
  };
  const tomorrowJob: ApiTechJobRow = {
    ...todayJob, id: "job-tomorrow", dueAt: "2026-11-12T10:00:00Z",
    requestedSlot: { startAt: "2026-11-11T01:00:00Z", endAt: "2026-11-11T05:00:00Z" }, scheduledSlot: { startAt: "2026-11-11T01:00:00Z", endAt: "2026-11-11T05:00:00Z" },
  };
  const counts = { counts: { total: 3, scheduledCount: 0, inProgressCount: 0, overdueCount: 0, assignedCount: 3 } };

  it("lists every assigned job on the all tab, sorted as asked, in another display time zone", async () => {
    dal.coreNow.mockResolvedValue(now);
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Tokyo" });
    dal.coreOp.mockResolvedValue(counts);
    dal.coreAll.mockImplementation((operation: string) => {
      if (operation === "jobs.list") return Promise.resolve([todayJob, laterJob, tomorrowJob]);
      return Promise.resolve(operation === "units.list" ? [{ id: "unit-1", displayName: "Lobby AC" }] : []);
    });
    const live = await loadOverview({ tab: "all", sort: "status" });
    expect(dal.coreAll).toHaveBeenCalledWith("jobs.list", { sort: { field: "status", direction: "asc" } });
    expect([live.tab, live.sort, live.title]).toStrictEqual(["all", "status", "All assigned — sorted by progress"]);
    expect(live.rows.map((r) => r.id)).toStrictEqual(["job-today", "job-later", "job-tomorrow"]);
    expect(live.todayRows.map((r) => r.id)).toStrictEqual(["job-today", "job-later"]);
    expect(live.tabs).toStrictEqual([{ id: "today", label: "Today", count: 2 }, { id: "all", label: "All assigned", count: 3 }]);
    expect(live.sorts.map((s) => s.id)).toStrictEqual(["severity", "dueAt", "status"]);
    expect(live.pending.map((r) => r.id)).toStrictEqual(["job-later"]);
    expect(live.readOnly.map((r) => r.id)).toStrictEqual(["job-later"]);
    expect(live.otherZone).toBe(true);
    expect(live.now).toBe(now.toISOString());
    expect(live.rows.map((r) => r.title)).toStrictEqual(["Lobby AC", "Unit", "Lobby AC"]); // unit-2 is not in units.list
  });

  it("defaults to today sorted by severity and keeps the page when units and alerts are refused", async () => {
    dal.coreNow.mockResolvedValue(now);
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreOp.mockResolvedValue(counts);
    dal.coreAll.mockImplementation((operation: string) => operation === "jobs.list" ? Promise.resolve([todayJob, tomorrowJob]) : Promise.reject(new Error("refused")));
    const live = await loadOverview({ tab: "unknown", sort: "unknown" });
    expect(dal.coreAll).toHaveBeenCalledWith("jobs.list", { sort: { field: "severity", direction: "desc" } });
    expect([live.tab, live.sort, live.title]).toStrictEqual(["today", "severity", "Today — sorted by severity, deadline, progress"]);
    expect(live.rows.map((r) => r.id)).toStrictEqual(["job-today"]);
    expect([live.alerts, live.pending, live.readOnly, live.otherZone]).toStrictEqual([[], [], [], false]);
    expect(live.day).toMatch(/^Today · /);
  });
});
