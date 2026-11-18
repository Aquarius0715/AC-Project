import { beforeEach, describe, expect, it, vi } from "vitest";

// the technician job workspace (FR-T04–T06, T08, T09, T13–T15) through the DAL: jobs.get, then the unit (or its name from
// units.list before the work window, IR570), the draft or latest report and the parts catalog
// CoreError copies the DAL's: the refusal code, its message key and the field errors
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; messageKey: string; fieldErrors: Record<string, string> };
    constructor(code: string, messageKey = "errors.x", fieldErrors: Record<string, string> = {}, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey, fieldErrors };
    }
  }
  return { CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), coreAll: vi.fn(), coreOp: vi.fn() };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadWorkspace } from "../app/technician/jobs/[id]/_lib/load";
import { componentsFor, warnAt, type ApiTechReport } from "@ac/web/lib/techJob";
import type { ApiUnitDetail } from "@ac/web/lib/units";

const assignment = { scheduledStart: "2026-11-10T01:00:00Z", scheduledEnd: "2026-11-10T03:00:00Z", status: "active", acknowledgement: "accepted" as const, cantMakeReason: null };
const detail = {
  projection: "detail", id: "job-1", version: 4, status: "in_progress", type: "maintenance", unitId: "unit-1", symptom: "Noisy", alertIds: ["a-1", "a-2"],
  origin: "scheduled", dueAt: null, timeOnSite: { arrivedAt: "2026-11-10T01:05:00Z", startedAt: "2026-11-10T01:10:00Z", finishedAt: null, pauses: [] },
  draftReportRef: { reportId: "rep-2", reportVersion: 3 }, reportRefs: [{ reportId: "rep-1", reportVersion: 1 }],
  problemPhotos: [{ id: "ph-1", name: "a.jpg", status: "ready" }, { id: "ph-2", name: "b.jpg", status: "scanning" }],
  assignment,
};

const unit: ApiUnitDetail = {
  id: "unit-1", iotModel: null, installState: "planned", modelId: "model-1", capabilityVersion: 3, serviceScope: ["indoor"], components: [], version: 1, spaceId: null, displayName: "Bedroom AC",
  alertPolicyIds: [], connection: "online", effectivePowerState: "on", observedState: { power: true, celsius: 24, mode: null, fanLevel: null, observedAt: null },
  lastSeenAt: null, latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKM", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] },
  effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: ["Site A", "Floor 2"], address: null, accessInstructions: "Ring twice" }, installedAt: null,
};

const report: ApiTechReport = {
  id: "rep-2", version: 3, jobId: "job-1", authorId: "tech-1", installation: null, items: [], parts: [], refrigerant: [],
  measurements: [{ id: "m-1", metric: "temperature", value: 12, unit: "°C", observedAt: "2026-11-10T01:20:00Z", quality: "valid" }],
  signOff: { signerName: "Ann", signedAt: "2026-11-10T01:50:00Z", signatureAttachmentId: null, absentReason: null, reportVersion: 3 },
  workText: "", nextAction: null, attachmentRefs: [], submittedAt: null, acceptedAt: null,
  reviewHistory: [{ reviewerUserId: "hq-1", reportVersion: 2, decision: "return", reason: "Photo missing", occurredAt: "2026-11-09T08:00:00Z" }],
};

// each read is a value, or a CoreError to reject with
interface Setup { job?: unknown; unit?: unknown; listed?: unknown; report?: unknown; parts?: unknown }
const calls: { operation: string; input: unknown }[] = [];
const settle = (x: unknown): Promise<unknown> => (x instanceof dal.CoreError ? Promise.reject(x) : Promise.resolve(x));
const byText = (a: string, b: string) => a.localeCompare(b);

function setup(s: Setup = {}): void {
  dal.coreNow.mockResolvedValue(new Date("2026-11-10T02:00:00Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.coreAll.mockImplementation((operation: string) => {
    calls.push({ operation, input: null });
    return settle(s.parts ?? [{ code: "P-1", name: "Filter", vanStockQuantity: 2 }]);
  });
  dal.coreOp.mockImplementation((operation: string, input: unknown) => {
    calls.push({ operation, input });
    const reads = new Map<string, unknown>([
      ["jobs.get", s.job ?? detail], ["units.get", s.unit ?? unit], ["units.list", s.listed ?? { items: [{ displayName: "Listed AC" }] }], ["reports.get", s.report ?? report],
    ]);
    return settle(reads.get(operation));
  });
}
const called = (operation: string) => calls.filter((c) => c.operation === operation);
const job = (patch: Record<string, unknown>) => ({ ...detail, ...patch }); // the jobs.get body with some fields changed

beforeEach(() => {
  vi.clearAllMocks();
  calls.length = 0;
});

describe("loadWorkspace: the job read", () => {
  it.each([
    ["NOT_FOUND", new dal.CoreError("NOT_FOUND")],
    ["FORBIDDEN", new dal.CoreError("FORBIDDEN")],
    ["a jobId field error", new dal.CoreError("VALIDATION", "errors.x", { jobId: "invalid" })],
  ])("%s reads as not found", async (_name, error) => {
    setup({ job: error });
    await expect(loadWorkspace("job-x")).resolves.toBe("not_found");
    expect(called("units.get")).toHaveLength(0);
  });

  it("any other refusal fails the page", async () => {
    setup({ job: new dal.CoreError("UNAVAILABLE") });
    await expect(loadWorkspace("job-1")).rejects.toThrow("UNAVAILABLE");
    setup({ job: new dal.CoreError("VALIDATION", "errors.x", { other: "invalid" }) });
    await expect(loadWorkspace("job-1")).rejects.toThrow("VALIDATION");
  });

  it("an offer projection (a contractor's) reads as not found", async () => {
    setup({ job: { projection: "offer" } });
    await expect(loadWorkspace("job-1")).resolves.toBe("not_found");
  });

  it("after the window the history card is shown, with no further reads", async () => {
    setup({ job: { projection: "history", jobId: "job-1", type: "maintenance", status: "completed", asOf: "2026-11-10T02:00:00Z", completedAt: "2026-11-10T02:00:00Z", redactedReportSummary: { hasReport: true, acceptance: "accepted" } } });
    const live = await loadWorkspace("job-1");
    expect(live).toMatchObject({ kind: "history" });
    expect(JSON.stringify(live)).toContain("Accepted in the quality review");
    expect(calls.map((c) => c.operation)).toStrictEqual(["jobs.get"]);
  });
});

describe("loadWorkspace: the workspace", () => {
  it("maps the job, the unit, the draft report and the parts while the job is in progress", async () => {
    setup();
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(called("jobs.get")[0]?.input).toStrictEqual({ jobId: "job-1" });
    expect(called("units.get")[0]?.input).toStrictEqual({ id: "unit-1" });
    expect(called("units.list")).toHaveLength(0);
    expect(called("reports.get")[0]?.input).toStrictEqual({ jobId: "job-1", reportId: "rep-2", reportVersion: 3 }); // the draft, not the older submitted one
    expect(live.now).toBe("2026-11-10T02:00:00.000Z");
    expect(live.zone).toBe("Asia/Kuala_Lumpur");
    expect(live.job).toMatchObject({ id: "job-1", version: 4, alertCount: 2, origin: "scheduled", problemPhotos: [{ id: "ph-1", name: "a.jpg" }], assignment });
    expect(live.unit).toStrictEqual({ name: "Bedroom AC", place: "Site A › Floor 2", access: "Ring twice", model: "Daikin FTKM", modelId: "model-1", planned: true, scope: ["indoor"] });
    expect(live.unitRefused).toBeNull();
    expect(live.unitName).toBe("Bedroom AC");
    expect(live.components).toStrictEqual(componentsFor(["indoor"]));
    expect(live.report).toStrictEqual(report);
    expect(live.reportRefused).toBeNull();
    expect(live.parts).toStrictEqual([{ code: "P-1", name: "Filter", vanStockQuantity: 2 }]);
    // the instants the view shows are formatted here: clocks, stamps and the window span
    expect(Object.keys(live.known.clock).toSorted(byText)).toStrictEqual([
      "2026-11-10T01:05:00Z", "2026-11-10T01:10:00Z", "2026-11-10T01:20:00Z", "2026-11-10T01:50:00Z", "2026-11-10T03:00:00Z", warnAt(assignment.scheduledEnd),
    ].toSorted(byText));
    expect(Object.keys(live.known.stamp).toSorted(byText)).toStrictEqual([assignment.scheduledStart, assignment.scheduledEnd, "2026-11-09T08:00:00Z"].toSorted(byText));
    expect(Object.keys(live.known.span)).toStrictEqual([`${assignment.scheduledStart}|${assignment.scheduledEnd}`]);
  });

  it("before the window the unit is refused, its name comes from units.list and the default checklist applies", async () => {
    setup({ unit: new dal.CoreError("FORBIDDEN", "errors.assignment_not_started"), job: job({ status: "assigned", draftReportRef: null, reportRefs: [], timeOnSite: null }) });
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(called("units.list")[0]?.input).toStrictEqual({ limit: 1, filters: { unitIds: ["unit-1"] } });
    expect(live.unit).toBeNull();
    expect(live.unitRefused).toBe("errors.assignment_not_started");
    expect(live.unitName).toBe("Listed AC");
    expect(live.components).toStrictEqual(componentsFor(["indoor", "outdoor", "electrical"]));
    // no report yet, and not in progress: neither reports.get nor parts.list is read
    expect(called("reports.get")).toHaveLength(0);
    expect(called("parts.list")).toHaveLength(0);
    expect(live.report).toBeNull();
    expect(live.reportRefused).toBeNull();
    expect(live.parts).toStrictEqual([]);
  });

  it.each([
    ["refused", new dal.CoreError("NOT_FOUND")],
    ["empty", { items: [] }],
  ])("the unit name is null when units.list is %s", async (_name, listed) => {
    setup({ unit: new dal.CoreError("FORBIDDEN"), listed });
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(live.unitName).toBeNull();
  });

  it("a unit read failing for another reason fails the page", async () => {
    setup({ unit: new dal.CoreError("UNAVAILABLE") });
    await expect(loadWorkspace("job-1")).rejects.toThrow("UNAVAILABLE");
  });

  it("without a draft the latest submitted report is read, and parts only while in progress", async () => {
    setup({ job: job({ status: "submitted", draftReportRef: null, reportRefs: [{ reportId: "rep-1", reportVersion: 1 }, { reportId: "rep-2", reportVersion: 2 }] }) });
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(called("reports.get")[0]?.input).toStrictEqual({ jobId: "job-1", reportId: "rep-2", reportVersion: 2 });
    expect(called("parts.list")).toHaveLength(0);
    expect(live.parts).toStrictEqual([]);
  });

  it("in progress with no draft still reads the parts", async () => {
    setup({ job: job({ draftReportRef: null, reportRefs: [] }) });
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(called("parts.list")).toHaveLength(1);
    expect(live.parts).toHaveLength(1);
  });

  it("a refused report and a refused parts catalog are kept apart from the page", async () => {
    setup({ report: new dal.CoreError("FORBIDDEN", "errors.assignment_ended"), parts: new dal.CoreError("FORBIDDEN") });
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(live.report).toBeNull();
    expect(live.reportRefused).toBe("errors.assignment_ended");
    expect(live.parts).toStrictEqual([]);
  });

  it("an installation job gets its own checklist, and a job without assignment or photos has no window span", async () => {
    setup({ job: Object.fromEntries(Object.entries(job({ type: "installation", assignment: null })).filter(([key]) => key !== "problemPhotos")) }); // the API omits the photos
    const live = await loadWorkspace("job-1");
    if (live === "not_found" || "kind" in live) throw new Error("expected the workspace");
    expect(live.components).toStrictEqual(componentsFor(["installation"]));
    expect(live.job.problemPhotos).toStrictEqual([]);
    expect(live.known.span).toStrictEqual({});
    expect(Object.keys(live.known.stamp)).toStrictEqual(["2026-11-09T08:00:00Z"]); // only the review, no window
  });
});
