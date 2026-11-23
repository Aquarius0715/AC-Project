// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { consentCard, ruleCard, type ApiAutomation, type ApiOccurrence, type Caps } from "@ac/web/lib/clientAutomations";

// The customer Automations page in API mode (FR-C04, FR-C05): the list's switch, delete and consent send the version
// shown, the editor saves at the rule's version and shows the Core API's field refusals, refuses locally before it
// sends, and its test runs the saved rules (an event) or the preview (a schedule); a rule deleted meanwhile says so
type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  saveAutomation: vi.fn<(input: unknown, version: number | null) => Promise<Result<unknown>>>(),
  deleteAutomation: vi.fn<(id: string, version: number) => Promise<Result<unknown>>>(),
  updateConsent: vi.fn<(granted: boolean, version: number) => Promise<Result<unknown>>>(),
  testEvent: vi.fn<(unitId: string, facts: unknown) => Promise<Result<unknown>>>(),
  approveStartProposal: vi.fn(),
}));
vi.mock("../app/customer/automations/actions", () => actions);
vi.mock("../app/customer/automations/_components/presence-switch", () => ({ PresenceSwitch: () => <p>presence switch</p> }));
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<Result<T>>, ok: string, after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(ok); after?.(r.value); } else failed?.(r);
    });
  }] as const,
}));
const patch = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
const preview = vi.hoisted((): { data: ApiOccurrence[]; args: unknown[] } => ({ data: [], args: [] }));
vi.mock("@ac/web/lib/useOp", () => ({ useOp: (_op: string, args: unknown) => { preview.args.push(args); return { data: preview.data, loading: false, error: null, source: "api" }; } }));
/** The schedule draft the latest preview asked for. */
const opDraft = (): unknown => {
  const last = preview.args.at(-1);
  return typeof last === "object" && last !== null && "draft" in last ? last.draft : undefined;
};

import { AutomationsView, type AutomationsLive } from "../app/customer/automations/_components/automations-view";

const UNIT = "u-1";
const base = { createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", ownerMembershipId: "m-1", timezone: "Asia/Kuala_Lumpur", priority: 0, disabledReason: null, onlyIf: null, lastRun: null } as const;
const schedule: ApiAutomation = {
  ...base, id: "r-sched", version: 3, name: "Weekday pre-cool", unitIds: [UNIT], enabled: true, kind: "schedule", weekdays: [3], startLocal: "18:00", endLocal: "22:00", endsNextDay: false,
  startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false },
};
const arrival: ApiAutomation = { ...base, id: "r-loc", version: 5, name: "Arrive home", unitIds: [UNIT], enabled: true, kind: "event", condition: { type: "location", event: "arrival" }, action: { kind: "set_power", power: true } };
const ac = { id: UNIT, name: "Bedroom AC", path: "Home", property: "Home" };
const caps: Caps = { control: true, modeControl: true, fanControl: true, temperature: { min: 16, max: 30, step: 1 }, modes: ["cool"], fanLevels: ["low"], ventilation: false, ventilationLevels: [] };
const live = (over: Partial<AutomationsLive> = {}): AutomationsLive => {
  const rules = over.rules ?? [schedule, arrival];
  const consent = { id: "c-1", version: 9, granted: true, grantedAt: "2026-09-10T00:00:00Z", revokedAt: null };
  return {
    proposals: [], rules, acs: [ac], caps: { [UNIT]: caps }, timezone: "Asia/Kuala_Lumpur", editing: null, nextRuns: {}, preset: [], places: {},
    cards: rules.map((r) => ruleCard(r, ac, null, true)), consent: { version: 9, ...consentCard(consent, 1, 0) }, ...over,
  };
};
const ok = <T,>(value: T): Result<T> => ({ ok: true, value });

beforeEach(() => {
  runs.toasts = [];
  preview.data = [];
  preview.args = [];
  patch.mockReset();
  actions.saveAutomation.mockReset().mockResolvedValue(ok(schedule));
  actions.deleteAutomation.mockReset().mockResolvedValue(ok({ id: "r-sched", deleted: true }));
  actions.updateConsent.mockReset().mockResolvedValue(ok(null));
  actions.testEvent.mockReset().mockResolvedValue(ok({ at: "2026-09-15T10:42:00.000Z", decision: { unitId: UNIT, decision: "selected", ruleId: "r-loc", reason: null } }));
});
afterEach(() => { cleanup(); });

describe("AutomationsView list", () => {
  it("switches a rule off at its version, keeping the rest of the rule", async () => {
    render(<AutomationsView live={live()} />);
    expect(screen.getByText("2 automations · 2 on · checked again when it runs")).toBeTruthy();
    fireEvent.click(screen.getByRole("switch", { name: "Switch Weekday pre-cool off" }));
    await vi.waitFor(() => { expect(runs.toasts).toStrictEqual(["“Weekday pre-cool” switched off"]); });
    const [input, version] = actions.saveAutomation.mock.calls[0] ?? [];
    expect(version).toBe(3);
    expect(input).toMatchObject({ id: "r-sched", enabled: false, kind: "schedule", weekdays: [3], startLocal: "18:00", endLocal: "22:00", unitIds: [UNIT] });
  });

  it("withdraws consent at its version, and offers no consent button without a record", async () => {
    render(<AutomationsView live={live()} />);
    expect(screen.getByText("presence switch")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Withdraw consent" }));
    await vi.waitFor(() => { expect(actions.updateConsent.mock.calls).toStrictEqual([[false, 9]]); });
    cleanup();
    render(<AutomationsView live={live({ consent: { version: null, ...consentCard(null, 0, 0) } })} />);
    expect(screen.queryByRole("button", { name: /consent/ })).toBeNull();
  });

  it("with no rule the empty state creates one, and Edit opens a rule in the URL", () => {
    render(<AutomationsView live={live({ rules: [] })} />);
    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    expect(patch).toHaveBeenLastCalledWith({ automationId: "new", preset: null, unitIds: null });
    cleanup();
    render(<AutomationsView live={live()} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Edit" })[0] ?? document.body);
    expect(patch).toHaveBeenLastCalledWith({ automationId: "r-sched" });
  });

  it("after a withdrawal the consent card warns, a rule turned off by it says Disabled, one switched off says Off", () => {
    const withdrawn = { id: "c-1", version: 9, granted: false, grantedAt: null, revokedAt: "2026-09-12T00:00:00Z" };
    const off = { ...schedule, enabled: false };
    const turnedOff = { ...arrival, enabled: false, disabledReason: "consent_revoked" as const };
    render(<AutomationsView live={live({ rules: [off, turnedOff], cards: [ruleCard(off, ac, null, false), ruleCard(turnedOff, ac, null, false)], consent: { version: 9, ...consentCard(withdrawn, 1, 1) } })} />);
    expect(screen.getByText(/^Withdrawn .+ 1 location automation was turned off/).closest(".rounded-2xl")?.className).toContain("border-warn");
    expect(screen.getByRole("button", { name: "Grant consent" }).className).toContain("bg-primary "); // the remedy is primary (IR619)
    const stateOf = (name: string) => within(screen.getByRole("switch", { name }).closest(".text-xs") ?? document.body);
    expect(stateOf("Switch Weekday pre-cool on").getByText("Off").className).toContain("text-muted-foreground");
    expect(stateOf("Switch Arrive home on").getByText("Disabled").className).toContain("text-warn");
    expect(screen.queryByText("presence switch")).toBeNull(); // no presence switch without consent
  });

  it("cancelling the delete dialog keeps the rule and sends nothing", async () => {
    render(<AutomationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "More for Arrive home" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
    expect(within(screen.getByRole("dialog")).getByText(/Location · Arrival at Home\./)).toBeTruthy();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "More for Arrive home" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    await vi.waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.deleteAutomation).not.toHaveBeenCalled();
  });

  it("deletes a rule at its version from its menu and says what still runs", async () => {
    render(<AutomationsView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "More for Weekday pre-cool" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("“Weekday pre-cool” will no longer run.")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete automation" }));
    await vi.waitFor(() => { expect(actions.deleteAutomation.mock.calls).toStrictEqual([["r-sched", 3]]); });
    expect(await screen.findByText("Automation deleted")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(runs.toasts).toStrictEqual(["“Weekday pre-cool” deleted"]);
  });
});

describe("AutomationsView editor", () => {
  it("says a rule deleted meanwhile no longer exists, and goes back to the list", () => {
    render(<AutomationsView live={live({ editing: "r-gone" })} />);
    expect(screen.getByText("This automation no longer exists")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Automations" }));
    expect(patch).toHaveBeenLastCalledWith({ automationId: null, preset: null, unitIds: null });
  });

  it("refuses a new rule locally until it has a name and an AC, sending nothing", () => {
    render(<AutomationsView live={live({ editing: "new", acs: [ac, { ...ac, id: "u-2", name: "Study AC" }] })} />);
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    expect(screen.getByText("1–120 characters")).toBeTruthy();
    expect(screen.getByText("Choose the AC")).toBeTruthy();
    expect(actions.saveAutomation).not.toHaveBeenCalled();
  });

  it("saves an edited schedule at its version and returns to the list with the next run", async () => {
    preview.data = [{ automationId: null, phase: "schedule_start", at: "2026-09-16T10:00:00Z", action: { kind: "set_power", power: true } }];
    const view = render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Evening pre-cool" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(patch).toHaveBeenLastCalledWith({ automationId: null, preset: null, unitIds: null }); });
    const [input, version] = actions.saveAutomation.mock.calls[0] ?? [];
    expect(version).toBe(3);
    expect(input).toMatchObject({ id: "r-sched", name: "Evening pre-cool" });
    view.rerender(<AutomationsView live={live()} />); // the URL left the editor: the list shows the saved notice
    expect(screen.getByText(/^“Weekday pre-cool” saved · next run .+ \(Asia\/Kuala_Lumpur\)\. Saving sends no command now\.$/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Automation saved")).toBeNull();
  });

  it("shows the Core API's field refusal under its field and stays in the editor", async () => {
    actions.saveAutomation.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { name: "duplicate" } });
    render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(await screen.findByText("duplicate")).toBeTruthy();
    expect(patch).not.toHaveBeenCalled();
  });

  it("tests a saved event rule with the saved rules and shows the result, sending no command", async () => {
    render(<AutomationsView live={live({ editing: "r-loc" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Test result — simulation only")).toBeTruthy();
    expect(actions.testEvent.mock.calls[0]?.[0]).toBe(UNIT);
    expect(runs.toasts).toStrictEqual(["Simulated — no command was sent"]);
  });

  it("applies a rule to more ACs: a property's, one more and Clear, saved after the primary", async () => {
    const study = { id: "u-2", name: "Study AC", path: "Home", property: "Home" };
    const office = { id: "u-3", name: "Office AC", path: "Office", property: "Office" };
    render(<AutomationsView live={live({ editing: "r-loc", acs: [ac, study, office], caps: { [UNIT]: caps, "u-2": caps, "u-3": caps } })} />);
    expect(screen.getByText("Also apply to other ACs")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Clear" })).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("button", { name: "All ACs in Home (2)" }));
    expect(screen.getByText("Also apply to 1 other AC")).toBeTruthy();
    expect(screen.getByRole("button", { name: "All ACs in Home (2)" })).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("checkbox", { name: "Office AC · Office" }));
    expect(screen.getByText("Also apply to 2 other ACs")).toBeTruthy();
    expect(screen.getByText("Action for 3 ACs")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Office AC · Office" }));
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.getByText("Also apply to other ACs")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Study AC · Home" }));
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ unitIds: [UNIT, "u-2"] });
  });

  it("refuses an action another chosen AC cannot take, sending nothing", () => {
    const fan = { id: "u-3", name: "Fan unit", path: "Office", property: "Office" };
    render(<AutomationsView live={live({ editing: "r-loc", acs: [ac, fan], caps: { [UNIT]: caps, "u-3": { ...caps, control: false } } })} />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan unit · Office" }));
    expect(within(screen.getByRole("combobox", { name: "Action — setting" })).getByRole("option", { name: "Power ON (not supported)" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    expect(screen.getByText("Not supported by every chosen AC — choose another action")).toBeTruthy();
    expect(actions.saveAutomation).not.toHaveBeenCalled();
  });

  it("changes the action: a temperature starts at 24 °C and takes another value; power by its choice", async () => {
    render(<AutomationsView live={live({ editing: "r-loc" })} />);
    const setting = screen.getByRole("combobox", { name: "Action — setting" });
    fireEvent.change(setting, { target: { value: "temp" } });
    const value = screen.getByRole("combobox", { name: "Action — value" });
    expect(value).toHaveProperty("value", "temp:24");
    fireEvent.change(value, { target: { value: "temp:26" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ action: { kind: "set_temperature", celsius: 26 } });
    fireEvent.change(setting, { target: { value: "power" } });
    fireEvent.click(within(screen.getByRole("group", { name: "Action — value" })).getByRole("button", { name: "OFF" }));
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledTimes(2); });
    expect(actions.saveAutomation.mock.calls[1]?.[0]).toMatchObject({ action: { kind: "set_power", power: false } });
  });

  it("adds an outdoor temperature condition, refuses a value out of range under it, and removes it", async () => {
    render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Add condition" }));
    expect(screen.queryByRole("button", { name: "Weekdays" })).toBeNull(); // a schedule has its own days
    expect(screen.getByRole("button", { name: "Occupancy Coming soon" })).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("button", { name: "Outdoor temperature" }));
    const celsius = screen.getByRole("textbox", { name: "Outdoor temperature °C" });
    fireEvent.change(celsius, { target: { value: "200" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    expect(screen.getByText("−50 to 100 °C")).toBeTruthy();
    expect(actions.saveAutomation).not.toHaveBeenCalled();
    fireEvent.change(celsius, { target: { value: "31" } });
    fireEvent.change(screen.getByDisplayValue("≥"), { target: { value: "lte" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ onlyIf: [{ type: "weather", metric: "temperature", operator: "lte", value: 31 }] });
    fireEvent.click(screen.getByRole("button", { name: "Remove Outdoor temperature" }));
    expect(screen.queryByRole("textbox", { name: "Outdoor temperature °C" })).toBeNull();
  });

  it("on a presence rule adds weekdays and outdoor temperature, then has no more types, and saves the days chosen", async () => {
    const presence: ApiAutomation = { ...base, id: "r-pre", version: 2, name: "Room empty stop", unitIds: [UNIT], enabled: true, kind: "event", condition: { type: "occupancy", occupied: false }, action: { kind: "set_power", power: false } };
    render(<AutomationsView live={live({ editing: "r-pre", rules: [presence] })} />);
    expect(screen.getByRole("combobox", { name: "Room" })).toHaveProperty("value", "empty");
    fireEvent.click(screen.getByRole("button", { name: "Add condition" }));
    expect(screen.queryByRole("button", { name: /^Occupancy/ })).toBeNull(); // the trigger's own type
    fireEvent.click(screen.getByRole("button", { name: "Weekdays" }));
    fireEvent.click(screen.getByRole("button", { name: "Add condition" }));
    fireEvent.click(screen.getByRole("button", { name: "Outdoor temperature" }));
    expect(screen.getByRole("button", { name: "Add condition" })).toHaveProperty("disabled", true);
    expect(screen.getByText("No more condition types for this trigger")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Mon" }));
    fireEvent.click(screen.getByRole("button", { name: "Sun" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Room" }), { target: { value: "occupied" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({
      condition: { type: "occupancy", occupied: true },
      onlyIf: [{ type: "weekday", weekdays: [2, 3, 4, 5, 7] }, { type: "weather", metric: "temperature", operator: "gte", value: 30 }],
    });
  });

  it("checks a schedule's days and overnight end before it saves them", async () => {
    render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Wed" }));
    fireEvent.change(screen.getAllByRole("combobox", { name: "Hour" })[1] ?? document.body, { target: { value: "06" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    expect(screen.getByText("Choose at least one weekday")).toBeTruthy();
    expect(screen.getByText("The end is before the start — tick “Ends next day” for an overnight schedule")).toBeTruthy();
    expect(screen.getByText("Fix the schedule to preview")).toBeTruthy();
    expect(actions.saveAutomation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Mon" }));
    fireEvent.click(screen.getByRole("checkbox", { name: /^Ends next day/ }));
    expect(screen.getByText("06:00 next day")).toBeTruthy();
    expect(screen.getByText("None in the next weeks")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ weekdays: [1], startLocal: "18:00", endLocal: "06:00", endsNextDay: true });
  });

  it("in a location rule grants the missing consent at its version and saves the event chosen", async () => {
    const withdrawn = { id: "c-1", version: 9, granted: false, grantedAt: null, revokedAt: "2026-09-12T00:00:00Z" };
    render(<AutomationsView live={live({ editing: "r-loc", consent: { version: 9, ...consentCard(withdrawn, 1, 0) } })} />);
    expect(screen.getByRole("textbox", { name: /^Place/ })).toHaveProperty("value", "Home");
    fireEvent.click(screen.getByRole("button", { name: "Grant" }));
    await vi.waitFor(() => { expect(actions.updateConsent.mock.calls).toStrictEqual([[true, 9]]); });
    expect(runs.toasts).toStrictEqual(["Location consent granted"]);
    fireEvent.change(screen.getByRole("combobox", { name: "Event" }), { target: { value: "departure" } });
    expect(screen.getByText("Save first — the test runs your saved automations for this AC")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Test / Simulate" })).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ condition: { type: "location", event: "departure" } });
  });

  it("a new routine rule says why it has no test, and saves the usual time picked", async () => {
    render(<AutomationsView live={live({ editing: "new" })} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Evening routine" } });
    fireEvent.click(screen.getByRole("radio", { name: /^Routine/ }));
    expect(screen.getByText("A routine runs at its usual time — try it with Demo controls")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Test / Simulate" })).toHaveProperty("disabled", true);
    const usual = within(screen.getByRole("group", { name: /^Your usual time/ }));
    fireEvent.change(usual.getByRole("combobox", { name: "Hour" }), { target: { value: "19" } });
    fireEvent.change(usual.getByRole("combobox", { name: "Minute" }), { target: { value: "30" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]).toMatchObject([{ kind: "event", condition: { type: "pattern", localTime: "19:30" }, unitIds: [UNIT] }, null]);
  });

  it("a new weather rule saves the comparison and the temperature typed", async () => {
    render(<AutomationsView live={live({ editing: "new" })} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Cold night" } });
    fireEvent.click(screen.getByRole("radio", { name: /^Weather/ }));
    fireEvent.change(screen.getByRole("combobox", { name: "Outdoor temperature" }), { target: { value: "lt" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^°C/ }), { target: { value: "18.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ condition: { type: "weather", metric: "temperature", operator: "lt", value: 18.5 } });
  });

  it("saves a rule for the AC chosen, switched off, and says it will not run until switched on", async () => {
    const study = { id: "u-2", name: "Study AC", path: "Home", property: "Home" };
    actions.saveAutomation.mockResolvedValue(ok({ ...schedule, enabled: false }));
    const view = render(<AutomationsView live={live({ editing: "r-sched", acs: [ac, study], caps: { [UNIT]: caps, "u-2": caps } })} />);
    fireEvent.change(screen.getByRole("combobox", { name: "Target AC" }), { target: { value: "u-2" } });
    fireEvent.click(screen.getByRole("switch", { name: "Enabled on save" }));
    expect(screen.getByText("Off on save")).toBeTruthy();
    expect(screen.getByText("Only after you turn it on — it is saved off")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(patch).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ enabled: false, unitIds: ["u-2"] });
    view.rerender(<AutomationsView live={live()} />);
    expect(screen.getByText("“Weekday pre-cool” saved · off — switch it on to run it. Saving sends no command now.")).toBeTruthy();
  });

  it("saves from the test result, and Back to edit closes it without saving", async () => {
    render(<AutomationsView live={live({ editing: "r-loc" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Back to edit" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    fireEvent.keyDown(await screen.findByRole("dialog"), { key: "Escape" });
    await vi.waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.saveAutomation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation.mock.calls).toHaveLength(1); });
    expect(actions.saveAutomation.mock.calls[0]?.[1]).toBe(5);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("the schedule preview reads the draft only after typing pauses", () => {
    vi.useFakeTimers();
    try {
      render(<AutomationsView live={live({ editing: "r-sched" })} />);
      const start = within(screen.getByRole("group", { name: /^Start$/ }));
      fireEvent.change(start.getByRole("combobox", { name: "Hour" }), { target: { value: "07" } });
      expect(opDraft()).toMatchObject({ startLocal: "18:00" }); // not yet
      act(() => { vi.advanceTimersByTime(400); });
      expect(opDraft()).toMatchObject({ startLocal: "07:00" });
    } finally {
      vi.useRealTimers();
    }
  });

  it("a saved rule's occupancy condition changes from its select", async () => {
    render(<AutomationsView live={live({ editing: "r-loc", rules: [schedule, { ...arrival, onlyIf: [{ type: "occupancy", occupied: true }] }] })} />);
    fireEvent.change(screen.getByDisplayValue("Someone is home"), { target: { value: "no" } });
    expect(screen.getByDisplayValue("No one is home")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));
    await vi.waitFor(() => { expect(actions.saveAutomation).toHaveBeenCalledOnce(); });
    expect(actions.saveAutomation.mock.calls[0]?.[0]).toMatchObject({ onlyIf: [{ type: "occupancy", occupied: false }] });
  });

  it("tests a schedule with its preview without calling the Core API", () => {
    preview.data = [];
    render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    expect(within(screen.getByRole("dialog")).getByText("No upcoming run in the next weeks.")).toBeTruthy();
    expect(actions.testEvent).not.toHaveBeenCalled();
  });
});
