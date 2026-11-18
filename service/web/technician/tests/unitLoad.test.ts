import { beforeEach, describe, expect, it, vi } from "vitest";

// the technician unit screen (FR-T02, FR-T03, SCR-T02) through the DAL: units.get, the work window that has not started
// (IR76), the series of a period (IR598, IR605) and the links that carry the URL's job
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; messageKey: string; fieldErrors: Record<string, string> };
    constructor(code: string, options: ErrorOptions & { messageKey?: string; fieldErrors?: Record<string, string> } = {}) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: options.messageKey ?? "", fieldErrors: options.fieldErrors ?? {} };
    }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), coreAll: vi.fn(),
    coreOp: vi.fn<(operation: string, input: Record<string, unknown>) => Promise<unknown>>(),
    coreItems: vi.fn<(operation: string, input: Record<string, unknown>) => Promise<unknown[]>>(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
      try {
        return await read;
      } catch (error) {
        if (error instanceof CoreError && error.error.code !== "UNAVAILABLE" && error.error.code !== "TIMEOUT") return fallback;
        throw error;
      }
    },
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadTechUnit, type UnitLive } from "../app/technician/units/[id]/_lib/load";
import type { ApiTechUnit } from "@ac/web/lib/techUnit";

const NOW = new Date("2026-09-14T01:35:00Z");
type Reading = ApiTechUnit["latestMeasurements"][number];
const reading = (metric: Reading["metric"], value: number, unit: Reading["unit"]): Reading =>
  ({ metric, value, unit, quality: "valid", id: metric, unitId: "unit-1", sensorId: "s", origin: "measured", qualityReason: null, observedAt: "2026-09-14T01:35:00Z" });
const unit: ApiTechUnit = {
  id: "unit-1", installState: "installed", modelId: "model-1", version: 1, spaceId: null, alertPolicyIds: [], effectivePowerState: "on", displayName: "Bedroom AC", type: "split",
  connection: "online", lastSeenAt: "2026-09-14T01:34:00Z", installedAt: "2025-03-01T00:00:00Z", capabilityVersion: 3, serviceScope: ["indoor"], components: [],
  location: { pathLabels: ["customer-a", "Home A"], address: null, accessInstructions: null },
  capabilities: {
    manufacturer: "AC-Co", model: "v3", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool"],
    fanLevels: ["low"], ventilation: false, ventilationLevels: [],
  },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: "2026-09-14T01:34:30Z" },
  latestMeasurements: [reading("temperature", 24.5, "°C"), reading("humidity", 60, "%"), reading("power", 1.2, "kW"), reading("co2", 600, "ppm"), reading("pm25", 12, "µg/m³")],
};
const slot = { startAt: "2026-09-14T01:00:00Z", endAt: "2026-09-14T03:00:00Z" };
const running = { projection: "summary", id: "job-run", version: 1, unitId: "unit-1", type: "repair", status: "in_progress", severity: "normal", dueAt: "2026-09-15T00:00:00Z", requestedSlot: slot, scheduledSlot: slot };
const openAlert = { id: "a&1", unitId: "unit-1", type: "sensor", severity: "warning", status: "open", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Hot.", detectedAt: "2026-09-14T01:12:00Z", acknowledgedAt: null, resolvedAt: null, resolutionReason: null };

/** coreOp answers: units.get the unit (or `get`), telemetry.series `series`, jobs.get `job`; coreItems the `items` of an op. */
function serve(o: { get?: () => Promise<unknown>; series?: (input: Record<string, unknown>) => unknown; job?: () => Promise<unknown>; items?: Record<string, unknown[]> } = {}) {
  dal.coreOp.mockImplementation((operation: string, input: Record<string, unknown>) => {
    if (operation === "units.get") return o.get ? o.get() : Promise.resolve(unit);
    if (operation === "jobs.get") return o.job ? o.job() : Promise.reject(new Error("jobs.get not expected"));
    if (operation === "telemetry.series") return Promise.resolve(o.series ? o.series(input) : { items: [], total: 0, buckets: [] });
    return Promise.reject(new Error(`unexpected ${operation}`));
  });
  const items = new Map(Object.entries(o.items ?? {}));
  dal.coreItems.mockImplementation((operation: string) => Promise.resolve(items.get(operation) ?? []));
  dal.coreAll.mockImplementation((operation: string, input: { filters: { status: string } }) => Promise.resolve(operation === "alerts.list" && input.filters.status === "open" ? (items.get("alerts") ?? []) : []));
}
const notStarted = () => Promise.reject(new dal.CoreError("FORBIDDEN", { messageKey: "errors.assignment_not_started" }));
const live = async (sp: Parameters<typeof loadTechUnit>[1]): Promise<UnitLive> => {
  const page = await loadTechUnit("unit-1", sp);
  if (page?.kind !== "live") throw new Error(`not live: ${JSON.stringify(page)}`);
  return page;
};
const calls = (operation: string) => dal.coreOp.mock.calls.filter(([op]) => op === operation).map(([, input]) => input);
const itemCalls = (operation: string) => dal.coreItems.mock.calls.filter(([op]) => op === operation).map(([, input]) => input);

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
});

describe("technician unit: before the work window and unreadable units", () => {
  it("says when the window opens from the URL's job, and links that job encoded (IR76)", async () => {
    serve({ get: notStarted, job: () => Promise.resolve({ projection: "detail", assignment: { scheduledStart: "2026-09-14T02:00:00Z" } }) });
    const page = await loadTechUnit("unit-1", { jobId: "job/1&x" });
    expect(page).toStrictEqual({
      kind: "not_started", title: "Not started yet", text: expect.stringContaining("10:00") as unknown,
      job: { href: "/technician/jobs/job%2F1%26x", label: "Open the job" },
    });
  });

  it("gives the general sentence without a job, when the job is refused and when it has no assignment", async () => {
    serve({ get: notStarted });
    const general = { kind: "not_started", title: "Not started yet", text: "Available from the work start time of your assigned job.", job: null };
    expect(await loadTechUnit("unit-1", {})).toStrictEqual(general);
    expect(calls("jobs.get")).toHaveLength(0);
    serve({ get: notStarted, job: () => Promise.reject(new dal.CoreError("FORBIDDEN")) });
    expect(await loadTechUnit("unit-1", { jobId: "job-1" })).toStrictEqual(general);
    serve({ get: notStarted, job: () => Promise.resolve({ projection: "detail", assignment: null }) });
    expect(await loadTechUnit("unit-1", { jobId: "job-1" })).toStrictEqual(general);
    serve({ get: notStarted, job: () => Promise.resolve({ projection: "summary" }) });
    expect(await loadTechUnit("unit-1", { jobId: "job-1" })).toStrictEqual(general);
  });

  it("is not found for a unit outside the assignments (NOT_FOUND, FORBIDDEN, a bad id; IR169)", async () => {
    for (const refusal of [new dal.CoreError("NOT_FOUND"), new dal.CoreError("FORBIDDEN"), new dal.CoreError("VALIDATION", { fieldErrors: { id: "invalid" } })]) {
      serve({ get: () => Promise.reject(refusal) });
      expect(await loadTechUnit("unit-1", {})).toBeNull();
    }
  });

  it("fails the page when the unit cannot be read now", async () => {
    serve({ get: () => Promise.reject(new dal.CoreError("UNAVAILABLE")) });
    await expect(loadTechUnit("unit-1", {})).rejects.toThrow("UNAVAILABLE");
    serve({ get: () => Promise.reject(new dal.CoreError("VALIDATION", { fieldErrors: { metric: "invalid" } })) });
    await expect(loadTechUnit("unit-1", {})).rejects.toThrow("VALIDATION");
  });
});

describe("technician unit: the live page", () => {
  it("keeps the URL's jobId one value in every link, and the control link opens the active job", async () => {
    serve({ items: { "jobs.list": [running], alerts: [openAlert] } });
    const page = await live({ jobId: "x&alertId=other" });
    expect(page.back).toBe("/technician/jobs/x%26alertId%3Dother");
    expect(page.alertsHref).toBe("/technician/units/unit-1/alerts?jobId=job-run"); // the active job wins over the URL's
    expect(page.alerts.map((a) => a.href)).toStrictEqual(["/technician/units/unit-1/alerts?jobId=job-run&alertId=a%261"]);
    expect(page.controlHref).toBe("/technician/units/unit-1/control?jobId=job-run");
    serve({ items: { alerts: [openAlert] } });
    const noActive = await live({ jobId: "x&alertId=other" });
    expect(noActive.alertsHref).toBe("/technician/units/unit-1/alerts?jobId=x%26alertId%3Dother");
    expect(noActive.alerts.map((a) => a.href)).toStrictEqual(["/technician/units/unit-1/alerts?jobId=x%26alertId%3Dother&alertId=a%261"]);
    expect(noActive.controlHref).toBeNull();
  });

  it("charts temperature without a unit for a unit that has measured nothing yet", async () => {
    serve({});
    dal.coreOp.mockImplementation((operation: string) => Promise.resolve(operation === "units.get" ? { ...unit, latestMeasurements: [] } : { items: [], total: 0, buckets: [] }));
    const page = await live({ metric: "humidity" });
    expect(page.metrics).toStrictEqual(["temperature"]);
    expect(page.chart.lines.map((l) => [l.metric, l.unit])).toStrictEqual([["temperature", ""]]);
  });

  it("goes back to the overview and opens the alerts without a job when there is none", async () => {
    serve({ items: { alerts: [openAlert] } });
    const page = await live({});
    expect(page.back).toBe("/technician");
    expect(page.alertsHref).toBe("/technician/units/unit-1/alerts");
    expect(page.alerts.map((a) => a.href)).toStrictEqual(["/technician/units/unit-1/alerts?alertId=a%261"]);
  });

  it("opens on the register tab without a metric: no device reads, no other metrics", async () => {
    serve({});
    const page = await live({});
    expect(page.tab).toBe("register");
    expect(page.metrics).toStrictEqual(["temperature"]);
    expect(page.period).toBe("24h");
    expect(itemCalls("devices.list")).toHaveLength(0);
    expect(calls("telemetry.series").map((input) => input.metric)).toStrictEqual(["temperature"]);
  });

  it("reads 24 h and 7 d as averages and drops the buckets with no reading (IR605, IR606)", async () => {
    // a bucket with no reading is dropped even if it carried a value
    const buckets = [{ start: "2026-09-13T12:00:00Z", value: 24, count: 3 }, { start: "2026-09-14T00:05:00Z", value: 99, count: 0 }];
    serve({ series: () => ({ items: [], total: 3, buckets }) });
    const day = await live({ metric: "temperature", period: "24h" });
    expect(calls("telemetry.series").at(0)).toMatchObject({ bucketMinutes: 5, unitIds: ["unit-1"], metric: "temperature", from: "2026-09-13T01:35:00.000Z", to: "2026-09-14T01:35:01.000Z" });
    expect(day.chart.lines.at(0)?.points.filter((v) => v !== null)).toStrictEqual([24]);
    expect(day.chart.lines.at(0)?.cut).toBeNull(); // averages are the whole period
    dal.coreOp.mockClear();
    serve({ series: () => ({ items: [], buckets: null }) }); // no buckets and no total: an empty week
    const week = await live({ metric: "temperature", period: "7d" });
    expect(calls("telemetry.series").at(0)).toMatchObject({ bucketMinutes: 60, from: "2026-09-07T16:00:00.000Z" });
    expect(week.chart.lines.at(0)?.empty).toBe(true);
    expect(week.chart.lines.at(0)?.count).toBe("0 measurements in this period");
  });

  it("pages an hour up to 4 pages of 100, newest first, and stops when there is no next page (IR598)", async () => {
    let n = 0;
    serve({ series: (input) => {
      if (input.metric !== "temperature") return { items: [], total: 0, nextCursor: null, buckets: null };
      n++;
      return { items: [{ value: n, observedAt: "2026-09-14T01:30:00Z" }], total: n === 1 ? 900 : 1, nextCursor: `c${String(n)}`, buckets: null };
    } });
    const hour = await live({ metric: "temperature", period: "1h" });
    const pages = calls("telemetry.series").filter((input) => input.metric === "temperature");
    expect(pages).toHaveLength(4);
    expect(hour.chart.lines.at(0)?.cut).not.toBeNull(); // 4 of the 900 readings loaded: says the rest is not loaded
    const sort = { field: "observedAt", direction: "desc" };
    expect(pages.map((input) => input.query)).toStrictEqual([{ limit: 100, sort }, ...["c1", "c2", "c3"].map((cursor) => ({ limit: 100, sort, cursor }))]);
    dal.coreOp.mockClear();
    serve({ series: () => ({ items: [{ value: 1, observedAt: "2026-09-14T01:30:00Z" }], buckets: null, nextCursor: null }) });
    await live({ metric: "temperature", period: "1h" });
    expect(calls("telemetry.series")).toHaveLength(1 + 3); // the chosen metric's one page, then one page of each other metric
  });

  it("on the monitoring tab reads up to 3 other metrics and the events of the unit's devices in the period", async () => {
    serve({ items: { "devices.list": [{ id: "dev-1" }, { id: "dev-2" }] } });
    const page = await live({ metric: "temperature" });
    expect(page.tab).toBe("monitoring");
    expect(calls("telemetry.series").map((input) => input.metric)).toStrictEqual(["temperature", "humidity", "power", "co2"]);
    expect(itemCalls("devices.list")).toStrictEqual([{ limit: 5, filters: { unitId: "unit-1" } }]);
    expect(itemCalls("devices.events")).toStrictEqual(["dev-1", "dev-2"].map((id) => ({
      id, query: { limit: 50, filters: { from: "2026-09-13T01:35:00.000Z", to: "2026-09-14T01:35:01.000Z" } },
    })));
  });

  it("shows an empty series when one is refused, and fails the page when the series cannot be read now", async () => {
    serve({});
    dal.coreOp.mockImplementation((operation: string) => (operation === "units.get" ? Promise.resolve(unit) : Promise.reject(new dal.CoreError("FORBIDDEN"))));
    const page = await live({ metric: "temperature" });
    expect(page.chart.lines.at(0)?.empty).toBe(true);
    dal.coreOp.mockImplementation((operation: string) => (operation === "units.get" ? Promise.resolve(unit) : Promise.reject(new dal.CoreError("UNAVAILABLE"))));
    await expect(loadTechUnit("unit-1", { metric: "temperature" })).rejects.toThrow("UNAVAILABLE");
  });
});
