// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

// The customer's default policy rules (FR-A02, Customers & units › Alert policies): each rule's switch is named after
// the rule and the customer, so a screen reader tells the rules apart (WCAG 4.1.2, R910); it was "On" / "Off" for all.
vi.mock("server-only", () => ({})); // the unit device panel's Server Actions are imported, not called
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }), usePathname: () => "/admin/units", useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/admin/units/actions", () => ({}));
vi.mock("@ac/web/components/LocationPicker", () => ({ LocationPicker: () => null, PinMap: () => null }));

import { PoliciesTab, type CustomerLive } from "../app/admin/units/_components/units-view";
import type { CustomerRow, RuleLine } from "@ac/web/lib/assets";

afterEach(cleanup);

const row: CustomerRow = {
  id: "cust-a", version: 1, name: "Demo Customer A", billingName: "Demo Customer A", status: "active", orgId: "org-a", orgVersion: 1, orgStatus: "active", profile: "general",
  since: "2026-01-01", properties: 1, propertyNames: ["Home"], units: 2, operation: "50%", running: 1, known: 2, alerts: 0, standing: null, marks: [],
};
const rule = (ruleKey: string, name: string, enabled: boolean): RuleLine => ({ ruleKey, name, enabled, condition: "—", version: 1, note: null });
const customer: CustomerLive = {
  row, tab: "policies", users: [], tree: [], selection: null, places: [], models: [], iotModels: [], canRegisterDevice: false, perProperty: "", alertsSub: null,
  filter: { connections: [], search: "" }, total: 0, units: [], lastEdit: null, policies: [],
  rules: { policyId: "pol-default", items: [rule("offline", "AC offline", true), rule("refrigerant_low", "Refrigerant low pressure", false)] },
  canRules: true, canAttach: false,
};

describe("Default policy rules of a customer", () => {
  it("names each rule's switch after the rule and the customer, whatever its state", () => {
    render(<PoliciesTab c={customer} run={vi.fn()} pending={false} />);
    expect(screen.getByRole("switch", { name: "AC offline for Demo Customer A" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("switch", { name: "Refrigerant low pressure for Demo Customer A" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("switch", { name: /^(On|Off)$/ })).toBeNull();
  });
});
