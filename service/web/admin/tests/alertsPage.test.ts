import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import { policyRows, type AdminPolicy, type ApiAdminAlert, type ApiPolicy } from "@ac/web/lib/adminAlerts";

// The HQ alerts screen (FR-A05): the Server Actions (acknowledge, resolve with evidence, policy saves and deletes at
// their version, default-rule switches, the demo test of a policy) and the Server Component (the URL scope, severity
// and status, the scoped and default policies, the permission a refused read needs).
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
  const orWhenForbidden = async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
    try {
      return await read;
    } catch (error) {
      if (error instanceof CoreError && error.error.code === "FORBIDDEN") return fallback;
      throw error;
    }
  };
  return {
    CoreError, orWhenForbidden, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["alert.read", "alert.policy.read", "alert.policy.write", "asset.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    // the list reads answer from one table in these tests, so they share the core read's signature
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/alerts/_components/alerts-view", () => ({ AdminAlertsView: () => null }));

import { acknowledgeAlert, deletePolicy, resolveAlert, saveDefaultRules, savePolicy, switchDefaultRule, testPolicy } from "../app/admin/alerts/actions";
import AdminAlertsPage from "../app/admin/alerts/page";
import { AdminAlertsView } from "../app/admin/alerts/_components/alerts-view";
import AlertsLoading from "../app/admin/alerts/loading";
import AlertsErrorState from "../app/admin/alerts/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const apiPolicy = (over: Partial<ApiPolicy> = {}): ApiPolicy => ({
  id: "pol-1", version: 2, kind: "alert", name: "Bedroom too hot", unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 50, customerId: "cust-a",
  recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 60, cooldownMinutes: 5, metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28,
  durationSeconds: 60, severity: "warning", ...over,
});
const defaultPolicy = apiPolicy({
  id: "pol-default", kind: "default_alert", name: "Default", customerId: null, unitIds: [],
  rules: [{ ruleKey: "high_temperature", name: "Room too hot", category: "fault", metric: "temperature", operator: "gte", threshold: 32, recoveryThreshold: 30, durationSeconds: 300, severity: "warning", activeWindow: null }],
});
const policyOf = (p: ApiPolicy): AdminPolicy => {
  const [row] = policyRows([p], [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a" }], [{ id: "m-1", displayName: "HQ Operator", role: "admin" }]);
  if (!row) throw new Error("no policy row");
  return row;
};
const alert = (id: string, over: Partial<ApiAdminAlert> = {}): ApiAdminAlert => ({
  id, version: 1, unitId: "u-1", policyId: "pol-1", type: "sensor", severity: "warning", status: "open", causeCode: "unknown", evidenceKind: "demo_observation", evidenceText: "temperature 31",
  observedAt: "2026-09-14T00:50:00Z", evidenceIds: [], detectedAt: "2026-09-14T00:55:00Z", acknowledgedAt: null, resolvedAt: null, resolutionReason: null, ...over,
});
const lastCall = () => dal.coreOp.mock.lastCall;

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.corePermissions.mockResolvedValue(new Set(["alert.read", "alert.policy.read", "alert.policy.write", "asset.read"]));
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
});

describe("alert Server Actions", () => {
  it("acknowledges and resolves an alert at its version, with the reason and evidence", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await acknowledgeAlert("al-1", 2)).toStrictEqual({ ok: true });
    expect(lastCall()).toStrictEqual(["alerts.acknowledge", { alertId: "al-1" }, { write: true, expectedVersion: 2 }]);
    expect(await resolveAlert("al-1", 3, "Window closed", ["ev-1"])).toStrictEqual({ ok: true });
    expect(lastCall()).toStrictEqual(["alerts.resolve", { alertId: "al-1", resolutionReason: "Window closed", resolutionEvidenceIds: ["ev-1"] }, { write: true, expectedVersion: 3 }]);
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("creates a policy without a version, updates one at its version, and names the saved one", async () => {
    dal.coreOp.mockResolvedValue({ id: "pol-new" });
    const created = await savePolicy({ ...policyOf(apiPolicy()), id: "" });
    expect(created).toStrictEqual({ ok: true, id: "pol-new" });
    expect(lastCall()?.[2]).toStrictEqual({ write: true });
    expect(lastCall()?.[1]).toMatchObject({ kind: "alert", customerId: "cust-a", metric: "temperature", threshold: 30, recipientMembershipIds: ["m-1"] });
    dal.coreOp.mockResolvedValue({ id: "pol-1" });
    expect(await savePolicy(policyOf(apiPolicy()))).toStrictEqual({ ok: true, id: "pol-1" });
    expect(lastCall()?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    dal.coreOp.mockRejectedValue(refused("CONFLICT"));
    expect(await savePolicy(policyOf(apiPolicy()))).toMatchObject({ ok: false, code: "CONFLICT" }); // no id on a refusal
  });

  it("deletes a policy, saves the default rules, and switches a default rule with a trimmed reason", async () => {
    dal.coreOp.mockResolvedValue({});
    expect(await deletePolicy("pol-1", 2)).toStrictEqual({ ok: true });
    expect(lastCall()).toStrictEqual(["policies.delete", { policyId: "pol-1" }, { write: true, expectedVersion: 2 }]);
    expect(await saveDefaultRules(policyOf(defaultPolicy))).toStrictEqual({ ok: true });
    expect(lastCall()?.[1]).toMatchObject({ id: "pol-default", kind: "default_alert", rules: [{ ruleKey: "high_temperature", threshold: 32 }] });
    expect(await switchDefaultRule("pol-default", "high_temperature", "cust-a", false, `  Seasonal${" ".repeat(2)}`, 0)).toStrictEqual({ ok: true });
    expect(lastCall()).toStrictEqual(["policies.setDefaultRule", { policyId: "pol-default", ruleKey: "high_temperature", customerId: "cust-a", enabled: false, reason: "Seasonal" }, { write: true, expectedVersion: 0 }]);
    await switchDefaultRule("pol-default", "high_temperature", "cust-a", true, " ".repeat(3), 1);
    expect(lastCall()?.[1]).toStrictEqual({ policyId: "pol-default", ruleKey: "high_temperature", customerId: "cust-a", enabled: true }); // a blank reason is left out
  });

  it("tests a policy with one synthetic reading at the API clock and returns its decision for the unit", async () => {
    dal.coreOp.mockResolvedValue({ notifications: [{ unitId: "u-2", policyId: "pol-1", decision: "selected", reason: null }, { unitId: "u-1", policyId: "pol-1", decision: "suppressed", reason: "cooldown" }] });
    expect(await testPolicy("pol-1", "u-1", "temperature", 31)).toStrictEqual({ ok: true, decision: { unitId: "u-1", policyId: "pol-1", decision: "suppressed", reason: "cooldown" } });
    expect(lastCall()?.[0]).toBe("automations.simulate");
    expect(lastCall()?.[1]).toMatchObject({ occurredAt: "2026-09-14T01:00:00.000Z", phase: "condition", unitIds: ["u-1"], facts: [{ unitId: "u-1", value: 31, metric: "temperature", unit: "°C", quality: "valid" }] });
    expect(cache.refresh).not.toHaveBeenCalled(); // a read
    dal.coreOp.mockResolvedValue({ notifications: [] });
    expect(await testPolicy("pol-1", "u-1", "co2", 1500)).toStrictEqual({ ok: true, decision: null });
    dal.coreOp.mockRejectedValue(refused("FORBIDDEN"));
    expect(await testPolicy("pol-1", "u-1", "temperature", 31)).toMatchObject({ ok: false, code: "FORBIDDEN" });
  });
});

/** The Core API of one page read: lists by operation, policies.list split by kind. */
function core(over: { alerts?: ApiAdminAlert[]; refuse?: string } = {}) {
  const lists = new Map<string, unknown[]>([
    ["units.list", [{ id: "u-1", displayName: "Bedroom AC", customerOrgId: "org-a", propertyId: "p-1" }, { id: "u-old", displayName: "Old AC", customerOrgId: "org-a", propertyId: "p-1", archived: true }, { id: "u-z", displayName: "Stray AC", customerOrgId: "org-z", propertyId: "p-9" }]],
    ["customers.list", [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a", status: "active" }, { id: "cust-x", name: "Gone Ltd", organizationId: "org-x", status: "inactive" }]],
    ["properties.list", [{ id: "p-1", name: "Home A", customerOrgId: "org-a" }]],
    ["members.list", [{ id: "m-1", displayName: "HQ Operator", role: "admin" }]],
  ]);
  const alerts = over.alerts ?? [alert("al-1"), alert("al-info", { severity: "normal", type: "maintenance" })];
  const answer = (op: string, input?: unknown) => {
    if (over.refuse === op) return Promise.reject(refused("FORBIDDEN"));
    if (op === "alerts.list") return Promise.resolve(JSON.stringify(input).includes('"resolved"') ? { items: [] } : alerts.filter((a) => JSON.stringify(input).includes(`"${a.status}"`)));
    if (op === "policies.list") return Promise.resolve(JSON.stringify(input).includes("default_alert") ? { items: [defaultPolicy] } : { items: [apiPolicy(), defaultPolicy], total: 2 });
    return Promise.resolve(op === "properties.list" ? lists.get(op) : { items: lists.get(op) ?? [], total: 0 });
  };
  dal.coreOp.mockImplementation(answer);
  dal.coreAll.mockImplementation(answer);
  dal.coreAllPage.mockImplementation(answer);
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = NonNullable<ComponentProps<typeof AdminAlertsView>["live"]>;
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminAlertsPage(props(sp));
  if (!isValidElement<ComponentProps<typeof AdminAlertsView>>(page) || page.type !== AdminAlertsView || !page.props.live) throw new TypeError("not the live AdminAlertsView");
  return page.props.live;
}
const policyCalls = () => dal.coreAllPage.mock.calls.filter(([op]) => op === "policies.list").map(([, input]) => input);

describe("the HQ alerts page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminAlertsPage(props());
    expect(isValidElement<ComponentProps<typeof AdminAlertsView>>(page) && page.type === AdminAlertsView && page.props.live).toBeUndefined();
    expect(AlertsLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = AlertsErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("reads the open alerts of the URL scope, the scoped policies and the default one, by default on the alerts tab", async () => {
    core();
    const live = await liveOf({ customerId: "cust-a", propertyId: "p-1", unitId: "u-1", severity: "warning", policyId: "pol-1", new: "cust-a", from: "pol-0" });
    expect(dal.coreAll.mock.calls.filter(([op]) => op === "alerts.list").map(([, input]) => input)).toStrictEqual([
      { filters: { customerId: "cust-a", propertyId: "p-1", unitId: "u-1", severity: "warning", status: "open" } }, { filters: { customerId: "cust-a", propertyId: "p-1", unitId: "u-1", severity: "warning", status: "acknowledged" } },
    ]);
    expect(policyCalls()).toStrictEqual([{ filters: { customerId: "cust-a", propertyId: "p-1", unitId: "u-1" } }]); // the scope without the severity
    expect([live.tab, live.q.status, live.q.severity, live.scopeName]).toStrictEqual(["alerts", "open", "warning", "Demo Customer A"]);
    expect(live.policies.map((p) => p.id)).toStrictEqual(["pol-default", "pol-1"]); // the default once, first
    expect([live.pol.policyId, live.pol.creatingFor, live.pol.from, live.pol.canWrite, live.pol.timezone]).toStrictEqual(["pol-1", "cust-a", "pol-0", true, "Asia/Kuala_Lumpur"]);
    expect(live.pol.customers).toStrictEqual([{ id: "cust-a", name: "Demo Customer A" }]); // an inactive customer gets no new policy
    expect(live.pol.units).toStrictEqual([{ id: "u-1", name: "Bedroom AC", customerId: "cust-a" }, { id: "u-z", name: "Stray AC", customerId: null }]); // archived left out; no customer known
  });

  it("leaves out information rows for Critical & warning, and ignores an unknown status or severity", async () => {
    core();
    const attention = await liveOf({ severity: "attention", tab: "policies" });
    expect([attention.tab, attention.q.severity, attention.alerts.map((a) => a.id)]).toStrictEqual(["policies", "attention", ["al-1"]]);
    const all = await liveOf({ severity: "loud", status: "sleeping" });
    expect([all.q.severity, all.q.status, all.alerts.map((a) => a.id).toSorted((x, y) => x.localeCompare(y))]).toStrictEqual([null, "open", ["al-1", "al-info"]]);
    const acknowledged = await liveOf({ status: "acknowledged" });
    expect([acknowledged.q.status, acknowledged.scopeName]).toStrictEqual(["acknowledged", null]);
  });

  it("names the permission of the first read the role may not make", async () => {
    for (const [op, permission] of [["alerts.list", "alert.read"], ["units.list", "asset.read"], ["policies.list", "alert.policy.read"]] as const) {
      core({ refuse: op });
      const page: ReactNode = await AdminAlertsPage(props());
      expect(isValidElement(page) && page.type === NeedsAccess && page.props).toStrictEqual({ what: "Alert policies", permission });
    }
  });

  it("shows the page without member names when they are refused, and fails on anything else", async () => {
    core({ refuse: "members.list" });
    const live = await liveOf();
    expect(live.policies.find((p) => p.id === "pol-1")?.madeBy).toBe("");
    dal.coreAllPage.mockRejectedValue(refused("UNAVAILABLE"));
    await expect(AdminAlertsPage(props())).rejects.toThrow("UNAVAILABLE");
  });
});
