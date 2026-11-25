// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { policyRows, type AdminPolicy, type ApiPolicy, type ApiRecipient } from "@ac/web/lib/adminAlerts";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The Policies tab of HQ alerts (DD-A05 items 7–9, IR345): the policies by customer and the editor of the selected
// one; a new policy for its owner (and from a default rule) created by Save, back to a unit's Attach dialog when it
// came from there; recipients from notifications.recipients; Delete detaches it everywhere. The demo saves nothing.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { savePolicy: vi.fn<Action>(), deletePolicy: vi.fn<Action>(), testPolicy: vi.fn<Action>(), saveDefaultRules: vi.fn<Action>(), switchDefaultRule: vi.fn<Action>() };
});
const nav = vi.hoisted(() => ({ push: vi.fn<(href: string) => void>(), search: "", toasts: [] as string[] }));
const op = vi.hoisted(() => {
  const state: { recipients: { data: unknown; loading: boolean; error: Error | null } | null; enabled: boolean[] } = { recipients: null, enabled: [] };
  return state;
});
vi.mock("../app/admin/alerts/actions", () => actions);
vi.mock("next/navigation", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn() }), useSearchParams: () => new URLSearchParams(nav.search),
}));
vi.mock("@ac/web/lib/useRefreshThen", () => ({ useRefreshThen: () => (then: () => void) => { then(); } }));
vi.mock("@ac/web/lib/useOp", () => ({
  useOp: (operation: string, _input: unknown, mock: unknown, map: (d: unknown) => unknown, enabled = true) => {
    if (operation === "notifications.recipients") op.enabled.push(enabled);
    const read = operation === "notifications.recipients" ? op.recipients : null;
    return enabled && read ? { data: read.data === null ? mock : map(read.data), loading: read.loading, error: read.error } : { data: mock, loading: false, error: null };
  },
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { nav.toasts.push(m); } }));
vi.mock("../app/admin/alerts/_components/default-policy", () => ({ DefaultPolicyEditor: ({ p }: Readonly<{ p: AdminPolicy }>) => <div>default policy editor {p.id}</div> }));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  TimeSelect: ({ value, onChange, "aria-label": label }: Readonly<{ value: string; onChange: (v: string) => void; "aria-label"?: string }>) => <input aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { NewPolicyModal, PoliciesTab, seedPolicies, type PoliciesLive } from "../app/admin/alerts/_components/policies-tab";

const customers = [{ id: "cust-a", name: "Demo Customer A", organizationId: "org-a" }, { id: "cust-b", name: "Beta Hotel", organizationId: "org-b" }];
const apiPolicy = (id: string, over: Partial<ApiPolicy> = {}): ApiPolicy => ({
  id, version: 2, kind: "alert", name: `Policy ${id}`, unitIds: ["u-1"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 50, customerId: "cust-a",
  recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 60, cooldownMinutes: 5, metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28,
  durationSeconds: 60, severity: "warning", ownerMembershipId: "m-1", ...over,
});
const apiDefault: ApiPolicy = {
  ...apiPolicy("pol-default"), kind: "default_alert", name: "Default", customerId: null, unitIds: [],
  rules: [{ ruleKey: "ventilation_co2", name: "Ventilation", category: "air_quality", metric: "co2", operator: "gte", threshold: 1000, recoveryThreshold: 900, durationSeconds: 600, severity: "warning", activeWindow: null }],
};
const rows = (ps: ApiPolicy[]): AdminPolicy[] => policyRows(ps, customers, [{ id: "m-1", displayName: "HQ Operator", role: "admin" }], undefined, [{ id: "u-1", displayName: "Bedroom AC" }]);
const pols = rows([apiDefault, apiPolicy("pol-1"), apiPolicy("pol-off", { enabled: false, customerId: "cust-b", unitIds: [] })]);
const live = (over: Partial<PoliciesLive> = {}): PoliciesLive => ({
  policyId: null, creatingFor: null, from: null, canWrite: true, timezone: "Asia/Tokyo",
  customers: [{ id: "cust-a", name: "Demo Customer A" }, { id: "cust-b", name: "Beta Hotel" }], units: [{ id: "u-1", name: "Bedroom AC", customerId: "cust-a" }], ...over,
});
type Props = ComponentProps<typeof PoliciesTab>;
const select = vi.fn<Props["select"]>();
const tab = (over: Partial<Props> = {}) => <PoliciesTab pols={pols} live={live()} sel={{ policyId: null, creatingFor: null, from: null }} select={select} copyFor={null} onCopy={vi.fn()} {...over} />;
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const candidates: ApiRecipient[] = [
  { id: "m-1", role: "admin", displayLabel: "HQ Operator", allowedChannels: ["inApp"] }, { id: "m-2", role: "client", displayLabel: "Ana", allowedChannels: ["inApp", "email", "whatsapp"] },
  { id: "m-3", role: "client", displayLabel: "Ben", allowedChannels: ["inApp"] },
];
const refusal = { ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} } satisfies { ok: false } & ActionFailure;

beforeEach(() => {
  nav.push.mockReset(); nav.search = ""; nav.toasts = []; select.mockReset();
  op.recipients = null; op.enabled = [];
  for (const fn of Object.values(actions)) fn.mockReset();
});
afterEach(cleanup);

describe("the policies list and editor", () => {
  it("groups the policies by customer, opens the first customer policy, and hides disabled ones unless open", () => {
    const { rerender } = render(tab());
    expect(screen.getByLabelText<HTMLInputElement>(/^Name/).value).toBe("Policy pol-1"); // the first customer policy, not the default
    expect(screen.getByText("DEMO CUSTOMER A")).toBeDefined();
    fireEvent.click(button(/Policy pol-off/));
    expect(select).toHaveBeenLastCalledWith("pol-off");
    rerender(tab({ showOff: false }));
    expect(screen.queryByRole("button", { name: /Policy pol-off/ })).toBeNull();
    rerender(tab({ showOff: false, sel: { policyId: "pol-off", creatingFor: null, from: null } }));
    expect(screen.getAllByText("Policy pol-off").length).toBeGreaterThan(0); // the open one stays listed
    rerender(tab({ sel: { policyId: "pol-default", creatingFor: null, from: null } }));
    expect(screen.getByText("default policy editor pol-default")).toBeDefined();
  });

  it("edits a saved policy, shows its unsaved changes, discards them, and saves it", async () => {
    actions.savePolicy.mockResolvedValueOnce(refusal).mockResolvedValueOnce({ ok: true, id: "pol-1" });
    render(tab());
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    expect(button("Save policy").disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Bedroom far too hot" } });
    fireEvent.change(screen.getByLabelText(/^Priority/), { target: { value: "70" } });
    expect(screen.getByText(/^2 unsaved changes · /)).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Bedroom far too hot" } });
    expect(screen.getByText(/^1 unsaved change · /)).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Timezone/), { target: { value: "Asia/Tokyo" } });
    fireEvent.change(screen.getByLabelText(/^Cooldown/), { target: { value: "10" } });
    fireEvent.change(screen.getByLabelText(/^Escalate after/), { target: { value: "120" } });
    fireEvent.click(screen.getByRole("switch", { name: "Enabled" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Email" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "WhatsApp" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "WhatsApp" }));
    fireEvent.click(button("Critical"));
    fireEvent.change(screen.getByLabelText(/^Alert when/), { target: { value: "31" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Only on some days and times" }));
    fireEvent.click(button("Save policy"));
    await flush();
    expect(nav.toasts).toHaveLength(1); // the refusal
    fireEvent.click(button("Save policy"));
    await flush();
    expect(actions.savePolicy).toHaveBeenLastCalledWith(expect.objectContaining({ id: "pol-1", name: "Bedroom far too hot", timezone: "Asia/Tokyo", cooldown: 10, escalate: 120, on: false, channels: ["inApp", "email"], severity: "critical", threshold: 31, activeWindow: { weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" } }));
    expect(nav.toasts.at(-1)).toBe("Policy saved");
  });

  it("refuses to save with errors and says how many to fix, and removes a recipient", () => {
    render(tab());
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: " " } });
    fireEvent.click(button("Remove HQ Operator"));
    expect(screen.getByText("No recipients yet — add at least one.")).toBeDefined();
    fireEvent.click(button("Save policy"));
    expect(nav.toasts).toStrictEqual(["Fix the validation errors first"]);
    expect(screen.getByText(/problems to fix · nothing saved$/)).toBeDefined();
    expect(actions.savePolicy).not.toHaveBeenCalled();
  });

  it("deletes a policy after saying which units it leaves, or that it has none", async () => {
    actions.deletePolicy.mockResolvedValueOnce(refusal).mockResolvedValueOnce({ ok: true });
    const { rerender } = render(tab());
    fireEvent.click(button("Delete policy"));
    expect(within(screen.getByRole("dialog")).getByText("“Policy pol-1” is detached from 1 unit and deleted.")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Delete policy"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete policy" }));
    await flush();
    expect(screen.getByRole("dialog")).toBeDefined(); // refused: still open
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete policy" }));
    await flush();
    expect(actions.deletePolicy).toHaveBeenLastCalledWith("pol-1", 2);
    expect(nav.toasts.at(-1)).toBe("“Policy pol-1” deleted");
    expect(select).toHaveBeenLastCalledWith(null);
    rerender(tab({ sel: { policyId: "pol-off", creatingFor: null, from: null } }));
    fireEvent.click(button("Delete policy"));
    expect(screen.getByText("“Policy pol-off” is deleted. It is not attached to any unit.")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says why no policy is shown", () => {
    const { rerender } = render(tab({ sel: { policyId: "pol-gone", creatingFor: null, from: null } }));
    expect(screen.getByText("This policy is not in the list")).toBeDefined();
    rerender(tab({ sel: { policyId: null, creatingFor: "cust-inactive", from: null } }));
    expect(screen.getByText("No such active customer")).toBeDefined();
    rerender(tab({ pols: [] }));
    expect(screen.getByText("No alert policies")).toBeDefined();
  });

  it("shows a read-only role the policy without anything to change", () => {
    render(tab({ live: live({ canWrite: false }) }));
    expect(screen.getByText("You can read this policy. Changing it needs “Edit alert policies”.")).toBeDefined();
    for (const name of ["Save policy", "Delete policy", /Add recipient/, "Remove HQ Operator"]) expect(screen.queryByRole("button", { name })).toBeNull();
  });
});

describe("a new policy", () => {
  it("starts blank for its owner, adds recipients from the list, and opens the created policy", async () => {
    op.recipients = { data: { items: candidates }, loading: false, error: null };
    actions.savePolicy.mockResolvedValue({ ok: true, id: "pol-new" });
    render(tab({ sel: { policyId: null, creatingFor: "cust-a", from: null } }));
    expect(screen.getByText("New policy · not saved yet")).toBeDefined();
    expect(screen.getAllByText("New alert policy").length).toBeGreaterThan(0);
    expect(screen.getByLabelText<HTMLSelectElement>(/^Timezone/).value).toBe("Asia/Tokyo"); // the display time zone
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Meeting room stuffy" } });
    expect(screen.getAllByText("Meeting room stuffy").length).toBeGreaterThan(1); // the list row follows the name
    fireEvent.click(button(/Add recipient/));
    const dialog = within(screen.getByRole("dialog"));
    const add = dialog.getByRole<HTMLButtonElement>("button", { name: "Add recipients" });
    expect(add.disabled).toBe(true);
    fireEvent.click(dialog.getByRole("checkbox", { name: /Ana/ }));
    fireEvent.click(dialog.getByRole("checkbox", { name: /Ben/ }));
    fireEvent.click(dialog.getByRole("checkbox", { name: /Ben/ }));
    fireEvent.click(dialog.getByRole("button", { name: "Add 1 recipient" }));
    expect(screen.getByText("Ana")).toBeDefined();
    fireEvent.click(button(/Add recipient/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("checkbox", { name: /Ben/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("checkbox", { name: /HQ Operator/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Add 2 recipients" }));
    fireEvent.click(button("Save policy"));
    await flush();
    expect(actions.savePolicy).toHaveBeenCalledWith(expect.objectContaining({ id: "", customerId: "cust-a", name: "Meeting room stuffy", recipients: [expect.objectContaining({ id: "m-2" }), expect.objectContaining({ id: "m-1" }), expect.objectContaining({ id: "m-3" })] })); // added in the list's order
    expect(nav.toasts).toStrictEqual(["Policy created — attach it to units on each unit’s page"]);
    expect(select).toHaveBeenCalledWith("pol-new");
  });

  it("goes back to the unit's Attach dialog with the new policy ticked when it came from there", async () => {
    nav.search = "returnTo=%2Fadmin%2Funits%3FunitId%3Du-1";
    op.recipients = { data: { items: candidates }, loading: false, error: null };
    actions.savePolicy.mockResolvedValue({ ok: true, id: "pol-new" });
    render(tab({ sel: { policyId: null, creatingFor: "cust-a", from: null } }));
    expect(screen.getByRole("link", { name: "Back to the unit without creating" }).getAttribute("href")).toBe("/admin/units?unitId=u-1");
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Lobby" } });
    fireEvent.click(button(/Add recipient/));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("checkbox", { name: /Ana/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Add 1 recipient" }));
    fireEvent.click(button("Save policy"));
    await flush();
    expect(nav.toasts).toStrictEqual(["Policy created — tick it to attach it to the unit"]);
    expect(nav.push).toHaveBeenCalledWith("/admin/units?unitId=u-1&attach=pol-new");
    expect(select).not.toHaveBeenCalled();
  });

  it("copies a default rule's condition, and Discard leaves the new policy", () => {
    render(tab({ sel: { policyId: null, creatingFor: "cust-b", from: "ventilation_co2" } }));
    expect(screen.getByLabelText<HTMLSelectElement>("Metric").value).toBe("co2");
    expect(screen.getByLabelText<HTMLInputElement>(/^Alert when/).value).toBe("1000");
    expect(screen.getByText("BETA HOTEL")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(select).toHaveBeenLastCalledWith(null);
  });

  it("says when the recipients load, fail, are all chosen, or when the customer has no unit", () => {
    for (const [read, text] of [
      [{ data: null, loading: true, error: null }, "Loading the recipients…"],
      [{ data: null, loading: false, error: new Error("down") }, "The recipients could not be loaded — close the dialog and try again."],
      [{ data: { items: [candidates[0]] }, loading: false, error: null }, "Everyone is already a recipient"],
    ] as const) {
      op.recipients = read;
      render(tab());
      fireEvent.click(button(/Add recipient/));
      expect(within(screen.getByRole("dialog")).getByText(text)).toBeDefined();
      fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
      cleanup();
    }
    render(tab({ live: live({ units: [] }), pols: rows([apiPolicy("pol-x", { unitIds: [] })]) }));
    fireEvent.click(button(/Add recipient/));
    expect(within(screen.getByRole("dialog")).getByText("Demo Customer A has no units yet")).toBeDefined();
    expect(op.enabled.at(-1)).toBe(false); // nothing to ask about
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("the policies demo", () => {
  it("saves and deletes nothing, and lists its fixture recipients", () => {
    render(<PoliciesTab pols={seedPolicies} sel={{ policyId: "policy-temp-a", creatingFor: null, from: null }} select={select} copyFor={null} onCopy={vi.fn()} />);
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "Bedroom hot" } });
    fireEvent.click(button("Save policy"));
    expect(nav.toasts).toStrictEqual(["Policy saved"]);
    fireEvent.click(button("Delete policy"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete policy" }));
    expect(nav.toasts.at(-1)).toBe("“Bedroom hot” deleted"); // the name as shown in the editor
    fireEvent.click(button(/Add recipient/));
    expect(within(screen.getByRole("dialog")).getByText("Everyone is already a recipient")).toBeDefined(); // both fixture recipients are on it
    expect(actions.savePolicy).not.toHaveBeenCalled();
    expect(actions.deletePolicy).not.toHaveBeenCalled();
  });
});

describe("a new policy in the demo", () => {
  it("starts in Kuala Lumpur time for the demo customer", () => {
    render(<PoliciesTab pols={seedPolicies} sel={{ policyId: null, creatingFor: "customer-a", from: null }} select={select} copyFor={null} onCopy={vi.fn()} />);
    expect(screen.getByLabelText<HTMLSelectElement>(/^Timezone/).value).toBe("Asia/Kuala_Lumpur");
    expect(screen.getByText("New policy · not saved yet")).toBeDefined();
  });
});

describe("the new policy dialog", () => {
  it("starts the policy for the chosen owner, preselected when scoped, and says when no customer may own one", async () => {
    const onStart = vi.fn(), onClose = vi.fn();
    render(<NewPolicyModal customers={[{ id: "cust-a", name: "Demo Customer A" }, { id: "cust-b", name: "Beta Hotel" }]} onClose={onClose} onStart={onStart} />);
    expect(button("Start the policy").disabled).toBe(true);
    const owner = screen.getByRole("combobox", { name: "Owner (customer)" });
    fireEvent.change(owner, { target: { value: "Beta" } });
    fireEvent.keyDown(owner, { key: "ArrowDown" });
    fireEvent.keyDown(owner, { key: "Enter" });
    await flush();
    fireEvent.click(button("Start the policy"));
    expect(onStart).toHaveBeenCalledWith("cust-b");
    cleanup();
    render(<NewPolicyModal customers={[{ id: "cust-a", name: "Demo Customer A" }]} initial="cust-a" onClose={onClose} onStart={onStart} />);
    fireEvent.click(button("Start the policy"));
    expect(onStart).toHaveBeenLastCalledWith("cust-a");
    fireEvent.click(button("Cancel"));
    expect(onClose).toHaveBeenCalledOnce();
    cleanup();
    render(<NewPolicyModal customers={[]} onClose={onClose} onStart={onStart} />);
    expect(screen.getByText("No active customers")).toBeDefined();
  });
});
