// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
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
const preview = vi.hoisted((): { data: ApiOccurrence[] } => ({ data: [] }));
vi.mock("@ac/web/lib/useOp", () => ({ useOp: () => ({ data: preview.data, loading: false, error: null, source: "api" }) }));

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

  it("tests a schedule with its preview without calling the Core API", () => {
    preview.data = [];
    render(<AutomationsView live={live({ editing: "r-sched" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Test / Simulate" }));
    expect(within(screen.getByRole("dialog")).getByText("No upcoming run in the next weeks.")).toBeTruthy();
    expect(actions.testEvent).not.toHaveBeenCalled();
  });
});
