// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// the job workspace's last branches: removing a refrigerant line, closing the check-in dialog, the old part's disposal,
// the customer sign-off through the view (saved first when edited), the autosave 20 s after an edit, and the work
// window's clock that moves on by itself (review round 1079)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const answers = vi.hoisted((): { save: Answer; sign: Answer } => ({ save: { ok: true, value: null }, sign: { ok: true, value: null } }));
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  saveDraft: vi.fn(() => Promise.resolve(answers.save)), signOff: vi.fn(() => Promise.resolve(answers.sign)),
  uploadPhoto: vi.fn(), submitReport: vi.fn(), pauseWork: vi.fn(), checkIn: vi.fn(), acknowledge: vi.fn(), resumeRework: vi.fn(),
}));
// the action runner without the transition, the toast and the refresh: `after` or `failed` runs once the action answers
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }],
}));

import { acknowledge, checkIn, saveDraft, signOff } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  version: 2, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" }, attachmentRefs: [], submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});
// a job in progress on its saved draft v2 (`job` other than in progress for the other states); `id` keeps the open
// view of one test from the next, as the view is kept per job
const live = (id: string, rep: Partial<ApiTechReport> = {}, job: Partial<WorkspaceLive["job"]> = {}, now = "2026-11-18T03:00:00Z"): WorkspaceLive => ({
  now, known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id, version: 5, status: "in_progress", type: "maintenance", unitId: "unit-12345678", symptom: "", alertCount: 0, origin: "manual",
    assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null },
    timeOnSite: { startedAt: "2026-11-18T02:10:00Z" }, draftReportRef: { reportId: "rep-1", reportVersion: 2 }, reportRefs: [], problemPhotos: [], ...job,
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [{ group: "indoor", key: "filter" }, { group: "indoor", key: "drain_pan" }],
  report: report(rep), reportRefused: null, parts: [],
});
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const workText = () => screen.getByRole("textbox", { name: /^Work performed/ });
const saved3 = { id: "rep-1", version: 3, signOff: null, attachmentRefs: [] }; // the draft as the save returns it, v3
// the sign-off of a customer who is not there: the signer, the reason and a site photo
const signAbsent = (dialog: HTMLElement) => {
  fireEvent.change(within(dialog).getByPlaceholderText("Ms Tan"), { target: { value: "Mr Lim" } });
  fireEvent.click(within(dialog).getByText("Customer not available"));
  fireEvent.change(within(dialog).getByRole("textbox", { name: "Why the customer could not sign" }), { target: { value: "Away" } });
  fireEvent.change(within(dialog).getByLabelText("Site photo (JPEG/PNG)"), { target: { files: [new File(["x"], "door.png", { type: "image/png" })] } });
  fireEvent.click(button("Save sign-off", dialog));
};

beforeEach(() => {
  answers.save = { ok: true, value: null }; answers.sign = { ok: true, value: null };
  for (const fn of [saveDraft, signOff, checkIn, acknowledge]) vi.mocked(fn).mockClear();
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("WorkspaceView parts and check-in", () => {
  it("removes a refrigerant line, which is an edit; a submitted report's line has no Remove", () => {
    const line = { refrigerant: "R32" as const, cylinderId: "CYL-1", recoveredKg: 0.5, chargedKg: 0.2, leakCheck: "pass" as const, leakCheckMethod: null };
    render(<WorkspaceView live={live("job-ref", { refrigerant: [line] })} />);
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    expect(screen.getByText("Refrigerant (R32) — cylinder CYL-1")).toBeDefined();
    expect(screen.getByText("−0.30 kg")).toBeDefined(); // more recovered than charged
    expect(screen.getByText("passed")).toBeDefined(); // no method: the leak check alone
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    fireEvent.click(button("Remove"));
    expect(screen.queryByText(/^Refrigerant \(R32\)/)).toBeNull();
    expect(screen.getByText("Unsaved changes")).toBeDefined();
    cleanup();
    render(<WorkspaceView live={live("job-ref-done", { refrigerant: [line] }, { status: "submitted", draftReportRef: null })} />);
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    expect(screen.getByText("Refrigerant (R32) — cylinder CYL-1")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
  });

  it("sends a part that replaces an item with its old part returned", async () => {
    answers.save = { ok: true, value: saved3 };
    render(<WorkspaceView live={live("job-part")} />);
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    fireEvent.click(button(/Add part/));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Part name"), { target: { value: "Drain hose" } });
    fireEvent.change(within(dialog).getByDisplayValue("—"), { target: { value: "drain_pan" } });
    fireEvent.change(within(dialog).getByDisplayValue("Disposed on site"), { target: { value: "returned" } });
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    fireEvent.click(button("Save draft"));
    expect(saveDraft).toHaveBeenCalledOnce();
    const input = vi.mocked(saveDraft).mock.calls[0]?.[0];
    expect(input?.parts.map((p) => [p.name, p.replacesComponentKey, p.oldPartDisposal])).toStrictEqual([["Drain hose", "drain_pan", "returned"]]);
  });

  it("closes the check-in dialog on Cancel without checking in", async () => {
    render(<WorkspaceView live={live("job-checkin", {}, { status: "assigned", timeOnSite: null, draftReportRef: null })} />);
    fireEvent.click(button("Start job (check in)"));
    fireEvent.click(button("Cancel", screen.getByRole("dialog")));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(checkIn).not.toHaveBeenCalled();
  });

  it("closes the Can't make, Add part, Refrigerant and sign-off dialogs on Cancel, leaving the draft as it was", async () => {
    const closes = async (open: () => void) => {
      open();
      fireEvent.click(button("Cancel", screen.getByRole("dialog")));
      await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    };
    render(<WorkspaceView live={live("job-cant", {}, { status: "assigned", timeOnSite: null, draftReportRef: null, assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "pending", acknowledgement: "pending", cantMakeReason: null } })} />);
    await closes(() => { fireEvent.click(button("Can’t make this time…")); });
    expect(acknowledge).not.toHaveBeenCalled();
    cleanup();
    render(<WorkspaceView live={live("job-cancel")} />);
    fireEvent.click(button("(see Parts & refrigerant)")); // the work report's link opens the Parts tab
    expect(screen.getByText("No parts used.")).toBeDefined();
    await closes(() => { fireEvent.click(button(/Add part/)); });
    await closes(() => { fireEvent.click(button(/^Refrigerant$/)); });
    await closes(() => { fireEvent.click(button("Get signature")); });
    expect(screen.getByText("No parts used.")).toBeDefined();
    expect(screen.queryByText(/^Refrigerant \(/)).toBeNull();
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    expect(signOff).not.toHaveBeenCalled();
  });

  it("shows a submitted report's results and reasons as text; on the outdoor tab Add reading starts on the refrigerant pipe", () => {
    const items = [
      { id: "it-1", componentKey: "filter" as const, componentGroup: "indoor" as const, result: "attention" as const, reason: "Dusty", evidenceIds: [], authorId: "tech-1", observedAt: "2026-11-18T02:20:00Z" },
      { id: "it-2", componentKey: "drain_pan" as const, componentGroup: "indoor" as const, result: "normal" as const, reason: null, evidenceIds: [], authorId: "tech-1", observedAt: "2026-11-18T02:20:00Z" },
    ];
    render(<WorkspaceView live={live("job-ro", { items }, { status: "submitted", draftReportRef: null })} />);
    expect(screen.getByText("Attention")).toBeDefined();
    expect(screen.getByText("Normal")).toBeDefined();
    expect(screen.getByText("Reason: Dusty")).toBeDefined();
    expect(screen.getAllByText(/^Reason:/)).toHaveLength(1); // a normal result has none
    expect(screen.getByText("No photos.")).toBeDefined();
    expect(screen.queryByRole("combobox")).toBeNull();
    cleanup();
    const view = live("job-outdoor");
    render(<WorkspaceView live={{ ...view, components: [...view.components, { group: "outdoor", key: "refrigerant_pipe" }, { group: "outdoor", key: "compressor" }] }} />);
    fireEvent.click(screen.getByText(/^Outdoor \(2\)/));
    fireEvent.click(button(/Add reading/));
    const row = screen.getAllByRole("row")[1];
    if (!row) throw new Error("no reading row");
    expect(within(row).getByLabelText<HTMLSelectElement>("Linked item").value).toBe("refrigerant_pipe");
  });
});

describe("WorkspaceView sign-off", () => {
  it("signs the saved draft as it is, and shows the signer once the sign-off lands", async () => {
    answers.sign = { ok: true, value: { id: "rep-1", version: 2, attachmentRefs: [], signOff: { signerName: "Mr Lim", signedAt: "2026-11-18T03:00:00Z", signatureAttachmentId: null, absentReason: "Away", reportVersion: 2 } } };
    render(<WorkspaceView live={live("job-sign")} />);
    expect(screen.getByText("Not signed")).toBeDefined();
    fireEvent.click(button("Get signature"));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Report draft v2 · 0 / 2 results · 0 parts")).toBeDefined();
    signAbsent(dialog);
    expect(saveDraft).not.toHaveBeenCalled(); // nothing unsaved: the sign-off is for draft v2 as it is
    expect(signOff).toHaveBeenCalledOnce();
    const form = vi.mocked(signOff).mock.calls[0]?.[0];
    expect([form?.get("jobId"), form?.get("reportId"), form?.get("reportVersion"), form?.get("signerName"), form?.get("absentReason")]).toStrictEqual(["job-sign", "rep-1", "2", "Mr Lim", "Away"]);
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText(/^Signed · Mr Lim · /)).toBeDefined();
    expect(button("Sign again")).toBeDefined();
  });

  it("saves an edited draft first and signs the saved version; a refused sign-off keeps the dialog and says why", async () => {
    answers.save = { ok: true, value: saved3 };
    answers.sign = { ok: false, code: "UNAVAILABLE", messageKey: "errors.unavailable", fieldErrors: {} };
    render(<WorkspaceView live={live("job-sign-dirty")} />);
    fireEvent.change(workText(), { target: { value: "Cleaned the filters and the drain pan" } });
    fireEvent.click(button("Get signature"));
    signAbsent(screen.getByRole("dialog"));
    expect(saveDraft).toHaveBeenCalledOnce();
    await waitFor(() => { expect(signOff).toHaveBeenCalledOnce(); });
    expect(vi.mocked(signOff).mock.calls[0]?.[0].get("reportVersion")).toBe("3");
    await waitFor(() => { expect(screen.getByText("Not sent; your draft is kept on this page. Try again.")).toBeDefined(); });
    expect(screen.getByRole("dialog")).toBeDefined();
    expect(screen.queryByText("Unsaved changes")).toBeNull(); // the save landed
  });
});

describe("WorkspaceView timers", () => {
  it("autosaves 20 s after the last edit, restarting with each edit, and not without one", async () => {
    vi.useFakeTimers();
    answers.save = { ok: true, value: saved3 };
    render(<WorkspaceView live={live("job-autosave")} />);
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(saveDraft).not.toHaveBeenCalled(); // a clean draft is not saved
    fireEvent.change(workText(), { target: { value: "Cleaned the filters and" } });
    act(() => { vi.advanceTimersByTime(15_000); });
    fireEvent.change(workText(), { target: { value: "Cleaned the filters and the coil" } });
    act(() => { vi.advanceTimersByTime(15_000); });
    expect(saveDraft).not.toHaveBeenCalled(); // 15 s after the last edit
    act(() => { vi.advanceTimersByTime(5000); });
    expect(saveDraft).toHaveBeenCalledOnce();
    const [input, version] = vi.mocked(saveDraft).mock.calls[0] ?? [];
    expect([input?.workText, version]).toStrictEqual(["Cleaned the filters and the coil", 2]); // the latest text, on v2
    await act(async () => { await Promise.resolve(); });
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(saveDraft).toHaveBeenCalledOnce();
  });

  it("does not autosave an edit once the work window has ended", () => {
    vi.useFakeTimers();
    render(<WorkspaceView live={live("job-autosave-ended", {}, {}, "2026-11-18T07:59:35Z")} />); // 25 s before the end
    act(() => { vi.advanceTimersByTime(15_000); });
    fireEvent.change(workText(), { target: { value: "Cleaned the filters and the coil" } }); // its autosave would run 20 s later
    act(() => { vi.advanceTimersByTime(15_000); }); // the clock passes the end first
    expect(screen.getByText(/^The work window ended/)).toBeDefined();
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(saveDraft).not.toHaveBeenCalled();
  });

  it("moves its clock on, so the window's end warning appears while the page stays open", () => {
    vi.useFakeTimers();
    render(<WorkspaceView live={live("job-clock", {}, {}, "2026-11-18T07:44:00Z")} />); // 16 min before the end
    expect(screen.queryByText(/^Work window ends in/)).toBeNull();
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(screen.getByText(/^Work window ends in 15 min/)).toBeDefined();
    expect(screen.getByText("ending")).toBeDefined();
  });
});
