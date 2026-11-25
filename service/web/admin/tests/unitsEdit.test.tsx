// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { Children, isValidElement, type ReactNode } from "react";
import { locationTree, registerKpis, selection, type ApiUnitDetail, type ClientUserRow, type CustomerRow, type PolicyLine } from "@ac/web/lib/assets";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// A unit and the customer's tabs on HQ customers & units (FR-A02, FR-A17): the unit edit (name, place, model, dates,
// scope; a move asks a reason), taking it out of use or removing it with what blocks that, its alert policies (attach,
// detach, back from a new policy), the Alert policies tab with the default rules, and the client users (add, role,
// sign-in, sign out everywhere, set-up e-mail, password reset preview, remove).
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return {
    saveUnit: fn(), archiveUnit: fn(), deleteUnit: fn(), setUnitPolicies: fn(), setDefaultRule: fn(), inviteClientUser: fn(), updateClientUser: fn(), removeClientUser: fn(), signOutEverywhere: fn(),
    resendInvite: fn(), previewPasswordReset: fn(),
  };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), push: vi.fn<(href: string) => void>(), toasts: [] as string[], params: new URLSearchParams() }));
vi.mock("server-only", () => ({}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: nav.push, replace: vi.fn() }), usePathname: () => "/admin/units", useSearchParams: () => nav.params }));
vi.mock("../app/admin/units/actions", () => actions);
vi.mock("../app/admin/units/_components/unit-device", () => ({ NewUnitDeviceSection: () => null, UnitDevicePanel: ({ canWrite }: Readonly<{ canWrite: boolean }>) => <p>{canWrite ? "device panel (writable)" : "device panel"}</p> }));
vi.mock("@ac/web/components/LocationPicker", () => ({ LocationPicker: () => null, PinMap: () => null }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav.patch }));
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
// a row's dropdown, as one button per offered item
vi.mock("@ac/web/components/ui", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  ActionMenu: ({ label, onChoose, children }: Readonly<{ label: string; onChoose: (v: string) => void; children: ReactNode }>) => (
    <span>{Children.toArray(children).filter((o) => isValidElement<{ value: string; children: string }>(o)).map((o) => {
      if (!isValidElement<{ value: string; children: string }>(o)) return null;
      return <button key={o.props.value} type="button" aria-label={`${label}: ${o.props.children}`} onClick={() => { onChoose(o.props.value); }}>{o.props.children}</button>;
    })}</span>
  ),
}));

import { UnitsView, type CustomerLive, type UnitLive, type UnitsLive } from "../app/admin/units/_components/units-view";

const tree = locationTree(
  [{ id: "p-home", version: 1, customerOrgId: "org-a", kind: "home", name: "Home", address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-01T00:00:00Z" }],
  [{ id: "s-room", version: 1, propertyId: "p-home", parentSpaceId: null, kind: "room", name: "Bedroom", archived: false }], [], "org-a",
);
const detail = (over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id: "u-12345678", version: 4, customerOrgId: "org-a", propertyId: "p-home", spaceId: "s-room", displayName: "Bedroom AC", modelId: "m-1", type: "split", installState: "installed", installedAt: "2026-01-10T00:00:00Z",
  serviceScope: ["indoor"], alertPolicyIds: ["pol-temp"], archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 2,
  location: { pathLabels: ["Home", "Bedroom"], address: null, accessInstructions: null }, pendingCommandIds: [], ...over,
});
const policy = (id: string, over: Partial<PolicyLine> = {}): PolicyLine => ({ id, version: 1, kind: "alert", name: `Policy ${id}`, type: "Temperature", condition: "> 28 °C for 10 min", madeBy: "Owner", units: 1, enabled: true, ...over });
const policies = [policy("pol-default", { kind: "default_alert", name: "Default alerts", type: "Default", units: 3 }), policy("pol-temp"), policy("pol-co2", { enabled: false, name: "CO2 high" })];
const unitLive = (over: Partial<UnitLive> = {}, u: Partial<ApiUnitDetail> = {}): UnitLive => ({
  u: detail(u), path: "Home › Bedroom", model: "AC-Co Split 1 · split", device: { id: "d-1", version: 2, serial: "AC-0001", connection: "online", iotModelId: "i-1", mqttNode: null }, deviceKnown: true,
  attached: [
    { id: "pol-default", kind: "default_alert", name: "Default alerts", type: "Default", condition: "4 rules", madeBy: "Always attached", on: "4 of 4 rules on" },
    { id: "pol-temp", kind: "alert", name: "Policy pol-temp", type: "Temperature", condition: "> 28 °C", madeBy: "Made by Owner", on: "" },
  ],
  blockers: [], archivedNote: null, seen: "5 min ago", ...over,
});
const user = (id: string, over: Partial<ClientUserRow> = {}): ClientUserRow => ({
  id, version: 1, email: `${id}@example.com`, name: `User ${id}`, role: "member", status: "active", sub: `${id}@example.com`, lastSignIn: "today", channels: "Email", lastOwner: false, you: false, membershipId: `m-${id}`, revoked: false, ...over,
});
const row: CustomerRow = {
  id: "cust-a", version: 1, name: "Demo Customer A", billingName: "Demo Customer A", status: "active", orgId: "org-a", orgVersion: 1, orgStatus: "active", profile: "general", since: "Jan 2026",
  properties: 1, propertyNames: ["Home"], units: 1, operation: "100%", running: 1, known: 1, alerts: 2, standing: null, marks: [],
};
function customerOf(over: Partial<CustomerLive> = {}): CustomerLive {
  return {
    row, tab: "overview", users: [], tree, selection: selection(tree, "s-room", []), places: [{ propertyId: "p-home", spaceId: "", label: "Home (no space)" }, { propertyId: "p-home", spaceId: "s-room", label: "Home › Bedroom" }],
    models: [{ id: "m-1", label: "AC-Co Split 1" }, { id: "m-2", label: "AC-Co Split 2" }], iotModels: [], canRegisterDevice: true, perProperty: "Home 1", alertsSub: null, filter: { connections: [], search: "" }, total: 1,
    units: [], lastEdit: null, policies, rules: { policyId: "pol-default", items: [
      { ruleKey: "offline", name: "AC offline", condition: "no heartbeat 10 min", enabled: true, version: 2, note: "set by the owner" },
      { ruleKey: "refrigerant_low", name: "Refrigerant low", condition: "< 400 kPa", enabled: false, version: 0, note: null },
    ] },
    canRules: true, canAttach: true, unit: unitLive(), ...over,
  };
}
const liveOf = (c: CustomerLive, over: Partial<UnitsLive> = {}): UnitsLive => ({
  now: "2026-09-14T01:00:00Z", todayKL: "2026-09-14", canWrite: true, canForce: false, standingKnown: true, rows: [row], kpis: registerKpis([row], []), search: "", missing: false, top: "customers",
  activeCustomers: [], coverageAttention: null, map: { styleUrl: null, search: false }, customer: c, ...over,
});
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };
const menu = (email: string, item: string) => { fireEvent.click(button(`Actions for ${email}: ${item}`)); }; // a row's dropdown item

beforeEach(() => {
  nav.patch.mockReset(); nav.push.mockReset(); nav.toasts = []; nav.params = new URLSearchParams();
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("the unit edit", () => {
  it("shows the unit's state, goes back to its place, and saves its changes at its version", async () => {
    actions.saveUnit.mockResolvedValue({ ok: true, value: "u-12345678" });
    render(<UnitsView live={liveOf(customerOf())} />);
    expect(screen.getByText(/\/ Demo Customer A \/ Bedroom AC/)).toBeDefined();
    expect(screen.getByText("2 open alerts")).toBeDefined();
    expect(screen.getByText("seen 5 min ago")).toBeDefined();
    expect(screen.getByText("device panel (writable)")).toBeDefined();
    expect(screen.getByRole("link", { name: /Device events/ }).getAttribute("href")).toBe("/admin/audit?tab=devices&deviceId=d-1");
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.click(button(/back to Bedroom/));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, locationId: "s-room" });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Master AC" } });
    fireEvent.change(screen.getByLabelText(/^Aircon model/), { target: { value: "m-2" } });
    fireEvent.change(screen.getByLabelText(/^Warranty end/), { target: { value: "2027-01-10" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "electrical" }));
    expect(screen.getByText("4 unsaved changes · Name, Model, Warranty end, Service scope")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "indoor" }));
    fireEvent.change(screen.getByLabelText(/^Installed at/), { target: { value: "2026-02-01" } });
    expect(screen.getByText("5 unsaved changes · Name, Model, Installed at, Warranty end, Service scope")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "" } });
    fireEvent.click(button("Save changes"));
    expect(screen.getByText("1–120 characters")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: " Master AC " } });
    fireEvent.click(button("Save changes"));
    await flush();
    expect(actions.saveUnit).toHaveBeenCalledWith(expect.objectContaining({ id: "u-12345678", displayName: "Master AC", propertyId: "p-home", spaceId: "s-room" }), 4);
    expect(nav.toasts).toStrictEqual(["Unit saved"]);
  });

  it("asks a reason to move the unit, and keeps a place or model it cannot list", async () => {
    actions.saveUnit.mockResolvedValue({ ok: true, value: "u-12345678" });
    const { rerender } = render(<UnitsView live={liveOf(customerOf())} />);
    fireEvent.change(screen.getByRole("combobox", { name: /^Location/ }), { target: { value: "p-home|" } });
    fireEvent.click(button("Save changes"));
    expect(dialog().getByText("Home (no space)")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Save location" }));
    expect(dialog().getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Save changes"));
    fireEvent.change(dialog().getByLabelText(/^Change reason/), { target: { value: " Moved downstairs " } });
    fireEvent.click(dialog().getByRole("button", { name: "Save location" }));
    await flush();
    expect(actions.saveUnit).toHaveBeenCalledWith(expect.objectContaining({ spaceId: null, changeReason: "Moved downstairs" }), 4);
    expect(nav.toasts).toStrictEqual(["Location changed — the old location stays in the history"]);
    rerender(<UnitsView live={liveOf(customerOf({ places: [], models: [], unit: unitLive({}, { version: 5, spaceId: null, installState: "planned", installedAt: null }) }))} />); // a newer version: the form starts again
    expect(screen.getByRole("option", { name: "Home › Bedroom (current)" })).toBeDefined();
    expect(screen.getByRole("option", { name: "AC-Co Split 1 · split" })).toBeDefined();
    expect(screen.getByLabelText<HTMLSelectElement>(/^Aircon model/).disabled).toBe(true);
    expect(screen.getByText(/^Installation planned: the installation date is set/)).toBeDefined();
    expect(button("Save changes").disabled).toBe(true); // nothing changed yet
    fireEvent.click(button(/back to Bedroom/));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, locationId: "unassigned:p-home" });
  });

  it("takes a unit out of use with a reason, or removes it, and says what blocks it", async () => {
    actions.archiveUnit.mockResolvedValue(ok);
    actions.deleteUnit.mockResolvedValue(ok);
    const { rerender } = render(<UnitsView live={liveOf(customerOf())} />);
    fireEvent.click(button("Delete unit…"));
    fireEvent.click(dialog().getByRole("button", { name: "Delete unit" }));
    expect(dialog().getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Replaced " } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete unit" }));
    await flush();
    expect(actions.archiveUnit).toHaveBeenCalledWith("u-12345678", 4, "Replaced");
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null });
    fireEvent.click(button("Delete unit…"));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Typo" } });
    fireEvent.click(dialog().getByRole("button", { name: "Remove permanently" }));
    await flush();
    expect(actions.deleteUnit).toHaveBeenCalledWith("u-12345678", 4, "Typo");
    expect(nav.toasts).toStrictEqual(["“Bedroom AC” taken out of use", "“Bedroom AC” deleted"]);
    rerender(<UnitsView live={liveOf(customerOf({ unit: unitLive({ blockers: [{ label: "Active job (repair, assigned)", href: "/admin/jobs?jobId=j-1" }, { label: "1 pending command", href: null }] }) }))} />);
    fireEvent.click(button("Delete unit…"));
    expect(dialog().getByText("This unit is still in use")).toBeDefined();
    expect(dialog().getByRole("link", { name: "Active job (repair, assigned)" }).getAttribute("href")).toBe("/admin/jobs?jobId=j-1");
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Delete unit" }).disabled).toBe(true);
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Remove permanently" }).disabled).toBe(true);
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    rerender(<UnitsView live={liveOf(customerOf({ unit: unitLive({ blockers: null }) }))} />);
    fireEvent.click(button("Delete unit…"));
    expect(dialog().getByText(/Your permissions don’t show this unit’s contracts/)).toBeDefined();
  });

  it("is read only for an archived unit and for a reader", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf({ unit: unitLive({ archivedNote: "Taken out of use 1 Aug 2026 · HQ", device: null, seen: null }, { archived: true, activeAlertCount: 0 }) }))} />);
    expect(screen.getByText("Read only — Taken out of use 1 Aug 2026 · HQ. Deleted (archived) units are left out of lists, summaries and totals.")).toBeDefined();
    for (const name of ["Delete unit…", "Save changes", /Attach policy/, "Detach"]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.getByLabelText<HTMLInputElement>(/^Name/).disabled).toBe(true);
    expect(screen.queryByRole("link", { name: /Device events/ })).toBeNull();
    fireEvent.click(button(/back to Bedroom/));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null, locationId: null }); // an archived unit has no place in the tree
    rerender(<UnitsView live={liveOf(customerOf(), { canWrite: false })} />);
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
    expect(screen.getByText("device panel")).toBeDefined();
  });
});

describe("the unit's alert policies", () => {
  it("attaches the customer's policies, detaches one, and links to a new one that comes back here", async () => {
    actions.setUnitPolicies.mockResolvedValue(ok);
    nav.params = new URLSearchParams("customerId=cust-a&unitId=u-12345678");
    render(<UnitsView live={liveOf(customerOf())} />);
    expect(screen.getByRole("link", { name: "View" }).getAttribute("href")).toBe("/admin/alerts?tab=policies&customerId=cust-a&unitId=u-12345678&policyId=pol-default");
    fireEvent.click(button("Detach"));
    await flush();
    expect(actions.setUnitPolicies).toHaveBeenLastCalledWith("u-12345678", 4, []);
    fireEvent.click(button(/Attach policy/));
    expect(dialog().getByRole("checkbox", { name: /Policy pol-temp/ }).getAttribute("aria-disabled") ?? dialog().getByText(/Already attached/)).toBeDefined();
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Attach policy" }).disabled).toBe(true);
    fireEvent.click(dialog().getByRole("checkbox", { name: /CO2 high/ }));
    expect(dialog().getByRole("link", { name: /New policy for Demo Customer A/ }).getAttribute("href")).toBe(`/admin/alerts?tab=policies&customerId=cust-a&new=cust-a&returnTo=${encodeURIComponent("/admin/units?customerId=cust-a&unitId=u-12345678")}`);
    fireEvent.click(dialog().getByRole("button", { name: "Attach 1 policy" }));
    await flush();
    expect(actions.setUnitPolicies).toHaveBeenLastCalledWith("u-12345678", 4, ["pol-temp", "pol-co2"]);
    expect(nav.toasts).toStrictEqual(["“Policy pol-temp” detached", "1 policy attached"]);
  });

  it("reopens Attach with the policy just made ticked, and offers a new one when the customer has none", () => {
    nav.params = new URLSearchParams("customerId=cust-a&unitId=u-12345678&attach=pol-co2");
    render(<UnitsView live={liveOf(customerOf())} />);
    expect(dialog().getByRole("button", { name: "Attach 1 policy" })).toBeDefined();
    fireEvent.click(dialog().getByRole("checkbox", { name: /CO2 high/ }));
    fireEvent.click(dialog().getByRole("checkbox", { name: /CO2 high/ }));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ attach: null });
    cleanup();
    nav.params = new URLSearchParams();
    nav.patch.mockClear();
    render(<UnitsView live={liveOf(customerOf({ policies: [policies[0] ?? policy("x")] }))} />);
    fireEvent.click(button(/Attach policy/));
    expect(dialog().getByText("No customer policies yet")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.patch).not.toHaveBeenCalledWith({ attach: null }); // nothing in the URL to clear
    cleanup();
    render(<UnitsView live={liveOf(customerOf({ policies: null }))} />);
    expect(screen.getByText("Policies need “View alert policies”.")).toBeDefined();
  });
});

describe("the Alert policies tab", () => {
  it("lists the customer's policies, opens one, and switches a default rule for the customer", async () => {
    actions.setDefaultRule.mockResolvedValue(ok);
    render(<UnitsView live={liveOf(customerOf({ tab: "policies" }))} />);
    expect(screen.getByText("All 3")).toBeDefined();
    expect(screen.getByText("Off")).toBeDefined();
    expect(screen.getByRole("link", { name: /New policy for Demo Customer A/ }).getAttribute("href")).toBe("/admin/alerts?tab=policies&customerId=cust-a&new=cust-a");
    fireEvent.click(screen.getAllByRole("row")[2] ?? document.body);
    expect(nav.push).toHaveBeenCalledWith("/admin/alerts?tab=policies&customerId=cust-a&policyId=pol-temp");
    expect(screen.getByText("no heartbeat 10 min · set by the owner")).toBeDefined();
    fireEvent.click(screen.getByRole("switch", { name: "AC offline for Demo Customer A" }));
    await flush();
    expect(actions.setDefaultRule).toHaveBeenCalledWith("pol-default", "offline", "cust-a", false, 2);
    fireEvent.click(screen.getByRole("switch", { name: "Refrigerant low for Demo Customer A" }));
    await flush();
    expect(actions.setDefaultRule).toHaveBeenLastCalledWith("pol-default", "refrigerant_low", "cust-a", true, 0); // version 0 creates the setting
    expect(nav.toasts).toStrictEqual(["AC offline switched off for Demo Customer A", "Refrigerant low switched on for Demo Customer A"]);
  });

  it("needs alert.policy.read, and alert.policy.write to switch rules", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf({ tab: "policies", policies: null }))} />);
    expect(screen.getByText("Alert policies need “View alert policies”")).toBeDefined();
    rerender(<UnitsView live={liveOf(customerOf({ tab: "policies", canRules: false, rules: { policyId: "pol-default", items: [{ ruleKey: "offline", name: "AC offline", condition: "x", enabled: false, version: 0, note: null }] } }))} />);
    expect(screen.getByText("Switching rules needs “Edit alert policies”.")).toBeDefined();
    expect(screen.getByRole("switch", { name: "AC offline for Demo Customer A" }).hasAttribute("disabled") || screen.getByRole("switch", { name: "AC offline for Demo Customer A" }).getAttribute("aria-disabled") === "true").toBe(true);
  });
});

describe("the client users", () => {
  const users = [
    user("owner", { role: "owner", lastOwner: true }), user("invitee", { status: "invited", name: null, lastSignIn: "—" }), user("off", { status: "disabled" }), user("member"),
  ];
  const usersTab = (over: Partial<UnitsLive> = {}) => render(<UnitsView live={liveOf(customerOf({ tab: "users", users }), over)} />);

  it("adds a user with a name, a new email and a role", async () => {
    actions.inviteClientUser.mockResolvedValue({ ok: true, value: "cu-new" });
    usersTab();
    fireEvent.click(button(/Add user/));
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "MEMBER@example.com" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add user" }));
    expect(dialog().getByText("A name of 1–120 characters")).toBeDefined();
    expect(dialog().getByText("Already a user of this customer")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: " Nadia " } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: " nadia@example.com " } });
    fireEvent.click(dialog().getByRole("button", { name: "Owner" }));
    fireEvent.click(dialog().getByRole("button", { name: "Add user" }));
    await flush();
    expect(actions.inviteClientUser).toHaveBeenCalledWith("cust-a", "nadia@example.com", "owner", " Nadia "); // an email input drops the spaces itself
    expect(nav.toasts).toStrictEqual(["Nadia added — a set-up e-mail is on its way to nadia@example.com"]);
    fireEvent.click(button(/Add user/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers each user only what fits them: the last owner keeps the role, an invitee gets the e-mail again", async () => {
    actions.resendInvite.mockResolvedValue(ok);
    actions.previewPasswordReset.mockResolvedValue(ok);
    usersTab();
    expect(screen.queryByRole("button", { name: /Actions for owner@example.com: Change role/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Actions for owner@example.com: Remove/ })).toBeNull();
    expect(screen.getByText("invitee@example.com", { selector: "b" })).toBeDefined(); // no name yet: the address
    expect(screen.getByText("Not signed in yet")).toBeDefined();
    expect(screen.queryByRole("button", { name: /Actions for invitee@example.com: Sign out everywhere/ })).toBeNull();
    menu("invitee@example.com", "Resend set-up e-mail");
    menu("member@example.com", "Reset password");
    await flush();
    expect(actions.resendInvite).toHaveBeenCalledWith("invitee");
    expect(actions.previewPasswordReset).toHaveBeenCalledWith("member@example.com");
    expect(nav.toasts).toStrictEqual(["Set-up e-mail sent again to invitee@example.com", "Password reset preview created — the same generic message for every address"]);
  });

  it("changes a role, disables and enables sign-in, signs out everywhere and removes a user, each with a reason", async () => {
    for (const f of [actions.updateClientUser, actions.removeClientUser, actions.signOutEverywhere]) f.mockResolvedValue(ok);
    usersTab();
    menu("member@example.com", "Change role (Owner / Member)");
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    expect(dialog().getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Runs the home " } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.updateClientUser).toHaveBeenLastCalledWith("member", 1, { customerId: "cust-a", email: "member@example.com", clientRole: "owner", reason: "Runs the home" });
    menu("member@example.com", "Disable sign-in");
    expect(dialog().getByText(/cannot sign in until sign-in is enabled again/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Moved out" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.updateClientUser).toHaveBeenLastCalledWith("member", 1, { customerId: "cust-a", email: "member@example.com", clientRole: "member", status: "disabled", reason: "Moved out" });
    menu("off@example.com", "Enable sign-in");
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Back" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.updateClientUser).toHaveBeenLastCalledWith("off", 1, expect.objectContaining({ status: "active" }));
    menu("member@example.com", "Sign out everywhere…");
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Lost phone" } });
    fireEvent.click(dialog().getByRole("button", { name: "Sign out everywhere" }));
    await flush();
    expect(actions.signOutEverywhere).toHaveBeenCalledWith("m-member", "Lost phone");
    menu("member@example.com", "Remove from customer");
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Left" } });
    fireEvent.click(dialog().getByRole("button", { name: "Remove user" }));
    await flush();
    expect(actions.removeClientUser).toHaveBeenCalledWith("member", 1, "Left");
    menu("member@example.com", "Change role (Owner / Member)");
    fireEvent.click(dialog().getByRole("button", { name: "Member" }));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.toasts).toStrictEqual([
      "Role changed to Owner", "Sign-in disabled", "Sign-in enabled", "Signed out everywhere — every session of member@example.com has ended", "member@example.com removed from Demo Customer A",
    ]);
  });

  it("changes an owner back to a member, and offers nothing to a reader or an empty list", async () => {
    actions.updateClientUser.mockResolvedValue(ok);
    render(<UnitsView live={liveOf(customerOf({ tab: "users", users: [user("boss", { role: "owner" })] }))} />);
    menu("boss@example.com", "Change role (Owner / Member)");
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Handover" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.updateClientUser).toHaveBeenCalledWith("boss", 1, expect.objectContaining({ clientRole: "member" }));
    expect(nav.toasts).toStrictEqual(["Role changed to Member"]);
    cleanup();
    usersTab({ canWrite: false });
    expect(screen.queryByRole("button", { name: /Add user/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Actions for/ })).toBeNull();
    cleanup();
    render(<UnitsView live={liveOf(customerOf({ tab: "users" }))} />);
    expect(screen.getByText("No client users yet")).toBeDefined();
  });
});
