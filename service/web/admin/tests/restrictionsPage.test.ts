import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiRestriction, PerUnit } from "@ac/web/lib/restrictions";
import type { ApiContract, ApiInvoice } from "@ac/web/lib/billing";

// The HQ restrictions screen (FR-A09, FR-A10): the Server Actions (each with the restriction's version) and the Server
// Component — the URL scope, the per-state counts, the selected restriction with its cause invoices and latest
// commands, the release view of an override-only role, and the schedule form for restriction.write.
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
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown[]>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<{ items: unknown[]; total: number }>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/restrictions/_components/restrictions-view", () => ({ RestrictionsView: () => null }));
vi.mock("../app/admin/restrictions/_components/restrictions-demo", () => ({ RestrictionsDemo: () => null }));

import {
  cancelRestriction, deferRestriction, executeRestriction, exemptRestriction, overrideRestriction, reconcileUnits, requestRelease, retryUnits, scheduleRestriction, type ScheduleInput,
} from "../app/admin/restrictions/actions";
import AdminRestrictionsPage from "../app/admin/restrictions/page";
import { RestrictionsView } from "../app/admin/restrictions/_components/restrictions-view";
import { RestrictionsDemo } from "../app/admin/restrictions/_components/restrictions-demo";
import RestrictionsLoading from "../app/admin/restrictions/loading";
import RestrictionsErrorState from "../app/admin/restrictions/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const perUnit = (unitId: string, over: Partial<PerUnit> = {}): PerUnit => ({ unitId, applyState: "applied", releaseState: "none", applyCommandIds: [], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: null, ...over });
const restriction = (id: string, over: Partial<ApiRestriction> = {}): ApiRestriction => ({
  id, version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u-1"], rulesVersion: "demo-v1", policy: { kind: "power_off" }, state: "applied", perUnit: [perUnit("u-1")], recoveryCases: [],
  noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, contractId: "k-1", causeInvoiceIds: ["inv-1"], noticeNotificationIds: ["n-1", "n-2"], ...over,
});
const contract = (id: string, over: Partial<ApiContract & { rulesVersion: string | null }> = {}) => ({
  id, version: 2, customerId: "cust-a", customerOrgId: "org-a", unitIds: ["u-1", "u-2", "u-3", "u-gone"], planType: "rto", startAt: "2026-01-01T00:00:00Z", endAt: "2027-01-01T00:00:00Z",
  priceMinor: 10_000, currency: "MYR", restrictionEligible: true, rulesVersion: "demo-v1", ...over,
});
const invoice = (id: string, over: Partial<ApiInvoice> = {}): ApiInvoice => ({
  id, version: 1, number: `INV-${id}`, amountMinor: 12_000, currency: "MYR", contractId: "k-1", contractVersion: 2, period: { from: "2026-08-01T00:00:00Z", to: "2026-09-01T00:00:00Z" },
  dueAt: "2026-09-05T00:00:00Z", status: "unpaid", paymentMethod: null, paymentStatus: null, paidAt: null, ...over,
});
const units = [
  { id: "u-1", displayName: "Lobby AC", modelId: "m-ctl", archived: false }, { id: "u-2", displayName: "Office AC", modelId: "m-none", archived: false },
  { id: "u-3", displayName: "Old AC", modelId: "m-ctl", archived: true }, { id: "u-4", displayName: "Hall AC", modelId: "m-ctl", archived: false },
];
const capabilities = [{ id: "m-ctl", control: true, temperature: { min: 16, max: 30, step: 1 } }, { id: "m-none", control: false, temperature: null }];
/** The Core API of one test; `lists` answers coreAllPage and coreAll by operation, `ops` coreOp. */
function core(ops: Record<string, (input: unknown) => unknown>, lists: Record<string, unknown[]> = {}) {
  const op = new Map(Object.entries(ops)), list = new Map(Object.entries(lists));
  dal.coreOp.mockImplementation((name, input) => Promise.try(() => op.get(name)?.(input)));
  dal.coreAllPage.mockImplementation((name) => Promise.resolve({ items: list.get(name) ?? [], total: 0 }));
  dal.coreAll.mockImplementation((name) => Promise.resolve(list.get(name) ?? []));
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = NonNullable<ComponentProps<typeof RestrictionsView>["live"]>;
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminRestrictionsPage(props(sp));
  if (!isValidElement<ComponentProps<typeof RestrictionsView>>(page) || page.type !== RestrictionsView || !page.props.live) throw new TypeError("not the live RestrictionsView");
  return page.props.live;
}
const callsOf = (op: string) => dal.coreOp.mock.calls.filter(([name]) => name === op).map(([, input]) => input);
const lists = {
  "units.list": units, "contracts.list": [contract("k-1"), contract("k-poc", { planType: "maintenance" }), contract("k-old", { endAt: "2026-06-01T00:00:00Z" }), contract("k-norules", { rulesVersion: null })],
  "customers.list": [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a", status: "active" }],
  "invoices.list": [invoice("inv-1"), invoice("inv-2"), invoice("inv-x")],
  "capabilities.list": capabilities, "clientUsers.list": [{ customerId: "cust-a", status: "active" }, { customerId: "cust-b", status: "active" }],
};

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.corePermissions.mockResolvedValue(new Set(["restriction.read", "restriction.write", "asset.read"]));
  dal.coreOp.mockReset(); cache.refresh.mockReset();
});

describe("restriction Server Actions", () => {
  it("send each write with the restriction's version and re-render", async () => {
    dal.coreOp.mockResolvedValue({ id: "r-new" });
    const input: ScheduleInput = { contractId: "k-1", expectedContractVersion: 2, causeInvoiceIds: ["inv-1"], unitIds: ["u-1"], policy: { kind: "power_off" }, executeAfter: "2026-09-20T00:00:00Z", reason: "Overdue", rulesVersion: "demo-v1" };
    expect(await scheduleRestriction(input)).toStrictEqual({ ok: true, value: "r-new" });
    expect(dal.coreOp).toHaveBeenLastCalledWith("restrictions.schedule", input, { write: true });
    const v = { write: true, expectedVersion: 3 };
    const cases: [() => Promise<unknown>, string, unknown][] = [
      [() => executeRestriction("r-1", 3, "demo-v1"), "restrictions.execute", { restrictionId: "r-1", confirmedRulesVersion: "demo-v1" }],
      [() => requestRelease("r-1", 3), "restrictions.release", { restrictionId: "r-1" }],
      [() => retryUnits("r-1", 3, ["u-1"], "apply", "demo-v1", "Offline"), "restrictions.retry", { restrictionId: "r-1", unitIds: ["u-1"], phase: "apply", confirmedRulesVersion: "demo-v1", reason: "Offline" }],
      [() => reconcileUnits("r-1", 3, ["u-1"]), "restrictions.reconcile", { restrictionId: "r-1", unitIds: ["u-1"] }],
      [() => deferRestriction("r-1", 3, "2026-09-20T00:00:00Z", "Paying"), "restrictions.defer", { restrictionId: "r-1", until: "2026-09-20T00:00:00Z", reason: "Paying" }],
      [() => exemptRestriction("r-1", 3, "2026-09-20T00:00:00Z", "Hospital"), "restrictions.exempt", { restrictionId: "r-1", until: "2026-09-20T00:00:00Z", reason: "Hospital" }],
      [() => cancelRestriction("r-1", 3, "Paid"), "restrictions.cancel", { restrictionId: "r-1", reason: "Paid" }],
      [() => overrideRestriction("r-1", 3, "Emergency"), "restrictions.override", { restrictionId: "r-1", reason: "Emergency" }],
    ];
    for (const [call, op, body] of cases) {
      expect(await call()).toStrictEqual({ ok: true, value: null });
      expect(dal.coreOp).toHaveBeenLastCalledWith(op, body, v);
    }
    expect(cache.refresh).toHaveBeenCalledTimes(cases.length + 1);
  });

  it("returns a refusal with its code", async () => {
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await cancelRestriction("r-1", 2, "Paid")).toMatchObject({ ok: false, code: "CONFLICT", messageKey: "error.conflict" });
  });
});

describe("the restrictions page (Server Component)", () => {
  it("names the restrictions while they load or fail", () => {
    expect(RestrictionsLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = RestrictionsErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("shows the demo outside API mode, and nothing to a role without any restriction permission", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo: ReactNode = await AdminRestrictionsPage(props());
    expect(isValidElement(demo) && demo.type).toBe(RestrictionsDemo);
    dal.apiMode.mockReturnValue(true);
    dal.corePermissions.mockResolvedValue(new Set(["asset.read"]));
    const none: ReactNode = await AdminRestrictionsPage(props());
    expect(isValidElement<ComponentProps<typeof RestrictionsView>>(none) && none.type === RestrictionsView && none.props.live).toBeNull();
  });

  it("reads the URL scope with per-state counts, the cause invoice filter, and the first restriction with its commands", async () => {
    const r = restriction("r-11112222", { perUnit: [perUnit("u-1", { applyCommandIds: ["c-old", "c-1"], releaseCommandIds: ["c-rel"] }), perUnit("u-gone")], unitIds: ["u-1", "u-gone"] });
    core({
      "restrictions.list": (q) => (JSON.stringify(q).includes('"limit":1,') ? { items: [], total: JSON.stringify(q).includes('"applied"') ? 1 : 0 } : { items: [r], total: 1 }),
      "restrictions.get": () => r,
      "commands.get": (q) => (JSON.stringify(q).includes("c-rel") ? Promise.reject(refused("NOT_FOUND")) : { id: "c-1", status: "acknowledged", delivery: "acked", requestedAt: "2026-09-13T01:00:00Z", failureCode: null }),
      "invoices.list": () => ({ items: [invoice("inv-1", { status: "paid" })] }),
    }, { ...lists, "restrictions.list": [r, restriction("r-2", { state: "released", causeInvoiceIds: ["inv-2"] })] });
    const live = await liveOf({ contractId: "k-1", invoiceId: "inv-1", status: "applied" });
    expect(callsOf("restrictions.list")[0]).toStrictEqual({ limit: 100, filters: { contractId: "k-1", invoiceId: "inv-1", status: "applied" } });
    expect([live.full, live.canWrite, live.canUnits, live.scope]).toStrictEqual([true, true, true, { contractId: "k-1", invoiceId: "inv-1", status: "applied" }]);
    expect([live.counts.applied, live.counts.released]).toStrictEqual([1, 0]);
    expect(live.causes).toStrictEqual([{ id: "inv-1", label: "INV-inv-1 · Demo Customer A" }, { id: "inv-2", label: "INV-inv-2 · Demo Customer A" }]); // the invoices restrictions cite
    expect(live.contracts.find((k) => k.id === "k-1")?.label).toBe("Demo Customer A · RTO contract v2");
    expect(callsOf("commands.get")).toStrictEqual([{ id: "c-1" }, { id: "c-rel" }]); // the latest of each phase
    expect(live.selected?.causes).toStrictEqual([{ id: "inv-1", number: "INV-inv-1", paid: true, processing: false, amountMinor: 12_000, currency: "MYR" }]);
    expect([live.selected?.allPaid, live.selected?.contract, live.selected?.texts.notice]).toStrictEqual([true, "Demo Customer A · RTO contract v2", "12 Sept 2026, 8:00 am MYT · 2 client notices"]);
  });

  it("offers the schedule form only eligible RTO contracts in term, with their overdue unpaid invoices and the units that can be restricted", async () => {
    const active = restriction("r-act", { unitIds: ["u-4"], state: "scheduled" });
    const recovering = restriction("r-rec", { state: "released", recoveryCases: [{ id: "rc-1", unitId: "u-2", state: "pending", successorRestrictionId: null }] });
    core({ "restrictions.list": () => ({ items: [], total: 0 }) }, {
      ...lists, "restrictions.list": [active, recovering],
      "invoices.list": [invoice("inv-1"), invoice("inv-p", { status: "processing" }), invoice("inv-k2", { contractId: "k-2" })],
      "contracts.list": [...lists["contracts.list"], contract("k-2", { unitIds: ["u-4"] })],
    });
    const live = await liveOf();
    expect(live.schedule?.map((k) => k.id)).toStrictEqual(["k-1", "k-2"]);
    const k1 = live.schedule?.[0];
    expect([k1?.recipients, k1?.invoices, k1?.rulesVersion]).toStrictEqual([1, [{ id: "inv-1", number: "INV-inv-1" }], "demo-v1"]);
    expect(k1?.units).toStrictEqual([
      { id: "u-1", name: "Lobby AC", blocked: null, temperature: { min: 16, max: 30, step: 1 } },
      { id: "u-2", name: "Office AC", blocked: "unresolved recovery case", temperature: null },
      { id: "u-3", name: "Old AC", blocked: "archived", temperature: { min: 16, max: 30, step: 1 } },
      { id: "u-gone", name: "u-gone", blocked: "archived", temperature: null },
    ]);
    expect(live.schedule?.[1]?.units[0]?.blocked).toBe("under restriction r-act (Scheduled)");
    expect(live.selected).toBeUndefined(); // nothing in the scope to open
  });

  it("names a unit without power control, and opens the URL's restriction", async () => {
    const r = restriction("r-2", { state: "scheduled", causeInvoiceIds: [], noticeNotificationIds: ["n-1"] });
    core({ "restrictions.list": () => ({ items: [restriction("r-1"), r], total: 2 }), "restrictions.get": () => r, "invoices.list": () => ({ items: [] }) },
      { ...lists, "contracts.list": [contract("k-1", { unitIds: ["u-2"] })], "capabilities.list": [{ id: "m-none", control: false, temperature: null }] });
    const live = await liveOf({ restrictionId: "r-2", status: "nonsense" });
    expect(callsOf("restrictions.get")).toStrictEqual([{ id: "r-2" }]);
    expect(live.scope.status).toBeUndefined();
    expect([live.selected?.allPaid, live.selected?.texts.notice]).toStrictEqual([false, "12 Sept 2026, 8:00 am MYT · 1 client notice"]);
    expect(live.schedule?.[0]?.units[0]?.blocked).toBe("no power control");
  });

  it("gives an override-only role the release view: no billing scope, unit IDs, no schedule form", async () => {
    dal.corePermissions.mockResolvedValue(new Set(["restriction.override"]));
    // the release projection (IR03): no contract, cause invoices or notices
    const r: ApiRestriction = {
      id: "r-1", version: 3, createdAt: "2026-09-12T00:00:00Z", unitIds: ["u-1"], rulesVersion: "demo-v1", policy: { kind: "power_off" }, state: "applied", perUnit: [perUnit("u-1")],
      recoveryCases: [], noticeAt: "2026-09-12T00:00:00Z", executeAfter: "2026-09-13T00:00:00Z", releaseIntent: null, projection: "release",
    };
    core({ "restrictions.list": () => ({ items: [r], total: 1 }), "restrictions.get": () => r }, { "restrictions.list": [r] });
    const live = await liveOf({ contractId: "k-1", invoiceId: "inv-1" });
    expect([live.full, live.canWrite, live.canUnits, live.scope.contractId, live.scope.invoiceId, live.contracts, live.causes, live.schedule]).toStrictEqual([false, false, false, undefined, undefined, [], [], undefined]);
    expect(dal.coreAllPage).not.toHaveBeenCalledWith("units.list");
    expect(callsOf("invoices.list")).toStrictEqual([]);
    expect([live.selected?.contract, live.selected?.causes, live.selected?.texts.notice]).toStrictEqual(["release view", [], "12 Sept 2026, 8:00 am MYT · 0 client notices"]);
    expect(live.rows[0]?.id).toBe("r-1");
  });

  it("labels a contract it cannot find, and a customer it cannot name", async () => {
    dal.corePermissions.mockResolvedValue(new Set(["restriction.read"]));
    const r = restriction("r-1", { contractId: "k-unknown" });
    core({ "restrictions.list": () => ({ items: [r], total: 1 }), "restrictions.get": () => r, "invoices.list": () => ({ items: [] }) },
      { "restrictions.list": [r], "contracts.list": [contract("k-1", { customerId: "cust-x" })], "invoices.list": [invoice("inv-1", { contractId: "k-unknown" })] });
    const live = await liveOf();
    expect([live.selected?.contract, live.contracts[0]?.label, live.causes[0]?.label, live.selected?.causes[0]?.number]).toStrictEqual(["contract", "customer · RTO contract v2", "INV-inv-1 · customer", "inv-1"]);
    expect(live.schedule).toBeUndefined(); // restriction.read without write
  });
});
