// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// the job workspace's Check in at site, Add part and Refrigerant dialogs, and the checklist's results and reasons with
// Submit report's issues (review round 1077)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const answer = vi.hoisted((): { next: Answer } => ({ next: { ok: true, value: null } }));
vi.mock("../app/technician/jobs/[id]/actions", () => ({
  checkIn: vi.fn(() => Promise.resolve(answer.next)),
  acknowledge: vi.fn(), resumeRework: vi.fn(), saveDraft: vi.fn(), uploadPhoto: vi.fn(), submitReport: vi.fn(), pauseWork: vi.fn(), signOff: vi.fn(),
}));
// the action runner without the transition, the toast and the refresh: `after` or `failed` runs once the action answers
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }],
}));

import { checkIn } from "../app/technician/jobs/[id]/actions";
import { WorkspaceView, type WorkspaceLive } from "../app/technician/jobs/[id]/_components/workspace-view";

type Job = WorkspaceLive["job"];
const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  version: 2, id: "rep-1", jobId: "job-1", authorId: "tech-1", installation: null, items: [], measurements: [], parts: [], refrigerant: [], signOff: null,
  workText: "Cleaned the filters", nextAction: { kind: "none" }, attachmentRefs: [], submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});
const live = (job: Partial<Job> = {}, rest: Partial<WorkspaceLive> = {}): WorkspaceLive => ({
  now: "2026-11-18T03:00:00Z", known: { clock: {}, stamp: {}, span: {} }, zone: "Asia/Kuala_Lumpur",
  job: {
    id: "job-1", version: 5, status: "assigned", type: "maintenance", unitId: "unit-12345678", symptom: "", alertCount: 0, origin: "manual",
    assignment: { scheduledStart: "2026-11-18T02:00:00Z", scheduledEnd: "2026-11-18T08:00:00Z", status: "accepted", acknowledgement: "accepted", cantMakeReason: null },
    timeOnSite: null, draftReportRef: null, reportRefs: [], problemPhotos: [], ...job,
  },
  unit: null, unitRefused: null, unitName: "Unit A", components: [], report: null, reportRefused: null, parts: [], ...rest,
});
// a job in progress on its saved draft, with two indoor items and one outdoor item
const inWork = (rest: Partial<WorkspaceLive> = {}) => live(
  { status: "in_progress", timeOnSite: { startedAt: "2026-11-18T02:10:00Z" }, draftReportRef: { reportId: "rep-1", reportVersion: 2 } },
  { report: report(), components: [{ group: "indoor", key: "filter" }, { group: "indoor", key: "drain_pan" }, { group: "outdoor", key: "compressor" }], ...rest },
);
const button = (name: string, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const focused = (id: string) => waitFor(() => { expect(document.activeElement?.id).toBe(id); });

// the device's location: a fix, a refusal, or none (the API is missing)
type Fix = "refused" | { coords: { latitude: number; longitude: number; accuracy: number } };
const locateWith = (fix: Fix | null) => {
  if (fix === null) { Reflect.deleteProperty(navigator, "geolocation"); return vi.fn(); }
  const getCurrentPosition = vi.fn((ok: (p: unknown) => void, no: () => void) => { if (fix === "refused") no(); else ok(fix); });
  Object.defineProperty(navigator, "geolocation", { configurable: true, value: { getCurrentPosition } });
  return getCurrentPosition;
};
const openCheckIn = () => {
  render(<WorkspaceView live={live()} />);
  fireEvent.click(button("Start job (check in)"));
  return screen.getByRole("dialog");
};

beforeEach(() => { answer.next = { ok: true, value: null }; vi.mocked(checkIn).mockClear(); });
afterEach(() => { cleanup(); Reflect.deleteProperty(navigator, "geolocation"); });

describe("WorkspaceView Check in at site", () => {
  it("refuses a location check-in without a fix and an emptied QR, focusing the location first; nothing is sent", async () => {
    locateWith(null);
    const dialog = openCheckIn();
    fireEvent.change(within(dialog).getByDisplayValue("ac-unit:unit-12345678"), { target: { value: "  " } });
    fireEvent.click(button("Check in & start", dialog));
    expect(within(dialog).getByText("Get your location first — or check in manually with a reason")).toBeDefined();
    expect(within(dialog).getByText("Scan the unit’s QR label")).toBeDefined();
    await focused("check-in-position");
    expect(checkIn).not.toHaveBeenCalled();
    fireEvent.change(within(dialog).getByDisplayValue(""), { target: { value: "ac-unit:unit-12345678" } }); // typing clears its own error only
    expect(within(dialog).queryByText("Scan the unit’s QR label")).toBeNull();
    expect(within(dialog).getByText("Get your location first — or check in manually with a reason")).toBeDefined();
  });

  it("says when the device has no location, or the location could not be read", () => {
    locateWith(null);
    const dialog = openCheckIn();
    fireEvent.click(button("Get my location", dialog));
    expect(within(dialog).getByText("This device cannot share its location — check in manually with a reason")).toBeDefined();
    cleanup();
    const get = locateWith("refused");
    const again = openCheckIn();
    fireEvent.click(button("Get my location", again));
    expect(get).toHaveBeenCalledWith(expect.any(Function), expect.any(Function), { enableHighAccuracy: true, timeout: 15_000, maximumAge: 0 });
    expect(within(again).getByText("The location could not be read — allow location for this app, or check in manually with a reason")).toBeDefined();
    expect(button("Get my location", again).disabled).toBe(false); // locating ended
  });

  it("refuses a fix coarser than 100 m, and checks in with a good fix and the trimmed QR; the next view focuses the title", async () => {
    locateWith({ coords: { latitude: 3.1, longitude: 101.6, accuracy: 140.4 } });
    const dialog = openCheckIn();
    fireEvent.click(button("Get my location", dialog));
    expect(within(dialog).getByText("Found, accurate to ±140 m — the server checks you are within 200 m of the site")).toBeDefined();
    fireEvent.click(button("Check in & start", dialog));
    expect(within(dialog).getByText(/^Your location is only known to ±140 m/)).toBeDefined();
    expect(checkIn).not.toHaveBeenCalled();
    locateWith({ coords: { latitude: 3.1, longitude: 101.6, accuracy: 12 } });
    fireEvent.click(button("Locate again", dialog));
    expect(within(dialog).queryByText(/^Your location is only known/)).toBeNull(); // locating clears the old error
    fireEvent.change(within(dialog).getByDisplayValue("ac-unit:unit-12345678"), { target: { value: " ac-unit:unit-12345678 " } });
    fireEvent.click(button("Check in & start", dialog));
    expect(checkIn).toHaveBeenCalledWith("job-1", 5, "location_qr", { latitude: 3.1, longitude: 101.6, accuracyMeters: 12 }, "ac-unit:unit-12345678", null);
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    cleanup();
    render(<WorkspaceView live={live({ version: 6, status: "in_progress" })} />);
    expect(document.activeElement?.id).toBe("job-title");
  });

  it("checks in manually with the trimmed reason; an empty reason is refused on its field", async () => {
    const dialog = openCheckIn();
    fireEvent.click(button("Manual (reason)", dialog));
    fireEvent.click(button("Check in & start", dialog));
    expect(within(dialog).getByText("A reason is required for a manual check-in (1–1000 characters)")).toBeDefined();
    await focused("check-in-reason");
    fireEvent.change(within(dialog).getByPlaceholderText("No GPS signal in the basement plant room"), { target: { value: "  Basement  " } });
    expect(within(dialog).queryByText(/^A reason is required/)).toBeNull();
    fireEvent.click(button("Check in & start", dialog));
    expect(checkIn).toHaveBeenCalledWith("job-1", 5, "manual", null, null, "Basement");
  });

  it("shows the Core API's field refusal under its field and focuses it; another refusal goes to the page banner", async () => {
    answer.next = { ok: false, code: "VALIDATION", messageKey: "", fieldErrors: { qrUnitId: "errors.qr_other_unit" } };
    locateWith({ coords: { latitude: 3.1, longitude: 101.6, accuracy: 12 } });
    const dialog = openCheckIn();
    fireEvent.click(button("Get my location", dialog));
    fireEvent.click(button("Check in & start", dialog));
    await focused("check-in-qr");
    expect(within(dialog).getByText(/qr other unit/i)).toBeDefined();
    answer.next = { ok: false, code: "FORBIDDEN", messageKey: "errors.assignment_ended", fieldErrors: {} };
    fireEvent.click(button("Check in & start", dialog));
    expect(await screen.findByText("Not allowed: your work window has ended.")).toBeDefined();
    expect(screen.getByRole("dialog")).toBeDefined(); // the dialog stays open
  });
});

const openParts = () => {
  render(<WorkspaceView live={inWork({ parts: [{ code: "FLT-01", name: "Filter mesh", vanStockQuantity: 3 }, { code: "CAP-35", name: "Capacitor 35µF", vanStockQuantity: null }] })} />);
  fireEvent.click(screen.getByRole("tab", { name: /^Parts & refrigerant/ }));
};

describe("WorkspaceView Add part and Refrigerant", () => {
  it("refuses a part without a name and a bad quantity, focusing the name; a catalog part is added and the form is reset", async () => {
    openParts();
    fireEvent.click(button("Add part"));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("option", { name: "Filter mesh · FLT-01 · 3 in van" })).toBeDefined();
    expect(within(dialog).getByRole("option", { name: "Capacitor 35µF · CAP-35" })).toBeDefined(); // no van stock line
    fireEvent.change(within(dialog).getByDisplayValue("1"), { target: { value: "0" } });
    fireEvent.click(button("Add", dialog));
    expect(within(dialog).getByText("Choose a catalog part or name it")).toBeDefined();
    expect(within(dialog).getByText("Quantity must be 1–999")).toBeDefined();
    await focused("part-name");
    fireEvent.change(within(dialog).getByDisplayValue("Not in the catalog…"), { target: { value: "FLT-01" } });
    expect(within(dialog).queryByText("Part name")).toBeNull(); // a catalog part has its name
    fireEvent.change(within(dialog).getByDisplayValue("0"), { target: { value: "2" } });
    expect(within(dialog).queryByText("Quantity must be 1–999")).toBeNull();
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText("Filter mesh")).toBeDefined();
    expect(screen.getByText("FLT-01")).toBeDefined();
    expect(screen.getByText("Van stock")).toBeDefined();
    expect(screen.getByText("Unsaved changes")).toBeDefined();
    fireEvent.click(button("Add part"));
    expect(within(screen.getByRole("dialog")).getByDisplayValue("Not in the catalog…")).toBeDefined(); // reset for the next part
  });

  it("adds a custom part bought locally that replaces an item, disposed on site by default, with its trimmed lot", async () => {
    openParts();
    fireEvent.click(button("Add part"));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Part name"), { target: { value: "  Drain hose  " } });
    fireEvent.change(within(dialog).getByDisplayValue("Van stock"), { target: { value: "bought_locally" } });
    expect(within(dialog).getByText("Keep the receipt — attach a photo of it with the report photos.")).toBeDefined();
    fireEvent.change(within(dialog).getByLabelText("Lot / serial"), { target: { value: " L-77 " } });
    expect(within(dialog).queryByText("Old part")).toBeNull();
    fireEvent.change(within(dialog).getByDisplayValue("—"), { target: { value: "drain_pan" } });
    expect(within(dialog).getByDisplayValue("Disposed on site")).toBeDefined();
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText("Drain hose")).toBeDefined();
    expect(screen.getByText("custom · replaces Drain pan")).toBeDefined();
    expect(screen.getByText("Bought locally")).toBeDefined();
    expect(screen.getByText("L-77")).toBeDefined();
    expect(screen.getByText(/^Replacement parts: 1 line/)).toBeDefined();
    fireEvent.click(button("Remove"));
    expect(screen.getByText("No parts used.")).toBeDefined();
  });

  it("refuses a refrigerant line without a cylinder or with an emptied amount, then adds it with its net and leak check", async () => {
    openParts();
    fireEvent.click(button("Refrigerant"));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/^Recovered \(kg\)/), { target: { value: "" } });
    fireEvent.click(button("Add", dialog));
    expect(within(dialog).getByText("The cylinder ID is required")).toBeDefined();
    expect(within(dialog).getByText("Kilograms, 0 or more")).toBeDefined(); // the emptied amount, not 0 kg
    await focused("refrigerant-cylinder");
    fireEvent.change(within(dialog).getByPlaceholderText("CYL-R32-0182"), { target: { value: " CYL-9 " } });
    // an error is part of its field's label (IR573)
    fireEvent.change(within(dialog).getByLabelText(/^Recovered \(kg\)/), { target: { value: "1.5" } });
    fireEvent.change(within(dialog).getByLabelText(/^Charged \(kg\)/), { target: { value: "0.25" } });
    expect(within(dialog).queryByText("Kilograms, 0 or more")).toBeNull();
    fireEvent.change(within(dialog).getByDisplayValue("R32"), { target: { value: "R410A" } });
    fireEvent.change(within(dialog).getByLabelText("Method"), { target: { value: " bubble test " } });
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText("Refrigerant (R410A) — cylinder CYL-9")).toBeDefined();
    expect(screen.getByText("−1.25 kg")).toBeDefined(); // net: more recovered than charged
    expect(screen.getByText("passed · bubble test")).toBeDefined();
    expect(screen.getByText(/· R410A −1\.25 kg/)).toBeDefined(); // the work report's summary
  });

  it("a refrigerant line without a leak check has no method", async () => {
    openParts();
    fireEvent.click(button("Refrigerant"));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByPlaceholderText("CYL-R32-0182"), { target: { value: "CYL-1" } });
    fireEvent.change(within(dialog).getByLabelText("Charged (kg)"), { target: { value: "0.4" } });
    fireEvent.change(within(dialog).getByDisplayValue("Pass"), { target: { value: "not_done" } });
    expect(within(dialog).queryByLabelText("Method")).toBeNull();
    fireEvent.click(button("Add", dialog));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(screen.getByText("+0.40 kg")).toBeDefined();
    expect(screen.getByText("not done")).toBeDefined();
  });
});

describe("WorkspaceView checklist", () => {
  it("asks a reason for a result other than normal; Submit report lists the issues, opens the item's tab and focuses it", async () => {
    render(<WorkspaceView live={inWork()} />);
    const filter = screen.getByRole<HTMLSelectElement>("combobox", { name: "Result of Filter" });
    expect(filter.value).toBe(""); // never preselected
    fireEvent.change(filter, { target: { value: "attention" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Result of Drain pan" }), { target: { value: "normal" } });
    expect(screen.getAllByPlaceholderText(/^Reason \(required/)).toHaveLength(1); // normal needs none
    fireEvent.click(screen.getByRole("tab", { name: /^Outdoor/ }));
    fireEvent.click(button("Submit report"));
    expect(screen.getByText(/^Not submitted — \d+ issues:/)).toBeDefined();
    await focused("result-filter"); // the first issue's tab opens again and its result takes the focus
    expect(screen.getByText(/^2 issues block submit/)).toBeDefined();
    // the tab was left and opened again: its reason field is a new element
    fireEvent.change(screen.getByPlaceholderText("Reason (required for Attention)"), { target: { value: "Clogged" } });
    expect(screen.getByText(/^1 issue blocks submit/)).toBeDefined(); // re-checked as the draft changes
  });
});
