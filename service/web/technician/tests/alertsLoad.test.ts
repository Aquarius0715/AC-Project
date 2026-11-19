import { beforeEach, describe, expect, it, vi } from "vitest";

// the technician's alert evidence (FR-T07, SCR-T07) through the DAL: the unit's alerts refused (IR76, the work window),
// the alert of the URL, the unit and job reads that may be refused, and the evidence only an unresolved alert with
// alert.resolve may cite (IR327)
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string; messageKey: string; fieldErrors: Record<string, string> };
    readonly status: number;
    constructor(code: string, options: ErrorOptions & { messageKey?: string; status: number }) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: options.messageKey ?? "", fieldErrors: {} };
      this.status = options.status;
    }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), corePermissions: vi.fn(),
    coreOp: vi.fn<(operation: string, input: Record<string, unknown>) => Promise<unknown>>(),
    coreAll: vi.fn<(operation: string, input: { filters: { status: string } }) => Promise<unknown[]>>(),
    coreItems: vi.fn<(operation: string, input: Record<string, unknown>) => Promise<unknown[]>>(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => { // as the DAL: UNAVAILABLE and TIMEOUT still fail
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

import { loadAlerts, type AlertsLive } from "../app/technician/units/[id]/alerts/_lib/load";
import type { ApiTechAlert } from "@ac/web/lib/techAlerts";

const NOW = new Date("2026-09-14T01:35:00Z");
const UNIT = "0123456789-unit";
const alert = (id: string, status: ApiTechAlert["status"], detectedAt = "2026-09-14T01:12:00Z"): ApiTechAlert => ({
  id, status, detectedAt, version: 1, unitId: UNIT, type: "sensor", severity: "warning", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Hot.",
  observedAt: detectedAt, acknowledgedAt: null, resolvedAt: status === "resolved" ? "2026-09-14T01:20:00Z" : null, resolutionReason: null, previousAlertId: null, policyId: null,
});
const candidate = { id: "ev-1", kind: "remeasurement", observedAt: "2026-09-14T01:30:00Z", metric: "temperature", value: 24, unit: "°C", origin: "measured", quality: "valid", eventType: null };
const refusal = (code: string, status: number, messageKey?: string) => new dal.CoreError(code, { status, messageKey });

/** alerts.list gives `open` and `resolved`; coreOp answers units.get, jobs.get and alerts.evidence (or refuses them). */
function serve(o: { open?: ApiTechAlert[]; resolved?: ApiTechAlert[]; list?: Error; unit?: Error; job?: () => Promise<unknown>; evidence?: Error } = {}) {
  dal.coreAll.mockImplementation((_operation: string, input: { filters: { status: string } }) => {
    if (o.list) return Promise.reject(o.list);
    return Promise.resolve(input.filters.status === "open" ? (o.open ?? []) : []);
  });
  dal.coreItems.mockImplementation(() => (o.list ? Promise.reject(o.list) : Promise.resolve(o.resolved ?? [])));
  dal.coreOp.mockImplementation((operation: string) => {
    if (operation === "units.get") return o.unit ? Promise.reject(o.unit) : Promise.resolve({ id: UNIT, displayName: "Bedroom AC", latestMeasurements: [], location: { pathLabels: ["customer-a", "Home A"], address: null, accessInstructions: null } });
    if (operation === "jobs.get") return o.job ? o.job() : Promise.resolve({ projection: "detail", id: "job-1", type: "repair", assignment: null });
    if (operation === "alerts.evidence") return o.evidence ? Promise.reject(o.evidence) : Promise.resolve({ items: [candidate] });
    return Promise.reject(new Error(`unexpected ${operation}`));
  });
}
const ops = () => dal.coreOp.mock.calls.map(([op]) => op);
const live = async (sp: Parameters<typeof loadAlerts>[1] = {}): Promise<AlertsLive> => {
  const page = await loadAlerts(UNIT, sp);
  if (page.kind !== "live") throw new Error(`not live: ${JSON.stringify(page)}`);
  return page;
};

beforeEach(() => {
  vi.clearAllMocks();
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePermissions.mockResolvedValue(new Set(["alert.read", "alert.resolve"]));
});

describe("loadAlerts: refused alerts", () => {
  it.each([
    ["errors.assignment_ended", "Your work window on this unit has ended."],
    ["errors.assignment_required", "You need an active assignment on this unit to see its alerts."],
    ["error.notFound", "This AC is not in your assignments."],
    ["errors.other", "FORBIDDEN"],
  ])("a refusal %s says why", async (messageKey, text) => {
    serve({ list: refusal("FORBIDDEN", 403, messageKey) });
    await expect(loadAlerts(UNIT, {})).resolves.toStrictEqual({ kind: "unavailable", title: "Alerts aren’t available", back: "Unit", unitHref: `/technician/units/${UNIT}`, text });
  });

  it("a server failure reaches the error boundary", async () => {
    serve({ list: refusal("INTERNAL", 500) });
    await expect(loadAlerts(UNIT, {})).rejects.toThrow("INTERNAL");
  });

  it("before the work window it says when the AC can be seen, from the URL's job", async () => {
    serve({ list: refusal("FORBIDDEN", 403, "errors.assignment_not_started"), job: () => Promise.resolve({ projection: "detail", assignment: { scheduledStart: "2026-09-14T01:00:00Z" } }) });
    await expect(loadAlerts(UNIT, { jobId: "job-1" })).resolves.toMatchObject({
      kind: "unavailable", title: "Not started yet", text: "You can see this AC from 14 Sept 2026, 9:00 am MYT, when your work window starts.", unitHref: `/technician/units/${UNIT}?jobId=job-1`,
    });
  });

  it.each([
    ["no job in the URL", undefined, undefined],
    ["a refused job", "job-1", () => Promise.reject(refusal("FORBIDDEN", 403))],
    ["a summary of the job", "job-1", () => Promise.resolve({ projection: "summary" })],
    ["a job without an assignment", "job-1", () => Promise.resolve({ projection: "detail", assignment: null })],
  ])("before the work window with %s it gives the general sentence", async (_name, jobId, job) => {
    serve({ list: refusal("FORBIDDEN", 403, "errors.assignment_not_started"), job });
    await expect(loadAlerts(UNIT, { jobId })).resolves.toMatchObject({ kind: "unavailable", text: "Available from the work start time of your assigned job." });
    expect(ops().includes("jobs.get")).toBe(jobId !== undefined);
  });
});

describe("loadAlerts: the unit's alerts", () => {
  it("an unresolved alert with alert.resolve gets its evidence and the URL's job", async () => {
    serve({ open: [alert("a-1", "open")], resolved: [alert("a-0", "resolved", "2026-09-13T01:00:00Z")] });
    const page = await live({ jobId: "job-1" });
    expect(page).toMatchObject({ unitName: "Bedroom AC", count: 2, unitId: UNIT, unitHref: `/technician/units/${UNIT}?jobId=job-1` });
    expect(page.choices).toHaveLength(2);
    expect(page.alert).not.toBeNull();
    expect(page.evidence).toHaveLength(1);
    expect(page.evidence?.[0]?.id).toBe("ev-1");
    expect(dal.coreOp).toHaveBeenCalledWith("alerts.evidence", { alertId: "a-1", query: { limit: 100 } });
    expect(dal.coreOp).toHaveBeenCalledWith("jobs.get", { jobId: "job-1" });
  });

  it("the URL's alertId picks a resolved alert, which has no evidence to cite", async () => {
    serve({ open: [alert("a-1", "open")], resolved: [alert("a-0", "resolved")] });
    const page = await live({ alertId: "a-0" });
    expect(page.alert).not.toBeNull();
    expect(page.evidence).toBeNull();
    expect(ops()).not.toContain("alerts.evidence");
    expect(ops()).not.toContain("jobs.get");
  });

  it("without alert.resolve there is no evidence read", async () => {
    dal.corePermissions.mockResolvedValue(new Set(["alert.read"]));
    serve({ open: [alert("a-1", "open")] });
    const page = await live();
    expect(page.evidence).toBeNull();
    expect(ops()).not.toContain("alerts.evidence");
  });

  it("refused unit, job and evidence reads leave the page with the unit id and no evidence", async () => {
    serve({ open: [alert("a-1", "open")], unit: refusal("FORBIDDEN", 403), job: () => Promise.reject(refusal("NOT_FOUND", 404)), evidence: refusal("FORBIDDEN", 403) });
    const page = await live({ jobId: "job-1" });
    expect(page.unitName).toBe("01234567");
    expect(page.alert).not.toBeNull();
    expect(page.evidence).toBeNull();
  });

  it("an evidence read that fails otherwise reaches the error boundary", async () => {
    serve({ open: [alert("a-1", "open")], evidence: refusal("UNAVAILABLE", 503) });
    await expect(loadAlerts(UNIT, {})).rejects.toThrow("UNAVAILABLE");
  });

  it("a unit without alerts has nothing selected", async () => {
    serve();
    const page = await live();
    expect(page).toMatchObject({ count: 0, choices: [], alert: null, evidence: null });
  });
});
