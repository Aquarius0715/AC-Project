// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { policyGroups, type ApiAutoPolicy } from "@ac/web/lib/automation";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ automation policy editor (FR-A11, DD-A11): the scope and the selected policy in the URL, the policy form (units
// by customer › property with Select all, the condition and the action, the temperature every target allows), saving
// at the version read, Discard, and the simulator: synthetic facts per unit, Run simulation, and Fire (demo) with its
// one-time event key kept for a repeat of the same evaluation only. The Phase 1A demo moves locally.
const actions = vi.hoisted(() => ({ saveAutoPolicy: vi.fn<(...args: unknown[]) => Promise<unknown>>(), simulateAuto: vi.fn<(...args: unknown[]) => Promise<unknown>>(), fireAuto: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[], ids: 0 }));
vi.mock("../app/admin/settings/automation/actions", () => actions);
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
vi.mock("@ac/web/lib/randomId", () => ({ randomId: () => { nav.ids += 1; return `ev-${String(nav.ids)}`; } }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, ok: string | ((v: unknown) => string), after?: (v: unknown) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      nav.toasts.push(typeof ok === "string" ? ok : ok(r.value));
      after?.(r.value);
    });
  }] as const,
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));

import { AutomationView } from "../app/admin/settings/automation/_components/automation-view";
import { AutomationDemo } from "../app/admin/settings/automation/_components/automation-demo";

type Live = ComponentProps<typeof AutomationView>["live"];
const policy = (id: string, over: Partial<ApiAutoPolicy> = {}): ApiAutoPolicy => ({
  id, version: 2, kind: "automation", name: `Policy ${id}`, unitIds: ["u-1", "u-2"], ownerMembershipId: "m-1", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 60, disabledReason: null,
  condition: { type: "tariff", operator: "gt", value: 0.6, unit: "MYR_per_kWh" }, action: { kind: "set_temperature", celsius: 26 }, ...over,
});
const units = [
  { id: "u-1", name: "Lobby AC", customerId: "cust-a", propertyId: "p-1", temperature: { min: 18, max: 30, step: 1 }, ventilation: true },
  { id: "u-2", name: "Office AC", customerId: "cust-a", propertyId: "p-1", temperature: { min: 20, max: 28, step: 1 }, ventilation: false },
  { id: "u-3", name: "Hall AC", customerId: "cust-b", propertyId: "p-2", temperature: { min: 31, max: 35, step: 1 }, ventilation: false }, // no overlap with the others
  { id: "u-4", name: "Fan room", customerId: "cust-b", propertyId: "p-2", temperature: null, ventilation: false }, // no setpoint at all
];
const customerOf = new Map([["u-1", "Alpha"], ["u-2", "Alpha"], ["u-3", "Beta"]]);
function liveOf(ps: ApiAutoPolicy[], selected: ApiAutoPolicy | null, over: Partial<Live> = {}): Live {
  return {
    now: "2026-09-14T01:02:00Z", canWrite: true, scope: {}, groups: policyGroups(ps, customerOf), selected, isNew: selected === null,
    customers: [{ id: "cust-b", name: "Beta" }, { id: "cust-a", name: "Alpha" }], properties: [{ id: "p-2", name: "Beach", customerId: "cust-b" }, { id: "p-1", name: "Tower", customerId: "cust-a" }], units, ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const sim = () => within(screen.getByText("Simulate").closest("[data-slot=card]") ?? document.body);
const scopeUnit = () => screen.getAllByLabelText<HTMLSelectElement>("Unit")[0] ?? document.body; // the scope's, before the simulator's
const decided = (decision: string, over: Record<string, unknown> = {}) => ({ unitId: "u-1", decision, ruleId: null, reason: null, ...over });

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = []; nav.ids = 0;
  actions.saveAutoPolicy.mockReset(); actions.simulateAuto.mockReset(); actions.fireAuto.mockReset();
});
afterEach(cleanup);

describe("the policy list and scope", () => {
  it("narrows the scope in the URL, hides disabled policies, and opens one", () => {
    const off = policy("p-off", { enabled: false, disabledReason: "capability_changed", priority: 10, unitIds: ["u-3"] });
    render(<AutomationView live={liveOf([policy("p-1"), off, policy("p-across", { unitIds: ["u-1", "u-3"], priority: 90 })], policy("p-1"), { scope: { customerId: "cust-a" } })} />);
    expect([...screen.getByLabelText<HTMLSelectElement>("Customer").options].map((o) => o.textContent)).toStrictEqual(["All customers", "Alpha", "Beta"]);
    expect([...screen.getByLabelText<HTMLSelectElement>("Property").options].map((o) => o.textContent)).toStrictEqual(["All properties", "Tower"]);
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "cust-b" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: "cust-b", propertyId: null, unitId: null, policyId: null });
    fireEvent.change(screen.getByLabelText("Customer"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, propertyId: null, unitId: null, policyId: null });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "p-1" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ propertyId: "p-1", unitId: null, policyId: null });
    fireEvent.change(screen.getByLabelText("Property"), { target: { value: "" } });
    fireEvent.change(scopeUnit(), { target: { value: "u-2" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: "u-2", policyId: null });
    fireEvent.change(scopeUnit(), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, policyId: null });
    expect(screen.getByText("Across customers")).toBeDefined();
    expect(screen.getByText("Priority 10 · Off · capability changed")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Show disabled" }));
    expect(screen.queryByText("Policy p-off")).toBeNull();
    expect(screen.queryByText("Beta", { selector: "div" })).toBeNull(); // the group goes with its last policy
    fireEvent.click(button(/Policy p-across/));
    expect(nav.patch).toHaveBeenLastCalledWith({ policyId: "p-across" });
    fireEvent.click(button(/New policy/));
    expect(nav.patch).toHaveBeenLastCalledWith({ policyId: "new" });
  });

  it("keeps a disabled policy listed while it is open, says why it was turned off, and lists nothing for an empty scope", () => {
    const off = policy("p-off", { enabled: false, disabledReason: "consent_revoked" });
    const { rerender } = render(<AutomationView live={liveOf([off], off)} />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Show disabled" }));
    expect(screen.getAllByText("Policy p-off").length).toBeGreaterThan(1); // still in the list
    expect(screen.getByText("Disabled automatically: consent revoked. Review the policy before enabling it again.")).toBeDefined();
    expect(screen.getByText("Disabled")).toBeDefined();
    rerender(<AutomationView live={liveOf([], null, { scope: { customerId: "cust-a", propertyId: "p-1" } })} />);
    expect(screen.getByText("No HQ automation policy targets this scope.")).toBeDefined();
    expect([...screen.getByLabelText<HTMLSelectElement>("Unit").options].map((o) => o.textContent)).toStrictEqual(["All units", "Lobby AC", "Office AC"]);
  });
});

describe("the policy form", () => {
  it("checks a new policy, picks units by group within the targets' range, and creates it disabled", async () => {
    actions.saveAutoPolicy.mockResolvedValue({ ok: true, value: { id: "p-new", version: 1 } });
    render(<AutomationView live={liveOf([], null)} />);
    expect(screen.getByText("Saving creates version 1 (disabled by default)")).toBeDefined();
    expect(screen.queryByText("Simulate")).toBeNull(); // simulated once saved
    fireEvent.change(screen.getByLabelText(/^Priority/), { target: { value: "101" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "-1" } });
    fireEvent.click(button("Create policy"));
    for (const text of ["1–120 characters", "An integer 0–100", "Choose at least one unit", "A value ≥ 0"]) expect(screen.getByText(text)).toBeDefined();
    fireEvent.click(button("Select all units of Alpha › Tower"));
    expect(screen.getByText("(targets allow 20–28 °C)", { exact: false })).toBeDefined(); // the range every target allows
    fireEvent.change(screen.getByLabelText("Temperature (°C)"), { target: { value: "30" } });
    expect(screen.getByText("Within 20–28 °C on every target unit")).toBeDefined();
    fireEvent.click(button("Clear the units of Alpha › Tower"));
    fireEvent.click(screen.getByRole("checkbox", { name: "Hall AC" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Lobby AC" }));
    expect(screen.getByText("The target units share no temperature setting")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Hall AC" }));
    fireEvent.change(screen.getByLabelText("Temperature (°C)"), { target: { value: "24" } });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: " Peak pre-cool " } });
    fireEvent.change(screen.getByLabelText(/^Priority/), { target: { value: "70" } });
    fireEvent.change(screen.getByLabelText("Comparison"), { target: { value: "gte" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "0.5" } });
    expect(screen.getByText(/When the electricity tariff is ≥ 0\.5 .* → set the temperature to 24 °C\./i)).toBeDefined();
    fireEvent.click(button("Create policy"));
    await flush();
    expect(actions.saveAutoPolicy).toHaveBeenCalledWith({
      kind: "automation", name: "Peak pre-cool", unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: false, priority: 70,
      condition: { type: "tariff", operator: "gte", value: 0.5, unit: "MYR_per_kWh" }, action: { kind: "set_temperature", celsius: 24 },
    }, undefined);
    expect(nav.toasts).toStrictEqual(["Policy created"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ policyId: "p-new" });
  });

  it("edits each condition and action kind, discards the edit, and saves the next version", async () => {
    actions.saveAutoPolicy.mockResolvedValue({ ok: true, value: { id: "p-1", version: 3 } });
    render(<AutomationView live={liveOf([policy("p-1")], policy("p-1"))} />);
    expect(screen.getByText("No unsaved changes · form loaded from policy v2")).toBeDefined();
    expect(button("Discard").disabled).toBe(true);
    fireEvent.click(button("Peak"));
    fireEvent.change(screen.getByLabelText("Condition"), { target: { value: "false" } });
    fireEvent.change(screen.getByLabelText("Action"), { target: { value: "set_power" } });
    fireEvent.change(screen.getByLabelText("Power"), { target: { value: "true" } });
    expect(screen.getByText("Unsaved changes — saving creates version 3")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.getByText("No unsaved changes · form loaded from policy v2")).toBeDefined();
    fireEvent.click(button("Solar"));
    expect(screen.getByText("kW")).toBeDefined();
    fireEvent.click(button("Battery"));
    fireEvent.change(screen.getByLabelText("Action"), { target: { value: "set_mode" } });
    fireEvent.change(screen.getByLabelText("Mode"), { target: { value: "dry" } });
    fireEvent.change(screen.getByLabelText("Action"), { target: { value: "ventilate" } });
    fireEvent.change(screen.getByLabelText("Level"), { target: { value: "high" } });
    fireEvent.change(screen.getByLabelText("Action"), { target: { value: "set_fan" } });
    fireEvent.change(screen.getByLabelText(/^Timezone/), { target: { value: "UTC" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Enabled" }));
    fireEvent.click(button("Save policy"));
    await flush();
    expect(actions.saveAutoPolicy).toHaveBeenCalledWith(expect.objectContaining({
      id: "p-1", timezone: "UTC", enabled: false, condition: { type: "battery", operator: "gt", value: 0.6, unit: "kW" }, action: { kind: "set_fan", fanLevel: "high" },
    }), 2);
    expect(nav.toasts).toStrictEqual(["Policy saved as v3"]);
  });

  it("shows a saved occupancy policy's condition, and starts the form and facts again for another policy", () => {
    const occupancy = policy("p-occ", { condition: { type: "occupancy", occupied: false }, action: { kind: "set_power", power: false }, unitIds: ["u-1"] });
    const { rerender } = render(<AutomationView live={liveOf([occupancy], occupancy)} />);
    expect(screen.getByLabelText<HTMLSelectElement>("Condition").value).toBe("false");
    expect(screen.getByRole("option", { name: "When the room is not occupied" })).toBeDefined();
    expect(sim().getAllByLabelText("Value")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Edited" } });
    const other = policy("p-1");
    rerender(<AutomationView live={liveOf([occupancy, other], other)} />);
    expect(screen.getByLabelText<HTMLInputElement>(/^Name/).value).toBe("Policy p-1"); // the other policy's own values
    expect(sim().getAllByLabelText("Value")).toHaveLength(2); // and its own units' facts
  });

  it("is read only without automation.policy.write", () => {
    render(<AutomationView live={liveOf([policy("p-1")], policy("p-1"), { canWrite: false })} />);
    for (const name of [/New policy/, "Save policy", "Discard", "Fire (demo)"]) expect(screen.queryByRole("button", { name })).toBeNull();
  });
});

describe("the simulator", () => {
  it("simulates the units' facts, adds and removes facts, and shows each decision with its rule or reason", async () => {
    actions.simulateAuto.mockResolvedValue({ ok: true, value: [
      decided("selected", { ruleId: "p-1" }), decided("suppressed", { unitId: "u-2", reason: "stale" }), decided("failed", { unitId: "u-gone-123", ruleId: "p-other-12345" }),
      decided("queued", { unitId: "u-4" }),
    ] });
    render(<AutomationView live={liveOf([policy("p-1")], policy("p-1"))} />);
    const rows = () => sim().getAllByLabelText("Value");
    expect(rows()).toHaveLength(2); // one fact per unit of the policy
    fireEvent.change(rows()[0] ?? document.body, { target: { value: "0.7" } });
    fireEvent.change(sim().getAllByLabelText("Quality")[1] ?? document.body, { target: { value: "stale" } });
    fireEvent.click(sim().getByRole("button", { name: /Add fact/ }));
    fireEvent.change(sim().getAllByLabelText("Fact")[2] ?? document.body, { target: { value: "peak" } });
    fireEvent.change(rows()[2] ?? document.body, { target: { value: "true" } });
    fireEvent.change(sim().getAllByLabelText("Unit")[2] ?? document.body, { target: { value: "u-2" } });
    fireEvent.click(sim().getByRole("button", { name: /Add fact/ }));
    fireEvent.click(sim().getAllByRole("button", { name: "Remove the fact" })[3] ?? document.body);
    expect(button("Fire (demo)").disabled).toBe(true); // simulate first
    fireEvent.click(button("Run simulation"));
    await flush();
    const [input] = actions.simulateAuto.mock.calls[0] ?? [];
    expect(input).toMatchObject({ eventId: "ev-1", occurredAt: "2026-09-14T01:02:00.000Z", unitIds: ["u-1", "u-2"] });
    expect(JSON.stringify(input)).toContain('"metric":"peak"');
    expect(nav.toasts).toStrictEqual(["Simulated — no commands created"]);
    expect(screen.getByText("Result per unit")).toBeDefined();
    expect(screen.getByText("this policy")).toBeDefined();
    expect(screen.getByText("stale data — skipped")).toBeDefined();
    expect(screen.getByText("rule p-other-")).toBeDefined();
    expect(screen.getByText("u-gone-1")).toBeDefined();
    expect(screen.getByText("queued")).toBeDefined(); // a decision the screen has no word for yet
    for (let left = sim().queryAllByRole("button", { name: "Remove the fact" }); left.length > 0; left = sim().queryAllByRole("button", { name: "Remove the fact" })) {
      fireEvent.click(left[0] ?? document.body); // rows are keyed by position: take the first each time
    }
    expect(button("Run simulation").disabled).toBe(true);
  });

  it("fires once per evaluation, repeating the same one under its event key and minute", async () => {
    actions.simulateAuto.mockResolvedValue({ ok: true, value: [decided("selected", { ruleId: "p-1" })] });
    actions.fireAuto.mockResolvedValueOnce({ ok: true, value: { results: [decided("requested", { ruleId: "p-1", commandId: "cmd-12345678-x" })], at: "2026-09-14T01:02:00.000Z" } })
      .mockResolvedValue({ ok: true, value: { results: [decided("requested")], at: "2026-09-14T01:05:00.000Z" } });
    render(<AutomationView live={liveOf([policy("p-1")], policy("p-1"))} />);
    fireEvent.click(button("Run simulation"));
    await flush();
    fireEvent.click(button("Fire (demo)"));
    await flush();
    expect(actions.fireAuto.mock.calls[0]?.[1]).toBeUndefined(); // a new event: the current minute
    expect(screen.getByText("Fire result per unit")).toBeDefined();
    expect(screen.getByText("this policy · command cmd-1234")).toBeDefined();
    fireEvent.click(button("Fire (demo)"));
    await flush();
    expect(actions.fireAuto.mock.calls[1]).toMatchObject([{ eventId: "ev-2" }, "2026-09-14T01:02:00.000Z"]); // the same evaluation again
    fireEvent.change(sim().getAllByLabelText("Value")[0] ?? document.body, { target: { value: "0.9" } });
    fireEvent.click(button("Fire (demo)"));
    await flush();
    expect(actions.fireAuto.mock.calls[2]?.[0]).toMatchObject({ eventId: "ev-3" }); // another evaluation, another key
    expect(actions.fireAuto.mock.calls[2]?.[1]).toBeUndefined();
    expect(nav.toasts.filter((m) => m.startsWith("Fired"))).toHaveLength(3);
  });

  it("has no fact to add for a policy without units", () => {
    render(<AutomationView live={liveOf([policy("p-0", { unitIds: [] })], policy("p-0", { unitIds: [] }))} />);
    expect(sim().getByRole<HTMLButtonElement>("button", { name: /Add fact/ }).disabled).toBe(true);
  });
});

describe("the automation demo", () => {
  it("simulates its fixture facts, checks the setpoint, and fires once", () => {
    render(<AutomationDemo />);
    fireEvent.click(button("Simulate"));
    expect(screen.getByText(/Peak-price pre-cool → set 26 °C/)).toBeDefined();
    expect(screen.getByText(/^missing_data/)).toBeDefined();
    fireEvent.change(screen.getAllByPlaceholderText("— no reading")[0] ?? document.body, { target: { value: "0.5" } });
    fireEvent.click(button(/Add fact/));
    fireEvent.click(button("Simulate"));
    expect(screen.getAllByText("no rule matched")).toHaveLength(2);
    fireEvent.click(button("Fire (demo)"));
    expect(button("Fire (demo)").disabled).toBe(true);
    expect(screen.getByText(/^Fire result → shared Command cmd-0500/)).toBeDefined();
    fireEvent.change(screen.getByDisplayValue("26"), { target: { value: "35" } });
    expect(screen.getByText("Outside 16–30 °C on at least one unit")).toBeDefined();
    fireEvent.click(button("Save (simulate old version)"));
    expect(screen.getByText(/^CONFLICT — saved from v1/)).toBeDefined();
    fireEvent.change(screen.getByDisplayValue("35"), { target: { value: "25" } });
    fireEvent.click(button("Save"));
    expect(screen.queryByText(/^CONFLICT/)).toBeNull();
    fireEvent.click(button(/Solar surplus cooling/));
    expect(screen.getByText("Disabled")).toBeDefined();
    fireEvent.click(button(/Peak-price pre-cool/));
    fireEvent.click(screen.getByRole("checkbox", { name: "Show disabled" }));
    expect(screen.queryByText("Solar surplus cooling")).toBeNull();
    fireEvent.click(button(/Occupancy stop/));
    fireEvent.click(button("Peak"));
    fireEvent.change(screen.getByDisplayValue("0.6"), { target: { value: "0.8" } });
    fireEvent.click(button(/New$/));
    expect(nav.toasts).toStrictEqual(["Fired — command cmd-0500 created (shared Command)", "Policy saved as v2", "New policy draft created"]);
  });
});
