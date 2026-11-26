// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import { contractRows, unitOptions, type ApiContractFull } from "@ac/web/lib/contracts";
import type { ApiCustomer } from "@ac/web/lib/billing";

// HQ contract plans (FR-A07, SCR-A07): the Server Component (the URL scope, the plan chip and the plan counts of the scope, the selected contract, the units
// already on another contract) and the editor (customer and units by place, plan, Kuala Lumpur dates, price,
// restriction eligibility for RTO only, read-only while a restriction is active). The Phase 1A demo moves locally.
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
    CoreError, apiMode: vi.fn(() => true), coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["contract.read", "asset.read", "restriction.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const nav = vi.hoisted(() => ({ replace: vi.fn<(href: string, options?: unknown) => void>(), toasts: [] as string[], save: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("next/navigation", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useRouter: () => ({ replace: nav.replace, refresh: vi.fn() }), usePathname: () => "/admin/billing/contracts" }));
vi.mock("@ac/web/lib/useRefreshThen", () => ({ useRefreshThen: () => (then: () => void) => { then(); } }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));

import AdminContractsPage from "../app/admin/billing/contracts/page";
import { ContractsView } from "../app/admin/billing/contracts/_components/contracts-view";
import ContractsLoading from "../app/admin/billing/contracts/loading";
import ContractsErrorState from "../app/admin/billing/contracts/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const customers: ApiCustomer[] = [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a", status: "active" }, { id: "cust-x", name: "Gone Ltd", organizationId: "org-x", status: "inactive" }];
const contract = (id: string, over: Partial<ApiContractFull> = {}): ApiContractFull => ({
  id, version: 2, activeRestrictionIds: [], hasUnresolvedRecovery: false, customerId: "cust-a", customerOrgId: "org-a", unitIds: ["u-1"], planType: "rto",
  startAt: "2025-12-31T16:00:00Z", endAt: "2026-12-31T16:00:00Z", priceMinor: 12_000, currency: "MYR", restrictionEligible: true, rulesVersion: "demo-v1", ...over,
});
const units = [
  { id: "u-1", customerOrgId: "org-a", propertyId: "p-1", displayName: "Lobby AC", archived: false }, { id: "u-2", customerOrgId: "org-a", propertyId: "p-1", displayName: "Office AC", archived: false },
  { id: "u-3", customerOrgId: "org-a", propertyId: "p-2", displayName: "Villa AC", archived: false },
];
const properties = [{ id: "p-1", name: "Tower", customerOrgId: "org-a", archived: false }, { id: "p-2", name: "Villa", customerOrgId: "org-a", archived: false }];

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
  nav.replace.mockReset(); nav.toasts = []; nav.save.mockReset();
  history.replaceState(null, "", "/admin/billing/contracts");
});
afterEach(cleanup);

type Live = NonNullable<ComponentProps<typeof ContractsView>["live"]>;
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminContractsPage(props(sp));
  if (!isValidElement<ComponentProps<typeof ContractsView>>(page) || page.type !== ContractsView || !page.props.live) throw new TypeError("not the live ContractsView");
  return page.props.live;
}
function core(all: ApiContractFull[]) {
  dal.coreAll.mockImplementation((_op, input) => {
    const q = JSON.stringify(input ?? {});
    return Promise.resolve(all.filter((k) => (!q.includes('"kind"') || q.includes(`"${k.planType}"`)) && (!q.includes("unitId") || k.unitIds.includes("u-3"))));
  });
  const pages = new Map<string, unknown[]>([["customers.list", customers], ["properties.list", properties], ["units.list", units]]);
  dal.coreAllPage.mockImplementation((op) => Promise.resolve({ items: pages.get(op) ?? [], total: 0 }));
}

describe("the contracts page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminContractsPage(props());
    expect(isValidElement<ComponentProps<typeof ContractsView>>(page) && page.type === ContractsView && page.props.live).toBeUndefined();
    expect(ContractsLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = ContractsErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("counts the plans of every contract and opens the first, active customers only", async () => {
    core([contract("k-1"), contract("k-2", { planType: "general", restrictionEligible: false, rulesVersion: null })]);
    const live = await liveOf();
    expect(dal.coreAll.mock.calls).toStrictEqual([["contracts.list"]]); // no scope: one read
    expect([live.counts, live.selectedId]).toStrictEqual([{ all: 2, rto: 1, general: 1, energy: 0, environment: 0 }, "k-1"]);
    expect(Object.values(live.scope).filter((v) => v !== undefined)).toStrictEqual([]); // no scope
    expect(live.customers).toStrictEqual([{ id: "cust-a", name: "Demo Customer A" }]);
    expect(live.units.find((u) => u.id === "u-1")?.otherContracts).toStrictEqual(["k-1", "k-2"]);
  });

  it("counts the plans of the URL scope and lists the URL's plan, opening the URL's contract", async () => {
    core([contract("k-1"), contract("k-3", { unitIds: ["u-3"], planType: "energy" }), contract("k-4", { unitIds: ["u-3"] })]);
    const live = await liveOf({ customerId: "cust-a", unitId: "u-3", kind: "rto", contractId: "k-4" });
    expect(dal.coreAll.mock.calls.map(([, input]) => input)).toStrictEqual([undefined, { filters: { customerId: "cust-a", unitId: "u-3" } }, { filters: { customerId: "cust-a", unitId: "u-3", kind: "rto" } }]);
    expect([live.counts.all, live.counts.energy, live.counts.rto, live.rows.map((r) => r.id), live.selectedId]).toStrictEqual([2, 1, 1, ["k-4"], "k-4"]);
    const unknown = await liveOf({ kind: "lease", contractId: "k-gone" });
    expect([unknown.scope.kind, unknown.selectedId]).toStrictEqual([undefined, "k-1"]);
    core([]);
    const empty = await liveOf();
    expect(empty.selectedId).toBeNull();
  });

  it("names contract.read when the contracts are refused (IR582)", async () => {
    dal.coreAll.mockRejectedValue(refused("FORBIDDEN"));
    dal.coreAllPage.mockResolvedValue({ items: [], total: 0 });
    dal.corePermissions.mockResolvedValueOnce(new Set(["contract.read", "asset.read"])).mockResolvedValueOnce(new Set());
    const page: ReactNode = await AdminContractsPage(props());
    expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Contracts", permission: "contract.read" });
  });
});

vi.mock("../app/admin/billing/contracts/actions", () => ({ saveContract: nav.save }));
const all = [
  contract("k-11112222"), contract("k-33334444", { planType: "general", restrictionEligible: false, rulesVersion: null, unitIds: ["u-2"] }),
  contract("k-55556666", { activeRestrictionIds: ["r-77778888"], unitIds: ["u-3"] }), contract("k-99990000", { hasUnresolvedRecovery: true, unitIds: ["u-3"] }),
];
function viewLive(over: Partial<Live> = {}): Live {
  const rows = contractRows(all, customers);
  return {
    canOpenRestrictions: true, scope: {}, rows, counts: { all: 4, rto: 3, general: 1, energy: 0, environment: 0 }, selectedId: rows[0]?.id ?? null,
    customers: [{ id: "cust-a", name: "Demo Customer A" }, { id: "cust-b", name: "Beta Hotel" }], units: unitOptions(units, customers, properties, all), ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const lastHref = () => nav.replace.mock.lastCall?.[0];

describe("the contract editor in API mode", () => {
  it("puts the scope, plan chip and selection in the URL, and says when nothing matches", () => {
    const { rerender } = render(<ContractsView live={viewLive({ scope: { customerId: "cust-a" } })} />);
    expect(screen.getByText("4 contracts in scope")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Customer", { selector: "select:not([disabled])" }), { target: { value: "cust-b" } });
    expect(lastHref()).toBe("/admin/billing/contracts?customerId=cust-b");
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "u-1" } });
    expect(lastHref()).toBe("/admin/billing/contracts?unitId=u-1");
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing/contracts");
    fireEvent.click(screen.getByRole("tab", { name: /^General/ }));
    expect(lastHref()).toBe("/admin/billing/contracts?kind=general");
    fireEvent.click(button(/k-333344/));
    expect(lastHref()).toBe("/admin/billing/contracts?contractId=k-33334444");
    rerender(<ContractsView live={viewLive({ scope: { customerId: "cust-a", kind: "rto" } })} />);
    fireEvent.click(screen.getByRole("tab", { name: /^All/ }));
    expect(lastHref()).toBe("/admin/billing/contracts"); // all plans is no key
    fireEvent.change(screen.getByLabelText("Customer", { selector: "select:not([disabled])" }), { target: { value: "" } });
    expect(lastHref()).toBe("/admin/billing/contracts");
    rerender(<ContractsView live={viewLive({ rows: [], selectedId: null, counts: { all: 1, rto: 1, general: 0, energy: 0, environment: 0 }, scope: { kind: "energy" } })} />);
    expect(screen.getByText("No contract matches this scope.")).toBeDefined();
    expect(screen.getByText("1 contract in scope")).toBeDefined();
    expect(screen.getByText("Nothing selected")).toBeDefined();
    rerender(<ContractsView live={viewLive({ rows: [], selectedId: null })} />);
    expect(screen.getByText("Create the first one with New contract.")).toBeDefined();
  });

  it("saves the next version of a contract with its changes, labels units on other contracts, and drops eligibility off RTO", async () => {
    nav.save.mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} }).mockResolvedValueOnce({ ok: true, value: { id: "k-11112222", version: 3 } });
    render(<ContractsView live={viewLive()} />);
    expect(screen.getByText("Demo Customer A · Version 2")).toBeDefined();
    expect(screen.getByText("also on k-333344")).toBeDefined(); // the selected one is not named
    fireEvent.click(screen.getByRole("checkbox", { name: /Office AC/ }));
    expect(screen.getByText(/Version 2 · Editing → v3/)).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Price/), { target: { value: "150" } });
    expect(screen.getByText("New invoices will use version 3 (150.00 MYR)")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^End/), { target: { value: "2027-06-30" } });
    fireEvent.change(screen.getByLabelText("Currency"), { target: { value: "USD" } });
    fireEvent.change(screen.getByLabelText(/^Rules version/), { target: { value: " demo-v2 " } });
    fireEvent.click(button("Save as version 3"));
    await flush();
    expect(nav.toasts).toHaveLength(1); // the refusal
    fireEvent.click(button("Save as version 3"));
    await flush();
    expect(nav.save).toHaveBeenLastCalledWith({
      id: "k-11112222", customerId: "cust-a", unitIds: ["u-1", "u-2"], planType: "rto", startAt: "2025-12-31T16:00:00Z", endAt: "2027-06-30T00:00:00+08:00", priceMinor: 15_000, currency: "USD",
      restrictionEligible: true, rulesVersion: "demo-v2",
    }, 2);
    expect(nav.toasts.at(-1)).toBe("Saved as version 3");
    expect(lastHref()).toBe("/admin/billing/contracts?contractId=k-11112222");
    fireEvent.click(screen.getByRole("checkbox", { name: /Office AC/ })); // and off again
    expect(screen.getByRole("checkbox", { name: /Office AC/ }).getAttribute("aria-checked")).toBe("false");
    fireEvent.click(screen.getByRole("checkbox", { name: /Allow cooling restrictions/ }));
    expect(screen.getByLabelText<HTMLInputElement>(/^Rules version/).disabled).toBe(true); // no longer eligible
    fireEvent.change(screen.getByLabelText("Plan type"), { target: { value: "energy" } });
    expect(screen.getByRole("checkbox", { name: /Allow cooling restrictions/ }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText("Not available for Energy contracts — only RTO contracts can be restriction eligible.")).toBeDefined();
  });

  it("creates a contract for the scoped customer, selects a whole property at once, and checks the draft first", async () => {
    nav.save.mockResolvedValue({ ok: true, value: { id: "k-new", version: 1 } });
    render(<ContractsView live={viewLive({ scope: { customerId: "cust-a" } })} />);
    fireEvent.click(button(/New contract/));
    expect(screen.getByText("Saving creates version 1")).toBeDefined();
    fireEvent.click(button("Create contract"));
    for (const text of ["Include at least one unit", "Required"]) expect(screen.getAllByText(text).length).toBeGreaterThan(0);
    expect(nav.save).not.toHaveBeenCalled();
    fireEvent.click(button("Select all units of Tower"));
    expect(button("Clear the units of Tower")).toBeDefined();
    fireEvent.click(button("Clear the units of Tower"));
    fireEvent.click(button("Select all units of Tower"));
    fireEvent.change(screen.getByLabelText(/^Start/), { target: { value: "2026-10-01" } });
    fireEvent.change(screen.getByLabelText(/^End/), { target: { value: "2027-10-01" } });
    fireEvent.change(screen.getByLabelText(/^Price/), { target: { value: "50" } });
    fireEvent.click(button("Create contract"));
    await flush();
    expect(nav.save.mock.lastCall?.[0]).toMatchObject({ customerId: "cust-a", unitIds: ["u-1", "u-2"], planType: "general", priceMinor: 5000, restrictionEligible: false, rulesVersion: null });
    expect(nav.save.mock.lastCall?.[1]).toBeUndefined(); // a new contract: no version
    expect(nav.save.mock.lastCall?.[0]).not.toHaveProperty("id");
    expect(nav.toasts).toStrictEqual(["Contract created"]);
  });

  it("asks for the customer first, says when it has no units, and cancels a new contract", () => {
    render(<ContractsView live={viewLive()} />);
    fireEvent.click(button(/New contract/));
    expect(screen.getByText("Choose a customer first.")).toBeDefined();
    const [, owner] = screen.getAllByLabelText(/^Customer/);
    if (!owner) throw new Error("no owner select");
    fireEvent.change(owner, { target: { value: "cust-b" } });
    expect(screen.getByText("This customer has no active units.")).toBeDefined();
    fireEvent.click(button("Cancel"));
    expect(screen.getByText("Demo Customer A · Version 2")).toBeDefined();
  });

  it("is read-only while a restriction is active or a recovery case is open, with the restriction to open", () => {
    const rows = contractRows(all, customers);
    const { rerender } = render(<ContractsView live={viewLive({ selectedId: rows[2]?.id ?? null })} />);
    expect(screen.getByText("Read-only while a restriction is active")).toBeDefined();
    expect(button("Save as version 3").disabled).toBe(true);
    expect(screen.getByRole("link", { name: /Open restriction/ }).getAttribute("href")).toBe("/admin/restrictions?restrictionId=r-77778888");
    expect(screen.getByText(/1 active restriction on this contract/)).toBeDefined();
    rerender(<ContractsView live={viewLive({ selectedId: rows[3]?.id ?? null, canOpenRestrictions: false })} />);
    expect(screen.getByText(/an unresolved recovery case exists/)).toBeDefined();
    expect(screen.queryByRole("link", { name: /Open restriction/ })).toBeNull();
  });
});

describe("the contracts demo", () => {
  it("saves a new version of a fixture contract, filters by plan and creates one locally", () => {
    render(<ContractsView />);
    expect(screen.getByText(/Read-only while a restriction is active/)).toBeDefined(); // contract-rto-a holds a restriction
    fireEvent.click(button(/contract-rto-b/));
    fireEvent.change(screen.getByLabelText(/^End/), { target: { value: "2027-06-01" } });
    fireEvent.click(button("Save as version 2"));
    expect(screen.getAllByText("2026-03-01 → 2027-06-01").length).toBeGreaterThan(0); // eligible, 1 unit
    fireEvent.click(button(/contract-general-a/));
    fireEvent.change(screen.getByLabelText(/^Price/), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("checkbox", { name: /unit-online-rto|Bedroom AC/ }));
    fireEvent.click(button("Save as version 2"));
    expect(nav.toasts).toStrictEqual(["Saved as version 2", "Saved as version 2"]);
    expect(screen.getAllByText(/2 units/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("60.00 MYR").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("tab", { name: /^General/ }));
    expect(screen.queryByRole("button", { name: /contract-rto-b/ })).toBeNull();
    fireEvent.click(button(/New contract/));
    fireEvent.change(screen.getAllByLabelText(/^Customer/)[0] ?? document.body, { target: { value: "customer-b" } });
    fireEvent.click(screen.getByRole("checkbox", { name: /unit-other-customer/ }));
    fireEvent.change(screen.getByLabelText(/^Start/), { target: { value: "2026-10-01" } });
    fireEvent.change(screen.getByLabelText(/^End/), { target: { value: "2027-10-01" } });
    fireEvent.change(screen.getByLabelText(/^Price/), { target: { value: "70" } });
    fireEvent.click(button("Create contract"));
    expect(nav.toasts.at(-1)).toBe("Contract created");
    expect(nav.replace).not.toHaveBeenCalled();
  });
});
