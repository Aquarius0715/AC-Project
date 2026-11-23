// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ApiCommand, ApiUnitDetail, UnitAction } from "@ac/web/lib/units";
import type { Followed } from "@ac/web/lib/unitCommands";
import { OpError } from "@ac/web/lib/ops";

// The customer unit page (FR-C03, FR-C15) in API mode: only the settings the user changed and the AC does not already
// have are sent; a newer report resets the controls; the power switch asks first; offline, HQ's check and a power-off
// restriction lock the controls; Undo is offered only once every command was acknowledged; the Attach dialog sends
// only policies still listed. The Phase 1A demo sends one temperature change and acknowledges it.
type Result<T> = { ok: true; value: T } | { ok: false; messageKey: string; code: string };
const actions = vi.hoisted(() => ({
  createCommand: vi.fn<(unitId: string, action: UnitAction) => Promise<Result<ApiCommand>>>(),
  setUnitAlertPolicies: vi.fn<(unitId: string, ids: string[]) => Promise<Result<null>>>(),
}));
vi.mock("../app/customer/units/[id]/actions", () => actions);
const commands = vi.hoisted(() => ({ followCommand: vi.fn<(first: ApiCommand, onUpdate: (c: ApiCommand) => void) => Promise<Followed>>() }));
vi.mock("@ac/web/lib/unitCommands", () => commands);
const router = vi.hoisted(() => ({ refresh: vi.fn(), replace: vi.fn(), params: new URLSearchParams() }));
vi.mock("next/navigation", () => ({ useRouter: () => router, useSearchParams: () => router.params }));
vi.mock("../app/customer/_lib/pins", () => ({ togglePinned: vi.fn() }));
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));
vi.mock("../app/customer/_components/rename-modal", () => ({ RenameModal: () => null }));

import { UnitView } from "../app/customer/units/[id]/_components/unit-view";

const detail = (over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id: "u-a", displayName: "Bedroom AC", version: 4, spaceId: "s-bed", alertPolicyIds: ["p-hot"], connection: "online", effectivePowerState: "on",
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: "2026-09-15T01:00:00Z" }, lastSeenAt: "2026-09-15T01:10:00Z", latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKF", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool", "dry", "fan"], fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [] },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: ["Home", "1F", "Bedroom"], address: null, accessInstructions: null }, installedAt: null, installState: "installed", modelId: "cap-1", capabilityVersion: 1, serviceScope: [], components: [],
  ...over,
});
const POLICIES = [{ id: "p-hot", name: "Too hot", rule: "Room ≥ 30 °C" }, { id: "p-co2", name: "Stuffy", rule: "CO₂ ≥ 1200 ppm" }];
const command = (action: UnitAction): ApiCommand => ({ action, id: "cmd-1", status: "requested", failureCode: null, requestedAt: "2026-09-15T01:12:00Z" });
const flush = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });
const view = (over: Partial<ApiUnitDetail> = {}, restrictionHref: string | null = null) => <UnitView id="u-a" detail={detail(over)} room={[]} policies={POLICIES} nowMs={Date.parse("2026-09-15T01:12:30Z")} restrictionHref={restrictionHref} />;
const reviewButton = () => screen.getByRole<HTMLButtonElement>("button", { name: "Review & send" });
const reviewText = () => reviewButton().parentElement?.textContent ?? "";
const press = (name: string) => { fireEvent.click(screen.getByRole("button", { name })); };
const sendReviewed = async () => {
  fireEvent.click(reviewButton());
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to Bedroom AC" }));
  await flush();
};
const powerLocked = () => screen.getByRole("switch", { name: "Power" }).dataset.disabled !== undefined;
const sent = () => actions.createCommand.mock.calls.map(([, a]) => a);

beforeEach(() => {
  toast.mockReset();
  router.refresh.mockReset();
  router.replace.mockReset();
  router.params = new URLSearchParams();
  actions.createCommand.mockReset().mockImplementation((_, a) => Promise.resolve({ ok: true, value: command(a) }));
  actions.setUnitAlertPolicies.mockReset().mockResolvedValue({ ok: true, value: null });
  commands.followCommand.mockReset().mockImplementation((first) => Promise.resolve({ answered: true, command: { ...first, status: "acknowledged" } }));
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("UnitView in API mode: what is sent", () => {
  it("plans only the edited settings that differ from the report, and sends them in order", async () => {
    render(view());
    expect(reviewText()).toContain("No change yet");
    expect(reviewButton().disabled).toBe(true);
    press("Raise the temperature");
    expect(reviewText()).toContain("Set temperature 24°C → 25°C");
    // back to the reported value: nothing left to send
    press("Lower the temperature");
    expect(reviewButton().disabled).toBe(true);
    press("Dry");
    press("High");
    expect(reviewText()).toContain("2 changes:");
    expect(reviewText()).toContain("Mode → Dry · Fan → High");
    await sendReviewed();
    expect(sent()).toStrictEqual([{ kind: "set_mode", mode: "dry" }, { kind: "set_fan", fanLevel: "high" }]);
    expect(actions.createCommand.mock.calls[0]?.[0]).toBe("u-a");
    expect(router.refresh).toHaveBeenCalledOnce();
  });

  it("a newer report starts the controls from it and drops the edits", () => {
    const { rerender } = render(view());
    press("Raise the temperature");
    press("High");
    expect(reviewText()).toContain("2 changes:");
    // the new report has no fan level: the fan stays High, and only a dropped edit keeps it from being sent
    rerender(view({ observedState: { power: true, celsius: 26, mode: "dry", fanLevel: null, observedAt: "2026-09-15T01:11:00Z" } }));
    expect(screen.getByText("26°C")).toBeDefined();
    expect(screen.getByRole("button", { name: "Dry" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "High" }).getAttribute("aria-pressed")).toBe("true");
    expect(reviewText()).toContain("No change yet");
  });

  it("the power switch asks first; Cancel sends nothing, Power OFF sends the power alone", async () => {
    render(view());
    expect(powerLocked()).toBe(false);
    fireEvent.click(screen.getByRole("switch", { name: "Power" }));
    expect(screen.getByRole("dialog", { name: "Turn this AC off?" })).toBeDefined();
    press("Cancel");
    expect(actions.createCommand).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("switch", { name: "Power" }));
    press("Power OFF");
    await flush();
    expect(sent()).toStrictEqual([{ kind: "set_power", power: false }]);
  });

  it("Undo puts back the confirmed setting once every command was acknowledged, and offers no Undo of its own", async () => {
    render(view());
    press("Raise the temperature");
    await sendReviewed();
    expect(toast).toHaveBeenLastCalledWith(expect.stringContaining("Bedroom AC: "), undefined);
    press("Undo");
    await flush();
    expect(sent()).toStrictEqual([{ kind: "set_temperature", celsius: 25 }, { kind: "set_temperature", celsius: 24 }]);
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    press("Dismiss");
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  it("a command the AC did not take stops the rest, says the AC keeps its setting and offers no Undo", async () => {
    commands.followCommand.mockImplementation((first) => Promise.resolve({ answered: true, command: { ...first, status: "failed", failureCode: "device_rejected" } }));
    render(view());
    press("Dry");
    press("High");
    await sendReviewed();
    expect(sent()).toStrictEqual([{ kind: "set_mode", mode: "dry" }]);
    expect(toast).toHaveBeenLastCalledWith(expect.stringContaining("Bedroom AC: "), "warn");
    expect(screen.getByText(/The AC keeps its confirmed setting; check it and send again\./)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("a refused, unanswered or failed send is reported and never called sent", async () => {
    actions.createCommand.mockResolvedValueOnce({ ok: false, messageKey: "errors.unit_busy", code: "CONFLICT" });
    render(view());
    press("Raise the temperature");
    await sendReviewed();
    expect(toast).toHaveBeenLastCalledWith("Not sent: another command is still waiting for this AC", "warn");
    commands.followCommand.mockResolvedValueOnce({ answered: false, command: command({ kind: "set_temperature", celsius: 25 }) });
    await sendReviewed();
    expect(toast).toHaveBeenLastCalledWith("Bedroom AC: sent — its answer could not be read. Refresh to see the AC's state before sending again.", "warn");
    actions.createCommand.mockRejectedValueOnce(new OpError({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {}, correlationId: "corr-1", retryAfterSeconds: null }, { status: 409 }));
    await sendReviewed();
    expect(toast).toHaveBeenLastCalledWith("Not sent: this AC changed — reload and try again", "warn");
    actions.createCommand.mockRejectedValueOnce(new Error("network"));
    await sendReviewed();
    expect(toast).toHaveBeenLastCalledWith("Not sent", "warn");
    expect(router.refresh).toHaveBeenCalledTimes(4);
  });
});

describe("UnitView in API mode: locks and policies", () => {
  it("offline, HQ's check and a power-off restriction lock the controls", () => {
    const { rerender } = render(view({ connection: "offline" }));
    expect(screen.getByText("Controls disabled while offline")).toBeDefined();
    expect(screen.getByRole("link", { name: "Request repair" }).getAttribute("href")).toBe("/customer/maintenance?new=u-a");
    expect(powerLocked()).toBe(true);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Raise the temperature" }).disabled).toBe(true);
    rerender(view({ controlAvailability: { state: "blocked", reasonKey: "control.reconciliation_required" } }));
    expect(screen.getByText(/Remote control is paused while HQ checks/)).toBeDefined();
    expect(powerLocked()).toBe(true);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Raise the temperature" }).disabled).toBe(true);
    rerender(view({ effectiveControlPolicy: { state: "restricted", phase: "active", policy: { kind: "power_off" } } }, "/customer/payments/inv-1?tab=restriction"));
    expect(powerLocked()).toBe(true);
    expect(screen.getByRole("link", { name: /View restriction details/ }).getAttribute("href")).toBe("/customer/payments/inv-1?tab=restriction");
    rerender(view({ effectiveControlPolicy: { state: "restricted", phase: "active", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 25 } } }));
    expect(screen.getByText(/setpoints below 25 °C are unavailable/)).toBeDefined();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Lower the temperature" }).disabled).toBe(true);
  });

  it("the Attach dialog opened from ?attach= sends only policies still listed, then clears the address; Detach keeps the rest", async () => {
    router.params = new URLSearchParams("attach=p-gone");
    const { rerender } = render(view());
    const dialog = within(screen.getByRole("dialog", { name: "Attach a policy to Bedroom AC" }));
    expect(dialog.getByRole<HTMLButtonElement>("button", { name: "Attach" }).disabled).toBe(true);
    fireEvent.click(dialog.getByRole("checkbox", { name: /Stuffy/ }));
    fireEvent.click(dialog.getByRole("button", { name: "Attach" }));
    await flush();
    expect(actions.setUnitAlertPolicies).toHaveBeenLastCalledWith("u-a", ["p-hot", "p-co2"]);
    expect(router.replace).toHaveBeenCalledWith("/customer/units/u-a", { scroll: false });
    rerender(view({ alertPolicyIds: ["p-hot", "p-co2"] }));
    actions.setUnitAlertPolicies.mockResolvedValueOnce({ ok: false, messageKey: "error.versionConflict", code: "CONFLICT" });
    const tooHot = screen.getByText("Too hot").parentElement;
    if (!tooHot) throw new Error("no Too hot card");
    fireEvent.click(within(tooHot).getByRole("button", { name: "Detach" }));
    await flush();
    expect(actions.setUnitAlertPolicies).toHaveBeenLastCalledWith("u-a", ["p-co2"]);
    expect(toast).toHaveBeenLastCalledWith("Not saved: this AC changed — reload and try again", "warn");
  });

  it("a measuring-only model has no controls, and stale readings are called the last ones", () => {
    const stale = { id: "m-1", unitId: "u-a", sensorId: "s-1", metric: "temperature", value: 27, unit: "°C", observedAt: "2026-09-15T00:00:00Z", origin: "measured", quality: "stale", qualityReason: null } as const;
    render(view({ latestMeasurements: [stale], capabilities: { ...detail().capabilities, control: false, temperature: null, modes: [], fanLevels: [] } }));
    expect(screen.getByText("No remote control on this model")).toBeDefined();
    expect(screen.queryByRole("switch", { name: "Power" })).toBeNull();
    expect(screen.getByText("Last telemetry")).toBeDefined();
  });
});

describe("UnitView demo", () => {
  it("sends the temperature change and the device acknowledges it", () => {
    vi.useFakeTimers();
    render(<UnitView id="unit-x" />);
    press("Raise the temperature");
    fireEvent.click(reviewButton());
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: /^Send to / }));
    expect(screen.getByRole("button", { name: "Waiting for device…" })).toBeDefined();
    act(() => { vi.advanceTimersByTime(1500); });
    expect(toast).toHaveBeenLastCalledWith(expect.stringContaining(": 25°C acknowledged by device"));
    expect(screen.getByText("Set temperature 25°C")).toBeDefined();
  });
});
