import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor overview's activity (FR-P01) reads the events of the 8 jobs that moved last: open ones first, then
// by due date, newest first. A history snapshot (after the delegation) has no due date and goes after the dated jobs.
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

import { loadOverview } from "../app/partner/(overview)/_lib/load";
import type { ApiPartnerJob } from "@ac/web/lib/partnerOverview";

const slot = { startAt: "2026-11-20T01:00:00Z", endAt: "2026-11-20T05:00:00Z" };
const summary = (id: string, status: string, dueAt: string): ApiPartnerJob => ({
  id, status, dueAt, projection: "summary", version: 1, unitId: "unit-1", type: "maintenance", requestedSlot: slot, scheduledSlot: null,
  assignmentId: null, displayStatus: status, technicianMembershipId: null, eventSummary: { count: 0, customerVisible: 0, notes: 0, latest: null },
});
const history = (jobId: string): ApiPartnerJob => ({ jobId, projection: "history", type: "maintenance", status: "completed", completedAt: null, asOf: "2026-11-15T00:00:00Z", ownDecisionEvents: [] });
const jobIdOf = (input: unknown) => (typeof input === "object" && input !== null && "jobId" in input && typeof input.jobId === "string" ? input.jobId : null);
const dateOf = (input: unknown) => (typeof input === "object" && input !== null && "date" in input && typeof input.date === "string" ? input.date : null);
const eventReads =() => dal.coreItems.mock.calls.filter(([operation]) => operation === "jobs.events").map(([, input]) => jobIdOf(input));

describe("partner overview loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreOp.mockResolvedValue({ counts: { offerCount: 0, activeCount: 0, reviewCount: 0, overdueCount: 0 } });
    dal.coreItems.mockResolvedValue([]);
  });

  it("reads the events of open jobs first, then completed ones newest due first, and histories last", async () => {
    const rows = [
      summary("done-old", "completed", "2026-11-10T00:00:00Z"), history("hist-a"), summary("done-new", "completed", "2026-11-14T00:00:00Z"),
      history("hist-b"), summary("open-1", "assigned", "2026-11-20T00:00:00Z"), summary("done-mid", "completed", "2026-11-12T00:00:00Z"),
    ];
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? rows : []));
    const live = await loadOverview({});
    expect(live.jobCount).toBe(6);
    expect(eventReads()).toStrictEqual(["open-1", "done-new", "done-mid", "done-old", "hist-a", "hist-b"]);
  });

  it("keeps the 8 jobs that moved last when histories outnumber the rest", async () => {
    const rows = [...["h1", "h2", "h3", "h4", "h5", "h6", "h7"].map((id) => history(id)), summary("done-1", "completed", "2026-11-14T00:00:00Z"), summary("done-2", "completed", "2026-11-13T00:00:00Z")];
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? rows : []));
    await loadOverview({});
    expect(eventReads().slice(0, 2)).toStrictEqual(["done-1", "done-2"]);
    expect(eventReads()).toHaveLength(8);
  });

  it("reads today's capacity on its own for a period without today, and names units and technicians", async () => {
    const job = { ...summary("job-1", "assigned", "2026-11-03T08:00:00Z"), technicianMembershipId: "m-1", scheduledSlot: { startAt: "2026-11-03T01:00:00Z", endAt: "2026-11-03T03:00:00Z" } };
    const member = { id: "m-1", userId: "u-1", displayName: "Aiman", role: "technician" };
    const lists = new Map<string, unknown[]>([["jobs.list", [job]], ["members.list", [member]], ["units.list", [{ id: "unit-1", displayName: "Lobby AC" }]]]);
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
    const todaySlot = { startAt: "2026-11-18T01:00:00Z", endAt: "2026-11-18T03:00:00Z" };
    dal.coreItems.mockImplementation((operation: string, input: { date?: string }) => Promise.resolve(operation === "members.capacity" && input.date === "2026-11-18"
      ? [{ membershipId: "m-1", date: "2026-11-18", availableSlots: [], assignedSlots: [todaySlot], availableMinutes: 480, assignedMinutes: 120, unavailability: null }]
      : []));
    const live = await loadOverview({ from: "2026-11-02", to: "2026-11-04" });
    const capacityDays = dal.coreItems.mock.calls.filter(([operation]) => operation === "members.capacity").map(([, input]) => dateOf(input));
    expect(capacityDays).toStrictEqual(["2026-11-02", "2026-11-03", "2026-11-04", "2026-11-18"]);
    expect(live.timeline.map((r) => [r.name, r.blocks.map((b) => b.text)])).toStrictEqual([["Aiman", ["09:00–11:00"]]]);
    expect(JSON.stringify(live)).toContain("Lobby AC");
  });
});
