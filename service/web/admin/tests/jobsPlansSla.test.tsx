// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The Plans and SLA tabs of HQ maintenance jobs (FR-A06 Plans, DD-A06 item 9, D16, FR-A22, DD-A22), from the data the
// loaders build: the scope selects shared by the tabs, the plan list and a plan (every, next due, Generate job, its
// occurrences, a CONFLICT on a second generate, delete one that made no job), New plan; the SLA filters, tiles,
// customers, breaches, the CSV export and the targets dialog.
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
  return { savePlan: fn(), generateJob: fn(), deletePlan: fn(), saveSlaTargets: fn(), createJob: fn() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), push: vi.fn<(href: string) => void>(), toasts: [] as string[], downloads: [] as string[] }));
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

import { loadPlans, loadSla } from "../app/admin/jobs/_lib/load";
import { PlansView } from "../app/admin/jobs/_components/plans-view";
import { SlaView } from "../app/admin/jobs/_components/sla-view";

const plan = (over: Record<string, unknown> = {}) => ({
  id: "plan-12345678", version: 3, unitId: "u-1", recurrence: { kind: "monthly", intervalMonths: 3 }, timezone: "UTC", anchorDay: 8, nextDueAt: "2026-12-08T02:00:00Z",
  generatedOccurrences: [{ occurrenceAt: "2026-09-08T02:00:00Z", jobId: "job-c02-0000" }], createdAt: "2026-05-01T00:00:00Z", updatedAt: "2026-09-08T00:00:00Z", ...over,
});
const metrics = { responseWithinTarget: 94, arrivalInWindow: 96, firstTimeFix: 86, averageRating: 4.5, ratingCount: 27, openOverdue: 1 };
const target = (planType: string, over: Record<string, unknown> = {}) => ({ planType, responseHours: 4, arrivalInWindowPercent: 90, firstTimeFixPercent: 85, version: 0, effectiveFrom: null, state: "default", ...over });
const scorecard = (over: Record<string, unknown> = {}) => ({
  period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, contractorOrgId: null, totals: metrics,
  customers: [{ ...metrics, customerId: "cust-a", planType: "rto", jobCount: 18, status: "breached" }],
  breaches: [{ jobId: "job-12345678", customerId: "cust-a", kind: "response", detail: "response 6 h vs 4 h" }],
  targets: [target("rto", { version: 2, effectiveFrom: "2026-08-01T00:00:00Z", state: "in_effect" }), target("general"), target("energy"), target("environment"), target("general", { responseHours: 2, version: 1, effectiveFrom: "2026-10-01T00:00:00Z", state: "scheduled" })],
  ...over,
});
function core(w: { plans?: unknown[]; plan?: unknown; jobs?: unknown[]; scorecard?: unknown } = {}) {
  const all = new Map<string, unknown>([
    ["organizations.list", [{ id: "org-hq", version: 1, name: "HQ", kind: "operator", status: "active" }, { id: "org-a", version: 1, name: "Alpha", kind: "customer", status: "active" }, { id: "org-c", version: 1, name: "Cool Partners", kind: "contractor", status: "active" }]],
    ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a" }]], ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a" }]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }, { id: "u-2", displayName: "Office AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }]],
    ["members.list", []], ["plans.list", w.plans ?? [plan()]], ["jobs.list", w.jobs ?? [{ projection: "summary", id: "job-c02-0000", status: "completed", displayStatus: "completed" }]],
  ]);
  dal.coreAll.mockImplementation((op) => Promise.resolve(all.get(op) ?? []));
  dal.coreOp.mockImplementation((op) => {
    if (op === "jobs.list" || op === "plans.list") return Promise.resolve({ items: [], total: 1 });
    if (op === "plans.get") return Promise.resolve(w.plan ?? plan());
    if (op === "units.get") return Promise.resolve({ displayName: "Lobby AC", location: { pathLabels: ["Tower", "Lobby"] } });
    if (op === "sla.scorecard") return Promise.resolve(w.scorecard ?? scorecard());
    return Promise.resolve({});
  });
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.toasts = []; nav.downloads = [];
  for (const f of Object.values(actions)) f.mockReset();
  core();
  URL.createObjectURL = vi.fn(() => "blob:csv");
  URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function record(this: HTMLAnchorElement) { nav.downloads.push(this.download); });
});
afterEach(cleanup);

describe("the scope bar and the tabs", () => {
  it("narrows customer, property and unit in the URL, clearing the narrower ones and the selection, and keeps the scope in the tab links", async () => {
    render(<PlansView live={await loadPlans({ customerId: "cust-a", propertyId: "p-1" })} />);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, propertyId: null, unitId: null, planId: null });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "p-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ propertyId: "p-1", unitId: null, planId: null });
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "u-2" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: "u-2", planId: null });
    expect(screen.getByRole("tab", { name: /Jobs/ }).getAttribute("href")).toBe("/admin/jobs?customerId=cust-a&propertyId=p-1");
    expect(screen.getByRole("tab", { name: /SLA by customer/ }).getAttribute("href")).toBe("/admin/jobs?customerId=cust-a&propertyId=p-1&tab=sla");
  });
});

describe("plans", () => {
  it("shows a plan with its occurrences, generates the next job, and says when that date already has one", async () => {
    actions.generateJob.mockResolvedValueOnce({ ok: true, value: { id: "job-new-1234" } }).mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<PlansView live={await loadPlans({})} />);
    expect(screen.getByText("1 plan in scope")).toBeDefined();
    expect(screen.getByRole("link", { name: /Open job/ }).getAttribute("href")).toBe("/admin/jobs?jobId=job-c02-0000");
    expect(screen.queryByRole("button", { name: "Delete plan…" })).toBeNull(); // it made a job
    fireEvent.click(button("Generate job"));
    await flush();
    expect(actions.generateJob).toHaveBeenCalledWith("plan-12345678", 3, "2026-12-08T02:00:00Z");
    expect(screen.getByText(/was just generated as job-new-/)).toBeDefined();
    fireEvent.click(button("Generate job"));
    await flush();
    expect(screen.getByText("The plan changed meanwhile")).toBeDefined(); // the date is not among this plan's occurrences
  });

  it("edits every and the next due date, refusing a past one, discards, and saves at the plan's version", async () => {
    actions.savePlan.mockResolvedValueOnce({ ok: true, value: { id: "plan-12345678" } }).mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: {} });
    render(<PlansView live={await loadPlans({})} />);
    expect(button("Save plan").disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Every/), { target: { value: "6" } });
    expect(button("Generate job").disabled).toBe(true);
    expect(screen.getAllByText("Save the plan first.").length).toBeGreaterThan(0);
    fireEvent.click(button("Discard"));
    expect(button("Save plan").disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("date and time"), { target: { value: "2026-09-01T10:00" } });
    expect(screen.getByText("The next date must be in the future.")).toBeDefined();
    fireEvent.change(screen.getByLabelText("date and time"), { target: { value: "2027-01-15T10:00" } });
    fireEvent.change(screen.getByLabelText(/^Every/), { target: { value: "6" } });
    fireEvent.click(button("Save plan"));
    await flush();
    expect(actions.savePlan).toHaveBeenCalledWith({ id: "plan-12345678", version: 3, unitId: "u-1", intervalMonths: 6, nextDueAt: "2027-01-15T02:00:00.000Z" });
    expect(screen.getByText(/^Plan saved — every 6 months, next due .* \(anchor day 15\)\.$/)).toBeDefined();
    fireEvent.click(button("Save plan"));
    await flush();
    expect(screen.queryByText(/^Plan saved/)).toBeNull(); // the refusal replaces it
  });

  it("deletes a plan that made no job with a reason", async () => {
    actions.deletePlan.mockResolvedValue(ok);
    core({ plans: [plan({ generatedOccurrences: [] })], plan: plan({ generatedOccurrences: [] }) });
    render(<PlansView live={await loadPlans({})} />);
    expect(screen.getByText("None yet.")).toBeDefined();
    fireEvent.click(button("Delete plan…"));
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Delete" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Unit sold " } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deletePlan).toHaveBeenCalledWith("plan-12345678", 3, " Unit sold ");
    expect(nav.push).toHaveBeenCalledWith("/admin/jobs?tab=plans");
    fireEvent.click(button("Delete plan…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("creates a plan for a unit, telling which plans it already has, and says why a refusal failed", async () => {
    actions.savePlan.mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} }).mockResolvedValueOnce({ ok: true, value: { id: "plan-new" } });
    render(<PlansView live={await loadPlans({})} />);
    fireEvent.click(button(/New plan/));
    fireEvent.change(dialog().getByLabelText(/^Unit/), { target: { value: "u-1" } });
    expect(dialog().getByText(/^This unit already has 1 plan/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Unit/), { target: { value: "u-2" } });
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Create plan" }));
    expect(dialog().getByText("Enter the next date and time.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-11-10T10:00" } });
    fireEvent.change(dialog().getByLabelText(/^Every/), { target: { value: "1" } });
    fireEvent.click(dialog().getByRole("button", { name: "Create plan" }));
    await flush();
    expect(screen.getByRole("dialog")).toBeDefined(); // the refusal keeps it open
    fireEvent.click(dialog().getByRole("button", { name: "Create plan" }));
    await flush();
    expect(actions.savePlan).toHaveBeenLastCalledWith({ id: null, version: null, intervalMonths: 1, nextDueAt: "2026-11-10T02:00:00.000Z", unitId: "u-2" });
    expect(nav.push).toHaveBeenCalledWith("/admin/jobs?tab=plans&planId=plan-new");
    expect(nav.toasts).toStrictEqual(["Plan created"]); // its notice shows on the new plan, which the push opens
  });

  it("asks for the first plan, and says when the URL's plan is gone", async () => {
    core({ plans: [] });
    render(<PlansView live={await loadPlans({})} />);
    expect(screen.getByText("No plan yet.")).toBeDefined();
    fireEvent.click(button(/New plan/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    cleanup();
    core({ plan: new dal.CoreError({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {}, retryAfterSeconds: null }) });
    dal.coreOp.mockImplementation((op) => (op === "plans.get" ? Promise.reject(new dal.CoreError({ code: "NOT_FOUND", messageKey: "e", fieldErrors: {}, retryAfterSeconds: null })) : Promise.resolve({ items: [], total: 1 })));
    render(<PlansView live={await loadPlans({ planId: "plan-gone" })} />);
    expect(screen.getByText("That plan no longer exists in your scope.")).toBeDefined();
  });
});

describe("SLA by customer", () => {
  it("filters the period and contractor in the URL, shows customers and breaches, and exports them", async () => {
    render(<SlaView live={await loadSla({})} />);
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "30" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ period: "30" });
    fireEvent.change(screen.getByLabelText("Period"), { target: { value: "90" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ period: null });
    fireEvent.change(screen.getByLabelText("Contractor"), { target: { value: "org-c" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ contractorId: "org-c" });
    fireEvent.change(screen.getByLabelText("Contractor"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ contractorId: null });
    expect(screen.getByText("Alpha")).toBeDefined();
    expect(screen.getByRole("link", { name: /job-1234/ }).getAttribute("href")).toBe("/admin/jobs?jobId=job-12345678");
    fireEvent.click(button("Export CSV"));
    expect(nav.downloads).toStrictEqual(["sla-by-customer-2026-06-16-2026-09-14.csv"]);
    fireEvent.click(button(/New job/));
    expect(screen.getByRole("dialog")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
  });

  it("says when the period has no jobs or breaches", async () => {
    core({ scorecard: scorecard({ customers: [], breaches: [] }) });
    render(<SlaView live={await loadSla({ contractorId: "org-c" })} />);
    expect(screen.getByText("The scorecard counts jobs created in the period and delivered by the contractor.")).toBeDefined();
    expect(screen.getByText("No breaches in this period.")).toBeDefined();
    expect(button("Export CSV").disabled).toBe(true);
  });

  it("saves a plan type's new targets from now or later, checking each value", async () => {
    actions.saveSlaTargets.mockResolvedValueOnce({ ok: true, value: { version: 3 } }).mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} });
    render(<SlaView live={await loadSla({})} />);
    fireEvent.click(button("Edit SLA targets"));
    expect(dialog().getAllByText(/^next: /)).toHaveLength(1); // general's scheduled targets
    fireEvent.change(dialog().getByLabelText(/^Plan type/), { target: { value: "general" } });
    fireEvent.change(dialog().getByLabelText(/^Response within/), { target: { value: "200" } });
    fireEvent.change(dialog().getByLabelText(/^Arrival in window/), { target: { value: "101" } });
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-09-01T10:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save targets" }));
    for (const text of ["1–168 whole hours.", "0–100 %.", "Now or later."]) expect(dialog().getByText(text)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Response within/), { target: { value: "6" } });
    fireEvent.change(dialog().getByLabelText(/^Arrival in window/), { target: { value: "92.5" } });
    fireEvent.change(dialog().getByLabelText("date and time"), { target: { value: "2026-10-01T09:00" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save targets" }));
    await flush();
    expect(actions.saveSlaTargets).toHaveBeenCalledWith({ planType: "general", responseHours: 6, arrivalInWindowPercent: 92.5, firstTimeFixPercent: 85, effectiveFrom: "2026-10-01T01:00:00.000Z" });
    expect(screen.getByText(/targets v3 saved — jobs created from/)).toBeDefined();
    fireEvent.click(button("Edit SLA targets"));
    fireEvent.click(dialog().getByRole("button", { name: "Save targets" }));
    await flush();
    expect(screen.queryByText(/targets v3 saved/)).toBeNull(); // the refusal's message instead
    fireEvent.click(button("Edit SLA targets"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
