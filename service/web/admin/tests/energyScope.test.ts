import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";

// HQ energy analysis (FR-A13, IR878): without a unit set or a baseline in the URL, the analysis reads the scope's units —
// the customer's, or one property's, as the Overview's Energy KPI counts them
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code };
    }
  }
  return {
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["energy.read", "asset.read"]))),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string) => Promise<{ items: unknown[]; total: number }>>(),
  };
});
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/energy/_components/energy-view", () => ({ EnergyView: () => null }));
vi.mock("../app/admin/energy/_components/energy-demo", () => ({ EnergyDemo: () => null }));

import AdminEnergyPage from "../app/admin/energy/page";
import { EnergyView } from "../app/admin/energy/_components/energy-view";

const lists = new Map<string, unknown[]>([
  ["customers.list", [{ id: "cust-a", organizationId: "org-a", name: "Alpha" }, { id: "cust-b", organizationId: "org-b", name: "Beta" }]],
  ["properties.list", [{ id: "tower", name: "Tower", customerOrgId: "org-a", archived: false }, { id: "wing", name: "Wing", customerOrgId: "org-a", archived: false }]],
  ["units.list", [
    { id: "u-tower", displayName: "Tower AC", customerOrgId: "org-a", propertyId: "tower", archived: false },
    { id: "u-wing", displayName: "Wing AC", customerOrgId: "org-a", propertyId: "wing", archived: false },
    { id: "u-beta", displayName: "Beta AC", customerOrgId: "org-b", propertyId: "beta-home", archived: false },
  ]],
  ["baselines.list", []],
]);
async function analysedUnits(sp: Record<string, string>): Promise<string[] | undefined> {
  const page: ReactNode = await AdminEnergyPage({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
  if (!isValidElement<ComponentProps<typeof EnergyView>>(page) || page.type !== EnergyView) throw new TypeError("not EnergyView");
  return page.props.live.analysis?.unitIds;
}

beforeEach(() => {
  dal.coreAllPage.mockImplementation((op) => Promise.resolve({ items: lists.get(op) ?? [], total: 0 }));
  dal.coreOp.mockReset();
  dal.coreOp.mockResolvedValue(null);
});

describe("energy analysis scope (IR878)", () => {
  it("analyses every unit, a customer's units, or one property's units", async () => {
    expect(await analysedUnits({})).toStrictEqual(["u-tower", "u-wing", "u-beta"]);
    expect(await analysedUnits({ customerId: "cust-a" })).toStrictEqual(["u-tower", "u-wing"]);
    expect(await analysedUnits({ customerId: "cust-a", propertyId: "wing" })).toStrictEqual(["u-wing"]);
    expect(dal.coreOp).toHaveBeenLastCalledWith("energy.summary", expect.objectContaining({ unitIds: ["u-wing"] }));
  });

  it("keeps a unit set chosen in the URL over the scope", async () => {
    expect(await analysedUnits({ customerId: "cust-a", propertyId: "wing", unitIds: "u-tower" })).toStrictEqual(["u-tower"]);
  });
});
