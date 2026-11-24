// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { restrictionRows, type ApiRestriction, type PerUnit, type RestrictionState, type UnitRow } from "@ac/web/lib/restrictions";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ restriction manager's screen (FR-A09): the state chips, scope and selection go to the URL; execute needs the
// rules confirmed and no blocker; release, retries and reconciliation act on the restriction's version; the schedule
// form checks its contract, units, setpoint, time and reason before it schedules. The Phase 1A demo moves locally.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { executeRestriction: vi.fn<Action>(), reconcileUnits: vi.fn<Action>(), requestRelease: vi.fn<Action>(), retryUnits: vi.fn<Action>(), scheduleRestriction: vi.fn<Action>() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/restrictions/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
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

import { RestrictionsView } from "../app/admin/restrictions/_components/restrictions-view";
import { RestrictionsDemo } from "../app/admin/restrictions/_components/restrictions-demo";

type Live = NonNullable<ComponentProps<typeof RestrictionsView>["live"]>;
type Selected = NonNullable<Live["selected"]>;
const NOW = "2026-09-14T01:00:00.000Z";
const perUnit = (unitId: string): PerUnit => ({ unitId, applyState: "applied", releaseState: "none", applyCommandIds: [], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: null });
const restriction = (state: RestrictionState, over: Partial<ApiRestriction> = {}): ApiRestriction => ({
  id: "r-11112222", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u-1"], rulesVersion: "demo-v1", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 26 }, state,
  perUnit: [perUnit("u-1")], recoveryCases: [], noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, contractId: "k-1", causeInvoiceIds: ["inv-1"], reason: "Overdue since 10 Sept", ...over,
});
const unitRow = (over: Partial<UnitRow> = {}): UnitRow => ({ unitId: "u-1", name: "Lobby AC", apply: "applied", release: "—", observed: "restricted", pending: null, retryApply: false, retryRelease: false, reconcile: false, commands: "c-1 acknowledged", ...over });
const counts: Record<RestrictionState, number> = { scheduled: 1, requested: 0, applied: 2, release_requested: 0, released: 4, cancelled: 0 };
function selectedOf(r: ApiRestriction, over: Partial<Selected> = {}): Selected {
  return {
    r, contract: "Demo Customer A · RTO contract v2", allPaid: false, units: [unitRow()],
    causes: [{ id: "inv-1", number: "INV-1", paid: false, processing: false, amountMinor: 12_000, currency: "MYR" }, { id: "inv-2", number: "INV-2", paid: false, processing: true, amountMinor: null, currency: "" }, { id: "inv-3", number: "INV-3", paid: true, processing: false, amountMinor: 500, currency: "MYR" }],
    texts: { notice: "12 Sept · 2 client notices", executeAfter: "13 Sept, 8:00 am", period: null, intent: "none", execute: null, release: null }, ...over,
  };
}
function liveOf(r: ApiRestriction | null, over: Partial<Live> = {}, selected: Partial<Selected> = {}): Live {
  return {
    now: NOW, full: true, canWrite: true, canUnits: true, scope: {}, rows: r ? restrictionRows([r], () => "Demo Customer A · RTO contract v2") : [], counts,
    contracts: [{ id: "k-1", label: "Demo Customer A · RTO contract v2" }], causes: [{ id: "inv-1", label: "INV-1 · Demo Customer A" }],
    ...(r && { selected: selectedOf(r, selected) }), ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const flush = () => new Promise((r) => { setTimeout(r, 0); });

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  for (const fn of Object.values(actions)) fn.mockReset();
  for (const fn of Object.values(actions)) fn.mockResolvedValue({ ok: true, value: null });
  history.replaceState(null, "", "/admin/restrictions");
});
afterEach(cleanup);

describe("the restriction manager in API mode", () => {
  it("names restriction.read to a role without any restriction permission", () => {
    const { container } = render(<RestrictionsView live={null} />);
    expect(container.textContent).toContain("View restrictions"); // the word label of restriction.read
    expect(screen.queryByRole("button", { name: /Schedule restriction/ })).toBeNull();
  });

  it("puts the state chips, the contract, the cause invoice and the selection in the URL", () => {
    const { rerender } = render(<RestrictionsView live={liveOf(restriction("applied"), { scope: { status: "applied" } })} />);
    expect(screen.getByRole("button", { name: /Applied 2/ }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(button(/Applied 2/));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: null, restrictionId: null }); // the picked chip clears
    fireEvent.click(button(/Released 4/));
    expect(nav.patch).toHaveBeenLastCalledWith({ status: "released", restrictionId: null });
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "k-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ contractId: "k-1", restrictionId: null });
    fireEvent.change(screen.getByLabelText("Contract"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ contractId: null, restrictionId: null });
    fireEvent.change(screen.getByLabelText("Cause invoice"), { target: { value: "inv-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ invoiceId: "inv-1", restrictionId: null });
    fireEvent.change(screen.getByLabelText("Cause invoice"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ invoiceId: null, restrictionId: null });
    fireEvent.click(button(/r-111122/));
    expect(nav.patch).toHaveBeenLastCalledWith({ restrictionId: "r-11112222" });
    rerender(<RestrictionsView live={liveOf(null, { scope: { invoiceId: "inv-99998888" } })} />);
    expect(screen.getByRole("option", { name: "invoice inv-9999 — no restriction cites it" })).toBeDefined();
    expect(screen.getByText("No restriction matches this scope.")).toBeDefined();
    expect(screen.getByText("Nothing selected")).toBeDefined();
    rerender(<RestrictionsView live={liveOf(null)} />);
    expect(screen.getByText("No restriction has been applied yet.")).toBeDefined();
    rerender(<RestrictionsView live={liveOf(restriction("applied", { unitIds: ["u-1", "u-2"], reason: "" }))} />);
    expect(screen.getByText(/· 2 units ·/)).toBeDefined();
    expect(screen.getByText("Reason shown to the customer").nextElementSibling?.textContent).toBe("—"); // no reason recorded
  });

  it("shows the selected restriction with its cause invoices, units and recovery cases", () => {
    const r = restriction("applied", { recoveryCases: [{ id: "rc-1", unitId: "u-12345678", state: "pending", successorRestrictionId: null }, { id: "rc-2", unitId: "u-2", state: "resolved", successorRestrictionId: null }] });
    render(<RestrictionsView live={liveOf(r, {}, { units: [unitRow({ pending: "offline" })] })} />);
    expect(screen.getByText("“Overdue since 10 Sept”")).toBeDefined();
    expect(screen.getAllByRole("link", { name: /Exception \/ override/ })[0]?.getAttribute("href")).toBe("/admin/restrictions/r-11112222");
    expect(screen.getByRole("link", { name: "INV-1" }).getAttribute("href")).toBe("/admin/billing?invoiceId=inv-1");
    expect(screen.getByText("processing")).toBeDefined();
    expect(screen.getByText("paid")).toBeDefined();
    expect(screen.getByText(/2 of 3 cause invoices unpaid/)).toBeDefined();
    expect(screen.getByText("restricted · pending: offline")).toBeDefined();
    expect(screen.getByRole("link", { name: /Details/ }).getAttribute("href")).toBe("/admin/units?unitId=u-1");
    expect(screen.getByText("pending")).toBeDefined();
    expect(screen.getByText("resolved")).toBeDefined();
  });

  it("executes a scheduled restriction only once the rules are confirmed and nothing blocks it", async () => {
    const { rerender } = render(<RestrictionsView live={liveOf(restriction("scheduled"))} />);
    const execute = screen.getByRole<HTMLButtonElement>("button", { name: "Execute" });
    expect(execute.disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: "I confirm the rules version demo-v1 still applies" }));
    expect(execute.disabled).toBe(false);
    fireEvent.click(execute);
    await flush();
    expect(actions.executeRestriction).toHaveBeenCalledWith("r-11112222", 3, "demo-v1");
    expect(nav.toasts).toStrictEqual(["Executed — apply requests sent per unit"]);
    rerender(<RestrictionsView live={liveOf(restriction("scheduled"), {}, { texts: { ...selectedOf(restriction("scheduled")).texts, execute: "Not before 13 Sept, 8:00 am" } })} />);
    expect(screen.getByText("Not before 13 Sept, 8:00 am")).toBeDefined();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Execute" }).disabled).toBe(true);
  });

  it("requests the release of an applied or requested restriction unless something blocks it", async () => {
    const { rerender } = render(<RestrictionsView live={liveOf(restriction("requested"))} />);
    fireEvent.click(button("Request release"));
    await flush();
    expect(actions.requestRelease).toHaveBeenCalledWith("r-11112222", 3);
    rerender(<RestrictionsView live={liveOf(restriction("applied"), {}, { texts: { ...selectedOf(restriction("applied")).texts, release: "Cause invoices still unpaid" } })} />);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Request release" }).disabled).toBe(true);
    expect(screen.getByText("Cause invoices still unpaid")).toBeDefined();
    rerender(<RestrictionsView live={liveOf(restriction("cancelled"))} />);
    expect(screen.getByText("Cancelled — no further commands.")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Request release" })).toBeNull();
  });

  it("retries a unit with a reason, and reconciles a unit with its latest observation", async () => {
    render(<RestrictionsView live={liveOf(restriction("applied"), {}, { units: [unitRow({ retryApply: true, reconcile: true }), unitRow({ unitId: "u-2", name: "Office AC", retryRelease: true })] })} />);
    fireEvent.click(button("Reconcile"));
    await flush();
    expect(actions.reconcileUnits).toHaveBeenCalledWith("r-11112222", 3, ["u-1"]);
    fireEvent.click(button("Retry release"));
    expect(within(screen.getByRole("dialog")).getByRole("heading", { name: "Retry release" })).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Retry apply"));
    const dialog = within(screen.getByRole("dialog"));
    const send = dialog.getByRole<HTMLButtonElement>("button", { name: "Retry" });
    expect(send.disabled).toBe(true);
    fireEvent.change(dialog.getByLabelText("Reason"), { target: { value: " Unit was offline " } });
    fireEvent.click(send);
    await flush();
    expect(actions.retryUnits).toHaveBeenCalledWith("r-11112222", 3, ["u-1"], "apply", "demo-v1", "Unit was offline");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(nav.toasts).toStrictEqual(["Reconciled with the latest observation", "Retry sent"]);
    fireEvent.click(button("Retry apply"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("gives the release view no billing fields, links or writes", () => {
    render(<RestrictionsView live={liveOf(restriction("applied"), { full: false, canWrite: false, canUnits: false }, { units: [unitRow({ retryApply: true })] })} />);
    expect(screen.getByText("Release view (“Override restrictions” only): billing fields are not shown.")).toBeDefined();
    for (const label of ["Contract", "Cause invoice"]) expect(screen.queryByLabelText(label)).toBeNull();
    expect(screen.queryByText("Cause invoices")).toBeNull();
    expect(screen.queryByText("Reason shown to the customer")).toBeNull();
    for (const name of [/Schedule restriction/, "Request release", "Retry apply"]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.queryByRole("link", { name: /Details/ })).toBeNull();
  });
});

type ScheduleContract = NonNullable<Live["schedule"]>[number];
const range = (min: number, max: number) => ({ min, max, step: 1 });
const scheduleContract = (id: string, over: Partial<ScheduleContract> = {}): ScheduleContract => ({
  id, version: 2, label: `Contract ${id}`, rulesVersion: "demo-v1", recipients: 2, invoices: [{ id: "inv-1", number: "INV-1" }],
  units: [
    { id: "u-1", name: "Lobby AC", blocked: null, temperature: range(16, 30) }, { id: "u-2", name: "Office AC", blocked: null, temperature: range(20, 26) },
    { id: "u-3", name: "Old AC", blocked: "archived", temperature: null }, { id: "u-4", name: "Fan coil", blocked: null, temperature: null },
    { id: "u-5", name: "Server room AC", blocked: null, temperature: range(27, 30) },
  ], ...over,
});
const schedule = [scheduleContract("k-1"), scheduleContract("k-noinv", { invoices: [], recipients: 1 }), scheduleContract("k-norec", { recipients: 0 })];

describe("scheduling a restriction", () => {
  it("checks the contract, units, setpoint, time and reason, then schedules and opens the new restriction", async () => {
    actions.scheduleRestriction.mockResolvedValue({ ok: true, value: "r-new" });
    render(<RestrictionsView live={liveOf(null, { schedule })} />);
    fireEvent.click(button(/Schedule restriction/));
    const dialog = within(screen.getByRole("dialog"));
    const submit = dialog.getByRole("button", { name: "Schedule" });
    fireEvent.click(submit);
    for (const text of ["Choose an eligible RTO contract", "The chosen units need a temperature range", "1–1000 characters, shown to the customer"]) expect(dialog.getByText(text)).toBeDefined();
    const contract = dialog.getByLabelText(/^Contract \(eligible RTO\)/);
    fireEvent.change(contract, { target: { value: "k-noinv" } });
    expect(dialog.getByText("This contract has no overdue unpaid invoice")).toBeDefined();
    expect(dialog.getByText(/: none · notice to 1 active client$/)).toBeDefined();
    fireEvent.change(contract, { target: { value: "k-norec" } });
    expect(dialog.getByText("No active client can receive the notice")).toBeDefined();
    fireEvent.change(contract, { target: { value: "k-1" } });
    expect(dialog.getByText(/INV-1 · notice to 2 active clients$/)).toBeDefined();
    expect(dialog.getByText("Choose at least one unit")).toBeDefined(); // shown with the contract's units
    expect(dialog.getByRole("checkbox", { name: /Old AC/ }).dataset.disabled).toBeDefined(); // a blocked unit cannot be chosen
    expect(dialog.getByText("no temperature setpoint")).toBeDefined();
    fireEvent.click(dialog.getByRole("checkbox", { name: /Office AC/ }));
    fireEvent.click(dialog.getByRole("checkbox", { name: /Server room AC/ }));
    expect(dialog.getByText("The chosen units share no temperature setting")).toBeDefined();
    fireEvent.click(dialog.getByRole("checkbox", { name: /Server room AC/ }));
    fireEvent.click(dialog.getByRole("checkbox", { name: /Lobby AC/ }));
    fireEvent.change(dialog.getByLabelText(/^Minimum cooling setpoint/), { target: { value: "30" } });
    expect(dialog.getByText("Within 20–26 °C for the chosen units")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Minimum cooling setpoint/), { target: { value: "25" } });
    const [when] = dialog.getAllByLabelText<HTMLInputElement>("date and time");
    if (!when) throw new Error("no time picker");
    expect(when.value).toBe("2026-09-15T10:00"); // 25 h ahead, a whole hour in Kuala Lumpur
    fireEvent.change(when, { target: { value: "2026-09-14T12:00" } });
    expect(dialog.getByText("At least 24 hours from now")).toBeDefined();
    fireEvent.change(when, { target: { value: "" } });
    expect(dialog.getByText("At least 24 hours from now")).toBeDefined();
    fireEvent.change(when, { target: { value: "2026-09-15T10:00" } });
    fireEvent.change(dialog.getByLabelText(/^Reason shown to the customer/), { target: { value: " Invoice overdue since 10 Sept " } });
    fireEvent.click(submit);
    await flush();
    expect(actions.scheduleRestriction).toHaveBeenCalledWith({
      contractId: "k-1", expectedContractVersion: 2, causeInvoiceIds: ["inv-1"], policy: { kind: "temperature_limit", minimumCoolingSetpoint: 25 }, executeAfter: "2026-09-15T02:00:00.000Z",
      reason: "Invoice overdue since 10 Sept", rulesVersion: "demo-v1", unitIds: ["u-2", "u-1"],
    });
    expect(nav.toasts).toStrictEqual(["Restriction scheduled — the notice was sent"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ restrictionId: "r-new", status: null });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("schedules a power-off without a setpoint, and drops the units when the contract changes", async () => {
    actions.scheduleRestriction.mockResolvedValue({ ok: true, value: "r-off" });
    render(<RestrictionsView live={liveOf(null, { schedule })} />);
    fireEvent.click(button(/Schedule restriction/));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText(/^Contract \(eligible RTO\)/), { target: { value: "k-1" } });
    fireEvent.click(dialog.getByRole("checkbox", { name: /Fan coil/ }));
    fireEvent.change(dialog.getByLabelText("Policy"), { target: { value: "power_off" } });
    expect(dialog.queryByLabelText(/^Minimum cooling setpoint/)).toBeNull();
    fireEvent.change(dialog.getByLabelText(/^Reason shown to the customer/), { target: { value: "Overdue" } });
    fireEvent.click(dialog.getByRole("button", { name: "Schedule" }));
    await flush();
    expect(actions.scheduleRestriction).toHaveBeenCalledWith(expect.objectContaining({ policy: { kind: "power_off" }, unitIds: ["u-4"] }));
    fireEvent.click(button(/Schedule restriction/));
    const again = within(screen.getByRole("dialog"));
    fireEvent.change(again.getByLabelText(/^Contract \(eligible RTO\)/), { target: { value: "k-1" } });
    fireEvent.click(again.getByRole("checkbox", { name: /Lobby AC/ }));
    fireEvent.change(again.getByLabelText(/^Contract \(eligible RTO\)/), { target: { value: "k-norec" } });
    expect(again.getByRole("checkbox", { name: /Lobby AC/ }).getAttribute("aria-checked")).toBe("false");
    fireEvent.click(again.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("the restrictions demo", () => {
  it("executes, requests release, retries and reconciles its fixture restrictions", () => {
    render(<RestrictionsDemo />);
    fireEvent.click(button("Request release…"));
    expect(nav.toasts).toStrictEqual(["Release requested"]);
    fireEvent.click(button("Retry"));
    fireEvent.click(button("Reconcile"));
    expect(nav.toasts.slice(1)).toStrictEqual(["Retry sent to offline unit", "Reconciled — released"]);
    fireEvent.click(button(/restriction-rto-b/));
    fireEvent.click(button(/Execute/));
    expect(nav.toasts.at(-1)).toBe("Executed — restriction applied");
    fireEvent.click(button(/restriction-x06/));
    expect(screen.getByText("unit-offline-rto · Study AC")).toBeDefined();
  });

  it("schedules a restriction once a contract and a time 24 h ahead are chosen", () => {
    render(<RestrictionsDemo />);
    fireEvent.click(button(/Schedule restriction/));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.click(dialog.getByRole("button", { name: "Schedule" }));
    expect(dialog.getByText("No client can view every unit (IR05) — pick an eligible contract")).toBeDefined();
    expect(dialog.getByText("Execute-after is required")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Contract/), { target: { value: "contract-rto-b" } });
    fireEvent.change(dialog.getByLabelText("Policy"), { target: { value: "Power off" } });
    const [when] = dialog.getAllByLabelText("date and time");
    if (!when) throw new Error("no time picker");
    fireEvent.change(when, { target: { value: new Date(Date.now() + 3600e3).toISOString().slice(0, 16) } });
    fireEvent.click(dialog.getByRole("button", { name: "Schedule" }));
    expect(dialog.getByText("Execute-after must be at least 24 h after the notice")).toBeDefined();
    fireEvent.change(when, { target: { value: new Date(Date.now() + 48 * 3600e3).toISOString().slice(0, 16) } });
    fireEvent.click(dialog.getByRole("button", { name: "Schedule" }));
    expect(nav.toasts).toStrictEqual(["Restriction scheduled; notice sent"]);
    expect(screen.getByText("restriction-new-1")).toBeDefined();
    fireEvent.click(button(/Schedule restriction/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button(/Schedule restriction/));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
