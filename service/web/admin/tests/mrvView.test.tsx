// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { mrvView, reportRows, reviewItems, type ApiEvidencePage, type ApiFactor, type ApiMRVReport } from "@ac/web/lib/mrv";
import { baselineRows, type ApiEnergySummary } from "@ac/web/lib/energy";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The MRV demo workspace's screens (FR-A14): in API mode the filters, the report and its version go to the URL; a draft
// is previewed before it is saved; only the latest complete version can be reviewed; factors are saved as new versions;
// evidence is picked from the work-report photos of the draft's units and period. The Phase 1A demo moves locally.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { previewReport: vi.fn<Action>(), saveReportDraft: vi.fn<Action>(), recordReview: vi.fn<Action>(), deleteReport: vi.fn<Action>(), deleteFactor: vi.fn<Action>(), saveFactor: vi.fn<Action>() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
const op = vi.hoisted(() => {
  interface Op { data: ApiEvidencePage; loading: boolean; error: Error | null; calls: { input: unknown; enabled: boolean }[] }
  const state: Op = { data: { items: [], total: 0 }, loading: false, error: null, calls: [] };
  return state;
});
vi.mock("../app/admin/mrv/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/lib/useOp", () => ({
  useOp: (_operation: string, input: unknown, mock: ApiEvidencePage, map: (data: ApiEvidencePage) => ApiEvidencePage, enabled: boolean) => {
    op.calls.push({ input, enabled });
    return enabled ? { data: map(op.data), loading: op.loading, error: op.error, source: "api" } : { data: mock, loading: false, error: null, source: null };
  },
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange }: Readonly<{ value: string; onChange: (v: string) => void }>) => <input aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { MRVView } from "../app/admin/mrv/_components/mrv-view";
import { MRVDemo } from "../app/admin/mrv/_components/mrv-demo";

type Live = ComponentProps<typeof MRVView>["live"];
const summary: ApiEnergySummary = {
  period: { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z" }, unitIds: ["u-1"],
  totals: { kWh: 120, amountMinor: 6000, savedKWh: 10, deltaKWh: -10, savingPercentage: 7.7, savedAmountMinor: 500, emissionsKg: 70.8, savedEmissionsKg: 5.9 },
  currency: "MYR", baselineRef: null, factorRef: { id: "f-1", version: 1 }, baselineSnapshot: null,
  factorSnapshot: { id: "f-1", version: 1, region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo (fictional)" },
  tariffVersion: "tariff-demo-1", boundaryId: "ac_input_electricity", boundary: "AC input", coverage: 1, qualityWarnings: ["modeled_baseline"],
};
const conditions = { from: "2026-08-31T16:00:00Z", to: "2026-09-07T16:00:00Z", unitIds: ["u-1"], baselineId: "b-1", baselineVersion: 1, factorId: "f-1", factorVersion: 1, boundaryId: "ac_input_electricity" as const, boundary: "AC input", organizationId: "org-a" };
/** A version of report r-1234567. */
const report = (version: number, over: Partial<ApiMRVReport> = {}): ApiMRVReport => ({
  id: "r-1234567", version, createdAt: "2026-09-08T01:00:00Z", status: "draft", evidenceIds: [], scope: "scope_2", isDemo: true, incomplete: false, conditions, reviewHistory: [], summary, ...over,
});
const orgs = [{ id: "org-b", name: "Beta Hotel" }, { id: "org-a", name: "Demo Customer A" }];
const units = [{ id: "u-1", label: "Lobby AC", organizationId: "org-a" }, { id: "u-2", label: "Office AC", organizationId: "org-a" }, { id: "u-9", label: "Hotel AC", organizationId: "org-b" }];
const factor = (over: Partial<ApiFactor> = {}): ApiFactor => ({ id: "f-1", version: 1, createdAt: "2026-09-01T00:00:00Z", region: "Demo region", year: 2026, kgCO2ePerKWh: 0.59, source: "Demo (fictional)", isDemo: true, ...over });
const baselines = baselineRows([{
  id: "b-1", version: 1, createdAt: "2026-08-01T00:00:00Z", unitIds: ["u-1"], period: { from: "2026-08-24T16:00:00Z", to: "2026-08-31T16:00:00Z" }, method: "demo_fixed", baselineKWh: 100,
  quality: { kind: "modeled", coverage: null }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "Same week", source: "demo",
}]);
function liveOf(shown: ApiMRVReport | null, over: Partial<Live> = {}, latest = shown?.version ?? 1): Live {
  const rows = shown ? reportRows([shown], orgs) : [];
  return {
    tab: "reports", canWrite: true, canReview: true, canFactors: true, orgs, units, baselines, factors: [factor()],
    reports: {
      rows, scope: { from: "", to: "" },
      ...(shown && { selected: { report: shown, latestVersion: latest, versions: Array.from({ length: latest }, (_, n) => ({ version: n + 1, status: n + 1 === latest ? shown.status : "draft" })), view: mrvView(shown, orgs, units), review: reviewItems(shown) } }),
    },
    ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const flush = () => new Promise((r) => { setTimeout(r, 0); });

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  op.data = { items: [], total: 0 }; op.loading = false; op.error = null; op.calls = [];
  for (const fn of Object.values(actions)) fn.mockReset();
  history.replaceState(null, "", "/admin/mrv"); // the demo keeps its tab in the URL
});
afterEach(cleanup);

describe("MRV reports in API mode", () => {
  it("puts the filters and the selection in the URL, and says whether an empty list is filtered", () => {
    const { rerender } = render(<MRVView live={liveOf(report(2))} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Customer").options].map((o) => o.textContent)).toStrictEqual(["All customers", "Beta Hotel", "Demo Customer A"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "org-a" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ organizationId: "org-a", unitId: null, reportId: null, reportVersion: null });
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ organizationId: null, unitId: null, reportId: null, reportVersion: null });
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "u-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: "u-1", reportId: null, reportVersion: null });
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, reportId: null, reportVersion: null });
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "draft" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ status: "draft", reportId: null, reportVersion: null });
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ status: null, reportId: null, reportVersion: null });
    const apply = screen.getByRole<HTMLButtonElement>("button", { name: "Apply" });
    expect(apply.disabled).toBe(true);
    const [from, to] = screen.getAllByLabelText("date and time");
    if (!from || !to) throw new Error("no period pickers");
    fireEvent.change(from, { target: { value: "2026-09-01T00:00" } });
    fireEvent.change(to, { target: { value: "2026-09-30T00:00" } });
    fireEvent.click(apply);
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-01T00:00", to: "2026-09-30T00:00", reportId: null, reportVersion: null });
    fireEvent.change(from, { target: { value: "" } });
    fireEvent.change(to, { target: { value: "" } });
    fireEvent.change(to, { target: { value: "2026-09-30T00:00" } });
    fireEvent.click(apply);
    expect(nav.patch).toHaveBeenLastCalledWith({ from: null, to: "2026-09-30T00:00", reportId: null, reportVersion: null });
    fireEvent.change(from, { target: { value: "2026-09-02T00:00" } });
    fireEvent.change(to, { target: { value: "" } });
    fireEvent.click(apply);
    expect(nav.patch).toHaveBeenLastCalledWith({ from: "2026-09-02T00:00", to: null, reportId: null, reportVersion: null });
    fireEvent.click(button(/r-123456 · v2/));
    expect(nav.patch).toHaveBeenLastCalledWith({ reportId: "r-1234567", reportVersion: null });
    fireEvent.click(screen.getByRole("tab", { name: /Emission factors/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "factors", reportId: null, reportVersion: null });
    rerender(<MRVView live={liveOf(null, { reports: { rows: [], scope: { status: "draft", from: "", to: "" } } })} />);
    expect(screen.getByText("No report matches these filters.")).toBeDefined();
    rerender(<MRVView live={liveOf(null)} />);
    expect(screen.getByText("No reports yet")).toBeDefined();
    expect(screen.getByText("Nothing selected")).toBeDefined();
    rerender(<MRVView live={liveOf(report(1, { incomplete: true }), { reports: { rows: reportRows([report(1, { incomplete: true })], orgs), scope: { organizationId: "org-a", from: "", to: "" } } })} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Unit").options].map((o) => o.textContent)).toStrictEqual(["All units", "Lobby AC", "Office AC"]); // the customer's units
  });

  it("switches versions in the URL; an earlier version is read only and cannot be reviewed", () => {
    const { rerender } = render(<MRVView live={liveOf(report(3))} />);
    fireEvent.change(screen.getByLabelText("Version"), { target: { value: "2" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ reportVersion: "2" });
    fireEvent.change(screen.getByLabelText("Version"), { target: { value: "3" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ reportVersion: null }); // the latest is no key
    rerender(<MRVView live={liveOf(report(2), {}, 3)} />);
    expect(screen.getByText("An earlier version — read only. Its conditions and results never change.")).toBeDefined();
    expect(screen.getByText("Only the latest version can be reviewed.")).toBeDefined();
    for (const name of ["New version…", "Delete report…", "Submit demo review"]) expect(screen.queryByRole("button", { name })).toBeNull();
  });

  it("records a demo review of the latest complete version with a trimmed comment", async () => {
    actions.recordReview.mockResolvedValue({ ok: true, value: 4 });
    render(<MRVView live={liveOf(report(3))} />);
    const submit = screen.getByRole<HTMLButtonElement>("button", { name: "Submit demo review" });
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Review comment"), { target: { value: "  Looks right  " } });
    fireEvent.click(submit);
    await flush();
    expect(actions.recordReview).toHaveBeenCalledWith("r-1234567", 3, "Looks right");
    expect(nav.toasts).toStrictEqual(["Demo review recorded — version 4"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ reportVersion: null });
  });

  it("offers no review of a reviewed, an incomplete, or a not-permitted version", () => {
    const reviewed = report(2, { status: "demo_reviewed", reviewHistory: [{ userId: "u-hq", reportVersion: 2, comment: "Fine", at: "2026-09-14T01:30:00Z" }] });
    const { rerender } = render(<MRVView live={liveOf(reviewed)} />);
    expect(screen.getByText("This version is demo reviewed. Sending the same review again returns the existing result.")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Delete report…" })).toBeNull(); // a reviewed version cannot be deleted
    rerender(<MRVView live={liveOf(report(1, { incomplete: true }))} />);
    expect(screen.getByText("An incomplete calculation cannot be reviewed.")).toBeDefined();
    expect(screen.getByText("Calculation incomplete — a factor, the boundary or full coverage is missing, so nothing is shown as zero.")).toBeDefined();
    rerender(<MRVView live={liveOf(report(1), { canReview: false, canWrite: false })} />);
    for (const name of ["Submit demo review", "New version…", /New report/]) expect(screen.queryByRole("button", { name })).toBeNull();
  });

  it("deletes an all-draft report with a reason and leaves the selection", async () => {
    actions.deleteReport.mockResolvedValue({ ok: true, value: null });
    render(<MRVView live={liveOf(report(2))} />);
    fireEvent.click(button("Delete report…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: "Created by mistake" } });
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteReport).toHaveBeenCalledWith("r-1234567", 2, "Created by mistake");
    expect(nav.patch).toHaveBeenLastCalledWith({ reportId: null, reportVersion: null });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete report…"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("checks a new report, previews it, saves the draft only after the preview, and opens it", async () => {
    actions.previewReport.mockResolvedValue({ ok: true, value: { conditions, summary, incomplete: false, scope: "scope_2", isDemo: true } });
    actions.saveReportDraft.mockResolvedValue({ ok: true, value: { id: "r-new", version: 1 } });
    render(<MRVView live={liveOf(null)} />);
    fireEvent.click(button(/New report/));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Choose the organization first.")).toBeDefined();
    const save = dialog.getByRole<HTMLButtonElement>("button", { name: "Save draft" });
    expect(save.disabled).toBe(true);
    fireEvent.click(dialog.getByRole("button", { name: "Preview" }));
    expect(dialog.getByText("Choose a customer organization")).toBeDefined();
    expect(dialog.getByText("Choose 1–100 units")).toBeDefined();
    expect(actions.previewReport).not.toHaveBeenCalled();
    fireEvent.change(dialog.getByLabelText(/^Customer organization/), { target: { value: "org-a" } });
    expect(dialog.queryByRole("checkbox", { name: "Hotel AC" })).toBeNull();
    fireEvent.click(dialog.getByRole("checkbox", { name: "Lobby AC" }));
    fireEvent.click(dialog.getByRole("checkbox", { name: "Office AC" }));
    fireEvent.click(dialog.getByRole("checkbox", { name: "Office AC" }));
    const [from, to] = dialog.getAllByLabelText("date and time");
    if (!from || !to) throw new Error("no period pickers");
    fireEvent.change(from, { target: { value: "2026-09-01T00:00" } });
    fireEvent.change(to, { target: { value: "2026-09-08T00:00" } });
    fireEvent.change(dialog.getByLabelText(/^Baseline version/), { target: { value: "b-1@1" } });
    fireEvent.change(dialog.getByLabelText(/^Emission factor version/), { target: { value: "f-1@1" } });
    fireEvent.change(dialog.getByLabelText("Boundary"), { target: { value: "whole_building_electricity" } });
    fireEvent.change(dialog.getByLabelText(/^Boundary description/), { target: { value: "Whole building" } });
    fireEvent.click(dialog.getByRole("button", { name: "Preview" }));
    await flush();
    expect(actions.previewReport).toHaveBeenCalledWith({
      organizationId: "org-a", unitIds: ["u-1"], from: "2026-08-31T16:00:00.000Z", to: "2026-09-07T16:00:00.000Z", baselineId: "b-1", baselineVersion: 1, factorId: "f-1", factorVersion: 1,
      boundaryId: "whole_building_electricity", boundary: "Whole building",
    });
    expect(dialog.getByText("Preview · not saved")).toBeDefined();
    fireEvent.click(save);
    await flush();
    expect(actions.saveReportDraft).toHaveBeenCalledWith(expect.objectContaining({ organizationId: "org-a", unitIds: ["u-1"] }), [], undefined, undefined);
    expect(nav.toasts).toStrictEqual(["Preview ready — nothing is saved", "Report draft created"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ reportId: "r-new", reportVersion: null });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("starts a new version from the latest one's conditions and evidence, drops the preview on a change, and saves it with the latest version", async () => {
    actions.previewReport.mockResolvedValue({ ok: true, value: { conditions, summary, incomplete: false, scope: "scope_2", isDemo: true } });
    actions.saveReportDraft.mockResolvedValue({ ok: true, value: { id: "r-1234567", version: 3 } });
    render(<MRVView live={liveOf(report(2, { evidenceIds: ["att-1"] }))} />);
    fireEvent.click(button("New version…"));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByRole("heading", { name: "New report version" })).toBeDefined();
    expect(dialog.getByLabelText<HTMLSelectElement>(/^Customer organization/).value).toBe("org-a");
    fireEvent.click(dialog.getByRole("button", { name: "Preview" }));
    await flush();
    expect(dialog.getByText("Preview · not saved")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Boundary description/), { target: { value: "AC only" } });
    expect(dialog.queryByText("Preview · not saved")).toBeNull(); // a change needs a new preview
    fireEvent.click(dialog.getByRole("button", { name: "Preview" }));
    await flush();
    fireEvent.click(dialog.getByRole("button", { name: "Save draft" }));
    await flush();
    expect(actions.saveReportDraft).toHaveBeenCalledWith(expect.objectContaining({ boundary: "AC only" }), ["att-1"], "r-1234567", 2);
    expect(nav.toasts.at(-1)).toBe("Saved as version 3 (draft)");
  });

  it("closes the draft with Cancel or Escape without saving", () => {
    render(<MRVView live={liveOf(null)} />);
    fireEvent.click(button(/New report/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/New report/));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(actions.saveReportDraft).not.toHaveBeenCalled();
  });
});

const photo = (id: string, name = `${id}.jpg`) => ({ id, name, jobId: "job-1", reportId: "rep-1", mime: "image/jpeg", size: 1000, status: "ready", createdAt: "2026-09-03T02:00:00Z", unitId: "u-1", acceptedAt: "2026-09-03T03:00:00Z" });

describe("MRV evidence", () => {
  it("lists a saved version's evidence by name, and an attachment that is no longer a candidate by its ID", () => {
    op.data = { items: [photo("att-1", "before.jpg")], total: 1 };
    const { rerender } = render(<MRVView live={liveOf(report(1, { evidenceIds: ["att-1", "att-gone"] }))} />);
    expect(screen.getByText("before.jpg")).toBeDefined();
    expect(screen.getByText("att-gone")).toBeDefined();
    expect(op.calls.at(-1)).toStrictEqual({ input: { unitIds: ["u-1"], from: conditions.from, to: conditions.to, query: { limit: 100 } }, enabled: true });
    rerender(<MRVView live={liveOf(report(1))} />);
    expect(screen.getByText("No evidence attached.")).toBeDefined();
    expect(op.calls.at(-1)?.enabled).toBe(false); // nothing to name, nothing read
  });

  it("picks evidence only once the units and period are set, says when more exist, and removes a chip", () => {
    op.data = { items: [photo("att-1", "before.jpg"), photo("att-2", "after.jpg")], total: 140 };
    render(<MRVView live={liveOf(null)} />);
    fireEvent.click(button(/New report/));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Add evidence" }).disabled).toBe(true);
    expect(dialog.getByText("Choose the units and the period first: the candidates are their work-report photos.")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Customer organization/), { target: { value: "org-a" } });
    fireEvent.click(dialog.getByRole("checkbox", { name: "Lobby AC" }));
    const [from, to] = dialog.getAllByLabelText("date and time");
    if (!from || !to) throw new Error("no period pickers");
    fireEvent.change(from, { target: { value: "2026-09-01T00:00" } });
    fireEvent.change(to, { target: { value: "2026-09-08T00:00" } });
    fireEvent.click(dialog.getByRole("button", { name: "Add evidence" }));
    expect(dialog.getByText("The newest 2 of 140 photos are listed: narrow the units or the period to find older ones.")).toBeDefined();
    fireEvent.click(dialog.getByRole("checkbox", { name: /before\.jpg/ }));
    fireEvent.click(dialog.getByRole("checkbox", { name: /after\.jpg/ }));
    fireEvent.click(dialog.getByRole("button", { name: "Remove after.jpg" }));
    expect(dialog.queryByRole("button", { name: "Remove after.jpg" })).toBeNull();
    fireEvent.click(dialog.getByRole("checkbox", { name: /before\.jpg/ }));
    expect(dialog.getByText("None yet.")).toBeDefined();
  });

  it("says when the candidates load, fail or are none, and stops adding at 20", () => {
    const twenty = Array.from({ length: 20 }, (_, n) => `att-${String(n)}`);
    op.data = { items: [...twenty.map((id) => photo(id)), photo("att-x")], total: 21 };
    actions.previewReport.mockResolvedValue({ ok: false, code: "X", messageKey: "x", fieldErrors: {} });
    render(<MRVView live={liveOf(report(1, { evidenceIds: twenty }))} />);
    fireEvent.click(button("New version…"));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.queryByRole("button", { name: "Add evidence" })).toBeNull(); // 20 is the most a draft keeps
    expect(dialog.getAllByRole("button", { name: /^Remove / })).toHaveLength(20);
    cleanup();
    for (const [state, text] of [["loading", "Loading the attachments…"], ["error", "The attachments could not be loaded — try again."], ["none", "No photos of accepted work reports on these units in this period."]] as const) {
      op.data = { items: [], total: 0 }; op.loading = state === "loading"; op.error = state === "error" ? new Error("down") : null;
      render(<MRVView live={liveOf(report(1))} />);
      fireEvent.click(button("New version…"));
      fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Add evidence" }));
      expect(within(screen.getByRole("dialog")).getByText(text)).toBeDefined();
      cleanup();
    }
  });
});

const factorsLive = (factors: ApiFactor[], over: Partial<Live> = {}) => liveOf(null, { tab: "factors", factors, ...over });

describe("MRV emission factors", () => {

  it("saves a changed factor as its next version, and discards the change", async () => {
    actions.saveFactor.mockResolvedValue({ ok: true, value: { id: "f-1", version: 2 } });
    render(<MRVView live={factorsLive([factor(), factor({ id: "f-2", region: "Other region", year: 2025 })])} />);
    const save = screen.getByRole<HTMLButtonElement>("button", { name: "Save as v2" });
    expect(save.disabled).toBe(true);
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^kgCO₂e\/kWh/), { target: { value: "0.61" } });
    expect(screen.getByText("Editing → v2")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.getByLabelText<HTMLInputElement>(/^kgCO₂e\/kWh/).value).toBe("0.59");
    fireEvent.change(screen.getByLabelText(/^kgCO₂e\/kWh/), { target: { value: "0.61" } });
    fireEvent.click(save);
    await flush();
    expect(actions.saveFactor).toHaveBeenCalledWith({ id: "f-1", region: "Demo region", year: 2026, kgCO2ePerKWh: 0.61, source: "Demo (fictional)", isDemo: true }, 1);
    expect(nav.toasts).toStrictEqual(["Saved as factor version 2"]);
    fireEvent.click(button(/Other region · 2025/));
    expect(screen.getByLabelText<HTMLInputElement>(/^Region/).value).toBe("Other region");
  });

  it("checks a new factor before creating it, and cancels back to the first one", async () => {
    actions.saveFactor.mockResolvedValue({ ok: true, value: { id: "f-new", version: 1 } });
    render(<MRVView live={factorsLive([factor()])} />);
    fireEvent.click(button(/New factor/));
    fireEvent.click(button("Create factor"));
    expect(screen.getByText("1–120 characters")).toBeDefined();
    expect(actions.saveFactor).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText(/^Region/), { target: { value: "North" } });
    fireEvent.change(screen.getByLabelText(/^Year/), { target: { value: "1999" } });
    fireEvent.change(screen.getByLabelText(/^kgCO₂e\/kWh/), { target: { value: "0.5" } });
    fireEvent.click(button("Create factor"));
    expect(screen.getByText("A year 2000–2100")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Year/), { target: { value: "2027" } });
    fireEvent.change(screen.getByLabelText(/^Source/), { target: { value: " Demo (fictional, north) " } });
    fireEvent.click(button("Create factor"));
    await flush();
    expect(actions.saveFactor).toHaveBeenCalledWith({ region: "North", year: 2027, kgCO2ePerKWh: 0.5, source: "Demo (fictional, north)", isDemo: true }, undefined);
    expect(nav.toasts).toStrictEqual(["Factor created"]);
    fireEvent.click(button(/New factor/));
    fireEvent.click(button("Cancel"));
    expect(screen.getByLabelText<HTMLInputElement>(/^Region/).value).toBe("Demo region");
  });

  it("deletes a factor with a reason and opens the next one, or the new form when none is left", async () => {
    actions.deleteFactor.mockResolvedValue({ ok: true, value: null });
    const { unmount } = render(<MRVView live={factorsLive([factor(), factor({ id: "f-2", region: "Other region" })])} />);
    fireEvent.click(button("Delete factor…"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(actions.deleteFactor).not.toHaveBeenCalled();
    fireEvent.click(button("Delete factor…"));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: "Unused" } });
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteFactor).toHaveBeenCalledWith("f-1", 1, "Unused");
    expect(screen.getByLabelText<HTMLInputElement>(/^Region/).value).toBe("Other region");
    unmount();
    render(<MRVView live={factorsLive([factor()])} />);
    fireEvent.click(button("Delete factor…"));
    fireEvent.change(within(screen.getByRole("dialog")).getByLabelText(/^Reason/), { target: { value: "Unused" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));
    await flush();
    expect(screen.getByText("Saving creates version 1")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Delete factor…" })).toBeNull(); // the new form has nothing to delete
  });

  it("shows an empty list without writes for a role that may not edit factors", () => {
    render(<MRVView live={factorsLive([], { canFactors: false })} />);
    expect(screen.getByText("No factors")).toBeDefined();
    for (const name of [/New factor/, "Create factor", "Cancel"]) expect(screen.queryByRole("button", { name })).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: /Reports/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null, reportId: null, reportVersion: null });
  });
});

describe("the MRV demo", () => {
  it("switches reports and versions, compares versions, previews a new report and shows the factors", () => {
    render(<MRVDemo />);
    expect(screen.getByText("“Inputs match the pilot agreement; demo only.”")).toBeDefined();
    fireEvent.click(button(/mrv-aug-a/));
    expect(screen.getByText("Calculation incomplete (draft) — missing readings block the totals. Nothing is shown as zero.")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Version"), { target: { value: "1" } });
    expect(screen.getByText(/version 1 · 2026-09-14 10:05/)).toBeDefined();
    fireEvent.click(button(/Compare versions/));
    expect(within(screen.getByRole("dialog")).getByText("factor-demo-2026 v2").className).toContain("font-bold");
    const closers = within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" }); // the corner and the footer
    fireEvent.click(closers.at(-1) ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/New/));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText("Customer"), { target: { value: "org-customer-b" } });
    fireEvent.change(dialog.getByLabelText("Unit(s)"), { target: { value: "unit-2" } });
    const [start, end] = dialog.getAllByLabelText("date and time");
    if (!start || !end) throw new Error("no period pickers");
    fireEvent.change(start, { target: { value: "2026-09-14T07:00" } });
    fireEvent.change(end, { target: { value: "2026-09-14T10:00" } });
    fireEvent.click(dialog.getByRole("button", { name: "Preview" }));
    expect(dialog.getByText(/^Preview: 80.0 kWh/)).toBeDefined();
    fireEvent.click(dialog.getByRole("button", { name: "Create report" }));
    expect(nav.toasts).toStrictEqual(["Report created (demo — unverified)"]);
    fireEvent.click(button(/New/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("tab", { name: /Emission factors/ }));
    expect(screen.getByText("factor-demo-2026")).toBeDefined();
  });

  it("closes its dialogs with Escape", () => {
    render(<MRVDemo />);
    fireEvent.click(button(/mrv-aug-a/));
    fireEvent.click(button(/Compare versions/));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/New/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Preview" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(button(/New/));
    expect(within(screen.getByRole("dialog")).queryByText(/^Preview: /)).toBeNull(); // Escape dropped the preview too
  });
});
