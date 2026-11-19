import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's job history (FR-P07): the latest 100 jobs of the 90-day period; an empty period says how many lie
// outside it, and a job in the URL that is older than the period is looked up without it — with one read of the
// wider list, not two
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

import { loadHistory } from "../app/partner/history/_lib/load";
import type { ApiPartnerJob } from "@ac/web/lib/partnerOverview";

const old: ApiPartnerJob = { projection: "history", jobId: "job-old-0001", type: "maintenance", status: "completed", completedAt: "2026-05-01T03:00:00Z", asOf: "2026-05-02T00:00:00Z", ownDecisionEvents: [] };
const listCalls = () => dal.coreOp.mock.calls.filter(([operation]) => operation === "jobs.list");
const withPeriod = (call: unknown[] | undefined) => {
  const input: unknown = call?.[1];
  return typeof input === "object" && input !== null && "filters" in input && typeof input.filters === "object" && input.filters !== null && "from" in input.filters;
};

describe("partner history loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date("2026-11-18T02:00:00Z"));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "Contractor A" });
    dal.corePrincipal.mockResolvedValue({ userId: "u-owner" });
    dal.coreAll.mockResolvedValue([]);
    dal.coreItems.mockResolvedValue([]);
    // the period holds no job; without the period the old one is there
    dal.coreOp.mockImplementation((operation: string, input: unknown) => {
      if (operation !== "jobs.list") return Promise.resolve({ items: [], total: 0 });
      const scoped = withPeriod([operation, input]);
      return Promise.resolve(scoped ? { items: [], total: 0 } : { items: [old], total: 1 });
    });
  });

  it("finds a job older than an empty period with one read of the list without the period", async () => {
    const live = await loadHistory({ period: "90d", jobId: "job-old-0001" });
    expect(live.outside).toBe(1);
    expect(live.rows).toStrictEqual([]);
    expect(live.detail?.missing).toBe(false);
    expect(live.detail && !live.detail.missing ? live.detail.mode.kind : null).toBe("closed");
    expect(listCalls().map((c) => withPeriod(c))).toStrictEqual([true, false]);
    expect(dal.coreOp).not.toHaveBeenCalledWith("notifications.recipients", expect.anything());
  });

  it("reads the list without the period once for a job older than a period that has jobs", async () => {
    const recent: ApiPartnerJob = { ...old, jobId: "job-new-0001", completedAt: "2026-11-10T03:00:00Z" };
    dal.coreOp.mockImplementation((operation: string, input: unknown) => {
      if (operation !== "jobs.list") return Promise.resolve({ items: [], total: 0 });
      return Promise.resolve(withPeriod([operation, input]) ? { items: [recent], total: 1 } : { items: [recent, old], total: 2 });
    });
    const live = await loadHistory({ period: "90d", jobId: "job-old-0001" });
    expect(live.outside).toBe(0);
    expect(live.rows.map((r) => r.id)).toStrictEqual(["job-new-0001"]);
    expect(live.detail && !live.detail.missing ? live.detail.row.id : null).toBe("job-old-0001");
    expect(listCalls().map((c) => withPeriod(c))).toStrictEqual([true, false]);
    // the job dropdown holds the open job, or the browser shows another job selected and choosing it does nothing
    expect(live.options.map((o) => o.id)).toStrictEqual(["job-new-0001", "job-old-0001"]);
  });

  it("adds a job older than an empty period to the job dropdown once", async () => {
    const live = await loadHistory({ period: "90d", jobId: "job-old-0001" });
    expect(live.options).toStrictEqual([{ id: "job-old-0001", label: "job-old- · maintenance" }]);
    const inPeriod = await loadHistory({ period: "all", jobId: "job-old-0001" });
    expect(inPeriod.options.map((o) => o.id)).toStrictEqual(["job-old-0001"]);
  });

  it("says a job is missing when it is in neither list", async () => {
    const live = await loadHistory({ period: "90d", jobId: "job-gone-001" });
    expect(live.detail).toStrictEqual({ missing: true, id: "job-gone-001" });
    expect(listCalls()).toHaveLength(2);
    // under all time the one list already is the wider one
    const allTime = await loadHistory({ period: "all", jobId: "job-gone-001" });
    expect([allTime.detail, allTime.options.map((o) => o.id)]).toStrictEqual([{ missing: true, id: "job-gone-001" }, ["job-old-0001"]]);
    expect(listCalls()).toHaveLength(3);
  });

  it("reads all time once, names the units, and has no detail without a job in the URL", async () => {
    dal.coreIdentity.mockResolvedValue({ displayName: "Owner", organizationName: "" });
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "units.list" ? [{ id: "unit-1", displayName: "Lobby AC" }] : []));
    dal.coreOp.mockResolvedValue({ items: [open, { projection: "offer", jobId: "job-offer-01" }, old], total: 3 });
    const live = await loadHistory({ period: "all" });
    expect(listCalls().map((c) => withPeriod(c))).toStrictEqual([false]);
    expect([live.period, live.outside, live.total, live.company, live.detail]).toStrictEqual(["all", 0, 3, "your company", null]);
    // the offer drops out; the open job is named by its unit
    expect(live.options).toStrictEqual([{ id: "job-open-001", label: "job-open · Lobby AC" }, { id: "job-old-0001", label: "job-old- · maintenance" }]);
    expect(dal.coreItems).not.toHaveBeenCalled();
  });

  it("shows an open job's timeline newest first and who may get a preview, without the contractor", async () => {
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "members.list" ? [{ id: "mem-1", userId: "u-tech", displayName: "Tech One" }] : []));
    const recipients = { items: [recipient("r-hq", "hq"), recipient("r-own", "contractor"), recipient("r-cust", "customer_contact")], total: 3 };
    dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? { items: [open], total: 1 } : recipients));
    // two events at the same time are ordered by ID; the API's order is not trusted
    dal.coreItems.mockResolvedValue([event("ev-1", "2026-11-10T01:00:00Z", null), event("ev-2", "2026-11-12T01:00:00Z", "u-tech"), event("ev-3", "2026-11-12T01:00:00Z", "u-owner")]);
    const live = await loadHistory({ jobId: "job-open-001" });
    const d = live.detail;
    if (!d || d.missing) throw new Error("expected the job");
    expect(d.items.map((x) => [x.id, x.detail])).toStrictEqual([["ev-3", "by you"], ["ev-2", "by Tech One"], ["ev-1", "by system"]]);
    expect(dal.coreItems).toHaveBeenCalledWith("jobs.events", { jobId: "job-open-001", query: { limit: 100, sort: { field: "occurredAt", direction: "desc" } } });
    expect(d.mode.kind).toBe("note");
    expect(d.recipients.map((r) => r.id)).toStrictEqual(["r-hq", "r-cust"]);
    expect(listCalls()).toHaveLength(1);
  });

  it("keeps a completed job without a timeline or recipients when they are refused, and fails when a read is unavailable", async () => {
    let refusal = "FORBIDDEN";
    dal.coreAll.mockRejectedValue(new dal.CoreError("FORBIDDEN"));
    dal.coreItems.mockImplementation(() => Promise.reject(new dal.CoreError(refusal)));
    dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.list" ? Promise.resolve({ items: [{ ...open, status: "completed" }], total: 1 }) : Promise.reject(new dal.CoreError(refusal))));
    const live = await loadHistory({ jobId: "job-open-001" });
    const d = live.detail;
    if (!d || d.missing) throw new Error("expected the job");
    // a completed job still gets a preview, so its recipients are read
    expect([d.mode.kind, d.items, d.recipients]).toStrictEqual(["preview", [], []]);
    expect(dal.coreOp).toHaveBeenCalledWith("notifications.recipients", expect.objectContaining({ target: { kind: "job", id: "job-open-001" } }));
    expect(d.row.title).toBe("maintenance");
    // an unavailable timeline or recipient list is not a refusal: the page fails
    refusal = "UNAVAILABLE";
    await expect(loadHistory({ jobId: "job-open-001" })).rejects.toThrow("UNAVAILABLE");
    dal.coreItems.mockResolvedValue([]);
    await expect(loadHistory({ jobId: "job-open-001" })).rejects.toThrow("UNAVAILABLE");
    expect(dal.coreOp).toHaveBeenLastCalledWith("notifications.recipients", expect.anything());
    dal.coreOp.mockRejectedValue(new dal.CoreError("UNAVAILABLE"));
    await expect(loadHistory({})).rejects.toThrow("UNAVAILABLE");
  });
});

const open: ApiPartnerJob = {
  projection: "summary", id: "job-open-001", version: 2, unitId: "unit-1", type: "maintenance", status: "assigned", dueAt: "2026-11-20T00:00:00Z",
  requestedSlot: { startAt: "2026-11-19T02:00:00Z", endAt: "2026-11-19T04:00:00Z" }, scheduledSlot: null, assignmentId: "as-1", displayStatus: "assigned", technicianMembershipId: "mem-1",
  eventSummary: { count: 3, customerVisible: 1, notes: 0, latest: { action: "job.assigned", occurredAt: "2026-11-12T01:00:00Z" } },
};
const event = (id: string, occurredAt: string, actorUserId: string | null) => ({ id, actorUserId, occurredAt, jobId: "job-open-001", action: "job.assigned", note: null });
const recipient = (id: string, role: string) => ({ id, role, displayLabel: id, allowedChannels: ["inApp"] });
