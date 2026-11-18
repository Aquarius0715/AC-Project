import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's unit view (FR-P04, FR-P08): the unit inside the accepted offer's window, or, once the window ended
// and the URL names the job, that job's history snapshot; the evidence reads (alerts, jobs, members, 24 h series) fall
// back to empty when refused, and fail the page only when the Core API is unavailable.
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; fieldErrors: Record<string, string> };
    constructor(code: string, fieldErrors: Record<string, string> = {}, options?: ErrorOptions) {
      super(code, options); this.name = "CoreError"; this.error = { code, fieldErrors };
    }
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

import { loadUnit } from "../app/partner/units/[id]/_lib/load";
import { unitDetail } from "@ac/web/lib/tests/unitFixture";
import type { ApiUnitDetail } from "@ac/web/lib/units";

type Reading = ApiUnitDetail["latestMeasurements"][number];
const NOW = "2026-09-14T01:35:00Z";
const reading = (metric: Reading["metric"], unit: Reading["unit"]): Reading =>
  ({ id: metric, unitId: "unit-p-01", sensorId: "s", observedAt: NOW, origin: "measured", qualityReason: null, value: 1, quality: "valid", metric, unit });
const unit = unitDetail({ id: "unit-p-01", displayName: "Rooftop unit", latestMeasurements: [reading("vibration", "mm/s"), reading("temperature", "°C")] });
const job = { projection: "summary", id: "job-p02-aaaa", type: "reactive", status: "accepted", scheduledSlot: null, technicianMembershipId: "mem-tech-1", accessValidFrom: "2026-09-13T17:00:00Z", accessValidUntil: "2026-09-14T16:00:00Z" };
const history = { projection: "history", jobId: "job-p02-aaaa", type: "reactive", status: "completed", asOf: "2026-09-21T16:00:00Z", completedAt: "2026-09-21T08:00:00Z", ownDecisionEvents: [] };

describe("partner unit loader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dal.coreNow.mockResolvedValue(new Date(NOW));
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.coreAll.mockImplementation((operation: string) => Promise.resolve(operation === "members.list" ? [{ id: "mem-tech-1", displayName: "Tech One" }] : []));
    dal.coreItems.mockImplementation((operation: string) => Promise.resolve(operation === "jobs.list" ? [job] : []));
    dal.coreOp.mockImplementation((operation: string, input: { metric?: string }) => {
      if (operation === "units.get") return Promise.resolve(unit);
      if (operation === "telemetry.series") {
        return Promise.resolve({ items: [], buckets: input.metric === "vibration" ? [{ start: "2026-09-14T00:00:00Z", value: 2.5, count: 4 }, { start: "2026-09-14T01:00:00Z", value: null, count: 0 }] : null });
      }
      return Promise.reject(new dal.CoreError("NOT_FOUND"));
    });
  });

  it("shows the unit with the job's context, the readings and a whole day of hourly averages", async () => {
    const live = await loadUnit("unit-p-01", "job-p02-aaaa");
    if (live?.kind !== "live") throw new Error("expected the live view");
    expect(live.name).toBe("Rooftop unit");
    expect(live.banner).toBe("Diagnosis-scoped view for job-p02- — no billing, payments, or other contracts.");
    expect(live.context?.id).toBe("job-p02-aaaa");
    expect(live.charts.map((c) => [c.metric, c.title, c.now])).toStrictEqual([["vibration", "Vibration (mm/s)", "now 2.5"], ["temperature", "Temperature (°C)", "no reading"]]);
    const series = dal.coreOp.mock.calls.filter(([operation]) => operation === "telemetry.series").map(([, input]): unknown => input);
    expect(series).toStrictEqual(["vibration", "temperature"].map((metric) => ({ from: "2026-09-13T01:35:00Z", to: "2026-09-14T01:35:00Z", unitIds: ["unit-p-01"], bucketMinutes: 60, query: {}, metric })));
  });

  it("falls back to empty evidence when a read is refused, and fails when the Core API is unavailable", async () => {
    dal.coreAll.mockRejectedValue(new dal.CoreError("FORBIDDEN"));
    dal.coreItems.mockRejectedValue(new dal.CoreError("FORBIDDEN"));
    dal.coreOp.mockImplementation((operation: string) => (operation === "units.get" ? Promise.resolve(unit) : Promise.reject(new dal.CoreError("FORBIDDEN"))));
    const live = await loadUnit("unit-p-01");
    if (live?.kind !== "live") throw new Error("expected the live view");
    expect([live.context, live.alerts, live.past]).toStrictEqual([null, [], []]);
    expect(live.banner).toBe("Diagnosis-scoped view for your accepted job — no billing, payments, or other contracts.");
    expect(live.charts.map((c) => c.now)).toStrictEqual(["no reading", "no reading"]);
    dal.coreItems.mockRejectedValue(new dal.CoreError("UNAVAILABLE"));
    await expect(loadUnit("unit-p-01")).rejects.toThrow("UNAVAILABLE");
  });

  it("charts only the metrics the unit reports, with their measurement unit", async () => {
    dal.coreOp.mockImplementation((operation: string) => (operation === "units.get" ? Promise.resolve({ ...unit, latestMeasurements: [reading("power", "kW")] }) : Promise.resolve({ items: [], buckets: [] })));
    const live = await loadUnit("unit-p-01");
    if (live?.kind !== "live") throw new Error("expected the live view");
    expect(live.charts.map((c) => c.title)).toStrictEqual(["Power (kW)"]);
  });

  it("shows the job's history snapshot once the window ended, and not found otherwise", async () => {
    for (const refusal of [new dal.CoreError("NOT_FOUND"), new dal.CoreError("FORBIDDEN"), new dal.CoreError("VALIDATION", { id: "uuid" })]) {
      dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.get" ? Promise.resolve(history) : Promise.reject(refusal)));
      const snapshot = await loadUnit("unit-p-01", "job-p02-aaaa");
      expect(snapshot?.kind).toBe("snapshot");
      expect(snapshot && "title" in snapshot ? [snapshot.title, snapshot.jobId] : null).toStrictEqual(["Job history snapshot — job-p02-", "job-p02-aaaa"]);
      expect(await loadUnit("unit-p-01")).toBeNull();
    }
    // the job still open (not a snapshot) or gone too: not found
    dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.get" ? Promise.resolve({ ...job, projection: "detail" }) : Promise.reject(new dal.CoreError("NOT_FOUND"))));
    expect(await loadUnit("unit-p-01", "job-p02-aaaa")).toBeNull();
    dal.coreOp.mockRejectedValue(new dal.CoreError("NOT_FOUND"));
    expect(await loadUnit("unit-p-01", "job-p02-aaaa")).toBeNull();
  });

  it("fails on any other error of the unit or the job read", async () => {
    dal.coreOp.mockRejectedValue(new dal.CoreError("UNAVAILABLE"));
    await expect(loadUnit("unit-p-01")).rejects.toThrow("UNAVAILABLE");
    dal.coreOp.mockImplementation((operation: string) => Promise.reject(new dal.CoreError(operation === "units.get" ? "NOT_FOUND" : "VALIDATION", { jobId: "uuid" })));
    await expect(loadUnit("unit-p-01", "job-p02-aaaa")).rejects.toThrow("VALIDATION");
  });
});
