// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { OpInput } from "@ac/web/lib/opTypes";
import { defaultRuleRows, policyCards, policyForm, toggledInput, type ApiAlertPolicy, type ApiDefaultPolicy } from "@ac/web/lib/customerPolicies";
import type { AlertsLive } from "../app/customer/alerts/_lib/load";

// The customer's Alert policies tab in API mode (FR-C15, DD-C15, Figma Client 06e/06f): the owner switches a default rule
// on at once and off after a confirm, a member sees the rules read-only; an own policy is switched, deleted after a
// confirm (the focus then goes to the list) and edited; the editor checks its fields before any write, focuses the first
// one to fix, saves with the session's customer, membership and time zone, and a new policy made from an AC's Attach
// dialog goes back to that AC with the policy to tick (returnTo). A refused write goes to onFail.
type Done<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  readAlert: vi.fn(),
  savePolicy: vi.fn<(input: OpInput<"policies.save">, version: number | null) => Promise<Done<string>>>(),
  deletePolicy: vi.fn<(policyId: string, version: number) => Promise<Done<null>>>(),
  setDefaultRule: vi.fn<(policyId: string, ruleKey: string, customerId: string, enabled: boolean, version: number) => Promise<Done<null>>>(),
}));
vi.mock("../app/customer/alerts/actions", () => actions);
const nav = vi.hoisted(() => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn(), params: new URLSearchParams() }));
vi.mock("next/navigation", () => ({ useRouter: () => nav, useSearchParams: () => nav.params }));

import { PoliciesLive } from "../app/customer/alerts/_components/policies-live";

type Live = AlertsLive["policies"];
const refused: ActionFailure = { code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null };
const policy = (id: string, over: Partial<ApiAlertPolicy> = {}): ApiAlertPolicy => ({
  id, version: 3, kind: "alert", name: `Policy ${id}`, unitIds: [], timezone: "Asia/Tokyo", enabled: true, priority: 50, disabledReason: null, customerId: "c-1",
  recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 60, cooldownMinutes: 5, metric: "temperature", operator: "gte", threshold: 30,
  recoveryThreshold: 28, durationSeconds: 60, activeWindow: null, severity: "warning", ...over,
});
const defaults: ApiDefaultPolicy = {
  id: "dp-1", version: 1, kind: "default_alert", name: "Default", rules: [
    { ruleKey: "co2", name: "CO₂ high", category: "air_quality", metric: "co2", operator: "gte", threshold: 1200, recoveryThreshold: 1000, durationSeconds: 900, activeWindow: null, severity: "warning" },
    { ruleKey: "gap", name: "Heartbeat gap", category: "connection", metric: "heartbeat_gap", operator: "gte", threshold: 10, recoveryThreshold: 5, durationSeconds: 600, activeWindow: null, severity: "critical" },
  ],
  ruleSettings: [{ id: "s-1", version: 4, ruleKey: "gap", customerId: "c-1", enabled: false, changedByMembershipId: "m-1", reason: null, updatedAt: "2026-09-10T02:00:00Z" }],
};
const names = new Map([["u-1", "Bedroom AC"], ["u-2", "Office AC"]]);
const live = (list: ApiAlertPolicy[], over: Partial<Live> = {}): Live => ({
  count: list.length + 1, owner: true, customerId: "c-1", membershipId: "m-1", timezone: "Asia/Kuala_Lumpur",
  defaults: { id: defaults.id, ...defaultRuleRows(defaults, "c-1", "m-1") }, cards: policyCards(list, (id) => names.get(id)), list, editor: null, ...over,
});
const editing = (p: ApiAlertPolicy | null, over: Partial<Live> = {}): Live =>
  live(p ? [p] : [], { editor: { policy: p, form: policyForm(p), attached: p ? p.unitIds.map((id) => names.get(id) ?? "AC") : [] }, ...over });
const onDone = vi.fn<(text: string) => void>();
const onFail = vi.fn<(f: ActionFailure) => void>();
const show = (p: Live) => render(<PoliciesLive p={p} onDone={onDone} onFail={onFail} />);
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const dialog = () => screen.getByRole("dialog");
const nameBox = () => {
  const el = document.getElementById("policy-name");
  if (!el) throw new Error("no name field");
  return el;
};
/** the page's words: the condition sentence's words stand between its selects and inputs */
const sentence = () => document.body.textContent;

beforeEach(() => {
  for (const f of [nav.refresh, nav.push, nav.replace, onDone, onFail]) f.mockReset();
  nav.params = new URLSearchParams();
  actions.savePolicy.mockReset().mockResolvedValue({ ok: true, value: "p-new" });
  actions.deletePolicy.mockReset().mockResolvedValue({ ok: true, value: null });
  actions.setDefaultRule.mockReset().mockResolvedValue({ ok: true, value: null });
});
afterEach(() => { cleanup(); });

describe("PoliciesLive list", () => {
  it("the owner switches a default rule on at once, and off only after the confirm", async () => {
    show(live([]));
    expect(screen.getByText("1 of 2 rules on")).toBeTruthy();
    expect(screen.getByText(/^Turned off: “Heartbeat gap”/)).toBeTruthy();
    expect(screen.queryByText("Only the account owner can change this.")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Heartbeat gap" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("Default rule “Heartbeat gap” switched on for all your ACs."); });

    fireEvent.click(screen.getByRole("switch", { name: "CO₂ high" }));
    expect(within(dialog()).getByText("Turn off “CO₂ high” for all your ACs?")).toBeTruthy();
    fireEvent.click(button("Keep it on"));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "CO₂ high" }));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    // only the switch-on was sent, at the setting's version
    expect(actions.setDefaultRule).toHaveBeenCalledExactlyOnceWith("dp-1", "gap", "c-1", true, 4);

    fireEvent.click(screen.getByRole("switch", { name: "CO₂ high" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Turn off" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("Default rule “CO₂ high” switched off for all your ACs."); });
    expect(actions.setDefaultRule).toHaveBeenLastCalledWith("dp-1", "co2", "c-1", false, 0);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a member sees the default rules read-only; a refused switch goes to onFail; no customer means no write", async () => {
    show(live([], { owner: false }));
    expect(screen.getByText("Only the account owner can change this.")).toBeTruthy();
    expect(screen.getByRole("switch", { name: "CO₂ high" }).getAttribute("aria-disabled")).toBe("true");
    cleanup();

    actions.setDefaultRule.mockResolvedValueOnce({ ok: false, ...refused });
    show(live([]));
    fireEvent.click(screen.getByRole("switch", { name: "Heartbeat gap" }));
    await waitFor(() => { expect(onFail).toHaveBeenCalledWith(expect.objectContaining(refused)); });
    expect(onDone).not.toHaveBeenCalled();
    cleanup();

    show(live([], { customerId: null }));
    fireEvent.click(screen.getByRole("switch", { name: "Heartbeat gap" }));
    expect(actions.setDefaultRule).toHaveBeenCalledOnce();
  });

  it("without a default policy and own policies the list says how to add one", () => {
    show(live([], { defaults: null }));
    expect(screen.queryByText("Default policy")).toBeNull();
    expect(screen.getByText("No policies of your own yet — “Create policy” adds one; attach it on an AC’s page.")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Create policy/ }).getAttribute("href")).toBe("/customer/alerts?tab=policies&policyId=new");
  });

  it("an own policy's switch saves the policy with only enabled changed; a refusal goes to onFail", async () => {
    const p = policy("p-1", { unitIds: ["u-1"], channels: ["inApp", "email"] });
    show(live([p]));
    expect(screen.getByText("Attached to 1 AC: Bedroom AC")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Edit" }).getAttribute("href")).toBe("/customer/alerts?tab=policies&policyId=p-1");
    fireEvent.click(screen.getByRole("switch", { name: "Policy p-1" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-1” switched off."); });
    expect(actions.savePolicy).toHaveBeenCalledWith(toggledInput(p, false), 3);
    cleanup();

    actions.savePolicy.mockResolvedValueOnce({ ok: false, ...refused });
    show(live([policy("p-1", { enabled: false })]));
    fireEvent.click(screen.getByRole("switch", { name: "Policy p-1" }));
    await waitFor(() => { expect(onFail).toHaveBeenCalledWith(expect.objectContaining(refused)); });
    expect(actions.savePolicy).toHaveBeenLastCalledWith(toggledInput(policy("p-1", { enabled: false }), true), 3);
    fireEvent.click(screen.getByRole("switch", { name: "Policy p-1" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-1” switched on."); });
    cleanup();

    show(live([policy("p-1", { disabledReason: "unit_archived", enabled: false })]));
    expect(screen.getByText("An attached AC was archived.")).toBeTruthy();
  });

  it("deleting a card asks first, names its ACs, deletes at the policy's version and focuses the list", async () => {
    show(live([policy("p-1", { unitIds: ["u-1", "u-2"], version: 7 })]));
    fireEvent.click(button("Delete"));
    expect(within(dialog()).getByText(/It is detached from 2 ACs first: Bedroom AC, Office AC\./)).toBeTruthy();
    fireEvent.click(button("Keep policy"));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.deletePolicy).not.toHaveBeenCalled();

    fireEvent.click(button("Delete"));
    fireEvent.click(button("Delete policy"));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-1” deleted and detached from Bedroom AC, Office AC."); });
    expect(actions.deletePolicy).toHaveBeenCalledWith("p-1", 7);
    expect(nav.replace).toHaveBeenCalledWith("/customer/alerts?tab=policies", { scroll: false });
    await waitFor(() => { expect(document.activeElement?.id).toBe("your-policies"); });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a refused delete closes the confirm and goes to onFail; an unattached policy says so", async () => {
    actions.deletePolicy.mockResolvedValueOnce({ ok: false, ...refused });
    show(live([policy("p-1")]));
    expect(screen.getByText("Not attached to any AC yet — attach it from an AC’s page")).toBeTruthy();
    fireEvent.click(button("Delete"));
    expect(within(dialog()).getByText(/It is not attached to any AC\./)).toBeTruthy();
    fireEvent.click(button("Delete policy"));
    await waitFor(() => { expect(onFail).toHaveBeenCalledWith(expect.objectContaining(refused)); });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(nav.replace).not.toHaveBeenCalled();
  });
});

describe("PolicyEditor", () => {
  it("a new policy is checked before any write, the first field to fix takes the focus, then it saves and the list focuses it", async () => {
    show(editing(null));
    expect(screen.getByText(/Create alert policy/)).toBeTruthy();
    expect(screen.getByText("Not attached to any AC yet.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Delete policy" })).toBeNull();
    fireEvent.click(button("Save policy"));
    expect(actions.savePolicy).not.toHaveBeenCalled();
    expect(screen.getByText("Name is 1–120 characters.")).toBeTruthy();
    await waitFor(() => { expect(document.activeElement?.id).toBe("policy-name"); });

    fireEvent.change(nameBox(), { target: { value: "  Hot room  " } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Email" }));
    fireEvent.click(button("Save policy"));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Hot room” saved — attach it on an AC’s page."); });
    const [input, version] = actions.savePolicy.mock.calls[0] ?? [];
    expect(version).toBeNull();
    expect(input).toMatchObject({ name: "Hot room", customerId: "c-1", recipientMembershipIds: ["m-1"], timezone: "Asia/Kuala_Lumpur", channels: ["inApp", "email"], metric: "temperature", threshold: 30, durationSeconds: 60 });
    expect(nav.replace).toHaveBeenCalledWith("/customer/alerts?tab=policies", { scroll: false });

    // the list then shows with the new policy: its title takes the focus
    cleanup();
    show(live([policy("p-new", { name: "Hot room" })]));
    expect(document.activeElement?.id).toBe("policy-title-p-new");
  });

  it("the condition names one problem at a time, marks its field, and follows the metric and the direction", () => {
    show(editing(null));
    // a duration out of range alone is named only once a save was tried
    fireEvent.change(screen.getByRole("textbox", { name: "Duration" }), { target: { value: "2" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Duration unit" }), { target: { value: "s" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Duration" }), { target: { value: "0" } });
    expect(screen.queryByText("Between 1 s and 24 h.")).toBeNull();
    fireEvent.change(nameBox(), { target: { value: "Hot room" } });
    fireEvent.click(button("Save policy"));
    expect(document.getElementById("policy-condition-error")?.textContent).toBe("Between 1 s and 24 h.");
    expect(screen.getByRole("textbox", { name: "Duration" }).getAttribute("aria-invalid")).toBe("true");
    expect(actions.savePolicy).not.toHaveBeenCalled();
    cleanup();

    show(editing(null));
    expect(screen.getByText("Recovery 28 °C is below the 30 °C limit · unit fixed to °C for this metric")).toBeTruthy();
    expect(sentence()).toContain("Alert when room temperature is");
    const threshold = screen.getByRole("textbox", { name: "Threshold" });
    fireEvent.change(threshold, { target: { value: "abc" } });
    expect(threshold.getAttribute("aria-invalid")).toBe("true");
    expect(threshold.getAttribute("aria-describedby")).toBe("policy-condition-error");
    expect(document.getElementById("policy-condition-error")?.textContent).toBe("Enter a number.");
    fireEvent.change(threshold, { target: { value: "30" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Recovery" }), { target: { value: "31" } });
    expect(screen.getByRole("textbox", { name: "Recovery" }).getAttribute("aria-invalid")).toBe("true");
    expect(threshold.getAttribute("aria-invalid")).toBeNull();

    // a falling limit recovers above it
    fireEvent.change(screen.getByRole("combobox", { name: "Operator" }), { target: { value: "lte" } });
    expect(sentence()).toContain("Recover when it rises above");
    expect(screen.getByText("Recovery 31 °C is above the 30 °C limit · unit fixed to °C for this metric")).toBeTruthy();

    for (const [group, subject] of [[/^Humidity/, "humidity"], [/^Power/, "power"], [/^Air quality/, "CO₂"]] as const) {
      fireEvent.click(button(group));
      expect(sentence()).toContain(`Alert when ${subject} is`);
    }
    fireEvent.click(button("PM2.5 (µg/m³)"));
    expect(sentence()).toContain("Alert when PM2.5 is");
  });

  it("the window, the severity and the summary: no day picked is refused after a try", async () => {
    show(editing(null));
    fireEvent.click(screen.getByRole("checkbox", { name: "Only on some days and times" }));
    expect(screen.getByText("Asia/Kuala_Lumpur")).toBeTruthy();
    fireEvent.click(button("Every day"));
    expect(screen.getByRole("button", { name: "Sun" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(button("Mon–Fri"));
    expect(screen.getByRole("button", { name: "Sun" }).getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(button("Sat"));
    expect(screen.getByRole("button", { name: "Sat" }).getAttribute("aria-pressed")).toBe("true");
    for (const d of ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]) fireEvent.click(button(d));
    fireEvent.change(within(screen.getByRole("group", { name: "From" })).getByRole("combobox", { name: "Hour" }), { target: { value: "09" } });
    fireEvent.change(within(screen.getByRole("group", { name: "To" })).getByRole("combobox", { name: "Hour" }), { target: { value: "17" } });
    fireEvent.click(button("Critical"));
    fireEvent.change(nameBox(), { target: { value: "Night" } });
    fireEvent.click(button("Save policy"));
    expect(screen.getByText("Pick at least one day and two different times.")).toBeTruthy();
    // the window is a set of controls: its first one takes the focus
    await waitFor(() => { expect(document.activeElement).toBe(button("Mon–Fri")); });
    expect(actions.savePolicy).not.toHaveBeenCalled();
    fireEvent.click(button("Wed"));
    expect(sentence()).toContain("Alert me (critical) when room temperature stays ≥ 30 °C for 1 min on Wed 09:00–17:00. Clears below 28 °C.");
  });

  it("a new policy from an AC's Attach dialog goes back to that AC with the policy to tick; another address is ignored", async () => {
    nav.params = new URLSearchParams("returnTo=/customer/units/u-1");
    show(editing(null));
    expect(screen.getByRole("link", { name: "Back to the AC without saving" }).getAttribute("href")).toBe("/customer/units/u-1");
    expect(screen.getByRole("link", { name: "Cancel" }).getAttribute("href")).toBe("/customer/units/u-1");
    fireEvent.change(nameBox(), { target: { value: "Hot room" } });
    fireEvent.click(button("Save policy"));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Hot room” saved — tick it to attach it to this AC."); });
    expect(nav.push).toHaveBeenCalledWith("/customer/units/u-1?attach=p-new");
    expect(nav.replace).not.toHaveBeenCalled();
    cleanup();

    nav.params = new URLSearchParams("returnTo=https://evil.example/customer/units/u-1");
    show(editing(null));
    expect(screen.queryByRole("link", { name: "Back to the AC without saving" })).toBeNull();
    expect(screen.getByRole("link", { name: "Cancel" }).getAttribute("href")).toBe("/customer/alerts?tab=policies");
  });

  it("an own policy saves at its version and is deleted from the editor; the delete hint counts its ACs", async () => {
    const p = policy("p-1", { unitIds: ["u-1"], activeWindow: { weekdays: [1], startLocal: "08:00", endLocal: "10:00" }, durationSeconds: 90 });
    nav.params = new URLSearchParams("returnTo=/customer/units/u-1");
    show(editing(p));
    expect(screen.getByText(/Edit alert policy/)).toBeTruthy();
    expect(screen.getByText("Asia/Tokyo")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Back to the AC without saving" })).toBeNull();
    expect(screen.getByText("Deleting detaches it from its AC first (you confirm). The default policy can’t be deleted.")).toBeTruthy();
    fireEvent.click(button("Save policy"));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-1” saved."); });
    expect(actions.savePolicy).toHaveBeenCalledWith(expect.objectContaining({ id: "p-1", timezone: "Asia/Tokyo", durationSeconds: 90 }), 3);

    fireEvent.click(button("Delete policy"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Delete policy" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-1” deleted and detached from Bedroom AC."); });
    expect(actions.deletePolicy).toHaveBeenCalledWith("p-1", 3);
    cleanup();

    show(editing(policy("p-2", { unitIds: ["u-1", "u-2"] })));
    expect(screen.getByText("Deleting detaches it from all 2 ACs first (you confirm). The default policy can’t be deleted.")).toBeTruthy();
    cleanup();

    actions.deletePolicy.mockResolvedValueOnce({ ok: false, ...refused });
    show(editing(policy("p-3")));
    expect(screen.getByText("Deleting detaches it from every AC first (you confirm). The default policy can’t be deleted.")).toBeTruthy();
    fireEvent.click(button("Delete policy"));
    fireEvent.click(button("Keep policy"));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete policy"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Delete policy" }));
    await waitFor(() => { expect(onFail).toHaveBeenCalledWith(expect.objectContaining(refused)); });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete policy"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Delete policy" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalledWith("“Policy p-3” deleted."); });
  });

  it("without the customer account the editor says saving is not possible and sends nothing", () => {
    show(editing(null, { customerId: null }));
    expect(screen.getByText("Your customer account could not be read — saving is not possible.")).toBeTruthy();
    fireEvent.change(nameBox(), { target: { value: "Hot room" } });
    fireEvent.click(button("Save policy"));
    expect(actions.savePolicy).not.toHaveBeenCalled();
  });
});
