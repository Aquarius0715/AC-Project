// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiRestriction, PerUnit, RestrictionState, UnitRow } from "@ac/web/lib/restrictions";
import type { ApiAudit } from "@ac/web/lib/audit";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ restriction exception screen (FR-A10, SCR-A10): the Server Component reads the restriction (404 when it is
// gone), unit names, unresolved alerts for the bypass banner and the audit timeline per permission; the view offers
// grace, exception, cancel and override for the current state with a reason (and a date within 90 days), and the unit
// follow-ups IR96 allows. The Phase 1A demo moves locally.
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
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set<string>())),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown[]>>(),
    coreAllPage: vi.fn<(op: string) => Promise<{ items: unknown[]; total: number }>>(),
  };
});
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { cancelRestriction: vi.fn<Action>(), deferRestriction: vi.fn<Action>(), exemptRestriction: vi.fn<Action>(), overrideRestriction: vi.fn<Action>(), reconcileUnits: vi.fn<Action>(), retryUnits: vi.fn<Action>() };
});
const nav = vi.hoisted(() => ({ toasts: [] as string[], notFound: vi.fn(() => { throw new Error("NEXT_NOT_FOUND"); }) }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), notFound: nav.notFound }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/restrictions/actions", () => actions);
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string, after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(ok);
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  DateTimeSelect: ({ value, onChange }: Readonly<{ value: string; onChange: (v: string) => void }>) => <input aria-label="date and time" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import AdminRestrictionPage from "../app/admin/restrictions/[id]/page";
import { ExceptionView } from "../app/admin/restrictions/[id]/_components/exception-view";
import { ExceptionDemo } from "../app/admin/restrictions/[id]/_components/exception-demo";
import ExceptionLoading from "../app/admin/restrictions/[id]/loading";
import ExceptionErrorState from "../app/admin/restrictions/[id]/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
/** Unit u-1 of the restriction, applied. */
const perUnit = (): PerUnit => ({ unitId: "u-1", applyState: "applied", releaseState: "none", applyCommandIds: [], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: null });
const restriction = (state: RestrictionState, over: Partial<ApiRestriction> = {}): ApiRestriction => ({
  id: "r-11112222", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u-1"], rulesVersion: "demo-v1", policy: { kind: "power_off" }, state, perUnit: [perUnit()], recoveryCases: [],
  noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, contractId: "k-1", causeInvoiceIds: ["inv-1", "inv-2"], ...over,
});
const audit: ApiAudit = {
  id: "a-1", actorId: "user-op", actorName: "HQ Operator", actorRoleAtTime: "admin", action: "restrictions.execute", targetRef: { kind: "restriction", id: "r-11112222" },
  previousVersion: 2, nextVersion: 3, occurredAt: "2026-09-13T00:00:00Z", correlationId: "corr-1", result: "success", maskedBefore: null, maskedAfter: null, reason: null,
};
const props = (id = "r-11112222") => ({ params: Promise.resolve({ id }), searchParams: Promise.resolve({}) });
type Live = ComponentProps<typeof ExceptionView>["live"];
async function liveOf(): Promise<Live> {
  const page: ReactNode = await AdminRestrictionPage(props());
  if (!isValidElement<ComponentProps<typeof ExceptionView>>(page) || page.type !== ExceptionView) throw new TypeError("not ExceptionView");
  return page.props.live;
}
const callsOf = (op: string) => [...dal.coreOp.mock.calls, ...dal.coreAll.mock.calls].filter(([name]) => name === op).map(([, input]) => input);

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); nav.toasts = [];
  dal.coreAllPage.mockResolvedValue({ items: [{ id: "u-1", displayName: "Lobby AC" }], total: 1 });
  for (const fn of Object.values(actions)) fn.mockReset();
  for (const fn of Object.values(actions)) fn.mockResolvedValue({ ok: true, value: null });
});
afterEach(cleanup);

describe("the restriction exception page (Server Component)", () => {
  it("shows the demo outside API mode, its loading and error states, and what a role without restriction permissions needs", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo: ReactNode = await AdminRestrictionPage(props("restriction-limited-a"));
    expect(isValidElement<{ id: string }>(demo) && demo.type === ExceptionDemo && demo.props.id).toBe("restriction-limited-a");
    expect(ExceptionLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = ExceptionErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
    dal.apiMode.mockReturnValue(true);
    dal.corePermissions.mockResolvedValue(new Set(["asset.read"]));
    const none: ReactNode = await AdminRestrictionPage(props());
    expect(isValidElement(none) && none.type === NeedsAccess && none.props).toStrictEqual({ what: "Restrictions", permission: "restriction.read" });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("reads the restriction, its units, unresolved alerts and the last year's audit", async () => {
    dal.corePermissions.mockResolvedValue(new Set(["restriction.read", "restriction.write", "asset.read", "alert.read", "audit.read"]));
    dal.coreOp.mockImplementation((op) => Promise.resolve(op === "restrictions.get" ? restriction("applied") : { items: [audit] }));
    dal.coreAll.mockImplementation((_op, input) => Promise.resolve(JSON.stringify(input).includes('"open"') ? [{ unitId: "u-1", type: "restriction_bypass", status: "open" }] : []));
    const live = await liveOf();
    expect(callsOf("alerts.list")).toStrictEqual([{ filters: { unitIds: ["u-1"], status: "open" } }, { filters: { unitIds: ["u-1"], status: "acknowledged" } }]);
    expect(callsOf("audit.list")).toStrictEqual([{ limit: 100, filters: { targetKind: "restriction", targetId: "r-11112222", from: "2025-09-14T01:00:00.000Z", to: "2026-09-15T01:00:00.000Z" }, sort: { field: "occurredAt", direction: "asc" } }]);
    expect([live.units[0]?.name, live.bypass, live.canWrite, live.canOverride, live.canAudit, live.audit.length]).toStrictEqual(["Lobby AC", ["Lobby AC"], true, false, true, 1]);
  });

  it("reads no names, alerts or audit without their permissions, and answers 404 for a restriction that is gone", async () => {
    dal.corePermissions.mockResolvedValue(new Set(["restriction.override"]));
    dal.coreOp.mockResolvedValue(restriction("applied", { projection: "release" }));
    const live = await liveOf();
    expect([live.units[0]?.name, live.bypass, live.audit, live.canAudit, live.canOverride]).toStrictEqual(["u-1", [], [], false, true]);
    expect(dal.coreAllPage).not.toHaveBeenCalled();
    expect(callsOf("alerts.list")).toStrictEqual([]);
    dal.corePermissions.mockResolvedValue(new Set(["restriction.read", "alert.read"]));
    dal.coreOp.mockResolvedValue(restriction("applied", { perUnit: [] }));
    await liveOf();
    expect(callsOf("alerts.list")).toStrictEqual([]); // no units, no alerts to read
    dal.coreOp.mockRejectedValue(refused("NOT_FOUND"));
    await expect(AdminRestrictionPage(props())).rejects.toThrow("NEXT_NOT_FOUND");
    dal.coreOp.mockRejectedValue(refused("UNAVAILABLE"));
    await expect(AdminRestrictionPage(props())).rejects.toThrow("UNAVAILABLE");
  });
});

const unitRow = (over: Partial<UnitRow> = {}): UnitRow => ({ unitId: "u-1", name: "Lobby AC", apply: "applied", release: "—", observed: "restricted", pending: null, retryApply: false, retryRelease: false, reconcile: false, commands: "", ...over });
function viewOf(r: ApiRestriction, over: Partial<Live> = {}): Live {
  return { now: "2026-09-14T01:00:00.000Z", r, units: [unitRow()], bypass: [], canWrite: true, canOverride: true, canAudit: true, period: null, intent: "none", audit: [], ...over };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => new Promise((r) => { setTimeout(r, 0); });
const typeReason = (text: string) => { fireEvent.change(screen.getByLabelText(/^Reason · required/), { target: { value: text } }); };
const setUntil = (value: string) => {
  const [picker] = screen.getAllByLabelText("date and time");
  if (!picker) throw new Error("no date picker");
  fireEvent.change(picker, { target: { value } });
};

/** A canonical restriction that names no cause invoices (the field is optional in the contract). */
const noCauses: ApiRestriction = {
  id: "r-11112222", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u-1"], rulesVersion: "demo-v1", policy: { kind: "power_off" }, state: "applied", perUnit: [perUnit()],
  recoveryCases: [], noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null,
};

describe("the restriction exception view", () => {
  it("opens on the first allowed action, checks the date and reason, and records a grace period", async () => {
    render(<ExceptionView live={viewOf(restriction("applied"))} />);
    expect(button(/^Grace period/).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getAllByText("Applied → Release requested (source: exception)").length).toBeGreaterThan(0);
    typeReason("");
    setUntil("2026-09-13T10:00");
    fireEvent.click(button("Apply grace period"));
    expect(screen.getByText("A future date within 90 days")).toBeDefined();
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeDefined();
    setUntil("2026-12-31T10:00");
    expect(screen.getByText("A future date within 90 days")).toBeDefined(); // more than 90 days ahead
    setUntil("");
    expect(screen.getByText("A future date within 90 days")).toBeDefined();
    setUntil("2026-09-21T09:00");
    typeReason(" Paying next week ");
    fireEvent.click(button("Apply grace period"));
    await flush();
    expect(actions.deferRestriction).toHaveBeenCalledWith("r-11112222", 3, "2026-09-21T01:00:00.000Z", "Paying next week");
    expect(nav.toasts).toStrictEqual(["Grace period recorded"]);
    expect(screen.getByLabelText<HTMLTextAreaElement>(/^Reason · required/).value).toBe("");
  });

  it("records an exception, a cancellation and an override with their reasons", async () => {
    render(<ExceptionView live={viewOf(restriction("applied"))} />);
    const [picker] = screen.getAllByLabelText<HTMLInputElement>("date and time");
    expect(picker?.value).toBe("2026-09-21T09:00"); // a week ahead in Kuala Lumpur
    fireEvent.click(button(/^Exception/));
    typeReason("Hospital");
    fireEvent.click(button("Apply exception"));
    await flush();
    expect(actions.exemptRestriction).toHaveBeenCalledWith("r-11112222", 3, "2026-09-21T01:00:00.000Z", "Hospital");
    fireEvent.click(button(/^Cancel restrictions\.cancel/));
    expect(screen.queryByLabelText("date and time")).toBeNull(); // no date for a cancellation
    typeReason("Paid at the counter");
    fireEvent.click(button("Cancel restriction"));
    await flush();
    expect(actions.cancelRestriction).toHaveBeenCalledWith("r-11112222", 3, "Paid at the counter");
    fireEvent.click(button(/^Override release restrictions/));
    typeReason("Emergency");
    fireEvent.click(screen.getAllByRole("button", { name: "Override release" }).at(-1) ?? document.body);
    await flush();
    expect(actions.overrideRestriction).toHaveBeenCalledWith("r-11112222", 3, "Emergency");
    expect(nav.toasts).toStrictEqual(["Exception recorded", "Cancellation recorded", "Override recorded"]);
  });

  it("says what each action does for the state, and offers nothing for a finished restriction", () => {
    const { unmount } = render(<ExceptionView live={viewOf(restriction("scheduled"))} />);
    expect(screen.getAllByText("Stays scheduled; execution waits until the date passes").length).toBeGreaterThan(0);
    expect(button(/^Override release restrictions/).disabled).toBe(true); // cancel a scheduled one instead
    unmount(); // a new screen: the chosen action starts again
    render(<ExceptionView live={viewOf(restriction("released"))} />);
    expect(button(/^Grace period/).disabled).toBe(true);
    expect(button("Cancel restriction").disabled).toBe(true); // nothing is allowed: it falls back to cancel, disabled
    fireEvent.click(button("Cancel restriction"));
    expect(actions.cancelRestriction).not.toHaveBeenCalled();
  });

  it("names the permission an action needs, and shows the release view and permissions", () => {
    render(<ExceptionView live={viewOf(restriction("applied", { projection: "release" }), { canWrite: false, canAudit: false, units: [] })} />);
    expect(screen.getAllByText("Needs “Edit restrictions”")).toHaveLength(3);
    expect(button(/^Override release restrictions/).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Release view (“Override restrictions” only): billing fields are not shown.")).toBeDefined();
    expect(screen.queryByText("Cause invoices")).toBeNull();
    expect(screen.getByText("Your permissions").nextElementSibling?.textContent).not.toBe("read only"); // the override permission is named
    expect(screen.queryByText("Audit")).toBeNull();
    expect(screen.getByText("—")).toBeDefined(); // no unit names
  });

  it("shows a read-only role nothing it can do, and the audit and cause counts of a full one", () => {
    const { rerender } = render(<ExceptionView live={viewOf(restriction("applied"), { canWrite: false, canOverride: false, canAudit: false })} />);
    expect(screen.getByText("read only")).toBeDefined();
    expect(screen.getAllByText("Needs “Override restrictions”")).toHaveLength(1);
    rerender(<ExceptionView live={viewOf(noCauses, { audit: [{ time: "13 Sept", title: "Executed", detail: "by HQ" }], period: "until 20 Sept" })} />);
    expect(screen.getByText("Executed")).toBeDefined();
    expect(screen.getByText("until 20 Sept")).toBeDefined();
    expect(screen.getByText("Cause invoices").nextElementSibling?.textContent).toBe("0");
    rerender(<ExceptionView live={viewOf(restriction("applied"))} />);
    expect(screen.getByText("No audit entries yet.")).toBeDefined();
    expect(screen.getByText("Cause invoices").nextElementSibling?.textContent).toBe("2");
  });

  it("warns of a possible bypass, and offers the unit follow-ups IR96 allows", async () => {
    const units = [unitRow({ reconcile: true, pending: "offline" }), unitRow({ unitId: "u-2", name: "Office AC", retryRelease: true }), unitRow({ unitId: "u-3", name: "Hall AC" })];
    const { rerender } = render(<ExceptionView live={viewOf(restriction("release_requested"), { units, bypass: ["Lobby AC"] })} />);
    expect(screen.getByText(/Restriction may be bypassed on Lobby AC/)).toBeDefined();
    expect(screen.getByText("Hall AC · applied/—", { exact: false })).toBeDefined();
    fireEvent.click(button("Reconcile"));
    fireEvent.click(button("Retry release"));
    await flush();
    expect(actions.reconcileUnits).toHaveBeenCalledWith("r-11112222", 3, ["u-1"]);
    expect(actions.retryUnits).toHaveBeenCalledWith("r-11112222", 3, ["u-2"], "release", "demo-v1", "Retry release from the exception screen");
    expect(nav.toasts).toStrictEqual(["Reconciled", "Release retried"]);
    // an override-only role sees them only for an override release or an unresolved recovery case
    rerender(<ExceptionView live={viewOf(restriction("release_requested"), { units, canWrite: false })} />);
    expect(screen.queryByText("Units needing follow-up")).toBeNull();
    rerender(<ExceptionView live={viewOf(restriction("release_requested", { releaseIntent: { source: "override", at: "2026-09-13T00:00:00Z" } }), { units, canWrite: false })} />);
    expect(screen.getByText("Units needing follow-up")).toBeDefined();
    rerender(<ExceptionView live={viewOf(restriction("release_requested", { recoveryCases: [{ id: "rc-1", unitId: "u-1", state: "removing", successorRestrictionId: null }] }), { units, canWrite: false })} />);
    expect(screen.getByText("Units needing follow-up")).toBeDefined();
    rerender(<ExceptionView live={viewOf(restriction("release_requested", { recoveryCases: [{ id: "rc-1", unitId: "u-1", state: "resolved", successorRestrictionId: null }] }), { units, canWrite: false })} />);
    expect(screen.queryByText("Units needing follow-up")).toBeNull();
    rerender(<ExceptionView live={viewOf(restriction("release_requested"), { units: [unitRow()] })} />);
    expect(screen.queryByText("Units needing follow-up")).toBeNull(); // nothing to follow up
  });
});

describe("the restriction exception demo", () => {
  it("checks the date and reason, refuses an override without the permission, and records an action", () => {
    render(<ExceptionDemo id="restriction-limited-a" />);
    const apply = screen.getAllByRole<HTMLButtonElement>("button").find((b) => b.textContent.startsWith("Apply"));
    if (!apply) throw new Error("no Apply button");
    fireEvent.change(screen.getByLabelText(/^Exception until/), { target: { value: "2020-01-01" } });
    fireEvent.change(screen.getByLabelText(/^Reason · required/), { target: { value: " " } });
    fireEvent.click(apply);
    expect(screen.getByText("Date must be in the future")).toBeDefined();
    expect(screen.getByText("A reason is required")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Exception until/), { target: { value: "" } });
    expect(screen.getByText("Date must be in the future")).toBeDefined();
    fireEvent.click(button(/^Override release/));
    fireEvent.change(screen.getByLabelText(/^Reason · required/), { target: { value: "Emergency" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "demo: I hold restriction.override" }));
    expect(button(/^Override release/).disabled).toBe(true);
    fireEvent.click(apply);
    expect(nav.toasts).toStrictEqual(["FORBIDDEN — restriction.override required"]);
    fireEvent.click(button(/^Cancel/));
    fireEvent.click(apply);
    expect(nav.toasts.at(-1)).toBe("Action recorded");
    expect(screen.getByText(/^State: applied → release_requested; a remove command was sent/)).toBeDefined();
    expect(apply.disabled).toBe(true);
  });

  it("records a grace period with a future date", () => {
    render(<ExceptionDemo id="restriction-limited-a" />);
    fireEvent.click(button(/^Grace period/));
    const future = new Date(Date.now() + 30 * 86_400e3).toISOString().slice(0, 10);
    fireEvent.change(screen.getByLabelText(/^Exception until/), { target: { value: future } });
    const apply = screen.getAllByRole("button").find((b) => b.textContent.startsWith("Apply"));
    if (!apply) throw new Error("no Apply button");
    fireEvent.click(apply);
    expect(nav.toasts).toStrictEqual(["Action recorded"]);
    expect(screen.getByText(`Until ${future}`)).toBeDefined();
  });
});
