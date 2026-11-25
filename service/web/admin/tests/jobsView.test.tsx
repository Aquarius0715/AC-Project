// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ Jobs tab (FR-A06, Figma Admin 06-1, 06-11…06-18), from the data the loader builds: the stage tiles and the
// filters in the URL, the list, the job's header and its hold / resume / cancel / reassign with a reason, New job (the
// customer's preferred times, save as requested or go on to book), booking a preferred time internally or as an
// offer, proposing another time, and classifying a follow-up request.
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
    CoreError, coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))), coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set<string>())),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreItems: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => { try { return await read; } catch { return fallback; } },
  };
});
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return {
    assignInternal: fn(), cancelJob: fn(), classifyFollowUp: fn(), createJob: fn(), extendAccess: fn(), holdJob: fn(), offerToContractor: fn(), proposeSlot: fn(), resolvePartnerSlot: fn(), resumeJob: fn(),
    reviewReport: fn(), saveCosts: fn(), withdrawProposal: fn(),
  };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), push: vi.fn<(href: string) => void>(), toasts: [] as string[] }));
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: nav.push, replace: vi.fn() }), usePathname: () => "/admin/jobs", useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/admin/jobs/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void, onFail?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (!r.ok) { onFail?.(r); return; }
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => {
  type P = Readonly<{ value: string; onChange: (v: string) => void }>;
  return {
    ...(await importOriginal<Record<string, unknown>>()),
    TimeSelect: ({ value, onChange }: P) => <input aria-label="time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
    DateTimeSelect: ({ value, onChange }: P) => <input aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
  };
});

import { loadJobs, type JobsLive } from "../app/admin/jobs/_lib/load";
import { JobsView } from "../app/admin/jobs/_components/jobs-view";

const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const first = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z");
const second = slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z");
const row = (id: string, over: Record<string, unknown> = {}) => ({
  projection: "summary", id, version: 3, unitId: "u-1", type: "reactive", status: "requested", dueAt: first.endAt, requestedSlot: first, scheduledSlot: null, assignmentId: null, origin: "client_request",
  preferredSlots: [first], severity: "normal", displayStatus: "requested", assignmentAcknowledgement: null, technicianMembershipId: null, accessValidFrom: null, accessValidUntil: null,
  createdAt: "2026-09-12T01:00:00Z", followUpOfJobId: null, followUpClass: null, ...over,
});
const job = (over: Record<string, unknown> = {}) => ({
  projection: "detail", id: "job-12345678", version: 4, unitId: "u-1", type: "reactive", status: "requested", symptom: "Water dripping", contactWindow: null, origin: "client_request", planId: null, occurrenceAt: null,
  requestedSlot: first, preferredSlots: [first, second], preferenceRound: 1, slotProposal: null, partnerSlotProposal: null, scheduledSlot: null, dueAt: first.endAt, startedAt: null, completedAt: null,
  contractorOrgId: null, createdAt: "2026-09-12T01:00:00Z", followUpOfJobId: null, followUpClass: null, assignment: null, offer: null, reportRefs: [], draftReportRef: null, costs: null, ...over,
});
const orgs = [
  { id: "org-hq", version: 1, name: "HQ", kind: "operator", status: "active" }, { id: "org-a", version: 1, name: "Alpha", kind: "customer", status: "active" },
  { id: "org-c", version: 2, name: "Cool Partners", kind: "contractor", status: "active" },
];
const members = [
  { id: "m-int", userId: "user-int", displayName: "Ali", role: "technician", employment: "internal", organizationId: "org-hq", validUntil: null },
  { id: "m-int2", userId: "user-int2", displayName: "Badrul", role: "technician", employment: "internal", organizationId: "org-hq", validUntil: null },
];
/** The Jobs tab's data as the loader builds it from these Core API answers. */
async function liveOf(sp: Record<string, string> = {}, w: { job?: unknown; rows?: unknown[]; eligible?: boolean; total?: number } = {}): Promise<JobsLive> {
  const all = new Map<string, unknown>([
    ["organizations.list", orgs], ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a" }]], ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a" }]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }, { id: "u-new", displayName: "Annex AC", customerOrgId: "org-a", propertyId: "p-1", installState: "planned" }]],
    ["members.list", members], ["contractors.list", []],
  ]);
  dal.coreAll.mockImplementation((op) => Promise.resolve(all.get(op) ?? []));
  dal.coreItems.mockImplementation((op) => Promise.resolve(op === "members.eligible" && w.eligible !== false ? [{ id: "m-int", displayName: "Ali" }, { id: "m-int2", displayName: "Badrul" }] : []));
  dal.coreOp.mockImplementation((op, input) => {
    const q = JSON.stringify(input);
    if (op === "jobs.list") return Promise.resolve(q.includes("\"limit\":100") ? { items: w.rows ?? [row("job-12345678"), row("job-2", { status: "assigned" })], total: w.total ?? 2 } : { items: [], total: q.includes("statuses") ? 5 : 1 });
    if (op === "plans.list") return Promise.resolve({ items: [], total: 0 });
    if (op === "jobs.get") return Promise.resolve(w.job ?? job());
    if (op === "units.get") return Promise.resolve({ displayName: "Lobby AC", location: { pathLabels: ["Tower", "Lobby"] } });
    return Promise.resolve({});
  });
  return loadJobs(sp);
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("the jobs list", () => {
  it("filters by stage, type, origin, delivery, assignee, sort and overdue in the URL, and links each job", async () => {
    render(<JobsView live={await liveOf({ from: "2026-09-01T00:00:00Z", to: "2026-09-30T00:00:00Z" })} />);
    expect(screen.getByText(/jobs in scope · in the period$/)).toBeDefined();
    const tile = screen.getAllByRole("button", { pressed: false }).find((b) => /^\D+\d+$/.test(b.textContent)); // a stage tile: its name and count
    fireEvent.click(tile ?? document.body);
    expect(nav.patch).toHaveBeenLastCalledWith(expect.objectContaining({ jobId: null }));
    for (const [label, value, key] of [["Type", "periodic", "type"], ["Origin", "periodic_plan", "origin"], ["Delivery", "org-c", "delivery"], ["Assignee", "m-int", "assignee"], ["Sort", "dueAt", "sort"]] as const) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
      expect(nav.patch).toHaveBeenLastCalledWith({ [key]: value });
    }
    fireEvent.change(screen.getByLabelText("Type"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ type: null });
    fireEvent.change(screen.getByLabelText("Sort"), { target: { value: "status" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ sort: null });
    fireEvent.click(screen.getByRole("checkbox"));
    expect(nav.patch).toHaveBeenLastCalledWith({ overdue: "1" });
    fireEvent.click(screen.getByRole("button", { name: /clear the requested-time period/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ from: null, to: null, jobId: null });
    expect(screen.getAllByRole("link").some((a) => a.getAttribute("href")?.includes("jobId=job-2"))).toBe(true);
  });

  it("clears a stage filter, says when nothing is listed, shows a part of a long list, and a gone job", async () => {
    render(<JobsView live={await liveOf({ stage: "requested", overdue: "1" }, { total: 250 })} />);
    expect(screen.getByText("Showing 2 of 250.")).toBeDefined();
    const pressed = screen.getAllByRole("button", { pressed: true }).find((b) => /\d+$/.test(b.textContent));
    fireEvent.click(pressed ?? document.body);
    expect(nav.patch).toHaveBeenLastCalledWith({ stage: null, jobId: null });
    fireEvent.click(screen.getByRole("checkbox"));
    expect(nav.patch).toHaveBeenLastCalledWith({ overdue: null });
    cleanup();
    render(<JobsView live={await liveOf({}, { rows: [] })} />);
    expect(screen.getByText("Change the scope or the filters.")).toBeDefined();
    expect(screen.getByText("Pick a job.")).toBeDefined();
  });
});

describe("a job's header and reasons", () => {
  it("cancels a requested job with a reason, and says why a refusal failed", async () => {
    actions.cancelJob.mockResolvedValueOnce(ok).mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<JobsView live={await liveOf()} />);
    expect(screen.getByText(/^Repair · Alpha · Tower › Lobby$/)).toBeDefined();
    fireEvent.click(button("Cancel…"));
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Duplicate " } });
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    await flush();
    expect(actions.cancelJob).toHaveBeenCalledWith("job-12345678", 4, " Duplicate ");
    expect(screen.getByText("Job cancelled.")).toBeDefined();
    fireEvent.click(button("Cancel…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Again" } });
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    await flush();
    expect(screen.queryByText("Job cancelled.")).toBeNull(); // the refusal replaces it
    fireEvent.click(button("Cancel…"));
    fireEvent.click(dialog().getByRole("button", { name: "Back" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("holds a running job, resumes it, and reassigns an assigned one to another HQ technician at the same time", async () => {
    for (const f of [actions.holdJob, actions.resumeJob, actions.assignInternal]) f.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: job({ status: "in_progress", scheduledSlot: first }) })} />);
    expect(button("Cancel…").disabled).toBe(true);
    expect(screen.getByText(/Cancel: Put the job on hold first\./)).toBeDefined();
    fireEvent.click(button("Hold…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Parts" } });
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    await flush();
    expect(actions.holdJob).toHaveBeenCalledWith("job-12345678", 4, "Parts");
    cleanup();
    const assignment = { technicianMembershipId: "m-int", scheduledStart: first.startAt, scheduledEnd: first.endAt, status: "active", acknowledgement: "cant_make", cantMakeReason: "Sick", alternativeSlot: null };
    render(<JobsView live={await liveOf({}, { job: job({ status: "assigned", scheduledSlot: first, assignment }) })} />);
    fireEvent.click(button("Reassign…"));
    expect([...dialog().getByLabelText<HTMLSelectElement>(/^New technician/).options].map((o) => o.textContent)).toStrictEqual(["Badrul"]); // not the one who can't make it
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Sick leave" } });
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    await flush();
    expect(actions.assignInternal).toHaveBeenCalledWith("job-12345678", 4, "m-int2", first, "Sick leave");
    expect(nav.toasts).toStrictEqual(["Job put on hold — scheduling and reports are blocked.", "Reassigned — the new technician must accept."]);
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ status: "on_hold" }) })} />);
    fireEvent.click(button("Resume…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Parts in" } });
    fireEvent.click(dialog().getByRole("button", { name: "Confirm" }));
    await flush();
    expect(actions.resumeJob).toHaveBeenCalledWith("job-12345678", 4, "Parts in");
  });
});

describe("booking a requested job", () => {
  it("assigns an HQ technician free at the client's time", async () => {
    actions.assignInternal.mockResolvedValue(ok);
    render(<JobsView live={await liveOf()} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Use this time" })[0] ?? document.body);
    expect(screen.getByText("Book job-1234")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Technician/), { target: { value: "m-int2" } });
    fireEvent.click(dialog().getByRole("button", { name: "Assign" }));
    await flush();
    expect(actions.assignInternal).toHaveBeenCalledWith("job-12345678", 4, "m-int2", first);
    expect(screen.getByText(/^Assigned Badrul for /)).toBeDefined();
  });

  it("offers the client's times to a contractor with the offer and access windows", async () => {
    actions.offerToContractor.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { eligible: false })} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Use this time" })[0] ?? document.body);
    expect(dialog().getByText("No qualified HQ technician is free at this time — offer it to a contractor.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Terms version/), { target: { value: " " } });
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Send offer" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Terms version/), { target: { value: " terms-v2 " } });
    const [expires] = dialog().getAllByLabelText("date and time");
    fireEvent.change(expires ?? document.body, { target: { value: "2026-09-20T09:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Send offer" }));
    await flush();
    expect(actions.offerToContractor).toHaveBeenCalledWith("job-12345678", 4, expect.objectContaining({ contractorOrgId: "org-c", termsVersion: "terms-v2", offerExpiresAt: "2026-09-20T01:00:00.000Z" }));
    expect(screen.getByText(/^Offered .* to Cool Partners\./)).toBeDefined();
  });

  it("proposes another time with held capacity, refusing one of the client's own times", async () => {
    actions.proposeSlot.mockResolvedValue(ok);
    render(<JobsView live={await liveOf()} />);
    fireEvent.click(button("Propose another time…"));
    const [from, to] = dialog().getAllByLabelText("time");
    fireEvent.change(from ?? document.body, { target: { value: "10:00" } });
    fireEvent.change(to ?? document.body, { target: { value: "09:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Send proposal" }));
    expect(dialog().getByText("The end must be after the start")).toBeDefined();
    expect(dialog().getByText("A message is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Date/), { target: { value: "2026-09-25" } });
    fireEvent.change(to ?? document.body, { target: { value: "12:00" } });
    fireEvent.change(dialog().getByLabelText(/^Capacity held/), { target: { value: "c:org-c" } });
    fireEvent.change(dialog().getByLabelText(/^Message to the client/), { target: { value: "Thursday?" } });
    fireEvent.change(dialog().getByLabelText(/^Reply by/), { target: { value: "24" } });
    fireEvent.click(dialog().getByRole("button", { name: "Send proposal" }));
    await flush();
    expect(actions.proposeSlot).toHaveBeenCalledWith("job-12345678", 4, { startAt: "2026-09-25T02:00:00.000Z", endAt: "2026-09-25T04:00:00.000Z" }, { kind: "contractor", contractorOrgId: "org-c", technicianMembershipId: null }, "Thursday?", "2026-09-15T01:00:00.000Z");
    expect(nav.toasts).toStrictEqual(["Proposal sent — waiting for the client"]);
  });

  it("classifies a follow-up request once, with a reason", async () => {
    actions.classifyFollowUp.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: job({ followUpOfJobId: "job-orig-1234", followUpClass: "pending", problemPhotos: [{ id: "ph-1", name: "leak.jpg", status: "ready" }, { id: "ph-2", name: "x.jpg", status: "pending" }] }) })} />);
    expect(screen.getByRole("link", { name: /leak\.jpg/ }).getAttribute("href")).toBe("/admin/jobs/job-12345678/photos/ph-1");
    expect(screen.queryByText("Use this time")).toBeNull(); // classified first
    fireEvent.click(button("Classify…"));
    fireEvent.click(dialog().getByRole("button", { name: "New request" }));
    fireEvent.click(dialog().getByRole("button", { name: "Classify" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Different fault" } });
    fireEvent.click(dialog().getByRole("button", { name: "Classify" }));
    await flush();
    expect(actions.classifyFollowUp).toHaveBeenCalledWith("job-12345678", 4, "new_request", "Different fault");
  });
});

describe("New job", () => {
  it("checks the request, adds and removes preferred times, and saves it as requested", async () => {
    actions.createJob.mockResolvedValue({ ok: true, value: { id: "job-new" } });
    render(<JobsView live={await liveOf()} />);
    fireEvent.click(button(/New job/));
    fireEvent.change(dialog().getByLabelText(/^Unit/), { target: { value: "u-1" } }); // the list starts with the planned Annex AC
    fireEvent.click(dialog().getByRole("button", { name: "Save as requested" }));
    expect(actions.createJob).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Symptom/), { target: { value: "The unit drips water onto the floor." } });
    fireEvent.click(dialog().getByRole("button", { name: /Add another time/ }));
    fireEvent.click(dialog().getByRole("button", { name: /Add another time/ }));
    expect(dialog().queryByRole("button", { name: /Add another time/ })).toBeNull(); // three at most
    fireEvent.click(dialog().getAllByRole("button", { name: "Remove" })[1] ?? document.body);
    fireEvent.click(dialog().getByRole("button", { name: "Preventive" }));
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-09-01T00:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save as requested" }));
    expect(dialog().getByText("The due time cannot be before the end of the 1st time.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save as requested" }));
    await flush();
    expect(actions.createJob).toHaveBeenCalledWith(expect.objectContaining({ unitId: "u-1", type: "preventive", dueAt: null, contactWindow: "9:00–18:00" }));
    expect(JSON.stringify(actions.createJob.mock.lastCall?.[0])).toMatch(/"slots":\[\{[^\]]*\},\{[^\]]*\}\]/); // two preferred times
    expect(nav.push).toHaveBeenCalledWith("/admin/jobs?jobId=job-new&created=requested");
  });

  it("makes an installation job for a planned unit, says why a refusal failed, and opens from an alert", async () => {
    actions.createJob.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: {} }).mockResolvedValueOnce({ ok: true, value: { id: "job-inst" } });
    const live = await liveOf({ new: "u-new" });
    render(<JobsView live={live} />);
    expect(dialog().getByText("Installation")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Symptom/), { target: { value: "Install a new split unit in the annex." } });
    fireEvent.click(dialog().getByRole("button", { name: /Next: choose delivery/ }));
    await flush();
    expect(nav.push).not.toHaveBeenCalled(); // refused: the dialog stays with the reason
    expect(screen.getByRole("dialog")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: /Next: choose delivery/ }));
    await flush();
    expect(actions.createJob).toHaveBeenLastCalledWith(expect.objectContaining({ unitId: "u-new", type: "installation" }));
    expect(nav.push).toHaveBeenLastCalledWith("/admin/jobs?jobId=job-inst&created=book");
    fireEvent.click(button(/New job/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ new: null, alertId: null });
  });

  it("opens step 2 on the job just created, or says it was saved as requested", async () => {
    render(<JobsView live={await liveOf({ jobId: "job-12345678", created: "book" })} />);
    expect(dialog().getByText(/^Created job-1234 \(requested\)/)).toBeDefined();
    expect(nav.patch).toHaveBeenCalledWith({ created: null, jobId: "job-12345678" });
    fireEvent.change(dialog().getByLabelText(/^Visit time/), { target: { value: "2" } });
    fireEvent.click(dialog().getByRole("button", { name: "Leave it requested" }));
    cleanup();
    render(<JobsView live={await liveOf({ jobId: "job-12345678", created: "requested" })} />);
    expect(screen.getByText("Job created as requested — book one of the preferred times or propose another.")).toBeDefined();
  });
});
