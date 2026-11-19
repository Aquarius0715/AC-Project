// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// the job workspace's banners by job state and acknowledgement, the refusal texts of a refused action, the unreadable
// report and the Job & unit card (review round 1076)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const answer = vi.hoisted((): { next: Answer } => ({ next: { ok: true, value: null } }));
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  acknowledge: vi.fn(() => Promise.resolve(answer.next)), resumeRework: vi.fn(() => Promise.resolve(answer.next)),
  saveDraft: vi.fn(), uploadPhoto: vi.fn(), submitReport: vi.fn(), checkIn: vi.fn(), pauseWork: vi.fn(), signOff: vi.fn(),
}));
// the action runner without the transition, the toast and the refresh: `after` or `failed` runs once the action answers
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }],
}));

import { acknowledge, resumeRework } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

type Job = WorkspaceLive["job"];
type Assignment = NonNullable<Job["assignment"]>;
const assignment = (over: Partial<Assignment> = {}): Assignment => ({
  scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null, ...over,
});
const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  version: 2, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" }, attachmentRefs: [], submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});
const live = (job: Partial<Job> = {}, rest: Partial<WorkspaceLive> = {}): WorkspaceLive => ({
  now: "2026-11-18T03:00:00Z", known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id: "job-1", version: 5, status: "assigned", type: "maintenance", unitId: "unit-12345678", symptom: "", alertCount: 0, origin: "manual",
    assignment: assignment(), timeOnSite: null, draftReportRef: null, reportRefs: [], problemPhotos: [], ...job,
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [], report: null, reportRefused: null, parts: [], ...rest,
});
const refusedWith = (over: Partial<ActionFailure>) => { answer.next = { ok: false, code: "VALIDATION", messageKey: "", fieldErrors: {}, ...over }; };
const button = (name: string) => screen.getByRole<HTMLButtonElement>("button", { name });

beforeEach(() => { answer.next = { ok: true, value: null }; vi.mocked(acknowledge).mockClear(); vi.mocked(resumeRework).mockClear(); });
afterEach(cleanup);

describe("WorkspaceView assignment banners", () => {
  it("accepts a pending assignment at the job's version, and the next view focuses the job title", async () => {
    const { unmount } = render(<WorkspaceView live={live({ assignment: assignment({ acknowledgement: "pending" }) })} />);
    expect(screen.getByText(/^New assignment for /)).toBeDefined();
    fireEvent.click(button("Accept assignment"));
    expect(acknowledge).toHaveBeenCalledWith("job-1", 5, "accept", null, null);
    await waitFor(() => { expect(acknowledge).toHaveBeenCalledOnce(); });
    await Promise.resolve();
    unmount();
    render(<WorkspaceView live={live({ version: 6 })} />); // the page mounts the view again for the new version
    expect(document.activeElement?.id).toBe("job-title");
  });

  it("sends can't make with the trimmed reason and the other time in the display zone, or without a time; an empty reason is refused", () => {
    render(<WorkspaceView live={live({ assignment: assignment({ acknowledgement: "pending" }) })} />);
    fireEvent.click(button("Can’t make this time…"));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Send to coordinator" }));
    expect(within(dialog).getByText("Tell your coordinator why (1–1000 characters)")).toBeDefined();
    expect(acknowledge).not.toHaveBeenCalled();
    fireEvent.change(within(dialog).getByPlaceholderText("Medical appointment on that morning"), { target: { value: "  Medical appointment  " } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Send to coordinator" }));
    expect(acknowledge).toHaveBeenLastCalledWith("job-1", 5, "cant_make", "Medical appointment", null);
    fireEvent.change(within(dialog).getByLabelText(/^A time you could do instead \(optional\)/), { target: { value: "2026-11-20" } });
    fireEvent.change(within(dialog).getByRole("combobox"), { target: { value: "14:00-16:00" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Send to coordinator" }));
    // 14:00–16:00 in Kuala Lumpur (UTC+8)
    expect(acknowledge).toHaveBeenLastCalledWith("job-1", 5, "cant_make", "Medical appointment", { startAt: "2026-11-20T06:00:00.000Z", endAt: "2026-11-20T08:00:00.000Z" });
  });

  it("shows the can't-make answer with its reason, or without one", () => {
    render(<WorkspaceView live={live({ assignment: assignment({ acknowledgement: "cant_make", cantMakeReason: "Clinic" }) })} />);
    expect(screen.getByText(/You said you can’t make this time \(“Clinic”\)/)).toBeDefined();
    cleanup();
    render(<WorkspaceView live={live({ assignment: assignment({ acknowledgement: "cant_make" }) })} />);
    expect(screen.getByText(/^You said you can’t make this time\. Your coordinator/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Accept assignment" })).toBeNull();
  });

  it.each([
    { phase: "before", over: { scheduledStart: "2026-11-18T05:00:00Z" }, text: /^Read-only until the window opens at /, enabled: false },
    { phase: "open", over: {}, text: /^On site: Start job opens Check in/, enabled: true },
    { phase: "ending", over: { scheduledEnd: "2026-11-18T03:10:00Z" }, text: /^On site: Start job opens Check in/, enabled: true },
    { phase: "ended", over: { scheduledEnd: "2026-11-18T02:30:00Z" }, text: /^The work window has ended — ask your coordinator/, enabled: false },
  ])("an accepted assignment $phase the window: its text, Start job enabled $enabled", ({ over, text, enabled }) => {
    render(<WorkspaceView live={live({ assignment: assignment(over) })} />);
    expect(screen.getByText(text)).toBeDefined();
    expect(button("Start job (check in)").disabled).toBe(!enabled);
  });
});

describe("WorkspaceView job state banners", () => {
  it.each([
    { status: "on_hold", text: /^On hold by HQ — saving and submitting are blocked/ },
    { status: "submitted", text: /^Submitted v3 — read-only, awaiting quality review/ },
    { status: "completed", text: /^Accepted — the job is completed/ },
  ])("$status", ({ status, text }) => {
    render(<WorkspaceView live={live({ status, reportRefs: [{ reportId: "rep-1", reportVersion: 2 }, { reportId: "rep-1", reportVersion: 3 }] })} />);
    expect(screen.getByText(text)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Start job (check in)" })).toBeNull();
  });

  it("resumes rework at the job's version with HQ's last reason; without a reason the plain text; disabled after the window", async () => {
    const returned = report({ reviewHistory: [
      { occurredAt: "2026-11-17T01:00:00Z", decision: "return", reason: "Photo of the coil missing", reviewerUserId: "hq-1", reportVersion: 1 },
      { occurredAt: "2026-11-17T02:00:00Z", decision: "return", reason: "Drain not checked", reviewerUserId: "hq-1", reportVersion: 2 },
    ] });
    render(<WorkspaceView live={live({ status: "rework_requested" }, { report: returned })} />);
    expect(screen.getByText(/^Returned for rework — “Drain not checked”/)).toBeDefined();
    fireEvent.click(button("Resume rework"));
    expect(resumeRework).toHaveBeenCalledWith("job-1", 5);
    await Promise.resolve();
    cleanup();
    render(<WorkspaceView live={live({ status: "rework_requested", assignment: assignment({ scheduledEnd: "2026-11-18T02:30:00Z" }) }, { report: report() })} />);
    expect(screen.getByText(/^Returned for rework\. Resume to continue/)).toBeDefined();
    expect(button("Resume rework").disabled).toBe(true);
  });

  it("an in-progress job past its window is read-only with the discard banner", () => {
    render(<WorkspaceView live={live({ status: "in_progress", assignment: assignment({ scheduledEnd: "2026-11-18T02:30:00Z" }), timeOnSite: { startedAt: "2026-11-18T02:10:00Z" } }, { report: report() })} />);
    expect(screen.getByText(/^The work window ended — unsaved input was discarded/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Save draft" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Submit report" })).toBeNull();
  });

  it("names why the report is not readable: a known key in words, another key humanized", () => {
    render(<WorkspaceView live={live({}, { reportRefused: "errors.assignment_not_started" })} />);
    expect(screen.getByText("The report is not readable now (your work window has not started).")).toBeDefined();
    cleanup();
    render(<WorkspaceView live={live({}, { reportRefused: "errors.report_archived" })} />);
    expect(screen.getByText("The report is not readable now (report archived).")).toBeDefined();
  });
});

describe("WorkspaceView refusals", () => {
  it.each([
    { name: "a changed report version", over: { code: "CONFLICT", messageKey: "errors.report_version_changed" }, text: "The report or job changed elsewhere; reload the page to continue from the latest version." },
    { name: "a version conflict", over: { code: "CONFLICT", messageKey: "error.versionConflict" }, text: "The report or job changed elsewhere; reload the page to continue from the latest version." },
    { name: "another conflict", over: { code: "CONFLICT", messageKey: "errors.invalid_state" }, text: "The job is not in a state that allows this (on hold, submitted or cancelled); the page shows the latest state." },
    { name: "a known forbidden reason", over: { code: "FORBIDDEN", messageKey: "errors.assignment_ended" }, text: "Not allowed: your work window has ended." },
    { name: "another forbidden reason", over: { code: "FORBIDDEN", messageKey: "error.not_your_company" }, text: "Not allowed: not your company." },
    { name: "an unreachable Core API", over: { code: "UNAVAILABLE" }, text: "Not sent; your draft is kept on this page. Try again." },
  ])("$name", async ({ over, text }) => {
    refusedWith(over);
    render(<WorkspaceView live={live({ assignment: assignment({ acknowledgement: "pending" }) })} />);
    fireEvent.click(button("Accept assignment"));
    expect(await screen.findByText(text)).toBeDefined();
  });
});

describe("WorkspaceView Job & unit card", () => {
  it("before the window: the site opens later, the unit's name, the request, the client's photos and the alert link; no diagnostics yet", () => {
    render(<WorkspaceView live={live({
      assignment: assignment({ scheduledStart: "2026-11-18T05:00:00Z" }), symptom: "Water dripping", alertCount: 2, origin: "hq_request",
      problemPhotos: [{ id: "ph-1", name: "drip.jpg" }],
    }, { unitRefused: "errors.assignment_not_started" })} />);
    const scope = screen;
    expect(scope.getByText("opens at the start of your window")).toBeDefined();
    expect(scope.getByText("Unit A")).toBeDefined();
    expect(scope.getByText("“Water dripping”")).toBeDefined();
    expect(scope.getByText(/· HQ request$/)).toBeDefined();
    expect(scope.getByRole("link", { name: "drip.jpg" }).getAttribute("href")).toBe("/technician/jobs/job-1/problem-photos/ph-1");
    expect(scope.getByRole("link", { name: /^2 alerts/ }).getAttribute("href")).toBe("/technician/units/unit-12345678/alerts?jobId=job-1");
    expect(screen.queryByRole("link", { name: /^Diagnostics/ })).toBeNull();
  });

  it("inside the window: the open unit's place, access, name and model, and the alert evidence and diagnostics links", () => {
    render(<WorkspaceView live={live({ unitId: "unit-12345678" }, {
      unit: { name: "Lobby AC", place: "Level 1, Lobby", access: "Ask the guard", model: "FTKF50", modelId: "m-1", planned: false, scope: [] }, unitName: null,
    })} />);
    expect(screen.getByText("Level 1, Lobby")).toBeDefined();
    expect(screen.getByText("Ask the guard")).toBeDefined();
    expect(screen.getByText("Lobby AC · FTKF50")).toBeDefined();
    expect(screen.getByText("none")).toBeDefined(); // no linked alerts
    expect(screen.getByRole("link", { name: /^Diagnostics/ }).getAttribute("href")).toBe("/technician/units/unit-12345678/control?jobId=job-1");
    expect(screen.getByRole("link", { name: /^Alert evidence/ }).getAttribute("href")).toBe("/technician/units/unit-12345678/alerts?jobId=job-1");
  });
});
