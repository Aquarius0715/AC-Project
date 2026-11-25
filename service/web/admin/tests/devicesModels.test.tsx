// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { iotModelRows, metrics, metricUnit, modelRows, type ApiCapability, type ApiDevice, type ApiIoTModel, type ApiUnitLite } from "@ac/web/lib/devices";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { entryOf } from "@ac/web/lib/lookup";

// The two model registers of the HQ device screen (FR-A04, DD-A04, DEC-105): the aircon model (capability) editor and
// the IoT model editor — a new model, the next version with its change reason and impact, Discard, the version history
// from the audit log, delete with a reason — and the tabs of the screen in the URL.
const actions = vi.hoisted(() => ({
  saveCapability: vi.fn<(...args: unknown[]) => Promise<unknown>>(), saveIotModel: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
  capabilityHistory: vi.fn<(...args: unknown[]) => Promise<unknown>>(), deleteCapability: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
  deleteIotModel: vi.fn<(...args: unknown[]) => Promise<unknown>>(), searchUnits: vi.fn(), searchDevices: vi.fn(),
}));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("../app/admin/devices/actions", () => actions);
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
vi.mock("../app/admin/devices/_components/device-tab", () => ({ DeviceTab: () => <p>device tab</p> }));
vi.mock("../app/admin/devices/_components/firmware-tab", () => ({ FirmwareTab: () => <p>firmware tab</p> }));

import { DevicesView, type DevicesLive } from "../app/admin/devices/_components/devices-view";

const cap = (id: string, over: Partial<ApiCapability> = {}): ApiCapability => ({
  id, version: 2, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "AC-Co", model: id, control: true, modeControl: true, fanControl: true, temperature: { min: 16, max: 30, step: 1 },
  modes: ["cool"], fanLevels: ["low"], ventilation: false, ventilationLevels: [], ...over,
});
const iot = (id: string, over: Partial<ApiIoTModel> = {}): ApiIoTModel => ({
  id, version: 3, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "IoT-Co", model: id, sensors: [{ metric: "temperature", unit: "°C", staleAfterSeconds: 300, boundaryId: null }], firmwareCandidates: ["1.0.0", "1.1.0"], ...over,
});
const units: ApiUnitLite[] = [
  { id: "u-1", displayName: "Lobby AC", modelId: "m-used", iotModelId: "i-used", customerOrgId: "org-a", archived: false },
  { id: "u-2", displayName: "Office AC", modelId: "m-used", iotModelId: "i-used", customerOrgId: "org-a", archived: false },
  { id: "u-3", displayName: "Hall AC", modelId: "m-one", iotModelId: "i-long", customerOrgId: "org-a", archived: false },
];
const devices: ApiDevice[] = [{ id: "d-1", version: 1, unitId: "u-1", serial: "AC-1", connection: "online", lastSeenAt: null, firmwareVersion: "1.0.0", iotModelId: "i-used", powerSignal: "on", tamper: "clear", sensors: [] }];
function liveOf(caps: ApiCapability[], iots: ApiIoTModel[], over: Partial<DevicesLive> = {}): DevicesLive {
  return {
    tab: "models", now: "2026-09-14T01:00:00Z", canAudit: true, canWrite: true, deviceTotal: 1, campaignTotal: 2,
    models: modelRows(caps, units, devices), iotModels: iotModelRows(iots, units, devices),
    devices: devices.map((d) => ({ id: d.id, version: d.version, serial: d.serial, unitId: d.unitId, iotModelId: d.iotModelId ?? null, targetUnitId: null, unit: "Lobby AC", conn: d.connection, fw: d.firmwareVersion, tamper: false, power: "on" })),
    units: units.map((u) => ({ id: u.id, label: u.displayName, modelId: u.modelId, iotModelId: u.iotModelId ?? null })), ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const closeButton = () => within(screen.getByRole("dialog")).getAllByRole("button", { name: "Close" }).at(-1) ?? document.body; // the footer's, after the corner ×
const iotTab = (iots: ApiIoTModel[], over: Partial<DevicesLive> = {}) => render(<DevicesView live={liveOf([], iots, { tab: "iot", ...over })} />);
const reason = () => screen.getByLabelText(/^Change reason/);

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  for (const f of [actions.saveCapability, actions.saveIotModel, actions.capabilityHistory, actions.deleteCapability, actions.deleteIotModel]) f.mockReset();
});
afterEach(cleanup);

describe("the device screen's tabs", () => {
  it("counts each register and moves between the tabs in the URL", () => {
    const { rerender } = render(<DevicesView live={liveOf([cap("m-used")], [iot("i-used")])} />);
    expect(screen.getByRole("tab", { name: /Firmware campaigns/ }).textContent).toContain("2");
    fireEvent.click(screen.getByRole("tab", { name: /IoT devices/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "devices", deviceId: null, campaignId: null, q: null, cursor: null });
    rerender(<DevicesView live={liveOf([cap("m-used")], [iot("i-used")], { tab: "iot" })} />);
    fireEvent.click(screen.getByRole("tab", { name: /Aircon models/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null, deviceId: null, campaignId: null, q: null, cursor: null });
    rerender(<DevicesView live={liveOf([], [], { campaignTotal: null })} />);
    expect(screen.getByRole("tab", { name: /Firmware campaigns/ }).textContent).toBe("Firmware campaigns"); // not read: no count
    rerender(<DevicesView live={liveOf([], [], { tab: "devices" })} />);
    expect(screen.getByText("device tab")).toBeDefined();
    rerender(<DevicesView live={liveOf([], [], { tab: "firmware", campaigns: [], campaignTotal: null })} />);
    expect(screen.getByText("firmware tab")).toBeDefined();
    expect(screen.getByRole("tab", { name: /Firmware campaigns/ }).textContent).toContain("0"); // the list read on that tab
  });
});

describe("aircon models", () => {
  it("saves the next capability version with a reason, shows the units it moves, and discards an edit", async () => {
    actions.saveCapability.mockResolvedValue({ ok: true, value: { id: "m-used", version: 3 } });
    render(<DevicesView live={liveOf([cap("m-used"), cap("m-one")], [])} />);
    expect(button("Delete model…").disabled).toBe(true); // units use it
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Ventilation supported" }));
    expect(screen.getByText("1 change · ventilation levels")).toBeDefined();
    expect(screen.getByText("2 units — Lobby AC, Office AC move to capability version 3")).toBeDefined();
    fireEvent.click(button("Save as capability v3"));
    expect(screen.getByText("Ventilation levels must include Low")).toBeDefined();
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.click(button("Discard"));
    expect(screen.queryByText("Ventilation levels must include Low")).toBeNull();
    fireEvent.click(screen.getByRole("checkbox", { name: "Ventilation supported" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Low" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Dry" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Cool" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan High" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan Low" }));
    fireEvent.change(screen.getByLabelText(/^Max °C/), { target: { value: "32" } });
    fireEvent.change(screen.getByLabelText(/^Step °C/), { target: { value: "0.5" } });
    fireEvent.change(reason(), { target: { value: " Vendor bulletin " } });
    expect(screen.getByText("4 changes · temperature, modes, fan levels, ventilation levels")).toBeDefined();
    fireEvent.click(button("Save as capability v3"));
    await flush();
    expect(actions.saveCapability).toHaveBeenCalledWith(expect.objectContaining({ id: "m-used", changeReason: "Vendor bulletin", modes: ["dry"], fanLevels: ["high"], ventilation: true, ventilationLevels: ["low"], temperature: { min: 16, max: 32, step: 0.5 } }), 2);
    expect(nav.toasts).toStrictEqual(["Saved — capability version 3"]);
  });

  it("checks a new model's rules, creates it, and cancels back to the first model", async () => {
    actions.saveCapability.mockResolvedValue({ ok: true, value: { id: "m-new", version: 1 } });
    render(<DevicesView live={liveOf([cap("m-one")], [])} />);
    fireEvent.click(button(/New model/));
    expect(screen.getByText("Saving creates capability version 1")).toBeDefined();
    expect(screen.queryByLabelText(/^Change reason/)).toBeNull();
    fireEvent.click(screen.getByRole("checkbox", { name: "Mode control" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan control" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Temperature setpoint" }));
    fireEvent.change(screen.getByLabelText(/^Min °C/), { target: { value: "30" } });
    fireEvent.click(button("Create model"));
    for (const text of ["Mode, fan or temperature control needs power control", "Choose at least one mode", "min must be below max"]) expect(screen.getByText(text)).toBeDefined();
    expect(screen.getAllByText("1–120 characters")).toHaveLength(2);
    fireEvent.change(screen.getByLabelText(/^Min °C/), { target: { value: "16" } });
    fireEvent.change(screen.getByLabelText(/^Step °C/), { target: { value: "40" } });
    expect(screen.getByText("0 < step ≤ max − min")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Mode control" }));
    expect(screen.getByText("Choose at least one fan level")).toBeDefined();
    fireEvent.click(screen.getByRole("checkbox", { name: "Fan control" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Temperature setpoint" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Power control" }));
    fireEvent.change(screen.getByLabelText(/^Manufacturer/), { target: { value: "Daikin" } });
    fireEvent.change(screen.getByLabelText(/^Model/), { target: { value: "FTKF" } });
    fireEvent.click(button("Create model"));
    await flush();
    expect(actions.saveCapability).toHaveBeenCalledWith({ manufacturer: "Daikin", model: "FTKF", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] }, undefined);
    expect(nav.toasts).toStrictEqual(["Model created"]);
    fireEvent.click(button(/New model/));
    fireEvent.click(button("Cancel"));
    expect(screen.getByText("Capability v2")).toBeDefined();
  });

  it("opens the version history, deletes an unused model with a reason, and says when there are none", async () => {
    actions.capabilityHistory.mockResolvedValueOnce({ ok: true, value: [{ at: "2026-09-01T00:00:00Z", actor: "user-op", from: 1, to: 2, reason: "Wider", changes: ["max"] }, { at: "2026-08-01T00:00:00Z", actor: "user-op", from: null, to: 1, reason: null, changes: [] }] })
      .mockResolvedValueOnce({ ok: true, value: [] });
    actions.deleteCapability.mockResolvedValue({ ok: true, value: null });
    const { rerender } = render(<DevicesView live={liveOf([cap("m-spare"), cap("m-one")], [])} />);
    fireEvent.click(button("Version history"));
    await flush();
    expect(nav.toasts).toStrictEqual(["2 version entries"]);
    expect(screen.getByText("v1 → v2 — Wider")).toBeDefined();
    expect(screen.getByText("user-op · changed max")).toBeDefined();
    expect(screen.getByText("v— → v1")).toBeDefined();
    fireEvent.click(closeButton());
    fireEvent.click(button("Version history"));
    await flush();
    expect(screen.getByText("No saved versions in the last 12 months.")).toBeDefined();
    fireEvent.click(closeButton());
    fireEvent.click(button("Delete model…"));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete model…"));
    expect(screen.getByText("Delete AC-Co m-spare? A model that units use cannot be deleted.")).toBeDefined();
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    expect(dialog.getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: " Duplicate " } });
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteCapability).toHaveBeenCalledWith("m-spare", 2, "Duplicate");
    expect(button("Delete model…").disabled).toBe(true); // the next model, which a unit uses, opened
    expect(screen.getByText("In use — a model that units use cannot be deleted")).toBeDefined();
    rerender(<DevicesView live={liveOf([], [])} />);
    expect(screen.getByText("Register the first aircon model.")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull(); // no model to go back to
  });

  it("picks a model from the list, turns ventilation off, closes dialogs with Escape, and deletes the last model", async () => {
    actions.capabilityHistory.mockResolvedValue({ ok: true, value: [{ at: "2026-09-01T00:00:00Z", actor: "user-op", from: 1, to: 2, reason: null, changes: [] }] });
    actions.deleteCapability.mockResolvedValue({ ok: true, value: null });
    const { rerender } = render(<DevicesView live={liveOf([cap("m-used"), cap("m-one", { ventilation: true, ventilationLevels: ["low", "mid"] })], [])} />);
    fireEvent.click(button(/AC-Co m-one/));
    fireEvent.click(screen.getByRole("checkbox", { name: "Ventilation supported" }));
    expect(screen.getByRole("checkbox", { name: "Mid" }).getAttribute("aria-checked")).toBe("false"); // its levels go with it
    expect(screen.getByText("1 unit — Hall AC moves to capability version 3")).toBeDefined();
    fireEvent.click(button("Version history"));
    await flush();
    expect(nav.toasts).toStrictEqual(["1 version entry"]);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    rerender(<DevicesView live={liveOf([cap("m-last")], [])} />);
    fireEvent.click(button("Delete model…"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete model…"));
    fireEvent.change(screen.getByLabelText(/^Reason/), { target: { value: "Gone" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));
    await flush();
    expect(screen.getByText("Saving creates capability version 1")).toBeDefined(); // no model left: a new one
  });

  it("offers no writes or history without the permissions", () => {
    render(<DevicesView live={liveOf([cap("m-used")], [], { canWrite: false, canAudit: false })} />);
    for (const name of [/New model/, "Delete model…", "Version history", "Discard", /Save as/]) expect(screen.queryByRole("button", { name })).toBeNull();
  });
});

describe("IoT models", () => {

  it("edits sensors and firmware candidates, then saves the next version with a reason", async () => {
    actions.saveIotModel.mockResolvedValue({ ok: true, value: { id: "i-used", version: 4 } });
    iotTab([iot("i-used")]);
    expect(button("Delete IoT model…").disabled).toBe(true);
    expect(screen.getByText("current on 1 device")).toBeDefined();
    expect(screen.getByText("latest")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Add sensor"), { target: { value: "humidity" } });
    fireEvent.click(button("Add"));
    fireEvent.change(screen.getAllByLabelText("Stale after (s)").at(-1) ?? document.body, { target: { value: "5" } });
    fireEvent.click(button("Save as IoT model v4"));
    expect(screen.getByText("Stale after must be 10–86400 seconds")).toBeDefined();
    fireEvent.change(screen.getAllByLabelText("Stale after (s)").at(-1) ?? document.body, { target: { value: "600" } });
    fireEvent.click(button("Add version"));
    const version = screen.getByLabelText("Firmware version");
    expect(document.activeElement).toBe(version);
    fireEvent.change(version, { target: { value: "1.1.0" } });
    expect(screen.getByText("1.1.0 is already a candidate")).toBeDefined();
    fireEvent.keyDown(version, { key: "Enter" }); // refused while the version is a candidate
    fireEvent.change(version, { target: { value: "1.2.0" } });
    fireEvent.keyDown(version, { key: "Enter" });
    expect(screen.queryByLabelText("Firmware version")).toBeNull();
    fireEvent.click(button("Remove 1.0.0"));
    fireEvent.click(button("Add version"));
    fireEvent.keyDown(screen.getByLabelText("Firmware version"), { key: "Escape" });
    fireEvent.click(button("Add version"));
    fireEvent.keyDown(screen.getByLabelText("Firmware version"), { key: "a" });
    fireEvent.click(within(screen.getByRole("region", { name: "Firmware candidates" })).getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("2 changes · sensors, firmware candidates")).toBeDefined();
    expect(screen.getByText("2 units — Lobby AC, Office AC move to IoT model version 4")).toBeDefined();
    fireEvent.change(reason(), { target: { value: "Humidity board" } });
    fireEvent.click(button("Save as IoT model v4"));
    await flush();
    expect(actions.saveIotModel).toHaveBeenCalledWith({
      id: "i-used", changeReason: "Humidity board", manufacturer: "IoT-Co", model: "i-used", firmwareCandidates: ["1.1.0", "1.2.0"],
      sensors: [{ metric: "temperature", unit: "°C", staleAfterSeconds: 300, boundaryId: null }, { metric: "humidity", unit: "%", staleAfterSeconds: 600, boundaryId: null }],
    }, 3);
    expect(nav.toasts).toStrictEqual(["Saved — IoT model version 4"]);
  });

  it("removes a sensor, discards the edit, and adds a power sensor on its boundary to a new model", async () => {
    actions.saveIotModel.mockResolvedValue({ ok: true, value: { id: "i-new", version: 1 } });
    iotTab([iot("i-spare", { firmwareCandidates: [] })]);
    fireEvent.click(button("Remove"));
    expect(screen.getByText("1 change · sensors")).toBeDefined();
    fireEvent.click(button("Add version"));
    fireEvent.click(button("Discard"));
    expect(screen.getByText("No unsaved changes")).toBeDefined();
    expect(screen.queryByLabelText("Firmware version")).toBeNull();
    fireEvent.click(button(/New IoT model/));
    expect(screen.getByText("Saving creates IoT model version 1")).toBeDefined();
    fireEvent.click(button("Create IoT model"));
    expect(screen.getAllByText("1–120 characters")).toHaveLength(2);
    fireEvent.change(screen.getByLabelText("Add sensor"), { target: { value: "power" } });
    fireEvent.click(button("Add"));
    expect(screen.getByText("AC input electricity")).toBeDefined();
    fireEvent.change(screen.getByLabelText(/^Manufacturer/), { target: { value: "Acme" } });
    fireEvent.change(screen.getByLabelText(/^Model/), { target: { value: "Node" } });
    fireEvent.click(button("Add version"));
    fireEvent.change(screen.getByLabelText("Firmware version"), { target: { value: "x".repeat(33) } });
    expect(screen.getByText("Firmware versions are at most 32 characters")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Firmware version"), { target: { value: " " } });
    expect(screen.queryByText("Enter a version")).toBeNull(); // nothing typed yet: no error
    const firmware = within(screen.getByRole("region", { name: "Firmware candidates" }));
    expect(firmware.getByRole<HTMLButtonElement>("button", { name: "Add" }).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Firmware version"), { target: { value: "2.0" } });
    fireEvent.click(firmware.getByRole("button", { name: "Add" }));
    fireEvent.click(button("Create IoT model"));
    await flush();
    expect(actions.saveIotModel).toHaveBeenCalledWith({ manufacturer: "Acme", model: "Node", sensors: [{ metric: "power", unit: "kW", staleAfterSeconds: 120, boundaryId: "ac_input_electricity" }], firmwareCandidates: ["2.0"] }, undefined);
    expect(nav.toasts).toStrictEqual(["IoT model created"]);
    fireEvent.click(button(/New IoT model/));
    fireEvent.click(button("Cancel"));
    expect(screen.getByText("IoT model v3")).toBeDefined();
  });

  it("picks a model from the list, refuses a saved candidate that is too long, closes dialogs with Escape, and deletes the last model", async () => {
    actions.capabilityHistory.mockResolvedValue({ ok: true, value: [] });
    actions.deleteIotModel.mockResolvedValue({ ok: true, value: null });
    const { rerender } = iotTab([iot("i-used"), iot("i-long", { firmwareCandidates: ["v".repeat(33)] })]);
    fireEvent.click(button(/IoT-Co i-long/));
    fireEvent.change(screen.getAllByLabelText("Stale after (s)")[0] ?? document.body, { target: { value: "301" } });
    expect(screen.getByText("1 unit — Hall AC moves to IoT model version 4")).toBeDefined();
    fireEvent.change(reason(), { target: { value: "x" } });
    fireEvent.click(button("Save as IoT model v4"));
    expect(screen.getAllByText("Firmware versions are at most 32 characters").length).toBeGreaterThan(0);
    expect(actions.saveIotModel).not.toHaveBeenCalled();
    fireEvent.click(button("Version history"));
    await flush();
    expect(screen.getByText("No saved versions in the last 12 months.")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    rerender(<DevicesView live={liveOf([], [iot("i-last")], { tab: "iot" })} />);
    fireEvent.click(button("Delete IoT model…"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Delete IoT model…"));
    fireEvent.change(screen.getByLabelText(/^Reason/), { target: { value: "Gone" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }));
    await flush();
    expect(screen.getByText("Saving creates IoT model version 1")).toBeDefined();
  });

  it("offers no sensor once every metric is on the model", () => {
    iotTab([iot("i-all", { sensors: metrics.map((metric) => ({ metric, unit: entryOf(metricUnit, metric), staleAfterSeconds: 300, boundaryId: null })) })]);
    expect(screen.queryByLabelText("Add sensor")).toBeNull();
  });

  it("opens the history, deletes an unused model, and shows an empty register", async () => {
    actions.capabilityHistory.mockResolvedValueOnce({ ok: true, value: [{ at: "2026-09-01T00:00:00Z", actor: "user-op", from: 2, to: 3, reason: "PM2.5 board", changes: ["sensors"] }, { at: "2026-08-01T00:00:00Z", actor: "user-op", from: null, to: 1, reason: null, changes: [] }] });
    actions.deleteIotModel.mockResolvedValue({ ok: true, value: null });
    const { rerender } = iotTab([iot("i-spare"), iot("i-next")]);
    fireEvent.click(button("Version history"));
    await flush();
    expect(nav.toasts).toStrictEqual(["2 version entries"]);
    expect(screen.getByText("v2 → v3 — PM2.5 board")).toBeDefined();
    expect(screen.getByText("user-op · changed sensors")).toBeDefined();
    expect(screen.getByText("v— → v1")).toBeDefined();
    fireEvent.click(closeButton());
    fireEvent.click(button("Delete IoT model…"));
    let dialog = within(screen.getByRole("dialog"));
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    expect(dialog.getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog.getByLabelText(/^Reason/), { target: { value: "Never shipped" } });
    fireEvent.click(dialog.getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteIotModel).toHaveBeenCalledWith("i-spare", 3, "Never shipped");
    expect(nav.toasts.at(-1)).toBe("IoT model deleted");
    fireEvent.click(button("Delete IoT model…"));
    dialog = within(screen.getByRole("dialog"));
    fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    rerender(<DevicesView live={liveOf([], [], { tab: "iot" })} />);
    expect(screen.getByText("Register the first IoT model.")).toBeDefined();
  });

  it("offers no writes or history without the permissions", () => {
    iotTab([iot("i-used")], { canWrite: false, canAudit: false });
    for (const name of [/New IoT model/, "Delete IoT model…", "Version history", "Discard", /Save as/]) expect(screen.queryByRole("button", { name })).toBeNull();
  });
});
