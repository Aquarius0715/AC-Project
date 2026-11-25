// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { policyRows, testOutcome, type AdminPolicy, type ApiPolicy } from "@ac/web/lib/adminAlerts";
import type { ApiNotification } from "@ac/web/lib/notifications";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

// The HQ default alert policy (DD-A05 items 7 and 9, IR347, IR348): its rules with the selected one's condition, window
// and severity saved for every unit; each customer's switch with an optional reason; the notification preview and the
// test with demo data; the condition and window editors both editors share.
const actions = vi.hoisted(() => {
  type Action = (...args: unknown[]) => Promise<unknown>;
  return { saveDefaultRules: vi.fn<Action>(), switchDefaultRule: vi.fn<Action>(), testPolicy: vi.fn<Action>() };
});
const ui = vi.hoisted(() => ({ toasts: [] as string[] }));
const op = vi.hoisted(() => {
  interface Read { data: unknown; loading: boolean; error: { error: { messageKey: string } } | null }
  const state: { reads: Map<string, Read>; inputs: { operation: string; input: unknown; enabled: boolean }[] } = { reads: new Map(), inputs: [] };
  return state;
});
vi.mock("../app/admin/alerts/actions", () => actions);
vi.mock("@ac/web/lib/useRefreshThen", () => ({ useRefreshThen: () => (then: () => void) => { then(); } }));
vi.mock("@ac/web/lib/useOp", () => ({
  useOp: (operation: string, input: unknown, mock: unknown, map: (d: unknown) => unknown, enabled = true) => {
    op.inputs.push({ enabled, operation, input });
    const read = op.reads.get(operation);
    return enabled && read ? { data: read.data === undefined ? mock : map(read.data), loading: read.loading, error: read.error } : { data: mock, loading: false, error: null };
  },
}));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => (m: string) => { ui.toasts.push(m); } }));
vi.mock("@ac/web/components/TimeSelect", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  TimeSelect: ({ value, onChange, "aria-label": label }: Readonly<{ value: string; onChange: (v: string) => void; "aria-label"?: string }>) => <input aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { DefaultPolicyEditor } from "../app/admin/alerts/_components/default-policy";
import { ConditionFields, WindowEditor } from "../app/admin/alerts/_components/policy-parts";
import { DemoTest, NotificationPreview } from "../app/admin/alerts/_components/policy-checks";

const customers = [{ id: "cust-b", name: "Beta Hotel" }, { id: "cust-a", name: "Demo Customer A" }];
const apiDefault = (over: Partial<ApiPolicy> = {}): ApiPolicy => ({
  id: "pol-default", version: 4, kind: "default_alert", name: "Default", unitIds: [], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 0, customerId: null,
  rules: [
    { ruleKey: "high_temperature", name: "Room too hot", category: "fault", metric: "temperature", operator: "gte", threshold: 32, recoveryThreshold: 30, durationSeconds: 300, severity: "warning", activeWindow: null },
    { ruleKey: "offline", name: "Device offline", category: "connection", metric: "heartbeat_gap", operator: "gte", threshold: 10, recoveryThreshold: 5, durationSeconds: 60, severity: "critical", activeWindow: null },
  ],
  ruleSettings: [{ ruleKey: "high_temperature", customerId: "cust-b", enabled: false, version: 2, reason: "Seasonal", changedByMembershipId: "m-1", updatedAt: "2026-09-10T00:00:00Z" }],
  ...over,
});
const policyOf = (p: ApiPolicy): AdminPolicy => {
  const [row] = policyRows([p], [], [{ id: "m-1", displayName: "HQ Operator", role: "admin" }]);
  if (!row) throw new Error("no policy row");
  return row;
};
const units = [{ id: "u-b", name: "Hotel AC", customerId: "cust-b" }, { id: "u-a", name: "Lobby AC", customerId: "cust-a" }];
const editor = (over: Partial<Parameters<typeof DefaultPolicyEditor>[0]> = {}) => (
  <DefaultPolicyEditor p={policyOf(apiDefault())} customers={customers} units={units} canWrite copyFor={null} onCopy={vi.fn()} demo={false} {...over} />
);
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); })); // lets an async transition end inside act
type Result = { ok: true; decision?: unknown } | ({ ok: false } & ActionFailure);
const refusal: Result = { ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {} };

beforeEach(() => {
  ui.toasts = []; op.reads = new Map(); op.inputs = [];
  for (const fn of Object.values(actions)) fn.mockReset();
});
afterEach(cleanup);

describe("the default policy editor", () => {
  it("edits the selected rule's condition, discards it, and saves every rule for every unit at the policy version", async () => {
    actions.saveDefaultRules.mockResolvedValueOnce(refusal).mockResolvedValueOnce({ ok: true });
    render(editor());
    expect(screen.getByText(/rule 1 of 2/)).toBeDefined();
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    expect(button("Save policy").disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Alert when/), { target: { value: "34" } });
    expect(screen.getByText("1 rule changed · Room too hot")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Alert when/), { target: { value: "34" } });
    fireEvent.click(button("Save policy"));
    await flush();
    expect(ui.toasts).toHaveLength(1); // the refusal
    fireEvent.click(button("Save policy"));
    await flush();
    expect(actions.saveDefaultRules).toHaveBeenLastCalledWith(expect.objectContaining({ id: "pol-default", version: 4, rules: [expect.objectContaining({ ruleKey: "high_temperature", threshold: 34 }), expect.objectContaining({ ruleKey: "offline" })] }));
    expect(ui.toasts.at(-1)).toBe("Default limits saved for every unit");
  });

  it("refuses to save a rule with errors, names the rules to fix, and edits another rule's window and severity", () => {
    render(editor());
    fireEvent.change(screen.getByLabelText(/^Recover/), { target: { value: "40" } }); // recovery past the threshold
    fireEvent.click(button(/Device offline/));
    fireEvent.change(screen.getByLabelText(/^For \(s\)/), { target: { value: "0" } });
    fireEvent.click(button("Save policy"));
    expect(ui.toasts).toStrictEqual(["Fix the validation errors first"]);
    expect(screen.getByText("2 rules to fix · nothing saved: Room too hot, Device offline")).toBeDefined();
    expect(actions.saveDefaultRules).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText(/^For \(s\)/), { target: { value: "60" } });
    expect(screen.getByText("1 rule to fix · nothing saved: Room too hot")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Only on some days and times" }));
    fireEvent.click(button("Info"));
    expect(screen.getByText(/^2 rules to fix|^1 rule to fix/)).toBeDefined();
    expect(screen.getAllByText(/edited/)).toHaveLength(2);
    expect(button("Info").getAttribute("aria-pressed")).toBe("true"); // Device offline is now Info
  });

  it("switches a rule off for one customer with a reason, at the version of that customer's setting", async () => {
    actions.switchDefaultRule.mockResolvedValueOnce(refusal).mockResolvedValue({ ok: true });
    render(editor());
    const rows = screen.getAllByRole("switch");
    expect(rows.map((s) => s.getAttribute("aria-label"))).toStrictEqual(["Room too hot for Beta Hotel", "Room too hot for Demo Customer A"]); // by name
    fireEvent.click(screen.getByRole("switch", { name: "Room too hot for Demo Customer A" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByRole("heading", { name: "Switch “Room too hot” off for Demo Customer A" })).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Reason · optional/), { target: { value: "x".repeat(501) } });
    expect(dialog.getByText("At most 500 characters")).toBeDefined();
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Switch off" }).disabled).toBe(true);
    fireEvent.change(dialog.getByLabelText(/^Reason · optional/), { target: { value: "Closed for renovation" } });
    fireEvent.click(dialog.getByRole("button", { name: "Switch off" }));
    await flush();
    expect(actions.switchDefaultRule).toHaveBeenLastCalledWith("pol-default", "high_temperature", "cust-a", false, "Closed for renovation", 0);
    expect(screen.getByRole("dialog")).toBeDefined(); // refused: still open
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Switch off" }));
    await flush();
    expect(ui.toasts.at(-1)).toBe("Room too hot switched off for Demo Customer A");
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Room too hot for Beta Hotel" })); // off at version 2
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Switch on" }));
    await flush();
    expect(actions.switchDefaultRule).toHaveBeenLastCalledWith("pol-default", "high_temperature", "cust-b", true, "", 2);
    fireEvent.click(screen.getByRole("switch", { name: "Room too hot for Beta Hotel" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Room too hot for Beta Hotel" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("tests the rule on a unit of the scoped customer, and words a rule whose metric or category it does not know", () => {
    op.reads.set("notifications.recipients", { data: { items: [{ id: "m-a", role: "client", displayLabel: "Ana", allowedChannels: ["inApp"] }] }, loading: false, error: null });
    const odd = apiDefault({ rules: [{ ruleKey: "lux", name: "Too dark", category: "comfort", metric: "lux", operator: "lt", threshold: 50, recoveryThreshold: 60, durationSeconds: 60, severity: "normal", activeWindow: null }] });
    const { unmount } = render(editor({ copyFor: { id: "cust-a", name: "Demo Customer A" }, units: [...units, { id: "u-x", name: "Orphan AC", customerId: null }] }));
    expect(screen.getByText("on Lobby AC")).toBeDefined(); // the scoped customer's unit, though Beta's comes first
    expect(screen.getByText("What Ana sees for Lobby AC (notifications.preview · in-app).")).toBeDefined();
    unmount();
    render(editor({ p: policyOf(odd), units: [{ id: "u-x", name: "Orphan AC", customerId: null }] }));
    expect(screen.getByText(/^comfort ·/)).toBeDefined(); // an unknown category by its code
    expect(screen.getByLabelText<HTMLInputElement>(/^Metric/).value).toBe("lux");
    expect(screen.getByText("lux is not a metric a customer policy can watch, so this rule cannot be copied.")).toBeDefined();
    expect(screen.getByText("What Ana sees for Orphan AC (notifications.preview · in-app).")).toBeDefined(); // a unit without a known customer is previewed too
  });

  it("lists 20 customers and finds the others with a search, and copies a rule as a customer's policy", async () => {
    const many = Array.from({ length: 22 }, (_, n) => ({ id: `c-${String(n).padStart(2, "0")}`, name: `Customer ${String(n).padStart(2, "0")}` }));
    const onCopy = vi.fn();
    render(editor({ onCopy, customers: many, copyFor: { id: "c-00", name: "Customer 00" }, units: [] }));
    expect(screen.getAllByRole("switch")).toHaveLength(20);
    fireEvent.click(button(/Copy as a Customer 00 policy/));
    expect(onCopy).toHaveBeenCalledWith("high_temperature");
    expect(screen.getByText("There is no unit to test the rule on.")).toBeDefined();
    const search = screen.getByRole("combobox", { name: /More customers/ });
    fireEvent.change(search, { target: { value: "Customer 21" } });
    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    await flush();
    expect(screen.getAllByRole("switch")).toHaveLength(21);
  });

  it("explains a rule it cannot copy or test, and shows a read-only role nothing to change", () => {
    render(editor({ canWrite: false, units: [{ id: "u-b", name: "Hotel AC", customerId: "cust-b" }] }));
    expect(screen.getByText("You can read the default policy. Changing it needs “Edit alert policies”.")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Save policy" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Copy as/ })).toBeNull();
    expect(screen.getByText("The rule is off for every customer in scope, so it cannot be tested here.")).toBeDefined(); // Beta Hotel switched it off
    fireEvent.click(button(/Device offline/));
    expect(screen.getByText(/is not a metric a customer policy can watch/)).toBeDefined();
    expect(screen.getByText("A synthetic reading cannot be entered for this rule's metric.")).toBeDefined();
  });

  it("tests the saved rule on a unit of a customer who has it on, and works in the demo without the API", async () => {
    actions.testPolicy.mockResolvedValueOnce({ ok: true, decision: { unitId: "u-a", policyId: "pol-default", decision: "selected", reason: null } });
    const { unmount } = render(editor());
    expect(screen.getByText("on Lobby AC")).toBeDefined(); // Beta Hotel has the rule off
    fireEvent.click(button("Evaluate (demo)"));
    await flush();
    expect(actions.testPolicy).toHaveBeenCalledWith("pol-default", "u-a", "temperature", 32);
    expect(screen.getAllByRole("status").find((el) => el.tagName === "P")?.textContent).not.toBe(""); // the test's outcome line
    unmount();
    render(editor({ demo: true }));
    fireEvent.change(screen.getByLabelText(/^Alert when/), { target: { value: "35" } });
    expect(screen.getByText("Unsaved changes are not tested: the saved version is.")).toBeDefined();
    fireEvent.click(button("Save policy"));
    fireEvent.click(screen.getByRole("switch", { name: "Room too hot for Demo Customer A" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Switch off" }));
    expect(ui.toasts).toStrictEqual(["Default limits saved for every unit", "Room too hot switched off for Demo Customer A"]);
    expect(actions.saveDefaultRules).not.toHaveBeenCalled();
  });

  it("follows a newer version of the policy, and says when it has no rules", () => {
    const { rerender } = render(editor());
    fireEvent.change(screen.getByLabelText(/^Alert when/), { target: { value: "34" } });
    rerender(editor({ p: policyOf(apiDefault({ version: 5 })) }));
    expect(screen.getByText("No unsaved changes")).toBeDefined(); // the new version's rules replace the edit
    rerender(editor({ p: policyOf(apiDefault({ version: 6, rules: [] })) }));
    expect(screen.getByText("The default policy has no rules.")).toBeDefined();
  });
});

const recipients = [{ id: "m-a", label: "Ana", role: "Customer" }, { id: "m-b", label: "Ben", role: "Customer" }];
const notification: ApiNotification = {
  id: "preview", version: 1, type: "fault", sourceAlertId: null, target: { kind: "unit", id: "u-a" }, templateKey: "alert",
  params: { targetName: "Lobby AC", at: "2026-09-14T01:00:00Z", status: "open", reason: null, message: null }, channel: "inApp", occurredAt: "2026-09-14T01:00:00Z", readAt: null, severity: "critical",
};

describe("the notification preview", () => {
  it("previews what the chosen recipient is shown for the unit", () => {
    op.reads.set("notifications.preview", { data: notification, loading: false, error: null });
    render(<NotificationPreview recipients={recipients} severity="critical" customerName="Demo Customer A" unit={{ id: "u-a", name: "Lobby AC" }} demo={false} />);
    expect(screen.getByText("What Ana sees for Lobby AC (notifications.preview · in-app).")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Preview as"), { target: { value: "m-b" } });
    expect(op.inputs.at(-1)?.input).toMatchObject({ recipientMembershipId: "m-b", severity: "critical", target: { kind: "unit", id: "u-a" } });
    expect(screen.getByText("What Ben sees for Lobby AC (notifications.preview · in-app).")).toBeDefined();
  });

  it("says it is loading, that a recipient may not see the unit, or that the preview failed", () => {
    const one = [{ id: "m-a", label: "Ana", role: "Customer" }];
    const unit = { id: "u-a", name: "Lobby AC" };
    for (const [read, text] of [
      [{ data: null, loading: true, error: null }, "Loading the preview…"],
      [{ data: null, loading: false, error: { error: { messageKey: "errors.recipient_ineligible" } } }, "Ana cannot get this notification: they may not see Lobby AC."],
      [{ data: null, loading: false, error: { error: { messageKey: "error.unavailable" } } }, "The preview could not be loaded — try again."],
    ] as const) {
      op.reads.set("notifications.preview", read);
      render(<NotificationPreview recipients={one} severity="warning" customerName="Demo Customer A" unit={unit} demo={false} />);
      expect(screen.getByText(text)).toBeDefined();
      expect(screen.queryByLabelText("Preview as")).toBeNull(); // one recipient: nothing to choose
      cleanup();
    }
  });

  it("asks for a recipient or a unit when there is none, and builds the demo's row itself", () => {
    const { rerender } = render(<NotificationPreview recipients={[]} severity="warning" customerName="Demo Customer A" unit={{ id: "u-a", name: "Lobby AC" }} demo={false} />);
    expect(screen.getByText("Add a recipient to see what they are shown.")).toBeDefined();
    rerender(<NotificationPreview recipients={recipients} severity="warning" customerName="Demo Customer A" unit={null} demo={false} />);
    expect(screen.getByText("The preview needs a unit of Demo Customer A, and it has none yet.")).toBeDefined();
    rerender(<NotificationPreview recipients={recipients} severity="warning" customerName="Demo Customer A" unit={{ id: "u-a", name: "Lobby AC" }} demo />);
    expect(screen.getByText(/Open unit/)).toBeDefined(); // the fixture row is shown without the Core API
  });
});

describe("the demo test", () => {
  const saved = { id: "pol-1", version: 2, metric: "temperature", operator: "gte" as const, threshold: 30, on: true };
  const unit = { id: "u-a", name: "Lobby AC" };

  it("asks for a reading, evaluates every comparison in the demo, and starts again for a newer version", () => {
    const { rerender } = render(<DemoTest saved={saved} dirty={false} unit={unit} demo />);
    fireEvent.change(screen.getByLabelText("Reading"), { target: { value: " " } });
    fireEvent.click(button("Evaluate (demo)"));
    expect(screen.getByRole("status").textContent).toBe("Enter a reading first.");
    // 30 meets ≥ 30 and ≤ 30, not > 30 or < 30
    const outcomes: string[] = [];
    for (const operator of ["gte", "gt", "lte", "lt"] as const) {
      rerender(<DemoTest saved={{ ...saved, operator }} dirty={false} unit={unit} demo />);
      fireEvent.change(screen.getByLabelText("Reading"), { target: { value: "30" } });
      fireEvent.click(button("Evaluate (demo)"));
      outcomes.push(screen.getByRole("status").textContent);
    }
    const expected = (["gte", "gt", "lte", "lt"] as const).map((operator) => {
      const isMet = operator === "gte" || operator === "lte";
      return testOutcome({ unitId: "u-a", policyId: "pol-1", decision: isMet ? "selected" : "suppressed", reason: isMet ? null : "not_due" }, { ...saved, operator }, 30, "Lobby AC").text;
    });
    expect(outcomes).toStrictEqual(expected);
    rerender(<DemoTest saved={{ ...saved, on: false }} dirty={false} unit={unit} demo />);
    fireEvent.click(button("Evaluate (demo)"));
    expect(screen.getByRole("status")).toBeDefined();
    rerender(<DemoTest saved={{ ...saved, version: 3 }} dirty={false} unit={unit} demo />);
    expect(screen.queryByRole("status")).toBeNull(); // a newer version starts again
    expect(screen.getByLabelText<HTMLInputElement>("Reading").value).toBe("30");
    rerender(<DemoTest saved={{ ...saved, version: 4, metric: "lux" }} dirty={false} unit={unit} demo />);
    expect(screen.getByText("Reading of lux")).toBeDefined(); // a metric without a label or unit by its code
  });

  it("sends the reading to the Core API, says when it is refused, and needs a saved policy on a unit", async () => {
    actions.testPolicy.mockResolvedValueOnce(refusal);
    const { rerender } = render(<DemoTest saved={saved} dirty={false} unit={unit} demo={false} />);
    fireEvent.change(screen.getByLabelText("Reading"), { target: { value: "31.5" } });
    fireEvent.click(button("Evaluate (demo)"));
    await flush();
    expect(actions.testPolicy).toHaveBeenCalledWith("pol-1", "u-a", "temperature", 31.5);
    expect(ui.toasts).toHaveLength(1);
    expect(screen.queryByRole("status")).toBeNull();
    rerender(<DemoTest saved={saved} dirty={false} unit={null} demo={false} />);
    expect(screen.getByText("Attach the policy to a unit first: the test runs on one of its units.")).toBeDefined();
    rerender(<DemoTest saved={{ ...saved, id: "" }} dirty={false} unit={unit} demo={false} />);
    expect(screen.getByText("Save the policy first: the test evaluates the saved version.")).toBeDefined();
  });
});

describe("the condition and window editors", () => {
  it("picks any HQ metric and comparison, and says whether the recovery is on the safe side", () => {
    const onChange = vi.fn();
    const { rerender } = render(<ConditionFields c={{ metric: "temperature", operator: "gte", threshold: 30, recovery: 28, duration: 30 }} onChange={onChange} errors={{}} tried={false} />);
    fireEvent.change(screen.getByLabelText("Metric"), { target: { value: "co2" } });
    expect(onChange).toHaveBeenLastCalledWith({ metric: "co2" });
    fireEvent.change(screen.getByLabelText("Comparison"), { target: { value: "lt" } });
    expect(onChange).toHaveBeenLastCalledWith({ operator: "lt" });
    fireEvent.change(screen.getByLabelText(/^Recover/), { target: { value: "27" } });
    expect(onChange).toHaveBeenLastCalledWith({ recovery: 27 });
    expect(screen.getByText("Recovery 28 is on the safe side of the 30 threshold (≥)")).toBeDefined();
    rerender(<ConditionFields c={{ metric: "lux", operator: "lt", threshold: 10, recovery: 12, duration: 30 }} onChange={onChange} errors={{}} tried={false} fixedMetric />);
    expect(screen.getByLabelText<HTMLInputElement>(/^Metric/).value).toBe("lux"); // no label, no unit hint
    rerender(<ConditionFields c={{ metric: "airflow_drop", operator: "lte", threshold: 10, recovery: 12, duration: 120 }} onChange={onChange} errors={{ recovery: "Recovery must be above 10" }} tried fixedMetric />);
    expect(screen.getAllByText("Recovery must be above 10")).toHaveLength(2); // on the field and in the safety line
    expect(screen.getByLabelText<HTMLInputElement>(/^Metric/).readOnly).toBe(true);
    expect(screen.getByText("= 2 min")).toBeDefined();
  });

  it("limits a window to some days and hours, with shortcuts, and clears it", () => {
    const onChange = vi.fn();
    const window = { weekdays: [1, 3], startLocal: "08:00", endLocal: "19:00" };
    const { rerender } = render(<WindowEditor value={null} onChange={onChange} timezone="Asia/Kuala_Lumpur" />);
    expect(screen.getByText("Any day · any time")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Only on some days and times" }));
    expect(onChange).toHaveBeenLastCalledWith({ weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" });
    rerender(<WindowEditor value={window} onChange={onChange} timezone="Asia/Kuala_Lumpur" />);
    fireEvent.click(button("Every day"));
    expect(onChange).toHaveBeenLastCalledWith({ ...window, weekdays: [1, 2, 3, 4, 5, 6, 7] });
    fireEvent.click(button("Mon–Fri"));
    expect(onChange).toHaveBeenLastCalledWith({ ...window, weekdays: [1, 2, 3, 4, 5] });
    fireEvent.click(button("Tue"));
    expect(onChange).toHaveBeenLastCalledWith({ ...window, weekdays: [1, 2, 3] }); // added in order
    fireEvent.click(button("Mon"));
    expect(onChange).toHaveBeenLastCalledWith({ ...window, weekdays: [3] });
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "09:00" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...window, startLocal: "09:00" });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "18:00" } });
    expect(onChange).toHaveBeenLastCalledWith({ ...window, endLocal: "18:00" });
    fireEvent.click(screen.getByRole("checkbox", { name: "Only on some days and times" }));
    expect(onChange).toHaveBeenLastCalledWith(null);
    rerender(<WindowEditor value={window} onChange={onChange} timezone="Asia/Kuala_Lumpur" error="End after start" />);
    expect(screen.getByText("End after start")).toBeDefined();
  });
});
