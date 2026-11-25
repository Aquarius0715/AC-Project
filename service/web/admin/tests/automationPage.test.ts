import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiAutoPolicy } from "@ac/web/lib/automation";

// HQ automation policies (FR-A11, SCR-A11): the Server Actions (a policy saved new or at its version, a simulation and
// a fire at the Core API's current minute, a repeated fire at its own event's minute under its one-time key) and the
// Server Component (the URL scope, the policy it opens, the unit options with their aircon model's range).
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
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:02:37Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["automation.policy.read", "automation.policy.write", "asset.read", "device.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/settings/automation/_components/automation-view", () => ({ AutomationView: () => null }));
vi.mock("../app/admin/settings/automation/_components/automation-demo", () => ({ AutomationDemo: () => null }));

import { fireAuto, saveAutoPolicy, simulateAuto } from "../app/admin/settings/automation/actions";
import AdminAutomationPage from "../app/admin/settings/automation/page";
import { AutomationView } from "../app/admin/settings/automation/_components/automation-view";
import { AutomationDemo } from "../app/admin/settings/automation/_components/automation-demo";
import AutomationLoading from "../app/admin/settings/automation/loading";
import AutomationErrorState from "../app/admin/settings/automation/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const policy = (id: string, over: Partial<ApiAutoPolicy> = {}): ApiAutoPolicy => ({
  id, version: 2, kind: "automation", name: `Policy ${id}`, unitIds: ["u-1"], ownerMembershipId: "m-1", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 60, disabledReason: null,
  condition: { type: "tariff", operator: "gt", value: 0.6, unit: "MYR_per_kWh" }, action: { kind: "set_temperature", celsius: 26 }, ...over,
});
const evaluation = {
  eventId: "ev-1", occurredAt: "2026-09-14T00:00:00Z", phase: "condition" as const, unitIds: ["u-1"],
  facts: [{ unitId: "u-1", metric: "tariff" as const, unit: "MYR_per_kWh", observedAt: "2026-09-14T00:00:00Z", quality: "valid" as const, value: 0.7 }],
};
const decided = { results: [{ unitId: "u-1", decision: "selected", ruleId: "p-1", reason: null }] };

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
});

describe("automation Server Actions", () => {
  it("saves a policy new without a version, or at the version it was read", async () => {
    dal.coreOp.mockResolvedValue(policy("p-1", { version: 3 }));
    const input = { kind: "automation" as const, name: "x", unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: false, priority: 50, condition: policy("p").condition, action: policy("p").action };
    expect(await saveAutoPolicy(input)).toStrictEqual({ ok: true, value: { id: "p-1", version: 3 } });
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}');
    await saveAutoPolicy({ ...input, id: "p-1" }, 2);
    expect(dal.coreOp.mock.lastCall?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await saveAutoPolicy(input)).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });

  it("simulates at the current minute without re-rendering, and fires once per event", async () => {
    dal.coreOp.mockResolvedValue(decided);
    expect(await simulateAuto(evaluation)).toStrictEqual({ ok: true, value: decided.results });
    const at = "2026-09-14T01:02:00.000Z"; // the minute of the Core API clock, not the page's
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["automations.simulate", { ...evaluation, occurredAt: at, facts: [{ ...evaluation.facts[0], observedAt: at }] }]);
    expect(cache.refresh).not.toHaveBeenCalled();
    expect(await fireAuto(evaluation)).toStrictEqual({ ok: true, value: { results: decided.results, at } });
    expect(dal.coreOp.mock.lastCall?.[2]).toStrictEqual({ write: true, idempotencyKey: "ev-1" });
    const earlier = "2026-09-14T00:59:00.000Z";
    expect(await fireAuto(evaluation, earlier)).toMatchObject({ ok: true, value: { at: earlier } }); // a repeat keeps its event's minute
    expect(dal.coreOp.mock.lastCall?.[1]).toMatchObject({ occurredAt: earlier });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
    dal.coreOp.mockRejectedValueOnce(refused("VALIDATION"));
    expect(await simulateAuto(evaluation)).toMatchObject({ ok: false, code: "VALIDATION" });
  });
});

const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof AutomationView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminAutomationPage(props(sp));
  if (!isValidElement<ComponentProps<typeof AutomationView>>(page) || page.type !== AutomationView) throw new TypeError("not AutomationView");
  return page.props.live;
}
function core(policies: ApiAutoPolicy[]) {
  const pages = new Map<string, unknown[]>([
    ["policies.list", policies],
    ["units.list", [
      { id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", modelId: "m-1", archived: false },
      { id: "u-2", displayName: "Stray AC", customerOrgId: "org-x", propertyId: "p-x", modelId: "m-gone", archived: false },
      { id: "u-old", displayName: "Old", customerOrgId: "org-a", propertyId: "p-1", modelId: "m-1", archived: true },
    ]],
    ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a", status: "active" }]],
    ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a", archived: false }, { id: "p-x", name: "Lost", customerOrgId: "org-x", archived: false }, { id: "p-old", name: "Gone", customerOrgId: "org-a", archived: true }]],
    ["capabilities.list", [{ id: "m-1", temperature: { min: 18, max: 30, step: 1 }, ventilation: true }]],
  ]);
  dal.coreAllPage.mockImplementation((op) => Promise.resolve({ items: pages.get(op) ?? [], total: 0 }));
  dal.coreOp.mockImplementation((_, q) => Promise.resolve(policies.find((p) => JSON.stringify(q).includes(p.id))));
}

describe("the automation page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminAutomationPage(props());
    expect(isValidElement(page) && page.type).toBe(AutomationDemo);
    expect(AutomationLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = AutomationErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("lists the scope's policies grouped by customer and opens the first, with each unit's model range", async () => {
    core([policy("p-1", { priority: 40 }), policy("p-2", { priority: 70, unitIds: ["u-2"] })]);
    const live = await liveOf();
    expect(dal.coreAllPage).toHaveBeenCalledWith("policies.list", { filters: { kind: "automation" } });
    expect(live.groups.map((g) => [g.label, g.rows.map((r) => r.id)])).toStrictEqual([["Alpha", ["p-1"]], ["unknown customer", ["p-2"]]]);
    expect([live.isNew, live.selected?.id, live.canWrite]).toStrictEqual([false, "p-1", true]); // the list's first
    expect(live.units).toStrictEqual([
      { id: "u-1", name: "Lobby AC", customerId: "cust-a", propertyId: "p-1", temperature: { min: 18, max: 30, step: 1 }, ventilation: true },
      { id: "u-2", name: "Stray AC", customerId: "", propertyId: "p-x", temperature: null, ventilation: false }, // no model in the register
    ]);
    expect(live.properties).toStrictEqual([{ id: "p-1", name: "Tower", customerId: "cust-a" }, { id: "p-x", name: "Lost", customerId: "" }]);
  });

  it("reads the URL scope and policy, a new one, and a new form without policies", async () => {
    core([policy("p-1"), policy("p-2")]);
    const scoped = await liveOf({ customerId: "cust-a", propertyId: "p-1", unitId: "u-1", policyId: "p-2" });
    expect(dal.coreAllPage).toHaveBeenCalledWith("policies.list", { filters: { kind: "automation", customerId: "cust-a", propertyId: "p-1", unitId: "u-1" } });
    expect([scoped.selected?.id, scoped.scope]).toStrictEqual(["p-2", { customerId: "cust-a", propertyId: "p-1", unitId: "u-1" }]);
    expect(dal.coreOp).toHaveBeenLastCalledWith("policies.get", { policyId: "p-2" });
    dal.coreOp.mockClear();
    const fresh = await liveOf({ policyId: "new" });
    expect([fresh.isNew, fresh.selected]).toStrictEqual([true, null]);
    expect(dal.coreOp).not.toHaveBeenCalled();
    core([]);
    const none = await liveOf({ policyId: "gone" });
    expect([none.isNew, none.groups]).toStrictEqual([true, []]);
  });
});
