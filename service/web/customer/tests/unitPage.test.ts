import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiAlertPolicy, ApiDefaultPolicy } from "@ac/web/lib/customerPolicies";

// The customer unit page (FR-C03, FR-C15): the demo outside API mode; a unit outside the customer's scope reading as
// absent; in API mode the room, every alert policy of the customer (all pages, as the Alerts screen reads them) in words,
// the default policy's rules on for this customer, the history, the pins and, for a restricted unit, the link to its
// restriction (never a failure). Its Server Actions send the current unit version and map a failure to its code.
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
    apiMode: vi.fn(), coreDisplay: vi.fn(), coreNow: vi.fn(), corePrincipal: vi.fn(), CoreError,
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  notFound: () => { throw new Error("notFound"); },
}));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/units/[id]/_components/unit-view", () => ({ UnitView: () => null }));

import CustomerUnitPage from "../app/customer/units/[id]/page";
import { createCommand, setUnitAlertPolicies } from "../app/customer/units/[id]/actions";
import { UnitView } from "../app/customer/units/[id]/_components/unit-view";

const failure = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code.toLowerCase()}`, retryAfterSeconds: null });
const alertPolicy = (id: string): ApiAlertPolicy => ({
  id, version: 1, kind: "alert", name: `Policy ${id}`, unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 1, disabledReason: null, customerId: "org-a",
  recipientMembershipIds: [], channels: [], escalateAfterMinutes: 0, cooldownMinutes: 0, metric: "heartbeat_gap", operator: "gte", threshold: 10, recoveryThreshold: 5,
  durationSeconds: 0, activeWindow: null, severity: "warning",
});
type Rule = ApiDefaultPolicy["rules"][number];
type Setting = ApiDefaultPolicy["ruleSettings"][number];
const rule = (ruleKey: string): Rule => ({ ruleKey, name: ruleKey, category: "connection", metric: "heartbeat_gap", operator: "gte", threshold: 10, recoveryThreshold: 5, durationSeconds: 0, activeWindow: null, severity: "warning" });
const setting = (customerId: string, ruleKey: string): Setting => ({ ruleKey, customerId, id: `s-${customerId}-${ruleKey}`, version: 2, enabled: false, changedByMembershipId: "m-0", reason: null, updatedAt: "2026-09-01T00:00:00Z" });
const defaultPolicy = (): ApiDefaultPolicy => ({
  id: "dp-1", version: 1, kind: "default_alert", name: "Default", rules: [rule("offline"), rule("filter")], ruleSettings: [setting("org-a", "filter"), setting("org-b", "offline")],
});
let detail: { id: string; version: number; spaceId: string | null; effectiveControlPolicy: { state: string } };
let alertPolicies: ApiAlertPolicy[];
let restrictionPages: { items: { id: string; unitIds: string[] }[] };
let restrictionFails: boolean;
const open = (id = "u-1") => CustomerUnitPage({ params: Promise.resolve({ id }), searchParams: Promise.resolve({}) });
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof UnitView>>(element) || element.type !== UnitView) throw new TypeError("not UnitView");
  return element.props;
};
const calls = (op: string) => dal.coreOp.mock.calls.filter((c) => c[0] === op).map((c) => c.slice(1));

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(new Date("2026-09-15T01:00:00Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePrincipal.mockResolvedValue({ customerId: "org-a", membershipId: "m-1" });
  detail = { id: "u-1", version: 7, spaceId: "sp-1", effectiveControlPolicy: { state: "normal" } };
  // the 101st policy is the one past the first page of 100
  alertPolicies = Array.from({ length: 101 }, (_, n) => alertPolicy(`p-${String(n + 1)}`));
  restrictionPages = { items: [{ id: "r-1", unitIds: ["u-1"] }] };
  restrictionFails = false;
  dal.coreAll.mockImplementation((op: string, input?: unknown) => {
    const kind = typeof input === "object" && input !== null && "filters" in input ? JSON.stringify(input.filters) : "";
    if (op === "policies.list") return Promise.resolve(kind.includes("default_alert") ? [defaultPolicy()] : alertPolicies);
    // k-0 has no active restriction, so its invoices are not read
    if (op === "contracts.list") return Promise.resolve([{ id: "k-0" }, { id: "k-1", activeRestrictionIds: ["r-1"] }]);
    if (op === "invoices.list") return Promise.resolve([{ id: "inv-1", status: "overdue", dueAt: "2026-09-01T00:00:00Z" }]);
    return Promise.resolve([]);
  });
  dal.coreOp.mockImplementation((op: string) => {
    if (op === "restrictions.forInvoice") return restrictionFails ? Promise.reject(failure("INTERNAL")) : Promise.resolve(restrictionPages);
    const answers = new Map<string, unknown>([
      ["units.get", detail], ["units.list", { items: [{ id: "u-1", displayName: "Bedroom AC", connection: "online", effectivePowerState: "on", observedState: { power: true, celsius: 24, observedAt: null }, lastSeenAt: null }] }], ["commands.list", { items: [{ id: "c-1" }] }], ["preferences.get", { pinnedUnitIds: ["u-1"] }],
    ]);
    return Promise.resolve(answers.has(op) ? answers.get(op) : { id: "cmd-1" });
  });
});

describe("customer unit page", () => {
  it("renders the fixture unit outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    expect(viewProps(await open("unit-demo"))).toStrictEqual({ id: "unit-demo" });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("a unit outside the customer's scope or not a unit id is the not-found page; any other failure reaches the error boundary", async () => {
    for (const error of [failure("NOT_FOUND"), failure("VALIDATION", { id: "invalid_format" })]) {
      dal.coreOp.mockRejectedValueOnce(error);
      await expect(open()).rejects.toThrow("notFound");
    }
    dal.coreOp.mockRejectedValueOnce(failure("VALIDATION", { other: "required" }));
    await expect(open()).rejects.toBeInstanceOf(dal.CoreError);
    dal.coreOp.mockRejectedValueOnce(failure("INTERNAL"));
    await expect(open()).rejects.toBeInstanceOf(dal.CoreError);
  });

  it("lists every alert policy of the customer in words, also past the first 100, and the default rules on for this customer", async () => {
    const props = viewProps(await open());
    expect(dal.coreAll).toHaveBeenCalledWith("policies.list", { filters: { kind: "alert" } });
    expect(dal.coreAll).toHaveBeenCalledWith("policies.list", { filters: { kind: "default_alert" } });
    expect(calls("policies.list")).toStrictEqual([]);
    expect(props.policies).toHaveLength(101);
    expect(props.policies?.at(-1)).toStrictEqual({ id: "p-101", name: "Policy p-101", rule: "No heartbeat for 10 min → Warning" });
    // org-a switched "filter" off; org-b's setting is not this customer's
    expect(props.defaults).toStrictEqual({ on: 1, n: 2 });
    expect(props.room).toMatchObject([{ id: "u-1", name: "Bedroom AC", state: "running", conn: "online" }]);
    expect(props).toMatchObject({ detail, id: "u-1", history: [{ id: "c-1" }], restrictionHref: null, nowMs: Date.parse("2026-09-15T01:00:00Z"), me: "m-1", pins: ["u-1"] });
    // the room is the unit's space; the history is the latest 20 commands
    expect(calls("units.list")).toStrictEqual([[{ limit: 100, filters: { spaceId: "sp-1" } }]]);
    expect(calls("commands.list")).toStrictEqual([[{ unitId: "u-1", query: { limit: 20 } }]]);
    expect(calls("contracts.list")).toStrictEqual([]);
  });

  it("without a space the room is the unit alone; no default policy and no preferences still render", async () => {
    detail = { ...detail, spaceId: null };
    alertPolicies = [{ ...alertPolicy("p-x"), severity: "critical" }];
    dal.coreAll.mockImplementation((op: string, input?: unknown) => Promise.resolve(op === "policies.list" && JSON.stringify(input).includes("\"alert\"") ? alertPolicies : []));
    const base = dal.coreOp.getMockImplementation();
    dal.coreOp.mockImplementation((op: string, ...rest: unknown[]) => (op === "preferences.get" ? Promise.reject(failure("INTERNAL")) : base?.(op, ...rest) ?? Promise.resolve(null)));
    const props = viewProps(await open());
    expect(calls("units.list")).toStrictEqual([[{ limit: 100, filters: { unitIds: ["u-1"] } }]]);
    expect(props).toMatchObject({ defaults: null, pins: [], policies: [{ id: "p-x", rule: "No heartbeat for 10 min → Critical" }] });
  });

  it("a restricted unit links to its restriction on the invoice; a failed or empty lookup leaves no link", async () => {
    detail = { ...detail, effectiveControlPolicy: { state: "restricted" } };
    expect(viewProps(await open()).restrictionHref).toBe("/customer/payments/inv-1?tab=restriction&restrictionId=r-1");
    expect(dal.coreAll.mock.calls.filter((c) => c[0] === "invoices.list")).toStrictEqual([["invoices.list", { filters: { contractId: "k-1" } }]]);
    restrictionPages = { items: [{ id: "r-1", unitIds: ["u-other"] }] };
    expect(viewProps(await open()).restrictionHref).toBeNull();
    restrictionFails = true;
    expect(viewProps(await open()).restrictionHref).toBeNull();
  });
});

describe("customer unit actions", () => {
  it("createCommand sends the current unit version; a refusal comes back with its code", async () => {
    await expect(createCommand("u-1", { kind: "set_power", power: true })).resolves.toStrictEqual({ ok: true, value: { id: "cmd-1" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["commands.create", { unitId: "u-1", action: { kind: "set_power", power: true }, expectedUnitVersion: 7 }, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(failure("NOT_FOUND"));
    await expect(createCommand("u-1", { kind: "set_power", power: false })).resolves.toMatchObject({ ok: false, code: "NOT_FOUND" });
    expect(calls("commands.create")).toHaveLength(1);
  });

  it("setUnitAlertPolicies sends the list with the unit version and re-renders the route, also after a CONFLICT", async () => {
    await expect(setUnitAlertPolicies("u-1", ["p-1", "p-101"])).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["units.setAlertPolicies", { unitId: "u-1", alertPolicyIds: ["p-1", "p-101"] }, { write: true, expectedVersion: 7 }]);
    dal.coreOp.mockImplementationOnce(() => Promise.resolve(detail)).mockRejectedValueOnce(failure("CONFLICT"));
    await expect(setUnitAlertPolicies("u-1", [])).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});
