import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiClientUser, ApiContractLite, ApiCoverage, ApiCustomerPolicy, ApiCustomerRow, ApiOrgRow, ApiPropertyRow, ApiSpaceRow, ApiUnitDetail, ApiUnitRow } from "@ac/web/lib/assets";

// HQ customers & units (FR-A02, FR-A19, SCR-A02): the Server Component reads the register with the contract standing
// where the session may read it; a customer's tree, models, policies, alerts, client users and last edit; a unit with
// its device, attached policies and what blocks taking it out of use (D05); a unit link without its customer
// redirects; tab=warranty reads the coverage and the claimable jobs.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    readonly status: number;
    constructor(error: DomainError, status = 422, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
      this.status = status;
    }
  }
  return {
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set<string>())),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const navigation = vi.hoisted(() => ({ redirect: vi.fn((url: string) => { throw new Error(`NEXT_REDIRECT ${url}`); }) }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", () => navigation);
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/units/_components/units-view", () => ({ UnitsView: () => null }));
vi.mock("../app/admin/units/_components/units-demo", () => ({ UnitsDemo: () => null }));

import AdminUnitsPage from "../app/admin/units/page";
import { UnitsView } from "../app/admin/units/_components/units-view";
import { UnitsDemo } from "../app/admin/units/_components/units-demo";
import UnitsLoading from "../app/admin/units/loading";
import UnitsErrorState from "../app/admin/units/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const ALL = [
  "asset.read", "asset.write", "contract.read", "billing.read", "restriction.read", "identity.read", "alert.read", "alert.policy.read", "alert.policy.write", "dashboard.read", "device.read",
  "device.write", "audit.read", "job.read", "job.write", "data.forceDelete",
];
const customer = (id: string, org: string, status: "active" | "inactive" = "active"): ApiCustomerRow => ({ id, status, version: 1, name: `Customer ${id}`, organizationId: org, serviceProfile: "general", createdAt: "2026-01-01T00:00:00Z" });
const property = (id: string, org: string): ApiPropertyRow => ({ id, version: 1, customerOrgId: org, kind: "home", name: `Home ${id}`, address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-01-01T00:00:00Z" });
const unit = (id: string, over: Partial<ApiUnitRow> = {}): ApiUnitRow => ({
  id, version: 3, customerOrgId: "org-a", propertyId: "p-1", spaceId: "s-1", displayName: `Unit ${id}`, modelId: "m-1", type: "split", installState: "installed", installedAt: null, serviceScope: ["indoor"],
  alertPolicyIds: [], archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: "2026-09-14T00:50:00Z", warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, ...over,
});
const detail = (id: string, over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({ ...unit(id), location: { pathLabels: ["Home p-1", "Bedroom"], address: null, accessInstructions: null }, pendingCommandIds: [], ...over });
const space: ApiSpaceRow = { id: "s-1", version: 1, propertyId: "p-1", parentSpaceId: null, kind: "room", name: "Bedroom", archived: false };
const contract = (id: string, unitIds: string[], endAt = "2027-01-01T00:00:00Z"): ApiContractLite => ({ id, unitIds, endAt, customerId: "cust-a", planType: "rto", startAt: "2026-01-01T00:00:00Z", activeRestrictionIds: [] });
const defaults: ApiCustomerPolicy = {
  id: "pol-default", version: 1, kind: "default_alert", name: "Default alerts", unitIds: [], enabled: true, ownerMembershipId: "m-hq",
  rules: [{ ruleKey: "offline", name: "AC offline", metric: "heartbeat_gap", operator: "gt", threshold: 10, durationSeconds: 0, category: "fault" }], ruleSettings: [],
};
const alertPolicy: ApiCustomerPolicy = { id: "pol-temp", version: 2, kind: "alert", name: "Too warm", unitIds: ["u-1"], enabled: false, ownerMembershipId: "m-owner", metric: "temperature", operator: "gt", threshold: 28, durationSeconds: 600, severity: "warning" };
const clientUser: ApiClientUser = {
  id: "cu-1", version: 1, customerId: "cust-a", membershipId: "m-cu", email: "a@example.com", displayName: "Aisha", clientRole: "owner", status: "active", lastSignInAt: null,
  allowedChannels: [], invitedAt: "2026-02-01T00:00:00Z", invitedByMembershipId: "m-hq",
};
interface World {
  units?: ApiUnitRow[]; detail?: ApiUnitDetail | Error; coverage?: ApiCoverage[]; jobs?: unknown[]; devices?: unknown[]; audit?: unknown[]; policies?: ApiCustomerPolicy[]; caps?: unknown[];
  iots?: unknown[]; users?: ApiClientUser[]; job?: unknown;
}
function core(w: World = {}) {
  const lists = new Map<string, unknown>([
    ["organizations.list", [{ id: "org-a", version: 2, name: "Alpha Sdn Bhd", kind: "customer", status: "active" }, { id: "org-b", version: 1, name: "Beta", kind: "customer", status: "active" }] satisfies ApiOrgRow[]],
    ["properties.list", [property("p-1", "org-a"), property("p-2", "org-b")]],
    ["units.list", w.units ?? [unit("u-1"), unit("u-2", { spaceId: null, connection: "offline" }), unit("u-b", { customerOrgId: "org-b", propertyId: "p-2", spaceId: null })]],
    ["contracts.list", [contract("k-1", ["u-1"]), contract("k-old", ["u-1"], "2026-01-31T00:00:00Z")]], ["invoices.list", []], ["restrictions.list", [{ id: "r-1", state: "applied", unitIds: ["u-1"] }, { id: "r-2", state: "released", unitIds: ["u-1"] }]],
    ["members.list", [{ id: "m-hq", displayName: "HQ Ops" }, { id: "m-owner", displayName: "Owner Olivia" }]],
    ["spaces.list", [space]], ["capabilities.list", w.caps ?? [{ id: "m-1", manufacturer: "AC-Co", model: "Split 1", temperature: null, ventilation: false }]],
    ["iotModels.list", w.iots ?? [{ id: "i-1", manufacturer: "IoT-Co", model: "Node", sensors: [{ metric: "temperature" }] }]], ["policies.list", w.policies ?? [defaults, alertPolicy]],
    ["clientUsers.list", w.users ?? [clientUser]], ["units.coverage", w.coverage ?? []],
    ["jobs.list", w.jobs ?? [{ projection: "summary", id: "job-1", type: "repair", status: "assigned" }, { projection: "summary", id: "job-2", type: "repair", status: "completed" }]],
    ["devices.list", w.devices ?? [{ id: "d-1", version: 2, serial: "AC-0001", unitId: "u-1", connection: "online", iotModelId: "i-1", mqttNode: "node_1" }]],
  ]);
  dal.coreAll.mockImplementation((op, query) => {
    if (op === "alerts.list") return Promise.resolve(JSON.stringify(query).includes("acknowledged") ? [] : [{ severity: "critical" }, { severity: "normal" }]);
    if (op === "customers.list") return Promise.resolve(JSON.stringify(query).includes("inactive") ? [customer("cust-old", "org-old", "inactive")] : [customer("cust-a", "org-a"), customer("cust-b", "org-b")]);
    return Promise.resolve(lists.get(op) ?? []);
  });
  dal.coreOp.mockImplementation((op, input) => {
    if (op === "units.get") return Error.isError(w.detail) ? Promise.reject(w.detail) : Promise.resolve(w.detail ?? detail("u-1"));
    if (op === "audit.list") return Promise.resolve({ items: w.audit ?? [{ occurredAt: "2026-09-01T00:00:00Z", actorId: "m-hq", reason: "Moved" }] });
    if (op === "jobs.get") return JSON.stringify(input).includes("job-gone") ? Promise.reject(new Error("gone")) : Promise.resolve(w.job ?? { projection: "detail", id: "job-9", version: 2, type: "repair", unitId: "u-1", completedAt: "2026-09-01T00:00:00Z", costs: [], reportRefs: [{ reportId: "rep-1", reportVersion: 1 }] });
    if (op === "reports.get") return Promise.resolve({ parts: [{ name: "Compressor", quantity: 1, catalogCode: null }], acceptedAt: "2026-09-01T00:00:00Z" });
    return Promise.resolve({});
  });
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = NonNullable<ComponentProps<typeof UnitsView>["live"]>;
async function liveOf(sp: Record<string, string> = {}): Promise<Live | null> {
  const page: ReactNode = await AdminUnitsPage(props(sp));
  if (!isValidElement<ComponentProps<typeof UnitsView>>(page) || page.type !== UnitsView) throw new TypeError("not UnitsView");
  return page.props.live;
}
async function readLive(sp: Record<string, string> = {}): Promise<Live> {
  const live = await liveOf(sp);
  if (!live) throw new TypeError("no live data");
  return live;
}
const customerOf = async (sp: Record<string, string>) => { const live = await readLive(sp); return live.customer; };
const unitOf = async (sp: Record<string, string>) => { const c = await customerOf(sp); return c?.unit; };
const warrantyOf = async (sp: Record<string, string>) => { const live = await readLive(sp); return live.warranty; };
const auditQuery = () => dal.coreOp.mock.calls.findLast(([op]) => op === "audit.list")?.[1];
const perms = (...ps: string[]) => { dal.corePermissions.mockResolvedValue(new Set(ps)); };

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); navigation.redirect.mockClear();
  perms(...ALL);
});

describe("the units page (Server Component)", () => {
  it("shows the demo outside API mode, its loading and error states, and nothing without asset.read", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminUnitsPage(props());
    expect(isValidElement(page) && page.type).toBe(UnitsDemo);
    expect(UnitsLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = UnitsErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
    dal.apiMode.mockReturnValue(true);
    perms("dashboard.read");
    expect(await liveOf()).toBeNull();
  });

  it("lists every customer of both statuses with its standing, and counts the coverage needing attention", async () => {
    core({ coverage: [{ unitId: "u-1", customerId: "cust-a", modelId: "m-1", warrantyEndsAt: null, contractIds: [], status: "expiring", claimableJobIds: [] }, { unitId: "u-2", customerId: "cust-a", modelId: "m-1", warrantyEndsAt: null, contractIds: [], status: "contract", claimableJobIds: [] }] });
    const live = await readLive({ search: "alp" });
    expect(live.rows.map((r) => r.id).toSorted((a, b) => a.localeCompare(b))).toStrictEqual(["cust-a", "cust-b", "cust-old"]);
    expect(live.activeCustomers.map((c) => c.id).toSorted((a, b) => a.localeCompare(b))).toStrictEqual(["cust-a", "cust-b"]);
    expect([live.standingKnown, live.canWrite, live.canForce, live.search, live.top, live.missing, live.coverageAttention, live.customer, live.todayKL]).toStrictEqual([true, true, true, "alp", "customers", false, 1, undefined, "2026-09-14"]);
    perms("asset.read");
    dal.coreAll.mockClear();
    const plain = await readLive({ customerId: "cust-gone" });
    expect([plain.standingKnown, plain.canWrite, plain.missing, plain.rows.every((r) => r.standing === null)]).toStrictEqual([false, false, true, true]);
    expect(dal.coreAll).not.toHaveBeenCalledWith("contracts.list");
  });

  it("redirects a unit link without its customer to the unit under its customer", async () => {
    core();
    await expect(AdminUnitsPage(props({ unitId: "u-b" }))).rejects.toThrow("NEXT_REDIRECT /admin/units?customerId=cust-b&unitId=u-b");
    navigation.redirect.mockClear();
    const unknown = await readLive({ unitId: "u-nowhere" });
    expect([navigation.redirect.mock.calls, unknown.customer]).toStrictEqual([[], undefined]);
  });

  it("opens a customer: its tree, the unit's location, models, policies and rules, alerts, client users and the last edit", async () => {
    core();
    const live = await readLive({ customerId: "cust-a", unitId: "u-1", tab: "users", connections: "online,bogus", powerState: "on" });
    const c = live.customer;
    expect([c?.tab, c?.selection?.kind, c?.filter, c?.total]).toStrictEqual(["users", "space", { powerState: "on", connections: ["online"], search: "" }, 1]);
    expect(c?.alertsSub).toBe("1 critical · 1 info, not counted");
    expect(c?.users.map((u) => u.id)).toStrictEqual(["cu-1"]);
    expect(c?.models).toStrictEqual([{ id: "m-1", label: "AC-Co Split 1" }]);
    expect(c?.iotModels).toStrictEqual([{ id: "i-1", label: "IoT-Co Node", sensors: ["temperature"] }]);
    expect(c?.rules?.policyId).toBe("pol-default");
    expect(c?.lastEdit).toMatch(/· HQ Ops$/);
    expect([c?.canRules, c?.canAttach, c?.canRegisterDevice, c?.unitMissing]).toStrictEqual([true, true, true, undefined]);
    expect(auditQuery()).toMatchObject({ filters: { targetKind: "space", targetId: "s-1" } });
  });

  it("shows the unit with its device, attached policies and every blocker of taking it out of use", async () => {
    core({ detail: detail("u-1", { alertPolicyIds: ["pol-temp", "pol-outside-9"], pendingCommandIds: ["cmd-1", "cmd-2"] }) });
    const unitLive = await unitOf({ customerId: "cust-a", unitId: "u-1" });
    expect(unitLive?.device).toStrictEqual({ id: "d-1", version: 2, serial: "AC-0001", connection: "online", iotModelId: "i-1", mqttNode: "node_1" });
    expect(unitLive?.blockers?.map((b) => b.label)).toStrictEqual([
      "Unexpired contract (RTO, until 1 Jan 2027)", "Active job (repair, assigned)", "Active device binding (AC-0001)", "Active restriction (applied)", "2 pending commands",
    ]);
    expect(unitLive?.attached.map((a) => [a.id, a.madeBy, a.on])).toStrictEqual([["pol-default", "Always attached", "1 of 1 rule on"], ["pol-temp", "Made by Owner Olivia", "Off"], ["pol-outside-9", "", ""]]);
    expect(unitLive?.attached[2]?.name).toBe("pol-outs");
    expect([unitLive?.path, unitLive?.archivedNote, unitLive?.deviceKnown, typeof unitLive?.seen]).toStrictEqual(["Home p-1 › Bedroom", null, true, "string"]);
  });

  it("explains an archived unit with when, why and who, and reads no blockers for it", async () => {
    core({ detail: detail("u-1", { archived: true, lastSeenAt: null }), audit: [{ occurredAt: "2026-08-01T00:00:00Z", actorId: "m-unknown", reason: "Replaced" }] });
    const archived = await unitOf({ customerId: "cust-a", unitId: "u-1" });
    expect(archived?.archivedNote).toMatch(/^Taken out of use .* · reason: Replaced · HQ$/);
    expect([archived?.blockers, archived?.seen]).toStrictEqual([[], null]);
    expect(dal.coreAll).not.toHaveBeenCalledWith("jobs.list", expect.anything());
    core({ detail: detail("u-1", { archived: true }), audit: [] });
    perms("asset.read", "device.read");
    const noAudit = await unitOf({ customerId: "cust-a", unitId: "u-1" });
    expect(noAudit?.archivedNote).toBe("Taken out of use");
  });

  it("says when the URL's unit is gone or another customer's, and keeps going without the optional reads", async () => {
    core({ detail: new dal.CoreError({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {}, retryAfterSeconds: null }, 404) });
    const gone = await customerOf({ customerId: "cust-a", unitId: "u-gone" });
    expect(gone?.unitMissing).toBe(true);
    core({ detail: detail("u-b", { customerOrgId: "org-b" }) });
    const others = await customerOf({ customerId: "cust-a", unitId: "u-b" });
    expect(others?.unitMissing).toBe(true);
    core({ detail: new dal.CoreError({ code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {}, retryAfterSeconds: null }, 503) });
    await expect(AdminUnitsPage(props({ customerId: "cust-a", unitId: "u-1" }))).rejects.toThrow("UNAVAILABLE");
    core({ devices: [] });
    perms("asset.read");
    const bare = await customerOf({ customerId: "cust-a", unitId: "u-1", locationId: "unassigned:p-1" });
    expect([bare?.policies, bare?.rules, bare?.alertsSub, bare?.lastEdit, bare?.models, bare?.unit?.blockers, bare?.unit?.deviceKnown, bare?.selection?.kind]).toStrictEqual([null, null, null, null, [], null, false, "unassigned"]);
    expect(bare?.users[0]?.id).toBe("cu-1");
  });

  it("opens the property the URL names and says when a customer has no properties", async () => {
    core({ units: [] });
    const p = await readLive({ customerId: "cust-a", propertyId: "p-1", tab: "policies" });
    expect([p.customer?.tab, p.customer?.selection?.kind, p.customer?.total]).toStrictEqual(["policies", "property", 0]);
    expect(auditQuery()).toMatchObject({ filters: { targetKind: "property", targetId: "p-1" } });
    const none = await readLive({ customerId: "cust-old" });
    expect([none.customer?.perProperty, none.customer?.tab, none.customer?.alertsSub]).toStrictEqual(["no properties yet", "overview", "1 critical · 1 info, not counted"]);
  });

  it("reads the warranty tab with the claimable jobs, skipping a job it cannot read", async () => {
    core({ coverage: [{ unitId: "u-1", customerId: "cust-a", modelId: "m-1", warrantyEndsAt: "2026-10-01T00:00:00Z", contractIds: ["k-1", "k-gone-123"], status: "expiring", claimableJobIds: ["job-9", "job-gone"] }] });
    const w = await warrantyOf({ tab: "warranty" });
    expect(w?.rows.map((r) => [r.unit, r.model, r.contracts])).toStrictEqual([["Unit u-1", "AC-Co Split 1", "RTO until 1 Jan 2027, contract k-gone-1"]]);
    expect(w?.claims?.map((c) => c.jobId)).toStrictEqual(["job-9"]);
    expect(w?.canClaim).toBe(true);
    perms("asset.read", "contract.read");
    const plain = await warrantyOf({ tab: "warranty" });
    expect([plain?.claims, plain?.canClaim, plain?.rows[0]?.model]).toStrictEqual([null, false, "m-1"]);
  });

  it("words the singular and missing cases: one pending command, no default policy, an unnamed inviter, models in order", async () => {
    core({
      detail: detail("u-1", { pendingCommandIds: ["cmd-1"], lastSeenAt: null }), policies: [{ ...defaults, rules: [] }], audit: [],
      caps: [{ id: "m-2", manufacturer: "Zeta", model: "Z" }, { id: "m-1", manufacturer: "AC-Co", model: "Split 1" }], iots: [{ id: "i-2", manufacturer: "Zed", model: "N", sensors: [] }, { id: "i-1", manufacturer: "Acme", model: "N", sensors: [] }],
      users: [{ ...clientUser, status: "invited", displayName: null, invitedByMembershipId: "m-nobody" }],
    });
    const c = await customerOf({ customerId: "cust-a", unitId: "u-1" });
    expect(c?.unit?.blockers?.at(-1)?.label).toBe("1 pending command");
    expect(c?.unit?.attached[0]).toMatchObject({ condition: "0 rules · ventilation and fault causes", on: "0 of 0 rules on" });
    expect([c?.models.map((m) => m.id), c?.iotModels.map((m) => m.id), c?.lastEdit]).toStrictEqual([["m-1", "m-2"], ["i-1", "i-2"], null]);
    expect(c?.users[0]?.sub).toMatch(/^invited .* by Customer cust-a owner$/); // an inviter HQ cannot name
    core({ policies: [alertPolicy] });
    const noDefault = await customerOf({ customerId: "cust-a", unitId: "u-1" });
    expect([noDefault?.rules, noDefault?.unit?.attached]).toStrictEqual([null, []]);
    core({ detail: detail("u-1", { archived: true }), audit: [{ occurredAt: "2026-08-01T00:00:00Z", actorId: "m-hq", reason: null }] });
    const unreasoned = await unitOf({ customerId: "cust-a", unitId: "u-1" });
    expect(unreasoned?.archivedNote).toMatch(/^Taken out of use .* · HQ Ops$/); // no reason recorded
  });

  it("claims a job without a report, on a unit no longer listed", async () => {
    core({
      coverage: [{ unitId: "u-gone-123", customerId: "cust-zzz-999", modelId: "m-gone-77", warrantyEndsAt: null, contractIds: [], status: "no_coverage", claimableJobIds: ["job-9"] }],
      job: { projection: "detail", id: "job-9", version: 2, type: "repair", unitId: "u-gone-123", completedAt: null, costs: [], reportRefs: [] },
    });
    const w = await warrantyOf({ tab: "warranty" });
    expect(dal.coreOp).not.toHaveBeenCalledWith("reports.get", expect.anything());
    expect(w?.claims?.map((c) => c.unit)).toStrictEqual(["u-gone-1"]);
    expect(w?.rows.map((r) => [r.customer, r.model])).toStrictEqual([["cust-zzz", "m-gone-7"]]);
  });
});
