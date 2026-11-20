// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiClientJob, ClientRow } from "@ac/web/lib/customerMaintenance";
import type { MaintenanceLive } from "../app/customer/maintenance/_lib/load";

// My requests (FR-C09, FR-C17) in API mode: the status tabs and the Origin filter, the reply and rate banners, New
// request with its checks and the focus on the first field to fix, the request's detail (accept or decline a proposed
// time, another plan time, Confirm & rate, Report a problem with photos, a note, cancel), a request that is gone, and the
// Filter care hand-offs (one Request cleaning prefilled, the batch for every overdue filter with a partial result).
type Done<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  createRequest: vi.fn<(input: unknown) => Promise<Done<{ id: string }>>>(),
  createCleaningRequests: vi.fn<(input: unknown) => Promise<Done<{ ids: string[]; stopped: ActionFailure | null }>>>(),
  respondProposal: vi.fn<(jobId: string, version: number, proposalId: string, answer: unknown) => Promise<Done<null>>>(),
  requestReschedule: vi.fn<(jobId: string, version: number, slots: unknown, comment: string) => Promise<Done<null>>>(),
  cancelRequest: vi.fn<(jobId: string, version: number, reason: string) => Promise<Done<null>>>(),
  addNote: vi.fn<(jobId: string, version: number, message: string) => Promise<Done<null>>>(),
  rateJob: vi.fn<(jobId: string, version: number, stars: number, tags: string[], comment: string) => Promise<Done<null>>>(),
  reportProblem: vi.fn<(form: FormData) => Promise<Done<{ id: string }>>>(),
  markCleaned: vi.fn<(unitId: string) => Promise<Done<null>>>(),
  saveReminders: vi.fn<(input: unknown) => Promise<Done<null>>>(),
}));
vi.mock("../app/customer/maintenance/actions", () => actions);
const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn(), params: new URLSearchParams() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: nav.push, replace: nav.replace }), useSearchParams: () => nav.params }));
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<Done<T>>, ok: string | ((v: T) => string), after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (!r.ok) { failed?.(r); return; }
      runs.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));

import { MaintenanceView } from "../app/customer/maintenance/_components/maintenance-view";

type Detail = NonNullable<MaintenanceLive["detail"]>;
const NOW = "2026-09-15T01:00:00.000Z";
const units = [{ id: "u-bed", name: "Bedroom AC", place: "Home A › 1F" }, { id: "u-liv", name: "Living AC", place: "Home A › 1F" }];
const clientRow = (id: string, over: Partial<ClientRow> = {}): ClientRow => ({
  id, short: id.slice(0, 8), unit: "Bedroom AC", place: "Home A › 1F", origin: "request", type: "Repair", status: "requested", tab: "requested", line: "Preferred 17 Sept", icon: "time", warn: false, ...over,
});
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const apiJob = (over: Partial<ApiClientJob> = {}): ApiClientJob => ({
  projection: "detail", id: "job-0001-aaaa", version: 4, unitId: "u-bed", type: "reactive", status: "requested", symptom: "Water drips from the indoor unit", contactWindow: null, origin: "client_request",
  planId: null, occurrenceAt: null, requestedSlot: slot("2026-09-17T02:00:00Z", "2026-09-17T04:00:00Z"), preferredSlots: [], preferenceRound: 1, slotProposal: null, scheduledSlot: null,
  dueAt: "2026-09-20T00:00:00Z", completedAt: null, contractorOrgId: null, followUpOfJobId: null, followUpClass: null, rating: null, customerConfirmedAt: null, reportRefs: [], assignment: null, ...over,
});
const detail = (over: Partial<Detail> = {}): Detail => ({
  job: apiJob(), unit: units[0] ?? { name: "", place: "" }, facts: [["Unit", "Bedroom AC · Home A › 1F"]], preferred: [], proposal: null, declined: null, plan: null,
  feedback: { kind: "none", text: "", report: false }, notes: [], history: [{ id: "e-1", time: "14 Sept, 9:00 am", title: "Request created" }], report: null, ...over,
});
const common = { units, now: NOW, owner: true, display: { locale: "en" as const, timeZone: "Asia/Kuala_Lumpur" }, page: "requests" as const, requestUnitId: null };
const live = (over: { rows?: ClientRow[]; rateBanner?: { jobId: string; text: string } | null; detail?: Detail; requestUnitId?: string | null; units?: typeof units } = {}): MaintenanceLive => {
  const { detail: shown, ...rest } = over;
  const base = { ...common, rows: [clientRow("job-0001-aaaa")], filters: null, rateBanner: null, ...rest };
  return shown ? { ...base, detail: shown } : { ...base, detail: null };
};
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const dialog = () => screen.getByRole("dialog");
/** a lookup that must have found its element: the test stops at the step that missed it */
const missing = (): never => { throw new Error("element not found"); };
const failure = (code: string, messageKey = "error.x", fieldErrors: Record<string, string> = {}): Done<never> => ({ ok: false, code, messageKey, fieldErrors });
const banner = (text: RegExp | string) => screen.findByText(text);
const photo = (name: string, type: string, size = 10) => new File([new Uint8Array(size)], name, { type });

beforeEach(() => {
  runs.toasts = [];
  nav.push.mockReset();
  nav.replace.mockReset();
  nav.params = new URLSearchParams();
  actions.createRequest.mockReset().mockResolvedValue({ ok: true, value: { id: "job-new-12345" } });
  actions.createCleaningRequests.mockReset().mockResolvedValue({ ok: true, value: { ids: ["job-c-1", "job-c-2"], stopped: null } });
  for (const fn of [actions.respondProposal, actions.requestReschedule, actions.cancelRequest, actions.addNote, actions.rateJob, actions.markCleaned, actions.saveReminders]) fn.mockReset().mockResolvedValue({ ok: true, value: null });
  actions.reportProblem.mockReset().mockResolvedValue({ ok: true, value: { id: "job-follow-9" } });
});
afterEach(() => { cleanup(); });

describe("MaintenanceView requests", () => {
  it("counts the status tabs, filters by status and origin, and says when a filter or the account has no requests", () => {
    const rows = [clientRow("job-req-1"), clientRow("job-plan-1", { origin: "plan", tab: "scheduled", status: "assigned", place: "" }), clientRow("job-done-1", { tab: "completed", status: "completed", warn: true })];
    const { unmount } = render(<MaintenanceView live={live({ rows })} />);
    expect(screen.getByText("3 requests · sorted by status (business order)")).toBeDefined();
    // no reply waiting: no tab for it, and no banner
    expect(screen.queryByRole("tab", { name: /Needs your reply/ })).toBeNull();
    expect(screen.getByRole("tab", { name: /Scheduled/ }).textContent).toBe("Scheduled1");
    fireEvent.click(screen.getByRole("tab", { name: /Completed/ }));
    expect(screen.getAllByRole("link", { name: /job-/ }).map((l) => l.getAttribute("href"))).toStrictEqual(["/customer/maintenance?jobId=job-done-1"]);
    fireEvent.change(screen.getByRole("combobox", { name: "Origin" }), { target: { value: "plan" } });
    expect(screen.getByText("No requests in this filter.")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /^All/ }));
    expect(screen.getAllByRole("link", { name: /job-/ }).map((l) => l.getAttribute("href"))).toStrictEqual(["/customer/maintenance?jobId=job-plan-1"]);
    unmount();
    render(<MaintenanceView live={live({ rows: [] })} />);
    expect(screen.getByText(/No requests yet/)).toBeDefined();
    expect(screen.getByText("0 requests · sorted by status (business order)")).toBeDefined();
  });

  it("the reply banner opens the first request to answer, and the rate banner opens the finished job", () => {
    const rows = [clientRow("job-req-1"), clientRow("job-prop-1", { tab: "reply", status: "time_proposed" }), clientRow("job-prop-2", { tab: "reply", status: "time_proposed" })];
    render(<MaintenanceView live={live({ rows, rateBanner: { jobId: "job-done-7", text: "Work finished — job-done · Bedroom AC." } })} />);
    expect(screen.getByText("A new visit time needs your reply (2). Nothing is booked until you accept.")).toBeDefined();
    fireEvent.click(button("Review"));
    expect(nav.push).toHaveBeenLastCalledWith("/customer/maintenance?jobId=job-prop-1");
    expect(screen.getByRole("tab", { name: /Needs your reply/ }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByText("3 requests · sorted by status (business order)")).toBeDefined();
    fireEvent.click(button("Confirm & rate"));
    expect(nav.push).toHaveBeenLastCalledWith("/customer/maintenance?jobId=job-done-7");
  });

  it("New request needs an AC and 10–2000 characters, takes focus to the first field to fix, and sends the 3 times in the display time zone", async () => {
    render(<MaintenanceView live={live()} />);
    fireEvent.click(button("New maintenance request"));
    fireEvent.click(button("Submit request"));
    expect(within(dialog()).getByText("Choose the AC that needs service")).toBeDefined();
    expect(within(dialog()).getByText("Symptoms must be 10–2000 characters")).toBeDefined();
    await waitFor(() => { expect(document.activeElement?.tagName).toBe("SELECT"); });
    fireEvent.change(within(dialog()).getByRole("combobox", { name: /Unit/ }), { target: { value: "u-liv" } });
    fireEvent.click(button("Submit request"));
    await waitFor(() => { expect(document.activeElement?.tagName).toBe("TEXTAREA"); });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "  Weak cold air today " } });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "call me on 0123456789" } });
    fireEvent.click(button("Submit request"));
    expect(within(dialog()).getByText(/No e-mail addresses or phone numbers/)).toBeDefined();
    expect(actions.createRequest).not.toHaveBeenCalled();
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "Weekdays 09:00-18:00" } });
    fireEvent.click(button("Preventive"));
    fireEvent.click(button("Submit request"));
    await banner(/Request job-new- sent with 3 preferred times/);
    // 2, 3 and 4 days ahead in Kuala Lumpur: 10–12, 14–16, 10–12 local time
    expect(actions.createRequest).toHaveBeenCalledWith({
      unitId: "u-liv", type: "preventive", symptom: "  Weak cold air today ", contactWindow: "Weekdays 09:00-18:00",
      slots: [slot("2026-09-17T02:00:00.000Z", "2026-09-17T04:00:00.000Z"), slot("2026-09-18T06:00:00.000Z", "2026-09-18T08:00:00.000Z"), slot("2026-09-19T02:00:00.000Z", "2026-09-19T04:00:00.000Z")],
    });
    expect(nav.push).toHaveBeenCalledWith("/customer/maintenance?jobId=job-new-12345");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("New request opens for the AC a link named, keeps the only AC chosen, and shows a refusal or a changed request", async () => {
    actions.createRequest.mockResolvedValueOnce(failure("VALIDATION", "error.validation", { symptom: "error.length" })).mockResolvedValueOnce(failure("CONFLICT", "error.versionConflict"));
    const { unmount } = render(<MaintenanceView live={live({ requestUnitId: "u-liv" })} />);
    expect(within(dialog()).getByRole<HTMLSelectElement>("combobox", { name: /Unit/ }).value).toBe("u-liv");
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "The AC smells of burning" } });
    fireEvent.click(button("Submit request"));
    const refused = await banner("symptom: wrong length");
    // a refusal is a crit banner, announced as an alert
    expect(refused.closest("[role=alert]")).not.toBeNull();
    unmount();
    render(<MaintenanceView live={live({ units: units.slice(0, 1) })} />);
    fireEvent.click(button("New maintenance request"));
    expect(within(dialog()).getByRole<HTMLSelectElement>("combobox", { name: /Unit/ }).value).toBe("u-bed");
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "The AC smells of burning" } });
    fireEvent.click(button("Submit request"));
    await banner("Not saved: the request changed meanwhile — the latest state is shown.");
    fireEvent.click(button("New maintenance request"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a request that is gone says so; a closed one offers neither cancel nor a note, and links the job it follows up", () => {
    const { unmount } = render(<MaintenanceView live={{ ...common, rows: [clientRow("job-0001-aaaa")], filters: null, rateBanner: null, detail: null, missing: "job-gone" }} />);
    expect(screen.getByText("That request no longer exists in your account.")).toBeDefined();
    expect(screen.getByText("1 request · sorted by status (business order)")).toBeDefined();
    unmount();
    const job = apiJob({ status: "cancelled", followUpOfJobId: "job-prev-123456", followUpClass: "rework", origin: "client_request" });
    render(<MaintenanceView live={live({ detail: detail({ job, notes: [{ id: "n-1", who: "You", at: "14 Sept", text: "Gate code 4821" }] }) })} />);
    expect(screen.queryByRole("button", { name: "Cancel request" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Add note/ })).toBeNull();
    expect(screen.getByRole("link", { name: "job-prev" }).getAttribute("href")).toBe("/customer/maintenance?jobId=job-prev-123456");
    expect(screen.getByText("Rework (free)", { exact: false })).toBeDefined();
    expect(screen.getByText("“Gate code 4821”", { exact: false })).toBeDefined();
    expect(screen.getByRole("link", { name: "Close" }).getAttribute("href")).toBe("/customer/maintenance");
  });

  it("a proposed time is accepted with the job's version, or declined with or without new times", async () => {
    const job = apiJob({ slotProposal: { id: "prop-1", source: "hq", slot: slot("2026-09-19T06:00:00Z", "2026-09-19T08:00:00Z"), hold: { kind: "internal" }, message: "Earlier slot", replyBy: "2026-09-16T01:00:00Z", status: "pending", decidedAt: null, declineReason: null, declineComment: null }, preferenceRound: 2 });
    const proposal = { id: "prop-1", title: "HQ proposes a new time", when: "Sat, 19 Sept, 2:00 – 4:00 pm MYT", who: "An HQ technician", message: "Earlier slot", replyBy: "16 Sept, 9:00 am", late: false };
    const d = detail({ job, proposal, preferred: [{ text: "1st · Thu, 17 Sept", note: null }, { text: "2nd · Fri, 18 Sept", note: "booked" }] });
    render(<MaintenanceView live={live({ detail: d })} />);
    expect(screen.getByText("Your preferred times (round 2)")).toBeDefined();
    expect(screen.getByText("Reply by 16 Sept, 9:00 am")).toBeDefined();
    fireEvent.click(button("Accept this time"));
    await banner("Time accepted — Sat, 19 Sept, 2:00 – 4:00 pm MYT is booked; the technician confirms it next.");
    expect(actions.respondProposal).toHaveBeenLastCalledWith("job-0001-aaaa", 4, "prop-1", { decision: "accept" });
    fireEvent.click(button("Decline…"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Too late for me" }));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment to HQ/ }), { target: { value: "Mornings only" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline & send new times" }));
    await banner("Declined — HQ looks at your 3 new times.");
    expect(actions.respondProposal.mock.lastCall?.[3]).toMatchObject({ decision: "decline", declineReason: "too_late", comment: "Mornings only", slots: [{ startAt: "2026-09-18T02:00:00.000Z" }, {}, {}] });
    fireEvent.click(button("Decline…"));
    fireEvent.click(within(dialog()).getByRole("checkbox"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline" }));
    await banner("Declined — HQ will look again.");
    expect(actions.respondProposal.mock.lastCall?.[3]).toMatchObject({ decision: "decline", declineReason: "not_home", slots: [] });
    actions.respondProposal.mockResolvedValueOnce(failure("CONFLICT", "errors.proposal_expired"));
    fireEvent.click(button("Accept this time"));
    await banner("Not saved: the proposal has expired — HQ will look again.");
  });

  it("a plan visit less than 48 h ahead cannot be moved; one further ahead asks HQ for another time", async () => {
    const plan = apiJob({ origin: "periodic_plan", status: "assigned", scheduledSlot: slot("2026-09-25T02:00:00Z", "2026-09-25T04:00:00Z") });
    const { unmount } = render(<MaintenanceView live={live({ detail: detail({ job: plan, plan: { can: false, why: "Less than 48 h before the visit — contact HQ to change it." } }) })} />);
    expect(button("Request another time…").hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Less than 48 h before the visit — contact HQ to change it.")).toBeDefined();
    expect(screen.getByText(/periodic inspection · job-0001/)).toBeDefined();
    // a plan visit is never cancelled by the client
    expect(screen.queryByRole("button", { name: "Cancel request" })).toBeNull();
    unmount();
    render(<MaintenanceView live={live({ detail: detail({ job: plan, plan: { can: true, why: null } }) })} />);
    fireEvent.click(button("Request another time…"));
    expect(within(dialog()).getByText(/periodic visit 25 Sept 2026, 10:00 am/)).toBeDefined();
    // the 3rd time at 10–12 on the day of the 1st
    fireEvent.change(within(dialog()).getAllByLabelText(/^Date/)[2] ?? missing(), { target: { value: "2026-09-18" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send to HQ" }));
    expect(within(dialog()).getByText("The 3 times must be different.")).toBeDefined();
    expect(actions.requestReschedule).not.toHaveBeenCalled();
    fireEvent.change(within(dialog()).getAllByLabelText(/^Date/)[2] ?? missing(), { target: { value: "2026-09-21" } });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment/ }), { target: { value: "Away" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send to HQ" }));
    await banner(/Asked HQ for another time/);
    expect(actions.requestReschedule.mock.lastCall?.slice(0, 2)).toStrictEqual(["job-0001-aaaa", 4]);
    expect(actions.requestReschedule.mock.lastCall?.[3]).toBe("Away");
  });

  it("a completed job shows the report and is confirmed and rated with stars and tags; a low rating points to Report a problem", async () => {
    const job = apiJob({ status: "completed", completedAt: "2026-09-14T03:00:00Z" });
    const report = {
      version: 2, accepted: "14 Sept, 1:00 pm", items: [{ id: "i-1", label: "Indoor unit — filter", result: "Attention", tone: "warn" as const, reason: "dusty" }, { id: "i-2", label: "Outdoor unit — fan", result: "Normal", tone: "ok" as const, reason: null }],
      readings: [["Supply air temperature", "12.5 °C"]] satisfies [string, string][], parts: ["Filter × 2"], workText: "Cleaned the filter.", photos: [{ id: "a-1", name: "before.jpg", url: "data:image/jpeg;base64,QUJD" }],
    };
    render(<MaintenanceView live={live({ detail: detail({ job, report, feedback: { kind: "rate", text: "Is everything working?", report: true } }) })} />);
    expect(screen.getByText("1 checks OK · parts: Filter × 2")).toBeDefined();
    expect(screen.getByText("v2 · accepted 14 Sept, 1:00 pm")).toBeDefined();
    expect(screen.getByRole("img", { name: "before.jpg" }).getAttribute("src")).toBe("data:image/jpeg;base64,QUJD");
    expect(screen.queryByRole("button", { name: /Add note/ })).toBeNull();
    fireEvent.click(button("Confirm & rate"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Confirm & submit" }));
    expect(within(dialog()).getByText("Choose 1–5 stars")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "2 stars" }));
    expect(within(dialog()).getByText(/What went wrong\?/)).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "5 stars" }));
    expect(within(dialog()).queryByText(/What went wrong\?/)).toBeNull();
    fireEvent.click(within(dialog()).getByRole("button", { name: "On time" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Polite" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "On time" }));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment/ }), { target: { value: "Tidy" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Confirm & submit" }));
    await banner("Thanks — job confirmed and rated (5 of 5 stars).");
    expect(actions.rateJob).toHaveBeenLastCalledWith("job-0001-aaaa", 4, 5, ["Polite"], "Tidy");
  });

  it("a rating still editable is edited from its stars; a report not yet accepted is said to come later", async () => {
    const job = apiJob({ status: "completed", rating: { stars: 3, tags: ["On time"], comment: "ok", ratedAt: "2026-09-14T04:00:00Z", editableUntil: "2026-09-21T04:00:00Z" } });
    render(<MaintenanceView live={live({ detail: detail({ job, feedback: { kind: "edit", stars: 3, text: " · On time", report: false } }) })} />);
    expect(screen.getByText("visible once HQ accepts it")).toBeDefined();
    expect(screen.getByRole("img", { name: "3 of 5 stars" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "Report a problem" })).toBeNull();
    fireEvent.click(button("Edit rating"));
    expect(within(dialog()).getByRole("button", { name: "3 stars" }).getAttribute("aria-pressed")).toBe("true");
    expect(within(dialog()).getByRole("button", { name: "On time" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(dialog()).getByRole("button", { name: "Save rating" }));
    await banner("Thanks — rating updated (3 of 5 stars).");
    expect(actions.rateJob).toHaveBeenLastCalledWith("job-0001-aaaa", 4, 3, ["On time"], "ok");
  });

  it("Report a problem checks the details, the photos and the visit, then sends them as form data and opens the follow-up", async () => {
    const job = apiJob({ status: "completed", rating: { stars: 2, tags: [], comment: null, ratedAt: "2026-09-14T04:00:00Z", editableUntil: "2026-09-14T05:00:00Z" } });
    render(<MaintenanceView live={live({ detail: detail({ job, feedback: { kind: "rated", stars: 2, text: "", report: true } }) })} />);
    fireEvent.click(button("Report a problem"));
    const files = within(dialog()).getByLabelText(/Photos/);
    fireEvent.change(files, { target: { files: Array.from({ length: 6 }, (_, k) => photo(`p${String(k)}.jpg`, "image/jpeg")) } });
    expect(within(dialog()).getByText("Up to 5 photos.")).toBeDefined();
    fireEvent.change(files, { target: { files: [photo("scan.pdf", "application/pdf")] } });
    expect(within(dialog()).getByText("Photos must be JPEG or PNG up to 5 MiB each.")).toBeDefined();
    fireEvent.change(files, { target: { files: [photo("a.jpg", "image/jpeg", 3 * 1024 * 1024), photo("b.png", "image/png", 3 * 1024 * 1024)] } });
    expect(within(dialog()).getByText("The photos together may be 5.5 MB at most.")).toBeDefined();
    fireEvent.change(files, { target: { files: [photo("a.jpg", "image/jpeg")] } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    expect(within(dialog()).getByText("Describe the problem (10–2000 characters)")).toBeDefined();
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Details/ }), { target: { value: "Dripping again since Monday" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "New damage" }));
    fireEvent.click(within(dialog()).getByRole("checkbox"));
    fireEvent.change(within(dialog()).getByLabelText(/^Date/), { target: { value: "2026-09-15" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    expect(within(dialog()).getByText("Times start on a later day than today (Kuala Lumpur).")).toBeDefined();
    fireEvent.change(within(dialog()).getByLabelText(/^Date/), { target: { value: "2026-09-18" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    await banner("Problem reported — follow-up job-foll is under HQ review.");
    const form = actions.reportProblem.mock.lastCall?.[0];
    expect([...(form?.entries() ?? [])].map(([k, v]) => [k, typeof v === "string" ? v : v.name])).toStrictEqual([
      ["jobId", "job-0001-aaaa"], ["version", "4"], ["reasonCode", "new_damage"], ["details", "Dripping again since Monday"], ["photos", "a.jpg"],
      ["visitStart", "2026-09-18T02:00:00.000Z"], ["visitEnd", "2026-09-18T04:00:00.000Z"],
    ]);
    expect(nav.push).toHaveBeenCalledWith("/customer/maintenance?jobId=job-follow-9");
  });

  it("an open request takes a note of 1–2000 characters and is cancelled with a reason", async () => {
    render(<MaintenanceView live={live({ detail: detail() })} />);
    fireEvent.click(button(/Add note/));
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: " ".repeat(3) } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send note" }));
    expect(within(dialog()).getByText("Write 1–2000 characters")).toBeDefined();
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Gate code 4821" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send note" }));
    await banner(/Note sent/);
    expect(actions.addNote).toHaveBeenLastCalledWith("job-0001-aaaa", 4, "Gate code 4821");
    fireEvent.click(button("Cancel request"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel request" }));
    expect(within(dialog()).getByText("A reason is required")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Keep request" }));
    fireEvent.click(button("Cancel request"));
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Resolved itself" } });
    actions.cancelRequest.mockResolvedValueOnce(failure("UNPROCESSABLE", "error.invalidState"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel request" }));
    await banner("not possible in the request's current state.");
    fireEvent.click(button("Cancel request"));
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Resolved itself" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel request" }));
    await banner("Request cancelled.");
    expect(actions.cancelRequest).toHaveBeenLastCalledWith("job-0001-aaaa", 4, "Resolved itself");
  });

  it("the Filter care tab is asked for through the URL", () => {
    render(<MaintenanceView live={live()} />);
    fireEvent.click(screen.getByRole("tab", { name: "Filter care" }));
    expect(nav.replace).toHaveBeenCalledWith("/customer/maintenance?tab=filter-care", { scroll: false });
  });

  it("from Filter care, My requests is asked for through the URL", () => {
    const filters = { lines: [], settings: { id: "fs", version: 0, customerId: "c", thresholdHours: null, fallbackDays: 90, recipients: "owners" as const, channels: ["inApp" as const] }, reminders: [], form: { useDefault: true, hours: "250", days: "90", recipients: "owners" as const, email: false } };
    render(<MaintenanceView live={{ ...common, page: "filters", rows: [], rateBanner: null, detail: null, filters }} />);
    fireEvent.click(screen.getByRole("tab", { name: "My requests" }));
    expect(nav.replace).toHaveBeenCalledWith("/customer/maintenance", { scroll: false });
  });

  it("every dialog closes by Escape or its own button without sending anything", () => {
    const closeBoth = (open: () => void, name: string) => {
      open();
      fireEvent.keyDown(dialog(), { key: "Escape" });
      expect(screen.queryByRole("dialog")).toBeNull();
      open();
      fireEvent.click(within(dialog()).getByRole("button", { name }));
      expect(screen.queryByRole("dialog")).toBeNull();
    };
    const proposal = { id: "prop-1", title: "HQ proposes a new time", when: "Sat, 19 Sept", who: "An HQ technician", message: "Earlier", replyBy: "16 Sept", late: false };
    const { unmount } = render(<MaintenanceView live={live({ detail: detail({ proposal }) })} />);
    closeBoth(() => { fireEvent.click(button("New maintenance request")); }, "Cancel");
    closeBoth(() => { fireEvent.click(button("Decline…")); }, "Back");
    closeBoth(() => { fireEvent.click(button(/Add note/)); }, "Cancel");
    closeBoth(() => { fireEvent.click(button("Cancel request")); }, "Keep request");
    unmount();
    const plan = apiJob({ origin: "periodic_plan", status: "assigned" });
    const r = render(<MaintenanceView live={live({ detail: detail({ job: plan, plan: { can: true, why: null } }) })} />);
    closeBoth(() => { fireEvent.click(button("Request another time…")); }, "Cancel");
    // a plan visit without its time yet names none
    fireEvent.click(button("Request another time…"));
    expect(within(dialog()).getByText("job-0001 · periodic visit · up to 48 h before the visit")).toBeDefined();
    r.unmount();
    render(<MaintenanceView live={live({ detail: detail({ job: apiJob({ status: "completed" }), feedback: { kind: "rate", text: "Is everything working?", report: true } }) })} />);
    closeBoth(() => { fireEvent.click(button("Confirm & rate")); }, "Not now");
    closeBoth(() => { fireEvent.click(button("Report a problem")); }, "Cancel");
    expect([actions.createRequest, actions.respondProposal, actions.addNote, actions.cancelRequest, actions.requestReschedule, actions.rateJob, actions.reportProblem].map((fn) => fn.mock.calls.length)).toStrictEqual([0, 0, 0, 0, 0, 0, 0]);
  });

  it("shows round 1 times, a declined proposal, a report without parts or accepted time, and groups ACs without a property as Other", () => {
    const job = apiJob({ status: "completed" });
    const report = { version: 1, accepted: null, items: [], readings: [], parts: [], workText: "", photos: [] };
    render(<MaintenanceView live={live({ units: [...units, { id: "u-x", name: "Loose AC", place: "" }], detail: detail({ job, report, preferred: [{ text: "1st · Thu", note: null }], declined: "You declined the proposed time (too late). HQ is looking at your new times." }) })} />);
    expect(screen.getByText("Your preferred times")).toBeDefined();
    expect(screen.getByText(/You declined the proposed time \(too late\)/)).toBeDefined();
    expect(screen.getByText("0 checks OK · no parts replaced")).toBeDefined();
    expect(screen.getByText("v1")).toBeDefined();
    fireEvent.click(button("New maintenance request"));
    expect(within(dialog()).getByRole("group", { name: "Other" })).toBeDefined();
  });

  it("New request takes focus to the times when one ends before it starts, and the contact window last", async () => {
    render(<MaintenanceView live={live({ units: units.slice(0, 1) })} />);
    fireEvent.click(button("New maintenance request"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "The AC smells of burning" } });
    const [fromHour] = within(within(dialog()).getByRole("group", { name: "From 1" })).getAllByRole("combobox");
    fireEvent.change(fromHour ?? missing(), { target: { value: "13" } });
    fireEvent.click(button("Submit request"));
    expect(within(dialog()).getByText("Each time is 1–4 hours long.")).toBeDefined();
    await waitFor(() => { expect(document.activeElement?.getAttribute("type")).toBe("date"); });
    const [toHour] = within(within(dialog()).getByRole("group", { name: "To 1" })).getAllByRole("combobox");
    fireEvent.change(toHour ?? missing(), { target: { value: "15" } });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "me@example.com" } });
    fireEvent.click(button("Submit request"));
    await waitFor(() => { expect(document.activeElement).toBe(within(dialog()).getByRole("textbox", { name: /Contact window/ })); });
    expect(actions.createRequest).not.toHaveBeenCalled();
  });

  it("a decline needs a comment of 1000 characters at most, and sends nothing once the proposal is gone", () => {
    const proposal = { id: "prop-1", title: "HQ proposes a new time", when: "Sat, 19 Sept", who: "An HQ technician", message: "Earlier", replyBy: "16 Sept", late: false };
    const { rerender } = render(<MaintenanceView live={live({ detail: detail({ proposal }) })} />);
    fireEvent.click(button("Decline…"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment to HQ/ }), { target: { value: "x".repeat(1001) } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline & send new times" }));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment to HQ/ }), { target: { value: "ok" } });
    // HQ withdrew the proposal meanwhile: the page re-read without it
    rerender(<MaintenanceView live={live({ detail: detail() })} />);
    expect(within(dialog()).getByText("job-0001 · Bedroom AC · proposed")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline & send new times" }));
    expect(actions.respondProposal).not.toHaveBeenCalled();
  });

  it("the batch needs an AC once it was tried, and says Not all requests were sent when a refusal stopped it", async () => {
    const lines = [{ kind: "unit" as const, row: { unitId: "u-bed", unit: "Bedroom AC", place: "", spaceId: null, offline: false, jobId: null, run: "300 h", runNote: null, state: "overdue" as const, threshold: 250, hours: 300, fallbackDays: 90, last: "last cleaned 21 Jul", pct: 100, progress: "120 %", days: 55 } },
      { kind: "unit" as const, row: { unitId: "u-liv", unit: "Living AC", place: "Home A", spaceId: null, offline: false, jobId: null, run: "280 h", runNote: null, state: "overdue" as const, threshold: 250, hours: 280, fallbackDays: 90, last: "last cleaned 21 Jul", pct: 100, progress: "112 %", days: 55 } }];
    const filters = { lines, settings: { id: "fs", version: 0, customerId: "c", thresholdHours: null, fallbackDays: 90, recipients: "owners" as const, channels: ["inApp" as const] }, reminders: [], form: { useDefault: true, hours: "250", days: "90", recipients: "owners" as const, email: false } };
    render(<MaintenanceView live={{ ...common, page: "filters", rows: [], rateBanner: null, detail: null, filters }} />);
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "me@example.com" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    for (const box of within(dialog()).getAllByRole("checkbox")) fireEvent.click(box);
    expect(within(dialog()).getByText("Choose at least one AC")).toBeDefined();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    actions.createCleaningRequests.mockResolvedValueOnce({ ok: true, value: { ids: ["job-c-1"], stopped: { code: "CONFLICT", messageKey: "error.x", fieldErrors: {} } } });
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    await screen.findByText(/^1 of 2 cleaning requests sent/);
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    await waitFor(() => { expect(runs.toasts).toStrictEqual(["Not all requests were sent", "Requests sent"]); });
  });

  it("Report a problem's visit can be suggested and taken back", async () => {
    render(<MaintenanceView live={live({ detail: detail({ job: apiJob({ status: "completed" }), feedback: { kind: "rate", text: "", report: true } }) })} />);
    fireEvent.click(button("Report a problem"));
    const suggest = within(dialog()).getByRole("checkbox");
    fireEvent.click(suggest);
    expect(within(dialog()).getByText("Preferred visit")).toBeDefined();
    fireEvent.click(suggest);
    expect(within(dialog()).queryByText("Preferred visit")).toBeNull();
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Details/ }), { target: { value: "Dripping again since Monday" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send report" }));
    await screen.findByText(/Problem reported/);
    expect([...(actions.reportProblem.mock.lastCall?.[0].keys() ?? [])]).toStrictEqual(["jobId", "version", "reasonCode", "details"]);
  });
});
