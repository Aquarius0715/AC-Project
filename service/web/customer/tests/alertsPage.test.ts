import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { AlertNote, ApiAlert } from "@ac/web/lib/alerts";
import { policyForm, policyInput, type ApiAlertPolicy, type ApiDefaultPolicy } from "@ac/web/lib/customerPolicies";

// The customer alert inbox (FR-C08, FR-C15): opening an alert marks its notifications read and re-reads the page even
// when the write fails; a policy save, delete or default-rule switch goes at its version, re-reads the page on success
// and on a CONFLICT (not on another refusal) and comes back with the refusal's code and fields. The page renders the
// demo outside API mode; the read takes the tab, severity, unit and unread filters from the URL and opens the editor
// for policyId=new or an own policy only.
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
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown[]>>(),
    apiMode: vi.fn(), coreNow: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), CoreError,
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/alerts/_components/alerts-view", () => ({ AlertsView: () => null }));
vi.mock("../app/customer/alerts/_components/alerts-live", () => ({ AlertsLiveView: () => null }));

import { deletePolicy, readAlert, savePolicy, setDefaultRule } from "../app/customer/alerts/actions";
import CustomerAlertsPage from "../app/customer/alerts/page";
import { AlertsView } from "../app/customer/alerts/_components/alerts-view";
import { AlertsLiveView } from "../app/customer/alerts/_components/alerts-live";
import AlertsLoading from "../app/customer/alerts/loading";
import AlertsErrorState from "../app/customer/alerts/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import type { OpInput } from "@ac/web/lib/opTypes";

const refusal = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code}`, retryAfterSeconds: null });
const alert = (id: string, unitId: string, over: Partial<ApiAlert> = {}): ApiAlert => ({
  id, unitId, type: "sensor", severity: "warning", status: "open", causeCode: "", evidenceKind: "inferred", evidenceText: "Temperature high", detectedAt: "2026-09-15T01:00:00Z", ...over,
});
const note = (id: string, alertId: string, readAt: string | null = null): AlertNote => ({ id, readAt, version: 1, sourceAlertId: alertId });
const policy = (id: string, unitIds: string[]): ApiAlertPolicy => ({
  id, unitIds, version: 3, kind: "alert", name: `Policy ${id}`, timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 0, disabledReason: null, customerId: "c-1",
  recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 0, cooldownMinutes: 0, metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28,
  durationSeconds: 300, activeWindow: null, severity: "warning",
});
const defaults: ApiDefaultPolicy = { id: "dp-1", version: 1, kind: "default_alert", name: "Default", rules: [], ruleSettings: [] };
const saveInput = policyInput(policyForm(null), null, { customerId: "c-1", membershipId: "m-1", timezone: "Asia/Kuala_Lumpur" });

interface Reads { alerts: ApiAlert[]; notes: AlertNote[]; policies: ApiAlertPolicy[]; defaults: ApiDefaultPolicy[] }
/** the Core API reads of the page; a null time zone makes preferences.get fail */
function answer(r: Partial<Reads>, timezone: string | null = "Asia/Tokyo") {
  const reads: Reads = { alerts: [], notes: [], policies: [], defaults: [defaults], ...r };
  dal.coreAll.mockImplementation((op: string, input?: unknown) => {
    const kind = JSON.stringify(input ?? {});
    const rows = new Map<string, unknown[]>([
      ["alerts.list", reads.alerts], ["notifications.list", reads.notes],
      ["units.list", [{ id: "u-2", displayName: "Kitchen AC", propertyId: "p-1", spaceId: null }, { id: "u-1", displayName: "Bedroom AC", propertyId: "p-1", spaceId: "s-1" }]],
      ["properties.list", [{ id: "p-1", name: "Home" }]], ["spaces.list", [{ id: "s-1", name: "Upstairs", parentSpaceId: null }]],
      ["policies.list", kind.includes("default_alert") ? reads.defaults : reads.policies],
    ]);
    return Promise.resolve(rows.get(op) ?? []);
  });
  dal.coreOp.mockImplementation((op: string) => {
    if (op === "summaries.get") return Promise.resolve({ counts: { alertCount: 2 } });
    if (op === "preferences.get") return timezone === null ? Promise.reject(new Error("preferences down")) : Promise.resolve({ timezone });
    return Promise.reject(new Error(`unexpected ${op}`));
  });
}
const open = async (searchParams: Record<string, string | string[]>) => {
  const element = await CustomerAlertsPage({ searchParams: Promise.resolve(searchParams), params: Promise.resolve({}) });
  if (!isValidElement<ComponentProps<typeof AlertsLiveView>>(element) || element.type !== AlertsLiveView) throw new TypeError("not AlertsLiveView");
  return element.props.live;
};
const alertsInput = () => dal.coreAll.mock.calls.find((c) => c[0] === "alerts.list")?.[1];

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(new Date("2026-09-15T01:12:30Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePrincipal.mockResolvedValue({ userId: "us-1", membershipId: "m-1", organizationId: "o-1", tenantId: "t-1", clientRole: "owner", customerId: "c-1" });
});

describe("alert actions", () => {
  it("readAlert marks the notifications read in one write and re-reads; a refusal comes back worded, still re-read", async () => {
    dal.coreOp.mockResolvedValueOnce(null);
    await expect(readAlert(["n-1", "n-2"])).resolves.toBeNull();
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["notifications.markAllRead", { ids: ["n-1", "n-2"] }, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(refusal("FORBIDDEN"));
    await expect(readAlert(["n-1"])).resolves.toMatchObject({ code: "FORBIDDEN" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("savePolicy creates without a version and edits at the policy version, returning the id", async () => {
    dal.coreOp.mockResolvedValue({ id: "pol-9" });
    const input: OpInput<"policies.save"> = saveInput;
    await expect(savePolicy(input, null)).resolves.toStrictEqual({ ok: true, value: "pol-9" });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["policies.save", input, { write: true }]);
    await savePolicy(input, 4);
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["policies.save", input, { write: true, expectedVersion: 4 }]);
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("deletePolicy and setDefaultRule go at their versions", async () => {
    dal.coreOp.mockResolvedValue(null);
    await expect(deletePolicy("pol-1", 3)).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["policies.delete", { policyId: "pol-1" }, { write: true, expectedVersion: 3 }]);
    await expect(setDefaultRule("dp-1", "high_temp", "c-1", false, 0)).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["policies.setDefaultRule", { policyId: "dp-1", ruleKey: "high_temp", customerId: "c-1", enabled: false }, { write: true, expectedVersion: 0 }]);
  });

  it("a CONFLICT re-reads the page, another refusal does not; both come back with the code and fields", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT"));
    await expect(deletePolicy("pol-1", 3)).resolves.toStrictEqual({ ok: false, code: "CONFLICT", messageKey: "error.CONFLICT", fieldErrors: {}, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledOnce();
    dal.coreOp.mockRejectedValueOnce(refusal("VALIDATION", { threshold: "out_of_range" }));
    await expect(savePolicy(saveInput, 3)).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { threshold: "out_of_range" } });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("a failure that is not a Core API refusal comes back as a failure, not a throw", async () => {
    dal.coreOp.mockRejectedValueOnce(new Error("socket hang up"));
    await expect(setDefaultRule("dp-1", "high_temp", "c-1", true, 1)).resolves.toMatchObject({ ok: false });
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("the alerts page", () => {
  it("renders the demo outside API mode without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    const element = await CustomerAlertsPage({ searchParams: Promise.resolve({}), params: Promise.resolve({}) });
    expect(isValidElement(element) && element.type === AlertsView).toBe(true);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("without filters reads every alert, counts the unread ones and sorts the units by name with their places", async () => {
    answer({ alerts: [alert("a-1", "u-1"), alert("a-2", "u-2", { severity: "normal" })], notes: [note("n-1", "a-1"), note("n-2", "a-2", "2026-09-15T00:00:00Z")] });
    const live = await open({});
    expect(alertsInput()).toStrictEqual({});
    expect(live).toMatchObject({ tab: "alerts", severity: null, unitId: null, unreadOnly: false, alertCount: 2, total: 2, unread: 1, now: "2026-09-15T01:12:30.000Z" });
    expect(live.rows.map((a) => a.id)).toStrictEqual(["a-1", "a-2"]);
    expect(live.units).toStrictEqual([{ id: "u-1", name: "Bedroom AC", place: "Home › Upstairs" }, { id: "u-2", name: "Kitchen AC", place: "Home" }]);
    expect(live.policies).toMatchObject({ owner: true, customerId: "c-1", membershipId: "m-1", timezone: "Asia/Tokyo", count: 1, editor: null });
  });

  it("passes the severity and a known unit to alerts.list, keeps only unread rows, and drops an unknown severity", async () => {
    answer({ alerts: [alert("a-1", "u-1"), alert("a-2", "u-1")], notes: [note("n-1", "a-1")] });
    const live = await open({ severity: "critical", unitId: ["u-1", "u-2"], unreadOnly: "true" });
    expect(alertsInput()).toStrictEqual({ filters: { severity: "critical", unitId: "u-1" } });
    expect(live).toMatchObject({ severity: "critical", unitId: "u-1", unreadOnly: true, total: 2, unread: 1 });
    expect(live.rows.map((a) => a.id)).toStrictEqual(["a-1"]);
    vi.clearAllMocks();
    const other = await open({ severity: "loud", unitId: "u-gone" });
    expect(alertsInput()).toStrictEqual({ filters: { unitId: "u-gone" } });
    expect(other).toMatchObject({ severity: null, unitId: null });
  });

  it("opens the editor for policyId=new and an own policy, not for an unknown id; a member sees the policies read-only", async () => {
    answer({ policies: [policy("pol-1", ["u-1", "u-x"])], defaults: [] }, null);
    dal.corePrincipal.mockResolvedValue({ userId: "us-2", membershipId: "m-2", organizationId: "o-1", tenantId: "t-1", clientRole: "member", customerId: "c-1" });
    const fresh = await open({ tab: "policies", policyId: "new" });
    expect(fresh.tab).toBe("policies");
    expect(fresh.policies).toMatchObject({ owner: false, count: 1, defaults: null, timezone: "Asia/Kuala_Lumpur", editor: { policy: null, attached: [] } });
    const own = await open({ tab: "policies", policyId: "pol-1" });
    expect(own.policies.editor).toMatchObject({ policy: { id: "pol-1" }, attached: ["Bedroom AC", "AC"] });
    const unknown = await open({ tab: "other", policyId: "pol-gone" });
    expect(unknown.tab).toBe("alerts");
    expect(unknown.policies.editor).toBeNull();
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = AlertsLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "alerts" });
    const failed = AlertsErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Alerts", error, retry });
  });
});
