// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The cards of an HQ job (FR-A06, Figma Admin 06-11…06-18), from the data the loader builds: a proposal waiting for the
// client (withdraw), a contractor's time change (keep, or send to the client), the delivery (can't make it, overdue,
// extend the access), the work report (accept, return for rework, accept by escalation, the full report), the cost
// lines (add, remove) and the history. The loader and mocks are those of jobsView.test.tsx.
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
async function liveOf(sp: Record<string, string> = {}, w: { job?: unknown; rows?: unknown[]; eligible?: boolean; total?: number; report?: unknown; events?: unknown[] } = {}): Promise<JobsLive> {
  const all = new Map<string, unknown>([
    ["organizations.list", orgs], ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a" }]], ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a" }]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }, { id: "u-new", displayName: "Annex AC", customerOrgId: "org-a", propertyId: "p-1", installState: "planned" }]],
    ["members.list", members], ["contractors.list", []],
  ]);
  dal.coreAll.mockImplementation((op) => Promise.resolve(all.get(op) ?? []));
  dal.coreItems.mockImplementation((op) => {
    if (op === "jobs.events") return Promise.resolve(w.events ?? []);
    return Promise.resolve(op === "members.eligible" && w.eligible !== false ? [{ id: "m-int", displayName: "Ali" }, { id: "m-int2", displayName: "Badrul" }] : []);
  });
  dal.coreOp.mockImplementation((op, input) => {
    const q = JSON.stringify(input);
    if (op === "jobs.list") return Promise.resolve(q.includes("\"limit\":100") ? { items: w.rows ?? [row("job-12345678"), row("job-2", { status: "assigned" })], total: w.total ?? 2 } : { items: [], total: q.includes("statuses") ? 5 : 1 });
    if (op === "plans.list") return Promise.resolve({ items: [], total: 0 });
    if (op === "jobs.get") return Promise.resolve(w.job ?? job());
    if (op === "units.get") return Promise.resolve({ displayName: "Lobby AC", location: { pathLabels: ["Tower", "Lobby"] } });
    if (op === "reports.get") return Promise.resolve(w.report);
    if (op === "attachments.getContent") return Promise.resolve({ mime: "image/jpeg", bytes: "AAAA" });
    return Promise.resolve({});
  });
  return loadJobs(sp);
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

const report = (over: Record<string, unknown> = {}) => ({
  id: "rep-1", version: 2, jobId: "job-12345678", authorId: "user-int", reviewAvailability: { allowed: true, reason: null }, workText: "Cleaned the drain pan",
  items: [
    { id: "it-1", componentGroup: "indoor", componentKey: "drain_pan", result: "attention", reason: "Slow drain", evidenceIds: ["a-1"], authorId: "user-int", observedAt: "2026-09-13T01:00:00Z" },
    { id: "it-2", componentGroup: "indoor", componentKey: "filter", result: "normal", reason: null, evidenceIds: [], authorId: "user-int", observedAt: "2026-09-13T01:00:00Z" },
  ],
  measurements: [{ id: "me-1", metric: "supply_air_temp", value: 14.5, unit: "°C", quality: "valid", observedAt: "2026-09-13T01:00:00Z" }],
  parts: [{ name: "Drain hose", quantity: 1, source: "hq_stock", replacesComponentKey: "drain_hose" }],
  refrigerant: [{ refrigerant: "R32", cylinderId: "cyl-9", recoveredKg: 0.2, chargedKg: 0.3, leakCheck: "passed" }],
  signOff: { signerName: "Mr Tan", signedAt: "2026-09-13T02:00:00Z", absentReason: null }, nextAction: { kind: "follow_up", date: "2026-10-01", note: "Check the slope" },
  attachmentRefs: [{ id: "a-1", name: "pan.jpg", mime: "image/jpeg", size: 1000, status: "ready" }], submittedAt: "2026-09-13T02:00:00Z", acceptedAt: null,
  reviewHistory: [{ reviewerUserId: "user-int2", reportVersion: 1, decision: "return", reason: "Photo missing", occurredAt: "2026-09-12T05:00:00Z" }],
  ...over,
});
const proposal = (source: string) => ({ source, id: "sp-1", slot: second, hold: { kind: "internal", membershipId: "m-int" }, message: "Thursday?", replyBy: "2026-09-16T00:00:00Z", status: "pending", decidedAt: null, declineReason: null, declineComment: null });
const submitted = (over: Record<string, unknown> = {}) => job({ status: "submitted", reportRefs: [{ reportId: "rep-0", reportVersion: 1 }, { reportId: "rep-1", reportVersion: 2 }], ...over });
const offer = { id: "of-1", contractorOrgId: "org-c", candidateSlots: [first, second], offerExpiresAt: "2026-09-15T00:00:00Z", accessValidFrom: "2026-09-14T00:00:00Z", accessValidUntil: "2026-09-30T00:00:00Z", decision: "accept", decidedAt: "2026-09-13T00:00:00Z", declineReason: null };
const assignment = (over: Record<string, unknown> = {}) => ({ technicianMembershipId: "m-int", scheduledStart: first.startAt, scheduledEnd: first.endAt, status: "active", acknowledgement: "accepted", cantMakeReason: null, alternativeSlot: null, ...over });

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("proposals and the delivery", () => {
  it("withdraws HQ's proposal waiting for the client, and only shows a contractor's one", async () => {
    actions.withdrawProposal.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: job({ slotProposal: proposal("hq") }) })} />);
    expect(screen.getByText("Proposal sent — waiting for the client")).toBeDefined();
    expect(screen.getByText("Ali — booked automatically when the client accepts")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Use this time" })).toBeNull();
    fireEvent.click(button("Withdraw proposal"));
    await flush();
    expect(actions.withdrawProposal).toHaveBeenCalledWith("job-12345678", 4, "sp-1");
    expect(screen.getByText("Proposal withdrawn — the job is back to its preferred times.")).toBeDefined();
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ slotProposal: proposal("contractor") }) })} />);
    expect(screen.getByText("Partner’s time sent to the client — waiting")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Withdraw proposal" })).toBeNull();
  });

  it("says when the client declined, when no time is agreed, and titles a plan's or a later round's times", async () => {
    const declined = { id: "sp-1", source: "hq", slot: second, hold: { kind: "internal", membershipId: "m-int" }, message: "x", replyBy: "2026-09-16T00:00:00Z", status: "declined", decidedAt: "2026-09-13T00:00:00Z", declineReason: "not_home", declineComment: "Away that week" };
    render(<JobsView live={await liveOf({}, { job: job({ slotProposal: declined, preferenceRound: 2 }) })} />);
    expect(screen.getByText(/^The client declined the proposed time/)).toBeDefined();
    expect(screen.getByText(/not home: “Away that week”/)).toBeDefined();
    expect(screen.getByText("Client’s preferred times (round 2) — book one of these")).toBeDefined();
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ preferredSlots: [] }) })} />);
    expect(screen.getByText(/^No agreed time on this job/)).toBeDefined();
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ preferredSlots: [], planId: "plan-1", occurrenceAt: first.startAt, origin: "periodic_plan" }) })} />);
    expect(screen.getByText("Plan occurrence — book this time")).toBeDefined();
  });

  it("keeps the agreed time or sends the contractor's time to the client", async () => {
    actions.resolvePartnerSlot.mockResolvedValue(ok);
    const pp = { id: "pp-1", offerId: "of-1", slot: second, technicianMembershipId: "m-ext-gone", reason: "Traffic", sentAt: "2026-09-13T02:00:00Z", status: "pending" };
    render(<JobsView live={await liveOf({}, { job: job({ offer, status: "accepted", contractorOrgId: "org-c", partnerSlotProposal: pp }) })} />);
    expect(screen.getByText("Cool Partners proposes a different time")).toBeDefined();
    fireEvent.click(button("Keep the agreed time"));
    await flush();
    expect(actions.resolvePartnerSlot).toHaveBeenLastCalledWith("job-12345678", 4, "pp-1", "keep");
    fireEvent.click(button("Send to client for approval"));
    await flush();
    expect(actions.resolvePartnerSlot).toHaveBeenLastCalledWith("job-12345678", 4, "pp-1", "send_to_client", "2026-09-16T01:00:00.000Z"); // 48 h, before the time starts
    expect(screen.getByText("Sent to the client for approval (reply within 48 h).")).toBeDefined();
  });

  it("extends a contractor's access with a later end and a reason", async () => {
    actions.extendAccess.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: job({ offer, status: "assigned", contractorOrgId: "org-c", assignment: assignment() }) })} />);
    expect(screen.getByText("Delivery · contractor Cool Partners")).toBeDefined();
    fireEvent.click(button("Extend access…"));
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-09-20T00:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Extend" }));
    expect(dialog().getByText("The new end must be later than the current end and in the future.")).toBeDefined();
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-10-02T18:00" } });
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Rework visit" } });
    fireEvent.click(dialog().getByRole("button", { name: "Extend" }));
    await flush();
    expect(actions.extendAccess).toHaveBeenCalledWith("job-12345678", 4, "2026-10-02T10:00:00.000Z", "Rework visit");
    fireEvent.click(button("Extend access…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers a reassignment or another time when the technician can't make it, and another time when the job is overdue", async () => {
    render(<JobsView live={await liveOf({}, { job: job({ status: "assigned", scheduledSlot: first, assignment: assignment({ acknowledgement: "cant_make", cantMakeReason: "Sick" }) }) })} />);
    expect(screen.getByText("Delivery · internal")).toBeDefined();
    expect(button("Reassign…")).toBeDefined();
    fireEvent.click(screen.getAllByRole("button", { name: "Propose another time…" })[0] ?? document.body);
    expect(dialog().getByText("Propose another time to the client")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    cleanup();
    const late = slot("2026-09-10T02:00:00Z", "2026-09-10T04:00:00Z");
    render(<JobsView live={await liveOf({}, { job: job({ status: "assigned", dueAt: late.endAt, scheduledSlot: late, assignment: assignment({ scheduledStart: late.startAt, scheduledEnd: late.endAt }) }) })} />);
    expect(screen.getAllByRole("button", { name: "Propose another time…" }).length).toBeGreaterThan(0);
  });
});

describe("the work report", () => {
  it("shows the report and accepts an internal job's report at once", async () => {
    actions.reviewReport.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: submitted({ draftReportRef: { reportId: "rep-3", reportVersion: 3 } }), report: report() })} />);
    expect(screen.getByText("Work report · version 2")).toBeDefined();
    expect(screen.getByRole("img", { name: "pan.jpg" }).getAttribute("src")).toBe("data:image/jpeg;base64,AAAA");
    expect(screen.getByText("Work: Cleaned the drain pan")).toBeDefined();
    expect(screen.getByText(/^v1 returned .* — “Photo missing”$/)).toBeDefined();
    expect(screen.getByText("1 check OK")).toBeDefined();
    expect(screen.getAllByText("A new version is being drafted by the technician.").length).toBeGreaterThan(0);
    fireEvent.click(button("Accept report"));
    await flush();
    expect(actions.reviewReport).toHaveBeenCalledWith("job-12345678", 4, 2, "accept", "normal", "");
    expect(screen.getByText(/^Report accepted — the job is completed\./)).toBeDefined();
  });

  it("opens the full report with its readings, parts, refrigerant, next action and sign-off", async () => {
    render(<JobsView live={await liveOf({}, { job: submitted(), report: report() })} />);
    fireEvent.click(button(/Open report/));
    expect(dialog().getByText("Drain hose × 1")).toBeDefined();
    expect(dialog().getByText(/^R32 · cylinder cyl-9$/)).toBeDefined();
    expect(dialog().getByText("Follow-up 2026-10-01 — Check the slope")).toBeDefined();
    expect(dialog().getByText(/Mr Tan/)).toBeDefined();
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("returns a report for rework with the ticked items and a reason", async () => {
    actions.reviewReport.mockResolvedValue(ok);
    render(<JobsView live={await liveOf({}, { job: submitted(), report: report() })} />);
    fireEvent.click(button("Return for rework…"));
    fireEvent.click(dialog().getByRole("button", { name: "Return for rework" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    const ticks = dialog().getAllByRole("checkbox");
    fireEvent.click(ticks[0] ?? document.body);
    fireEvent.click(ticks[0] ?? document.body);
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Re-check the slope" } });
    fireEvent.click(dialog().getByRole("button", { name: "Return for rework" }));
    await flush();
    const reason = String(actions.reviewReport.mock.lastCall?.[5]);
    expect(actions.reviewReport.mock.lastCall?.slice(0, 5)).toStrictEqual(["job-12345678", 4, 2, "return", "normal"]);
    expect(reason).toContain("Re-check the slope");
    expect(screen.getByText(/^Returned v2 for rework/)).toBeDefined();
  });

  it("accepts a contractor's report only by escalation with a reason, and says when HQ may not review", async () => {
    actions.reviewReport.mockResolvedValue({ ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {} });
    render(<JobsView live={await liveOf({}, { job: submitted({ offer, contractorOrgId: "org-c", assignment: assignment() }), report: report() })} />);
    fireEvent.click(button("Accept report…"));
    expect(dialog().getByText(/HQ escalation/)).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Accept report" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "x".repeat(10) } });
    fireEvent.click(dialog().getByRole("button", { name: "Accept report" }));
    await flush();
    expect(actions.reviewReport).toHaveBeenCalledWith("job-12345678", 4, 2, "accept", "hq_escalation", "xxxxxxxxxx");
    expect(screen.queryByRole("dialog")).toBeNull(); // the refusal shows on the job
    cleanup();
    render(<JobsView live={await liveOf({}, { job: submitted(), report: report({ reviewAvailability: { allowed: false, reason: "self_authored" }, submittedAt: null, attachmentRefs: [], reviewHistory: [], workText: "" }) })} />);
    expect(screen.queryByRole("button", { name: /^Accept report/ })).toBeNull();
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ status: "in_progress", draftReportRef: { reportId: "rep-1", reportVersion: 1 } }) })} />);
    expect(screen.getByText("The technician is drafting the report — nothing submitted yet.")).toBeDefined();
  });
});

describe("costs and history", () => {
  it("adds an estimate line, checks its amount, and removes a line", async () => {
    actions.saveCosts.mockResolvedValue(ok);
    const costs = [{ kind: "estimate", description: "Visit", visibility: "customer", amountMinor: 8000, currency: "MYR" }];
    render(<JobsView live={await liveOf({}, { job: job({ costs }), events: [{ id: "e-1", action: "jobs.create", occurredAt: "2026-09-12T01:00:00Z", actorUserId: "user-int" }] })} />);
    expect(screen.getByText(/· Ali$/)).toBeDefined(); // the history names who acted
    fireEvent.click(button(/Add cost line/));
    expect(dialog().getByText(/saved with the job’s other 1 line/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Description/), { target: { value: "Drain hose" } });
    fireEvent.change(dialog().getByLabelText(/^Amount/), { target: { value: "abc" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    expect(actions.saveCosts).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Amount/), { target: { value: "45.50" } });
    fireEvent.click(dialog().getByRole("button", { name: "Internal" }));
    fireEvent.change(dialog().getByLabelText(/^Currency/), { target: { value: "USD" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.saveCosts).toHaveBeenLastCalledWith("job-12345678", 4, [costs[0], { kind: "estimate", description: "Drain hose", visibility: "internal", amountMinor: 4550, currency: "USD" }]);
    fireEvent.click(button("Remove Visit"));
    await flush();
    expect(actions.saveCosts).toHaveBeenLastCalledWith("job-12345678", 4, []);
    expect(nav.toasts).toStrictEqual(["Cost line added", "Cost line removed"]);
  });

  it("starts an actual line after the work, and offers no change to a cancelled job's costs", async () => {
    render(<JobsView live={await liveOf({}, { job: submitted(), report: report() })} />);
    fireEvent.click(button(/Add cost line/));
    expect(dialog().getByRole("button", { name: "Actual", pressed: true })).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("No cost lines yet — add estimates before the work and actuals after it.")).toBeDefined();
    expect(screen.getByText("No events.")).toBeDefined();
    cleanup();
    render(<JobsView live={await liveOf({}, { job: job({ status: "cancelled" }) })} />);
    expect(screen.getByText("A cancelled job’s costs cannot change.")).toBeDefined();
    expect(button(/Add cost line/).disabled).toBe(true);
  });
});
