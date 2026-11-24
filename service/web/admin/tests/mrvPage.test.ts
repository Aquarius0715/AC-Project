import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiMRVReport } from "@ac/web/lib/mrv";
import type { ApiEnergySummary } from "@ac/web/lib/energy";

// The MRV demo workspace of HQ (FR-A14, SCR-A14): the Server Actions (preview, drafts with their version, demo reviews,
// deletes, factor versions) and the Server Component (the URL filters, the selected report and version, the tabs).
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return {
    CoreError, apiMode: vi.fn(() => true), coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["mrv.read", "mrv.write"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<{ items: unknown[]; total: number }>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/mrv/_components/mrv-view", () => ({ MRVView: () => null }));
vi.mock("../app/admin/mrv/_components/mrv-demo", () => ({ MRVDemo: () => null }));

import { deleteFactor, deleteReport, previewReport, recordReview, saveFactor, saveReportDraft } from "../app/admin/mrv/actions";
import AdminMRVPage from "../app/admin/mrv/page";
import { MRVView } from "../app/admin/mrv/_components/mrv-view";
import { MRVDemo } from "../app/admin/mrv/_components/mrv-demo";
import MRVLoading from "../app/admin/mrv/loading";
import MRVErrorState from "../app/admin/mrv/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const summary: ApiEnergySummary = {
  period: { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z" }, unitIds: ["u-1"],
  totals: { kWh: 120, amountMinor: 6000, savedKWh: 10, deltaKWh: -10, savingPercentage: 7.7, savedAmountMinor: 500, emissionsKg: 70.8, savedEmissionsKg: 5.9 },
  currency: "MYR", baselineRef: null, factorRef: { id: "f-1", version: 1 }, baselineSnapshot: null,
  factorSnapshot: { id: "f-1", version: 1, region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo (fictional)" },
  tariffVersion: "tariff-demo-1", boundaryId: "ac_input_electricity", boundary: "AC input", coverage: 1, qualityWarnings: [],
};
const conditions = { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z", unitIds: ["u-1"], baselineId: "b-1", baselineVersion: 1, factorId: "f-1", factorVersion: 1, boundaryId: "ac_input_electricity" as const, boundary: "AC input", organizationId: "org-a" };
const report = (id: string, version: number, over: Partial<ApiMRVReport> = {}): ApiMRVReport => ({
  id, version, createdAt: "2026-09-08T01:00:00Z", status: "draft", evidenceIds: [], scope: "scope_2", isDemo: true, incomplete: false, conditions, reviewHistory: [], summary, ...over,
});
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof MRVView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminMRVPage(props(sp));
  if (!isValidElement<ComponentProps<typeof MRVView>>(page) || page.type !== MRVView) throw new TypeError("not MRVView");
  return page.props.live;
}
const callsOf = (op: string) => dal.coreOp.mock.calls.filter(([name]) => name === op).map(([, input]) => input);
/** mrv.list answers `rows`; mrv.get the latest version 3 of a report, or the version asked for. */
function reports(rows: ApiMRVReport[]) {
  dal.coreOp.mockImplementation((op, input) => {
    if (op === "mrv.list") return Promise.resolve({ items: rows });
    if (op === "mrv.versions") return Promise.resolve({ items: [report("r-1", 1, { status: "demo_reviewed" }), report("r-1", 2), report("r-1", 3)] });
    const version = /"reportVersion":(\d+)/.exec(JSON.stringify(input))?.[1];
    return Promise.resolve(report("r-1", Number(version ?? 3)));
  });
}

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.corePermissions.mockResolvedValue(new Set(["mrv.read", "mrv.write"]));
  dal.coreOp.mockReset(); cache.refresh.mockReset();
  const lists = new Map<string, unknown[]>([
    ["organizations.list", [{ id: "org-a", name: "Demo Customer A", kind: "customer", status: "active" }]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", archived: false }, { id: "u-old", displayName: "Old AC", customerOrgId: "org-a", archived: true }]],
    ["baselines.list", []], ["factors.list", [{ id: "f-1", version: 1, createdAt: "2026-09-01T00:00:00Z", region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo", isDemo: true }]],
  ]);
  dal.coreAllPage.mockImplementation((op) => Promise.resolve({ items: lists.get(op) ?? [], total: 0 }));
});

describe("MRV Server Actions", () => {
  it("previews without storing or re-rendering, and saves a new draft or the next version with its version", async () => {
    dal.coreOp.mockResolvedValueOnce({ conditions, summary });
    expect(await previewReport(conditions)).toMatchObject({ ok: true });
    expect(dal.coreOp).toHaveBeenLastCalledWith("mrv.preview", conditions);
    expect(cache.refresh).not.toHaveBeenCalled();
    dal.coreOp.mockResolvedValueOnce({ id: "r-new", version: 1, status: "draft" });
    expect(await saveReportDraft(conditions, ["att-1"])).toStrictEqual({ ok: true, value: { id: "r-new", version: 1 } });
    expect(dal.coreOp.mock.lastCall?.slice(0, 2)).toStrictEqual(["mrv.saveDraft", { conditions, evidenceIds: ["att-1"] }]);
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}'); // a new report has no version to expect
    dal.coreOp.mockResolvedValueOnce({ id: "r-1", version: 4 });
    expect(await saveReportDraft(conditions, [], "r-1", 3)).toStrictEqual({ ok: true, value: { id: "r-1", version: 4 } });
    expect(dal.coreOp).toHaveBeenLastCalledWith("mrv.saveDraft", { conditions, id: "r-1", evidenceIds: [] }, { write: true, expectedVersion: 3 });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("records a demo review of the version, and deletes a report or a factor with a reason", async () => {
    dal.coreOp.mockResolvedValueOnce({ version: 3 });
    expect(await recordReview("r-1", 3, "Looks right")).toStrictEqual({ ok: true, value: 3 });
    expect(dal.coreOp).toHaveBeenLastCalledWith("mrv.recordReview", { reportVersion: 3, reportId: "r-1", reviewComment: "Looks right" }, { write: true, expectedVersion: 3 });
    dal.coreOp.mockResolvedValue({});
    expect(await deleteReport("r-1", 3, "Mistake")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenLastCalledWith("mrv.deleteDraft", { id: "r-1", reason: "Mistake" }, { write: true, expectedVersion: 3 });
    expect(await deleteFactor("f-1", 2, "Unused")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenLastCalledWith("factors.delete", { id: "f-1", reason: "Unused" }, { write: true, expectedVersion: 2 });
  });

  it("saves a new factor or the next version, and returns a refusal with its code", async () => {
    const input = { region: "Demo region", year: 2026, kgCO2ePerKWh: 0.6, source: "Demo", isDemo: true as const };
    dal.coreOp.mockResolvedValueOnce({ id: "f-2", version: 1 });
    expect(await saveFactor(input)).toStrictEqual({ ok: true, value: { id: "f-2", version: 1 } });
    expect(dal.coreOp.mock.lastCall?.slice(0, 2)).toStrictEqual(["factors.save", input]);
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}'); // a new factor has no version to expect
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await saveFactor({ ...input, id: "f-1" }, 1)).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(dal.coreOp).toHaveBeenLastCalledWith("factors.save", { ...input, id: "f-1" }, { write: true, expectedVersion: 1 });
  });
});

describe("the MRV page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminMRVPage(props());
    expect(isValidElement(page) && page.type).toBe(MRVDemo);
    expect(MRVLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = MRVErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("lists reports for the URL filters and opens the first one at its latest version", async () => {
    reports([report("r-1", 3), report("r-2", 1)]);
    const live = await liveOf({ organizationId: "org-a", unitId: "u-1", status: "draft", from: "2026-09-01T00:00", to: "2026-09-30T00:00" });
    expect(callsOf("mrv.list")).toStrictEqual([{ limit: 100, filters: { organizationId: "org-a", unitId: "u-1", status: "draft", from: "2026-08-31T16:00:00.000Z", to: "2026-09-29T16:00:00.000Z" } }]);
    expect([live.tab, live.canWrite, live.canReview, live.canFactors]).toStrictEqual(["reports", true, false, false]);
    expect(live.units).toStrictEqual([{ id: "u-1", label: "Lobby AC", organizationId: "org-a" }]); // archived units are left out
    expect(live.reports?.scope).toStrictEqual({ organizationId: "org-a", unitId: "u-1", from: "2026-09-01T00:00", to: "2026-09-30T00:00", status: "draft" });
    expect([live.reports?.selected?.report.version, live.reports?.selected?.latestVersion, live.reports?.selected?.versions]).toStrictEqual([3, 3, [{ version: 1, status: "demo_reviewed" }, { version: 2, status: "draft" }, { version: 3, status: "draft" }]]);
    expect(callsOf("mrv.get")).toStrictEqual([{ id: "r-1" }]); // the latest only
  });

  it("opens the URL's report at an earlier version, and ignores an unknown status or a version that is not a number", async () => {
    reports([report("r-1", 3)]);
    const live = await liveOf({ reportId: "r-1", reportVersion: "2", status: "archived" });
    expect(callsOf("mrv.list")).toStrictEqual([{ limit: 100, filters: {} }]);
    expect(callsOf("mrv.get")).toStrictEqual([{ id: "r-1" }, { id: "r-1", reportVersion: 2 }]);
    expect([live.reports?.selected?.report.version, live.reports?.selected?.latestVersion, live.reports?.scope.status]).toStrictEqual([2, 3, undefined]);
    dal.coreOp.mockClear();
    const latest = await liveOf({ reportId: "r-1", reportVersion: "x" });
    expect([latest.reports?.selected?.report.version, callsOf("mrv.get")]).toStrictEqual([3, [{ id: "r-1" }]]);
    dal.coreOp.mockClear();
    const same = await liveOf({ reportId: "r-1", reportVersion: "3" });
    expect(callsOf("mrv.get")).toStrictEqual([{ id: "r-1" }]); // the latest asked for by number is read once
    expect(same.reports?.selected?.report.version).toBe(3);
  });

  it("selects nothing without reports, and reads no reports on the factors tab", async () => {
    reports([]);
    const empty = await liveOf({ status: "demo_reviewed" });
    expect([empty.reports?.rows, empty.reports?.selected, empty.reports?.scope.status]).toStrictEqual([[], undefined, "demo_reviewed"]);
    dal.coreOp.mockClear();
    dal.corePermissions.mockResolvedValue(new Set(["mrv.read", "mrv.review", "mrv.factors"]));
    const factors = await liveOf({ tab: "factors" });
    expect([factors.tab, factors.reports, factors.canWrite, factors.canReview, factors.canFactors, factors.factors.length]).toStrictEqual(["factors", undefined, false, true, true, 1]);
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("names mrv.read when the reports are refused (IR582)", async () => {
    dal.coreOp.mockRejectedValue(refused("FORBIDDEN"));
    dal.corePermissions.mockResolvedValue(new Set());
    const page: ReactNode = await AdminMRVPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "MRV", permission: "mrv.read" });
  });
});
