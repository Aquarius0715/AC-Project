// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// the job workspace's branches no other test took: a job without its assignment or unit, editing one reading of
// several, a submitted report's parts, photos and a metric the form does not offer, the first save of a new draft, a
// unit without components, a refrigerant line without a method, and the photo buttons while an action runs (review
// round 1087)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const state = vi.hoisted((): { pending: boolean } => ({ pending: false }));
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  saveDraft: vi.fn(() => Promise.resolve({ ok: true, value: { id: "rep-new", version: 1, signOff: null, attachmentRefs: [] } })),
  signOff: vi.fn(), uploadPhoto: vi.fn(), submitReport: vi.fn(), pauseWork: vi.fn(), checkIn: vi.fn(), acknowledge: vi.fn(), resumeRework: vi.fn(),
}));
// the action runner without the transition, the toast and the refresh; `state.pending` is the running action
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [state.pending, (fn: () => Promise<Answer>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }],
}));

import { saveDraft } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

type Measurement = ApiTechReport["measurements"][number];
const reading = (id: string, over: Partial<Measurement> = {}): Measurement => ({
  id, componentKey: "filter", metric: "temperature", value: 12, unit: "°C", observedAt: "2026-11-18T02:30:00Z", quality: "good", ...over,
});
const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  version: 2, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" }, attachmentRefs: [], submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});
// a job in progress on its saved draft v2; `id` keeps one test's open tab from the next, as the view is kept per job
const live = (id: string, rep: Partial<ApiTechReport> | null = {}, job: Partial<WorkspaceLive["job"]> = {}, over: Partial<WorkspaceLive> = {}): WorkspaceLive => ({
  now: "2026-11-18T03:00:00Z", known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id, version: 5, status: "in_progress", type: "maintenance", unitId: "unit-12345678", symptom: "", alertCount: 0, origin: "manual",
    assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null },
    timeOnSite: { startedAt: "2026-11-18T02:10:00Z" }, draftReportRef: { reportId: "rep-1", reportVersion: 2 }, reportRefs: [], problemPhotos: [], ...job,
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [{ group: "indoor", key: "filter" }, { group: "indoor", key: "drain_pan" }],
  report: rep ? report(rep) : null, reportRefused: null, parts: [], ...over,
});
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
// the readings table's rows, without its head
const rows = () => screen.getAllByRole("row").slice(1);
const select = (row: HTMLElement, name: string) => within(row).getByLabelText<HTMLSelectElement>(name).value;
const lastSave = () => vi.mocked(saveDraft).mock.lastCall;

beforeEach(() => { state.pending = false; vi.mocked(saveDraft).mockClear(); });
afterEach(cleanup);

describe("WorkspaceView without its assignment or unit", () => {
  it("shows no window, the unit's short id and the job's own status", () => {
    render(<WorkspaceView live={live("job-none", null, { status: "accepted", assignment: null, draftReportRef: null, timeOnSite: null }, { unitName: null })} />);
    expect(document.getElementById("job-title")?.textContent).toBe("job-none · Unit — accepted"); // a status the page has no text for
    expect(screen.getByText("Window").parentElement?.textContent).toBe("Window—");
    expect(screen.getByText("unit-123")).toBeDefined(); // neither the unit nor its name is readable
    expect(screen.queryByRole("button", { name: "Accept assignment" })).toBeNull();
    expect(screen.getByText("The report opens when you check in at the site.")).toBeDefined();
  });
});

describe("WorkspaceView readings", () => {
  it("edits one reading of several and leaves the others as they were", () => {
    render(<WorkspaceView live={live("job-readings", { measurements: [reading("m-1"), reading("m-2", { componentKey: "drain_pan", value: 3 })] })} />);
    const [first, second] = rows();
    if (!first || !second) throw new Error("two readings expected");
    fireEvent.change(within(second).getByLabelText("Metric"), { target: { value: "humidity" } });
    fireEvent.change(within(second).getByRole("textbox"), { target: { value: "55" } });
    fireEvent.change(within(second).getByLabelText("Linked item"), { target: { value: "filter" } });
    expect([select(second, "Metric"), within(second).getByRole<HTMLInputElement>("textbox").value, select(second, "Linked item")]).toStrictEqual(["humidity", "55", "filter"]);
    expect([select(first, "Metric"), within(first).getByRole<HTMLInputElement>("textbox").value, select(first, "Linked item")]).toStrictEqual(["temperature", "12", "filter"]);
  });

  it("a submitted report shows a metric the form does not offer by its id, its parts without Remove and its photos but not the signature", () => {
    const photo = (id: string, name: string) => ({ id, name, mime: "image/png", size: 10, status: "ready" as const, tag: null });
    const part = { name: "Filter set", quantity: 1, catalogCode: null, source: "van_stock" as const, lotSerial: null, replacesComponentKey: null, oldPartDisposal: null, receiptAttachmentId: null };
    render(<WorkspaceView live={live("job-sent", {
      items: [{ id: "i-1", componentGroup: "indoor", componentKey: "filter", result: "normal", reason: null, evidenceIds: [], authorId: "tech-1", observedAt: "2026-11-18T02:30:00Z" }],
      measurements: [reading("m-1", { metric: "heartbeat_gap", value: 4 })], parts: [part],
      attachmentRefs: [photo("ph-1", "coil.png"), photo("sig-1", "signature.png")],
      signOff: { signerName: "Ms Tan", signedAt: "2026-11-18T03:00:00Z", signatureAttachmentId: "sig-1", absentReason: null, reportVersion: 2 },
    }, { status: "submitted", draftReportRef: null, reportRefs: [{ reportId: "rep-1", reportVersion: 2 }] })} />);
    expect(screen.getByRole("img", { name: "coil.png" }).getAttribute("src")).toBe("/technician/jobs/job-sent/photos/ph-1?reportId=rep-1&reportVersion=2");
    expect(screen.queryByRole("img", { name: "signature.png" })).toBeNull();
    const [row] = rows();
    if (!row) throw new Error("one reading expected");
    expect(within(row).getAllByRole("cell")[0]?.textContent).toBe("heartbeat_gap"); // the readings table
    expect(screen.getByText(/^heartbeat_gap · 4/)).toBeDefined(); // and the finding's own reading
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    expect(screen.getByText("Filter set")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
  });
});

describe("WorkspaceView saving", () => {
  it("saves a new draft without a report id or a version", async () => {
    render(<WorkspaceView live={live("job-new", null, { draftReportRef: null })} />);
    fireEvent.change(screen.getByRole("textbox", { name: /^Work performed/ }), { target: { value: "Started the service" } });
    fireEvent.click(button("Save draft"));
    await waitFor(() => { expect(lastSave()).toBeDefined(); });
    const [input, version] = lastSave() ?? [];
    expect(version).toBeNull();
    expect(input && "reportId" in input).toBe(false);
    expect(input?.workText).toBe("Started the service");
    await waitFor(() => { expect(screen.getByText("Work report — draft v1")).toBeDefined(); });
  });

  it("a unit without components: the reading goes on the filter and Submit stays on the page with the report's issue", async () => {
    render(<WorkspaceView live={live("job-empty", { workText: "short" }, {}, { components: [] })} />);
    expect(screen.getByText("No components")).toBeDefined();
    fireEvent.click(button(/Add reading/));
    fireEvent.click(button("Save draft"));
    await waitFor(() => { expect(lastSave()).toBeDefined(); });
    expect(lastSave()?.[0].measurements.map((m) => m.componentKey)).toStrictEqual(["filter"]);
    fireEvent.click(button("Submit report"));
    expect(screen.getByText("Not submitted — 1 issue:")).toBeDefined();
    expect(screen.getByText(/Work performed must be 10–4000 characters/)).toBeDefined();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("sends a refrigerant line whose method was cleared without a method", async () => {
    render(<WorkspaceView live={live("job-gas")} />);
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    fireEvent.click(button("Refrigerant"));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByPlaceholderText("CYL-R32-0182"), { target: { value: "CYL-2" } });
    fireEvent.change(within(dialog).getByLabelText("Method"), { target: { value: "  " } });
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText("passed")).toBeDefined(); // the leak check alone
    fireEvent.click(button("Save draft"));
    await waitFor(() => { expect(lastSave()).toBeDefined(); });
    expect(lastSave()?.[0].refrigerant[0]?.leakCheckMethod).toBeNull();
  });
});

describe("WorkspaceView while an action runs", () => {
  it("dims both photo pickers", () => {
    state.pending = true;
    render(<WorkspaceView live={live("job-busy", {
      items: [{ id: "i-1", componentGroup: "indoor", componentKey: "filter", result: "attention", reason: "Dusty", evidenceIds: [], authorId: "tech-1", observedAt: "2026-11-18T02:30:00Z" }],
    })} />);
    expect(screen.getByLabelText("Add photos").closest("label")?.className).toContain("pointer-events-none");
    expect(screen.getByLabelText("Add a photo to Filter").closest("label")?.className).toContain("pointer-events-none");
    expect(screen.getByLabelText<HTMLInputElement>("Add a photo to Filter").disabled).toBe(true);
  });
});
