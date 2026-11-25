import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiBaseline, ApiEnergySummary } from "@ac/web/lib/energy";

// HQ energy analysis (FR-A13, SCR-A13): the Server Actions (a baseline saved new or at its version, deleted with a
// reason) and the Server Component (the analysis period and baseline from the URL, a validation refusal shown beside
// the form, the baselines tab's scope and period). The default unit set is energyScope.test.ts.
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
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:23:45Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["energy.read", "energy.write", "asset.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/energy/_components/energy-view", () => ({ EnergyView: () => null }));
vi.mock("../app/admin/energy/_components/energy-demo", () => ({ EnergyDemo: () => null }));

import { deleteBaseline, saveBaseline } from "../app/admin/energy/actions";
import AdminEnergyPage from "../app/admin/energy/page";
import { EnergyView } from "../app/admin/energy/_components/energy-view";
import { EnergyDemo } from "../app/admin/energy/_components/energy-demo";
import EnergyLoading from "../app/admin/energy/loading";
import EnergyErrorState from "../app/admin/energy/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const refused = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code.toLowerCase()}`, retryAfterSeconds: null });
const baseline = (id: string, over: Partial<ApiBaseline> = {}): ApiBaseline => ({
  id, version: 2, createdAt: "2026-08-01T00:00:00Z", unitIds: ["u-1"], period: { from: "2026-08-24T16:00:00Z", to: "2026-08-31T16:00:00Z" }, method: "demo_fixed", baselineKWh: 100,
  quality: { kind: "modeled", coverage: null }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "Same week", source: "demo", ...over,
});
const summary: ApiEnergySummary = {
  period: { from: "2026-09-13T16:00:00Z", to: "2026-09-14T01:23:00Z" }, unitIds: ["u-1"],
  totals: { kWh: 9, amountMinor: 329, savedKWh: null, deltaKWh: null, savingPercentage: null, savedAmountMinor: null, emissionsKg: 5.3, savedEmissionsKg: null },
  currency: "MYR", baselineRef: null, factorRef: null, baselineSnapshot: null, factorSnapshot: null, tariffVersion: "tariff-demo-1", boundaryId: "ac_input_electricity", boundary: "AC input",
  coverage: 1, qualityWarnings: [],
};

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
  const pages = new Map<string, unknown[]>([["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", archived: false }]], ["baselines.list", [baseline("b-1"), baseline("b-2")]]]);
  dal.coreAllPage.mockImplementation((op) => Promise.resolve({ items: pages.get(op) ?? [], total: 0 }));
});

describe("energy Server Actions", () => {
  it("saves a new baseline without a version, the next version at its version, and deletes one with a reason", async () => {
    const input = { unitIds: ["u-1"], period: { from: "a", to: "b" }, boundaryId: "ac_input_electricity" as const, boundary: "AC", assumptions: "x", source: "y", method: "demo_fixed" as const, baselineKWh: 100 };
    dal.coreOp.mockResolvedValueOnce(baseline("b-new", { version: 1 }));
    expect(await saveBaseline(input)).toStrictEqual({ ok: true, value: { id: "b-new", version: 1 } });
    expect(JSON.stringify(dal.coreOp.mock.lastCall?.[2])).toBe('{"write":true}');
    dal.coreOp.mockResolvedValueOnce(baseline("b-1", { version: 3 }));
    expect(await saveBaseline({ ...input, id: "b-1" }, 2)).toStrictEqual({ ok: true, value: { id: "b-1", version: 3 } });
    expect(dal.coreOp.mock.lastCall?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    dal.coreOp.mockResolvedValueOnce({});
    expect(await deleteBaseline("b-1", 3, "Wrong unit set")).toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["baselines.delete", { id: "b-1", reason: "Wrong unit set" }, { write: true, expectedVersion: 3 }]);
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await deleteBaseline("b-1", 3, "x")).toMatchObject({ ok: false, code: "CONFLICT" });
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await saveBaseline(input)).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(5);
  });
});

const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof EnergyView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminEnergyPage(props(sp));
  if (!isValidElement<ComponentProps<typeof EnergyView>>(page) || page.type !== EnergyView) throw new TypeError("not EnergyView");
  return page.props.live;
}
const summaryCalls = () => dal.coreOp.mock.calls.filter(([name]) => name === "energy.summary").map(([, input]) => input);

describe("the energy page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminEnergyPage(props());
    expect(isValidElement(page) && page.type).toBe(EnergyDemo);
    expect(EnergyLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = EnergyErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("analyses today up to the current minute by default, or a baseline's own units and period", async () => {
    dal.coreOp.mockResolvedValue(summary);
    const today = await liveOf();
    expect(summaryCalls()[0]).toStrictEqual({ from: "2026-09-13T16:00:00.000Z", to: "2026-09-14T01:23:00.000Z", unitIds: ["u-1"] });
    expect([today.tab, today.canWrite, today.analysis?.from, today.analysis?.baselineId, today.analysis?.summary?.actual]).toStrictEqual(["analysis", true, "2026-09-14T00:00", null, "9.0 kWh"]);
    dal.coreOp.mockClear();
    const withBaseline = await liveOf({ baselineId: "b-2" });
    expect(summaryCalls()[0]).toStrictEqual({ from: "2026-08-24T16:00:00Z", to: "2026-08-31T16:00:00Z", unitIds: ["u-1"], baselineId: "b-2" });
    expect(withBaseline.analysis?.baselineId).toBe("b-2");
    dal.coreOp.mockClear();
    await liveOf({ from: "2026-09-01T00:00", to: "2026-09-08T00:00" });
    expect(summaryCalls()[0]).toMatchObject({ from: "2026-08-31T16:00:00.000Z", to: "2026-09-07T16:00:00.000Z" });
  });

  it("shows a validation refusal beside the form, reads nothing without units, and throws other failures", async () => {
    dal.coreOp.mockRejectedValueOnce(refused("VALIDATION", { period: "errors.period_too_long" }));
    const refusedRead = await liveOf();
    expect([refusedRead.analysis?.summary, typeof refusedRead.analysis?.error]).toStrictEqual([null, "string"]);
    dal.coreOp.mockClear();
    const none = await liveOf({ unitIds: "," });
    expect([none.analysis?.unitIds, none.analysis?.summary, summaryCalls()]).toStrictEqual([[], null, []]);
    dal.coreOp.mockRejectedValueOnce(refused("UNAVAILABLE"));
    await expect(AdminEnergyPage(props())).rejects.toThrow("UNAVAILABLE");
  });

  it("orders the unit options by customer, property and name, an unknown customer or property first", async () => {
    dal.coreOp.mockResolvedValue(summary);
    dal.coreAllPage.mockImplementation((op) => {
      const pages = new Map<string, unknown[]>([
        ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a", status: "active" }]],
        ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a", archived: false }, { id: "p-z", name: "Stray", customerOrgId: "org-z", archived: false }]],
        ["units.list", [
          { id: "u-b", displayName: "B AC", customerOrgId: "org-a", propertyId: "p-1", archived: false }, { id: "u-a", displayName: "A AC", customerOrgId: "org-a", propertyId: "p-1", archived: false },
          { id: "u-x", displayName: "X AC", customerOrgId: "org-z", propertyId: "p-none", archived: false },
        ]],
      ]);
      return Promise.resolve({ items: pages.get(op) ?? [], total: 0 });
    });
    const live = await liveOf();
    expect(live.units.map((u) => [u.id, u.customerId])).toStrictEqual([["u-x", ""], ["u-a", "cust-a"], ["u-b", "cust-a"]]);
    expect(live.properties).toStrictEqual([{ id: "p-1", name: "Tower", customerId: "cust-a" }, { id: "p-z", name: "Stray", customerId: "" }]);
  });

  it("lists every baseline on the baselines tab, or those of the URL scope and period, opening the URL's one", async () => {
    dal.coreAll.mockResolvedValue([baseline("b-2")]);
    const all = await liveOf({ tab: "baselines", baselineId: "b-2" });
    expect(dal.coreAll).not.toHaveBeenCalled();
    expect([all.list?.rows.map((r) => r.id), all.list?.selectedId, all.analysis]).toStrictEqual([["b-1", "b-2"], "b-2", undefined]);
    const scoped = await liveOf({ tab: "baselines", customerId: "cust-a", propertyId: "p-1", unitId: "u-1", from: "2026-08-01T00:00", to: "2026-09-01T00:00" });
    expect(dal.coreAll).toHaveBeenCalledWith("baselines.list", { filters: { customerId: "cust-a", propertyId: "p-1", unitId: "u-1", from: "2026-07-31T16:00:00.000Z", to: "2026-08-31T16:00:00.000Z" } });
    expect([scoped.list?.rows.map((r) => r.id), scoped.list?.selectedId, scoped.list?.from, scoped.list?.to]).toStrictEqual([["b-2"], "b-2", "2026-08-01T00:00", "2026-09-01T00:00"]);
    dal.coreAll.mockResolvedValue([]);
    const empty = await liveOf({ tab: "baselines", unitId: "u-9" });
    expect([empty.list?.rows, empty.list?.selectedId, empty.list?.from]).toStrictEqual([[], null, ""]);
  });
});
