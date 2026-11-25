// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The Propose another time dialog of an HQ job (FR-A06): one of the client's own preferred times is refused, compared as
// instants. Before the fix it compared strings, and the API's "…:00Z" never matched the form's "…:00.000Z".
// The loader and mocks are those of jobsView.test.tsx.
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

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("proposing another time", () => {
  it("refuses one of the client's own times, however the API writes the instant", async () => {
    render(<JobsView live={await liveOf()} />); // the API's preferred times end in "…:00Z", the form's in "…:00.000Z"
    fireEvent.click(button("Propose another time…"));
    fireEvent.change(dialog().getByLabelText(/^Date/), { target: { value: "2026-09-22" } });
    const [from, to] = dialog().getAllByLabelText("time");
    fireEvent.change(from ?? document.body, { target: { value: "10:00" } });
    fireEvent.change(to ?? document.body, { target: { value: "12:00" } });
    expect(dialog().getByText("This is one of the client’s own times — book it directly instead.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Message to the client/), { target: { value: "Same time?" } });
    fireEvent.click(dialog().getByRole("button", { name: "Send proposal" }));
    await flush();
    expect(actions.proposeSlot).not.toHaveBeenCalled();
    fireEvent.change(to ?? document.body, { target: { value: "11:00" } });
    expect(dialog().queryByText(/^This is one of the client’s own times/)).toBeNull();
  });
});
