// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";

// the job workspace's "Unsaved changes": text typed while a save or a photo upload is on its way must stay unsaved, so the
// autosave, the Save draft button and Submit (which saves first) still send it (R1030)
interface Pending { resolve: (v: unknown) => void }
const calls = vi.hoisted(() => ({ save: [] as Pending[], upload: [] as Pending[], submit: vi.fn() }));
const deferred = (list: Pending[]) => new Promise((resolve) => { list.push({ resolve }); });
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  saveDraft: vi.fn(async () => deferred(calls.save)),
  uploadPhoto: vi.fn(async () => deferred(calls.upload)),
  submitReport: calls.submit,
  acknowledge: vi.fn(), checkIn: vi.fn(), pauseWork: vi.fn(), resumeRework: vi.fn(), signOff: vi.fn(),
}));
// the action runner without the transition and the refresh: `after` runs when the action's promise resolves
type Result = { ok: true; value: unknown } | { ok: false };
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, _ok: unknown, after?: (v: unknown) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); });
  }],
}));

import { saveDraft } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

const report = (version: number, photos: string[] = []): ApiTechReport => ({
  version, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" },
  attachmentRefs: photos.map((id) => ({ id, name: `${id}.png`, mime: "image/png", size: 10, status: "ready" as const, tag: null })),
  submittedAt: null, acceptedAt: null, reviewHistory: [],
});
const state = ({ id, version, attachmentRefs, signOff }: ApiTechReport) => ({ id, version, attachmentRefs, signOff });
const live: WorkspaceLive = {
  now: "2026-11-18T03:00:00Z", known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id: "job-1", version: 5, status: "in_progress", type: "maintenance", unitId: "unit-1", symptom: "", alertCount: 0, origin: "manual",
    assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null },
    timeOnSite: { startedAt: "2026-11-18T02:10:00Z" }, draftReportRef: { reportId: "rep-1", reportVersion: 2 }, reportRefs: [], problemPhotos: [],
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [], report: report(2), reportRefused: null, parts: [],
};

const workText = () => screen.getByLabelText<HTMLTextAreaElement>("Work performed & next action (10–4000 characters)");
const type = (text: string) => { fireEvent.change(workText(), { target: { value: text } }); };
const unsaved = () => screen.queryByText("Unsaved changes");
const sentText = (call: number) => vi.mocked(saveDraft).mock.calls.at(call)?.[0].workText;

beforeEach(() => { calls.save.length = 0; calls.upload.length = 0; vi.mocked(saveDraft).mockClear(); });
afterEach(cleanup);

describe("WorkspaceView unsaved changes", () => {
  it("clears Unsaved changes after a save when nothing was typed meanwhile", async () => {
    render(<WorkspaceView live={live} />);
    type("Cleaned the filters and the coil");
    expect(unsaved()).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    calls.save[0]?.resolve({ ok: true, value: state(report(3)) });
    await waitFor(() => { expect(unsaved()).toBeNull(); });
  });

  it("keeps text typed while a save is on its way unsaved, and the next save sends it", async () => {
    render(<WorkspaceView live={live} />);
    type("Cleaned the filters and the coil");
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    type("Cleaned the filters and the coil, drain flushed");
    calls.save[0]?.resolve({ ok: true, value: state(report(3)) });
    await waitFor(() => { expect(calls.save).toHaveLength(1); });
    await waitFor(() => { expect(unsaved()).not.toBeNull(); });
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    expect(sentText(1)).toBe("Cleaned the filters and the coil, drain flushed");
    expect(vi.mocked(saveDraft).mock.calls[1]?.[1]).toBe(3);
  });

  it("keeps text typed while a photo uploads unsaved", async () => {
    render(<WorkspaceView live={live} />);
    const file = new File([new Uint8Array(8)], "coil.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText("Add photos"), { target: { files: [file] } });
    expect(calls.upload).toHaveLength(1); // nothing was unsaved, so the upload goes straight on the draft
    type("Cleaned the filters, photo of the coil");
    calls.upload[0]?.resolve({ ok: true, value: state(report(3, ["att-1"])) });
    await waitFor(() => { expect(screen.getByRole("img", { name: "att-1.png" })).toBeDefined(); });
    expect(unsaved()).not.toBeNull();
  });

  it("keeps text typed during the save before an upload unsaved once the upload lands", async () => {
    render(<WorkspaceView live={live} />);
    type("Cleaned the filters and the coil");
    const file = new File([new Uint8Array(8)], "coil.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText("Add photos"), { target: { files: [file] } });
    expect(calls.upload).toHaveLength(0); // the unsaved text is saved first
    type("Cleaned the filters and the coil, drain flushed");
    calls.save[0]?.resolve({ ok: true, value: state(report(3)) });
    await waitFor(() => { expect(calls.upload).toHaveLength(1); });
    calls.upload[0]?.resolve({ ok: true, value: state(report(4, ["att-1"])) });
    await waitFor(() => { expect(screen.getByRole("img", { name: "att-1.png" })).toBeDefined(); });
    expect(unsaved()).not.toBeNull();
    expect(sentText(0)).toBe("Cleaned the filters and the coil");
  });

  it("clears Unsaved changes after an upload when nothing was typed meanwhile", async () => {
    render(<WorkspaceView live={live} />);
    const file = new File([new Uint8Array(8)], "coil.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText("Add photos"), { target: { files: [file] } });
    calls.upload[0]?.resolve({ ok: true, value: state(report(3, ["att-1"])) });
    await waitFor(() => { expect(screen.getByRole("img", { name: "att-1.png" })).toBeDefined(); });
    expect(unsaved()).toBeNull();
  });

  it("shows a photo again once a later report version loads it", async () => {
    render(<WorkspaceView live={{ ...live, report: report(2, ["att-1"]) }} />);
    fireEvent.error(screen.getByRole("img", { name: "att-1.png" }));
    expect(screen.getByText("Photo unavailable")).toBeDefined();
    type("Cleaned the filters and the coil");
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    calls.save[0]?.resolve({ ok: true, value: state(report(3, ["att-1"])) });
    const img = await screen.findByRole("img", { name: "att-1.png" });
    expect(img.getAttribute("src")).toContain("reportVersion=3");
  });
});
