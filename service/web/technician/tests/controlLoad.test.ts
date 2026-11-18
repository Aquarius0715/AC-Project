import { beforeEach, describe, expect, it, vi } from "vitest";

// technician diagnostic control (FR-T10, SCR-T10) through the DAL: the unit that is gone or not started yet (IR169,
// IR76), the job of the URL, the permission control.diagnose, the work window and the open command or run
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
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), corePermissions: vi.fn(),
    coreOp: vi.fn<(operation: string, input: Record<string, unknown>) => Promise<unknown>>(),
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadControl, type ControlLive } from "../app/technician/units/[id]/control/_lib/load";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiCommandRow, ApiRun } from "@ac/web/lib/techControl";

const NOW = new Date("2026-09-14T01:35:00Z");
const unit: ApiUnitDetail = {
  id: "unit-1", installState: "installed", modelId: "model-1", version: 4, spaceId: null, alertPolicyIds: [], effectivePowerState: "on", displayName: "Bedroom AC",
  connection: "online", lastSeenAt: "2026-09-14T01:34:00Z", installedAt: "2025-03-01T00:00:00Z", capabilityVersion: 3, serviceScope: ["indoor"], components: [],
  location: { pathLabels: ["customer-a", "Home A"], address: null, accessInstructions: null },
  capabilities: {
    manufacturer: "AC-Co", model: "v3", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool"],
    fanLevels: ["low"], ventilation: false, ventilationLevels: [],
  },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: "2026-09-14T01:34:30Z" },
  latestMeasurements: [],
};
const assignment = { scheduledStart: "2026-09-14T01:00:00Z", scheduledEnd: "2026-09-14T03:00:00Z", status: "active" };
const job = (a: typeof assignment | null = assignment) => ({ projection: "detail", id: "job-1", version: 7, status: "in_progress", assignment: a });
const run = (state: ApiRun["state"]): ApiRun => ({
  state, id: "run-1", version: 1, jobId: "job-1", unitId: "unit-1", startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false },
  durationMinutes: 5, reason: "Check", startCommandId: "cmd-1", endCommandId: null, startedAt: "2026-09-14T01:30:00Z", endAt: null, failureCode: null, createdAt: "2026-09-14T01:30:00Z",
});
const command = (status: ApiCommandRow["status"]): ApiCommandRow => ({
  status, id: "cmd-1", action: { kind: "set_power", power: true }, requestedAt: "2026-09-14T01:30:00Z", acknowledgedAt: null, failureCode: null, reason: "Check", diagnosticRunId: null, source: "technician",
});

/** units.get `get` (the unit), jobs.get `job`, the two lists `runs` and `cmds` (empty). */
function serve(o: { get?: () => Promise<unknown>; job?: () => Promise<unknown>; runs?: ApiRun[]; cmds?: ApiCommandRow[] } = {}) {
  dal.coreOp.mockImplementation((operation: string) => {
    if (operation === "units.get") return o.get ? o.get() : Promise.resolve(unit);
    if (operation === "jobs.get") return o.job ? o.job() : Promise.resolve(job());
    if (operation === "diagnosticRuns.list") return Promise.resolve({ items: o.runs ?? [] });
    if (operation === "commands.list") return Promise.resolve({ items: o.cmds ?? [] });
    return Promise.reject(new Error(`unexpected ${operation}`));
  });
}
const refuse = (code: string, options: { messageKey?: string; fieldErrors?: Record<string, string> } = {}) => () => Promise.reject(new dal.CoreError(code, options));
const ops = () => dal.coreOp.mock.calls.map(([op]) => op);
const live = async (jobId: string | null): Promise<ControlLive> => {
  const page = await loadControl("unit-1", jobId);
  if (page?.kind !== "live") throw new Error(`not live: ${JSON.stringify(page)}`);
  return page;
};

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePermissions.mockResolvedValue(new Set(["control.diagnose"]));
});

describe("loadControl: the unit", () => {
  it.each([
    ["NOT_FOUND", {}], ["FORBIDDEN", {}], ["VALIDATION", { fieldErrors: { id: "invalid" } }], ["VALIDATION", { fieldErrors: { jobId: "invalid" } }],
  ])("a unit refused with %s %j is not found", async (code, options) => {
    serve({ get: refuse(code, options) });
    await expect(loadControl("unit-1", "job-1")).resolves.toBeNull();
  });

  it("any other failure reaches the error boundary", async () => {
    serve({ get: refuse("UNAVAILABLE") });
    await expect(loadControl("unit-1", null)).rejects.toThrow("UNAVAILABLE");
    serve({ get: refuse("VALIDATION", { fieldErrors: { limit: "too_large" } }) });
    await expect(loadControl("unit-1", null)).rejects.toThrow("VALIDATION");
  });

  it("before the work window it says when control opens, from the URL's job", async () => {
    serve({ get: refuse("FORBIDDEN", { messageKey: "errors.assignment_not_started" }) });
    const page = await loadControl("unit-1", "job-1");
    expect(page).toMatchObject({ kind: "not_started", title: "Not started yet", unitHref: "/technician/units/unit-1?jobId=job-1" });
    expect(page?.kind === "not_started" ? page.text : "").toBe("Diagnostic control opens at 14 Sept 2026, 9:00 am MYT, when your work window starts.");
  });

  it.each([
    ["no job in the URL", null, () => Promise.resolve(job())],
    ["a refused job", "job-1", refuse("FORBIDDEN")],
    ["a summary of the job", "job-1", () => Promise.resolve({ projection: "summary", id: "job-1" })],
    ["a job without an assignment", "job-1", () => Promise.resolve(job(null))],
  ])("before the work window with %s it gives the general sentence", async (_name, jobId, read) => {
    serve({ get: refuse("FORBIDDEN", { messageKey: "errors.assignment_not_started" }), job: read });
    const page = await loadControl("unit-1", jobId);
    expect(page).toMatchObject({ kind: "not_started", text: "Available from the work start time of your assigned job.", unitHref: jobId ? "/technician/units/unit-1?jobId=job-1" : "/technician/units/unit-1" });
    expect(ops().includes("jobs.get")).toBe(jobId !== null);
  });
});

describe("loadControl: the job", () => {
  it("without a job in the URL there is nothing to control, and no job reads", async () => {
    serve();
    const page = await live(null);
    expect(page).toMatchObject({ canDiagnose: true, job: null, run: null, banner: null, active: false, history: [], unitHref: "/technician/units/unit-1" });
    expect(page.unit).toMatchObject({ id: "unit-1", name: "Bedroom AC", version: 4, connection: "online", pending: 0 });
    expect(ops()).toStrictEqual(["units.get"]);
  });

  it("without control.diagnose the page says so instead of failing on the job's lists", async () => { // R1028
    dal.corePermissions.mockResolvedValue(new Set(["alert.read"]));
    dal.coreOp.mockImplementation((operation: string) => { // the job reads; its lists need control.diagnose
      if (operation === "units.get") return Promise.resolve(unit);
      if (operation === "jobs.get") return Promise.resolve(job());
      return Promise.reject(new dal.CoreError("FORBIDDEN"));
    });
    const page = await live("job-1");
    expect(page).toMatchObject({ canDiagnose: false, job: null, history: [] });
    expect(ops()).toStrictEqual(["units.get"]);
  });

  it.each([
    ["refused", refuse("FORBIDDEN")], ["not found", refuse("NOT_FOUND")], ["an invalid id", refuse("VALIDATION", { fieldErrors: { jobId: "invalid" } })],
    ["a history snapshot", () => Promise.resolve({ projection: "summary", id: "job-1" })],
  ])("a job that is %s is not the technician's to control", async (_name, read) => {
    serve({ job: read });
    await expect(loadControl("unit-1", "job-1")).resolves.toBeNull();
    expect(ops()).not.toContain("commands.list");
  });

  it("a job read that fails otherwise reaches the error boundary", async () => {
    serve({ job: refuse("UNAVAILABLE") });
    await expect(loadControl("unit-1", "job-1")).rejects.toThrow("UNAVAILABLE");
  });

  it("inside the work window the job can be controlled; the lists are the job's", async () => {
    serve();
    const page = await live("job-1");
    expect(page.job).toMatchObject({ id: "job-1", version: 7, status: "in_progress", inWindow: true });
    expect(page.job?.window).toBe("14 Sept, 9:00 – 11:00 am MYT");
    expect(page).toMatchObject({ run: null, banner: null, active: false, history: [] });
    expect(dal.coreOp).toHaveBeenCalledWith("diagnosticRuns.list", { unitId: "unit-1", jobId: "job-1", query: { limit: 10 } });
    expect(dal.coreOp).toHaveBeenCalledWith("commands.list", { unitId: "unit-1", jobId: "job-1", query: { limit: 20 } });
  });

  it.each([
    ["the assignment is not active", { ...assignment, status: "released" }],
    ["the window has not started", { ...assignment, scheduledStart: "2026-09-14T01:36:00Z" }],
    ["the window ends now", { ...assignment, scheduledEnd: NOW.toISOString() }],
  ])("outside the work window when %s", async (_name, a) => {
    serve({ job: () => Promise.resolve(job(a)) });
    const page = await live("job-1");
    expect(page.job?.inWindow).toBe(false);
  });

  it("the window starting now is inside it", async () => {
    serve({ job: () => Promise.resolve(job({ ...assignment, scheduledStart: NOW.toISOString() })) });
    const page = await live("job-1");
    expect(page.job?.inWindow).toBe(true);
  });

  it("a job without an assignment says so and is outside the window", async () => {
    serve({ job: () => Promise.resolve(job(null)) });
    const page = await live("job-1");
    expect(page.job).toMatchObject({ window: "no assignment", inWindow: false });
  });

  it.each([
    ["requested", true], ["sent", true], ["acknowledged", false], ["failed", false],
  ] as const)("a %s command keeps the page busy: %s", async (status, active) => {
    serve({ cmds: [command(status)] });
    const page = await live("job-1");
    expect(page.active).toBe(active);
    expect(page.history).toHaveLength(1);
  });

  it("the newest run gives the banner and keeps the page busy while it runs", async () => {
    serve({ runs: [run("running"), run("completed")] });
    const page = await live("job-1");
    expect(page.run?.state).toBe("running");
    expect(page.banner).not.toBeNull();
    expect(page.active).toBe(true);
  });

  it("a finished run leaves the page idle", async () => {
    serve({ runs: [run("completed")] });
    const page = await live("job-1");
    expect(page.active).toBe(false);
  });
});
