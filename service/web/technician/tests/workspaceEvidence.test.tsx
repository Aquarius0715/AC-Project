// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// the job workspace's evidence under a finding (readings, linked photos, a photo taken for it), the readings table,
// the photo refusals, Time on site with Pause / Resume, the next action and Submit (review round 1078)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const answers = vi.hoisted((): { save: Answer; upload: Answer; other: Answer } => ({ save: { ok: true, value: null }, upload: { ok: true, value: null }, other: { ok: true, value: null } }));
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  saveDraft: vi.fn(() => Promise.resolve(answers.save)), uploadPhoto: vi.fn(() => Promise.resolve(answers.upload)),
  submitReport: vi.fn(() => Promise.resolve(answers.other)), pauseWork: vi.fn(() => Promise.resolve(answers.other)),
  checkIn: vi.fn(), acknowledge: vi.fn(), resumeRework: vi.fn(), signOff: vi.fn(),
}));
// the action runner without the transition, the toast and the refresh: `after` or `failed` runs once the action answers
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }],
}));

import { pauseWork, saveDraft, submitReport, uploadPhoto } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

type Photo = ApiTechReport["attachmentRefs"][number];
const photo = (id: string, name: string, status: Photo["status"] = "ready"): Photo => ({ id, name, status, mime: "image/jpeg", size: 1000, tag: null });
const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  version: 2, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" }, attachmentRefs: [], submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});
const item = (componentKey: "filter" | "drain_pan", result: "normal" | "attention", evidenceIds: string[] = []) => ({ componentKey, result, evidenceIds, id: `it-${componentKey}`, componentGroup: "indoor" as const, reason: result === "normal" ? null : "Dusty", authorId: "tech-1", observedAt: "2026-11-18T02:20:00Z" });
// a job in progress on its saved draft v2, with two indoor items; `status` other than in_progress shows it read-only
const live = (rep: Partial<ApiTechReport> = {}, job: Partial<WorkspaceLive["job"]> = {}): WorkspaceLive => ({
  now: "2026-11-18T03:00:00Z", known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id: "job-1", version: 5, status: "in_progress", type: "maintenance", unitId: "unit-12345678", symptom: "", alertCount: 0, origin: "manual",
    assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null },
    timeOnSite: { startedAt: "2026-11-18T02:10:00Z" }, draftReportRef: { reportId: "rep-1", reportVersion: 2 }, reportRefs: [], problemPhotos: [], ...job,
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [{ group: "indoor", key: "filter" }, { group: "indoor", key: "drain_pan" }],
  report: report(rep), reportRefused: null, parts: [],
});
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const file = (name: string, type: string, size = 100) => { const f = new File(["x"], name, { type }); Object.defineProperty(f, "size", { value: size }); return f; };
const choose = (label: string, f: File) => { fireEvent.change(screen.getByLabelText(label), { target: { files: [f] } }); };
const unsaved = () => screen.queryByText("Unsaved changes");

beforeEach(() => {
  answers.save = { ok: true, value: null }; answers.upload = { ok: true, value: null }; answers.other = { ok: true, value: null };
  for (const fn of [saveDraft, uploadPhoto, submitReport, pauseWork]) vi.mocked(fn).mockClear();
});
afterEach(() => { cleanup(); });

describe("WorkspaceView readings", () => {
  it("adds a reading on the first component, edits its metric, value and item, flags a non-number, and removes it", () => {
    render(<WorkspaceView live={live({ items: [item("filter", "normal"), item("drain_pan", "normal")] })} />);
    expect(screen.getByText(/^No readings yet/)).toBeDefined();
    fireEvent.click(button(/Add reading/));
    const row = screen.getAllByRole("row")[1];
    if (!row) throw new Error("no reading row");
    expect(within(row).getByLabelText<HTMLSelectElement>("Linked item").value).toBe("filter");
    expect(within(row).getByText("°C")).toBeDefined();
    fireEvent.change(within(row).getByLabelText("Metric"), { target: { value: "refrigerant_pressure" } });
    expect(within(row).getByLabelText<HTMLSelectElement>("Metric").value).toBe("refrigerant_pressure");
    expect(within(row).queryByText("°C")).toBeNull();
    fireEvent.change(within(row).getByLabelText("Metric"), { target: { value: "bogus" } }); // not a metric: kept as it was
    expect(within(row).getByLabelText<HTMLSelectElement>("Metric").value).toBe("refrigerant_pressure");
    const value = within(row).getByRole("textbox");
    fireEvent.change(value, { target: { value: "1,5" } });
    expect(value.getAttribute("aria-invalid")).toBe("true");
    fireEvent.change(value, { target: { value: "1.5" } });
    expect(value.getAttribute("aria-invalid")).toBe("false");
    fireEvent.change(within(row).getByLabelText("Linked item"), { target: { value: "drain_pan" } });
    expect(within(row).getByLabelText<HTMLSelectElement>("Linked item").value).toBe("drain_pan");
    expect(unsaved()).not.toBeNull();
    fireEvent.click(button("Remove", row));
    expect(screen.getByText(/^No readings yet/)).toBeDefined();
  });

  it("shows a submitted report's readings as text, an unmeasured value as —", () => {
    render(<WorkspaceView live={live({ measurements: [{ id: "m1", componentKey: "filter", metric: "temperature", value: null, unit: "°C", observedAt: "2026-11-18T02:30:00Z", quality: "valid" }] }, { status: "submitted", draftReportRef: null })} />);
    const row = screen.getAllByRole("row")[1];
    if (!row) throw new Error("no reading row");
    expect(within(row).queryByRole("combobox")).toBeNull();
    expect(within(row).getByText("Temperature")).toBeDefined();
    expect(within(row).getByText("—")).toBeDefined();
    expect(within(row).getByText("Filter")).toBeDefined();
  });
});

describe("WorkspaceView evidence under a finding", () => {
  it("offers a finding that needs attention its own reading, which then shows under it and in the Readings tab", () => {
    render(<WorkspaceView live={live({ items: [item("filter", "attention"), item("drain_pan", "normal")] })} />);
    expect(screen.getByText("Take or add a photo here: it is linked to this finding.")).toBeDefined(); // no ready photo yet
    expect(button("Link photo").disabled).toBe(true);
    fireEvent.click(screen.getByText("Parts & refrigerant"));
    fireEvent.click(button(/Add reading/)); // the Parts tab has no Add reading of its own: this is the finding's, the only one
    expect(screen.getByText("Reading")).toBeDefined();
    expect(screen.getByText(/^Temperature · — °C/)).toBeDefined();
    expect(screen.getAllByRole("row")).toHaveLength(2); // the Readings tab is open again, with the new row
  });

  it("links a ready photo to the finding from its list, opens it, and unlinks it; a photo the report lost keeps its ID", () => {
    const att = [photo("ph-1", "coil.jpg"), photo("ph-2", "drain.jpg"), photo("ph-3", "busy.jpg", "processing")];
    render(<WorkspaceView live={live({ attachmentRefs: att, items: [item("filter", "attention", ["gone-1234567"]), item("drain_pan", "normal")] })} />);
    expect(screen.getByText("gone-123").closest("a")).toBeNull(); // missing: no link
    expect(screen.queryByText("Take or add a photo here: it is linked to this finding.")).toBeNull();
    const link = button("Link photo");
    expect(link.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(link);
    expect(link.getAttribute("aria-expanded")).toBe("true");
    const list = screen.getByRole("group", { name: "Report photos to link to Filter" });
    expect(within(list).getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["coil.jpg", "drain.jpg"]); // ready ones only
    fireEvent.click(button("coil.jpg", list));
    expect(screen.queryByRole("group", { name: /Report photos to link/ })).toBeNull();
    expect(screen.getByRole("link", { name: "coil.jpg" }).getAttribute("href")).toBe("/technician/jobs/job-1/photos/ph-1?reportId=rep-1&reportVersion=2");
    expect(unsaved()).not.toBeNull();
    fireEvent.click(button("Link photo"));
    expect(within(screen.getByRole("group", { name: /Report photos to link/ })).getAllByRole("button").map((b) => b.textContent)).toStrictEqual(["drain.jpg"]);
    fireEvent.click(button("Link photo")); // a second click closes the list
    expect(screen.queryByRole("group", { name: /Report photos to link/ })).toBeNull();
    fireEvent.click(button("Unlink coil.jpg from Filter"));
    expect(screen.queryByRole("link", { name: "coil.jpg" })).toBeNull();
  });

  it("sends an installation photo with what it shows (DEC-106)", async () => {
    answers.upload = { ok: true, value: { id: "rep-1", version: 2, signOff: null, attachmentRefs: [{ ...photo("ph-9", "plate.jpg", "processing"), tag: "name_plate" }] } };
    render(<WorkspaceView live={live({ installation: { confirmedModelId: "m-1", serial: "SN-1" }, items: [item("filter", "normal"), item("drain_pan", "normal")] }, { type: "installation" })} />);
    choose("Photo of the name plate", file("plate.jpg", "image/jpeg"));
    expect(uploadPhoto).toHaveBeenCalledOnce();
    const form = vi.mocked(uploadPhoto).mock.calls[0]?.[0];
    expect([form?.get("reportId"), form?.get("tag")]).toStrictEqual(["rep-1", "name_plate"]);
    await waitFor(() => { expect(screen.getByRole("img", { name: "plate.jpg" })).toBeDefined(); });
  });

  it("uploads a photo taken for a finding on the saved draft and links the new photo to it", async () => {
    answers.upload = { ok: true, value: { id: "rep-1", version: 2, signOff: null, attachmentRefs: [photo("ph-1", "coil.jpg"), photo("ph-9", "new.jpg", "processing")] } };
    render(<WorkspaceView live={live({ attachmentRefs: [photo("ph-1", "coil.jpg")], items: [item("filter", "attention"), item("drain_pan", "normal")] })} />);
    choose("Add a photo to Filter", file("new.jpg", "image/jpeg"));
    expect(saveDraft).not.toHaveBeenCalled(); // nothing unsaved: the photo goes to draft v2 as it is
    expect(uploadPhoto).toHaveBeenCalledOnce();
    const form = vi.mocked(uploadPhoto).mock.calls[0]?.[0];
    expect([form?.get("jobId"), form?.get("reportId"), form?.get("reportVersion"), form?.get("tag")]).toStrictEqual(["job-1", "rep-1", "2", null]);
    await waitFor(() => { expect(button("Unlink new.jpg from Filter")).toBeDefined(); });
    expect(screen.queryByRole("button", { name: "Unlink coil.jpg from Filter" })).toBeNull(); // only the new one is linked
    expect(unsaved()).not.toBeNull(); // the link is an edit of the draft
  });

  it("saves an edited draft before the upload, which goes to the saved version", async () => {
    answers.save = { ok: true, value: { id: "rep-1", version: 3, signOff: null, attachmentRefs: [] } };
    answers.upload = { ok: true, value: { id: "rep-1", version: 3, signOff: null, attachmentRefs: [photo("ph-9", "after.png", "processing")] } };
    render(<WorkspaceView live={live({ items: [item("filter", "normal"), item("drain_pan", "normal")] })} />);
    fireEvent.change(screen.getByDisplayValue("Cleaned the filters"), { target: { value: "Cleaned the filters and the pan" } });
    choose("Add photos", file("after.png", "image/png"));
    expect(saveDraft).toHaveBeenCalledOnce();
    expect(vi.mocked(saveDraft).mock.calls[0]?.[1]).toBe(2);
    await waitFor(() => { expect(uploadPhoto).toHaveBeenCalledOnce(); });
    expect(vi.mocked(uploadPhoto).mock.calls[0]?.[0].get("reportVersion")).toBe("3");
    await waitFor(() => { expect(unsaved()).toBeNull(); }); // the saved text and the uploaded photo: nothing left unsaved
    expect(screen.getByRole("img", { name: "after.png" }).getAttribute("src")).toBe("/technician/jobs/job-1/photos/ph-9?reportId=rep-1&reportVersion=3");
  });

  it("refuses a photo that is not JPEG/PNG or larger than 5 MiB, and offers no add tile at 10 photos", () => {
    render(<WorkspaceView live={live({ attachmentRefs: [photo("ph-1", "coil.jpg")] })} />);
    choose("Add photos", file("scan.gif", "image/gif"));
    expect(screen.getByText("“scan.gif” was not added — only JPEG/PNG up to 5 MiB, max 10 photos. Saved text and photos are kept.")).toBeDefined();
    choose("Add photos", file("huge.jpg", "image/jpeg", (5 << 20) + 1));
    expect(screen.getByText(/^“huge.jpg” was not added/)).toBeDefined();
    fireEvent.change(screen.getByLabelText("Add photos"), { target: { files: [] } }); // no file chosen: nothing changes
    expect(screen.getByText(/^“huge.jpg” was not added/)).toBeDefined();
    expect(uploadPhoto).not.toHaveBeenCalled();
    expect(saveDraft).not.toHaveBeenCalled();
    fireEvent.error(screen.getByRole("img", { name: "coil.jpg" })); // the file is gone
    expect(screen.getByTitle("coil.jpg").textContent).toBe("Photo unavailable");
    cleanup();
    render(<WorkspaceView live={live({ attachmentRefs: Array.from({ length: 10 }, (_, k) => photo(`ph-${String(k)}`, `p${String(k)}.jpg`)) })} />);
    expect(screen.queryByLabelText("Add photos")).toBeNull();
  });
});

describe("WorkspaceView time on site, next action and submit", () => {
  it("pauses the work on the saved draft and resumes a paused one; the next view focuses the same button", async () => {
    // its own job: the open view of a job outlives the test (the view keeps it per job, IR625)
    const job = { id: "job-pause" };
    render(<WorkspaceView live={live({}, job)} />);
    fireEvent.click(screen.getByText("Time on site"));
    expect(screen.getByText("Started")).toBeDefined();
    fireEvent.click(button("Pause work"));
    expect(saveDraft).not.toHaveBeenCalled();
    expect(pauseWork).toHaveBeenCalledExactlyOnceWith("job-pause", 5, true);
    await vi.mocked(pauseWork).mock.results[0]?.value; // its `after` ran first: the focus hand-off is set
    cleanup();
    render(<WorkspaceView live={live({}, { ...job, version: 6, timeOnSite: { startedAt: "2026-11-18T02:10:00Z", pauses: [{ from: "2026-11-18T02:50:00Z", to: null }] } })} />);
    expect(document.activeElement?.id).toBe("pause-toggle");
    const toggle = button("Resume work"); // the Time on site view is still open, and the work is paused
    expect(toggle.id).toBe("pause-toggle");
    expect(screen.getByText(/^since /)).toBeDefined();
    fireEvent.click(toggle);
    expect(pauseWork).toHaveBeenLastCalledWith("job-pause", 6, false);
  });

  it("asks a follow-up for its date and note, and shows a submitted one as text", () => {
    render(<WorkspaceView live={live()} />);
    const nextAction = () => screen.getByRole("combobox", { name: "Next action" });
    fireEvent.change(nextAction(), { target: { value: "follow_up" } });
    fireEvent.change(screen.getByLabelText("Follow-up date"), { target: { value: "2026-11-25" } });
    fireEvent.change(screen.getByLabelText("What to do (required)"), { target: { value: "Replace the drain pan" } });
    expect(screen.getByLabelText<HTMLInputElement>("Follow-up date").value).toBe("2026-11-25");
    expect(screen.getByLabelText<HTMLInputElement>("What to do (required)").value).toBe("Replace the drain pan");
    fireEvent.change(screen.getByLabelText("Follow-up date"), { target: { value: "" } }); // cleared: no date, the note stays
    expect(screen.getByLabelText<HTMLInputElement>("What to do (required)").value).toBe("Replace the drain pan");
    expect(screen.getByLabelText<HTMLInputElement>("Follow-up date").value).toBe("");
    fireEvent.change(nextAction(), { target: { value: "none" } });
    expect(screen.queryByLabelText("Follow-up date")).toBeNull();
    cleanup();
    render(<WorkspaceView live={live({ nextAction: { kind: "follow_up", date: "2026-11-25T01:00:00Z", note: "Replace the drain pan" } }, { status: "submitted", draftReportRef: null })} />);
    expect(screen.queryByRole("combobox", { name: "Next action" })).toBeNull();
    expect(screen.getByText(/^Follow-up .+ — Replace the drain pan$/)).toBeDefined();
  });

  it("submits a complete draft from the dialog, which warns that it is not signed; a refusal shows on the page", async () => {
    render(<WorkspaceView live={live({ items: [item("filter", "normal"), item("drain_pan", "normal")] })} />);
    expect(button("Save draft").disabled).toBe(true); // nothing to save
    fireEvent.click(button("Submit report"));
    const dialog = screen.getByRole("dialog", { name: "Submit report v2?" });
    expect(within(dialog).getByText(/^No customer sign-off yet/)).toBeDefined();
    fireEvent.click(button("Cancel", dialog));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Submit report"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" }); // Escape closes it too, without submitting
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(submitReport).not.toHaveBeenCalled();
    fireEvent.click(button("Submit report"));
    fireEvent.click(button("Submit", screen.getByRole("dialog")));
    expect(saveDraft).not.toHaveBeenCalled();
    expect(submitReport).toHaveBeenCalledWith("job-1", 5, 2);
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    answers.other = { ok: false, code: "UNAVAILABLE", messageKey: "errors.unavailable", fieldErrors: {} };
    fireEvent.click(button("Submit report"));
    fireEvent.click(button("Submit", screen.getByRole("dialog")));
    await waitFor(() => { expect(screen.getByText("Not sent; your draft is kept on this page. Try again.")).toBeDefined(); });
    expect(screen.getByRole("dialog")).toBeDefined(); // kept open to try again
  });

  it("saves an edited draft before it submits the saved version; a failed save submits nothing", async () => {
    answers.save = { ok: false, code: "UNAVAILABLE", messageKey: "errors.unavailable", fieldErrors: {} };
    render(<WorkspaceView live={live({ items: [item("filter", "normal"), item("drain_pan", "normal")] })} />);
    fireEvent.change(screen.getByDisplayValue("Cleaned the filters"), { target: { value: "Cleaned both filters" } });
    expect(button("Save draft").disabled).toBe(false);
    fireEvent.click(button("Submit report"));
    fireEvent.click(button("Submit", screen.getByRole("dialog")));
    await waitFor(() => { expect(screen.getByText("Not sent; your draft is kept on this page. Try again.")).toBeDefined(); });
    expect(submitReport).not.toHaveBeenCalled();
    expect(unsaved()).not.toBeNull();
    answers.save = { ok: true, value: { id: "rep-1", version: 3, signOff: null, attachmentRefs: [] } };
    fireEvent.click(button("Submit", screen.getByRole("dialog")));
    await waitFor(() => { expect(submitReport).toHaveBeenCalledWith("job-1", 5, 3); });
    expect(screen.queryByText("Not sent; your draft is kept on this page. Try again.")).toBeNull(); // the save cleared it
  });
});
