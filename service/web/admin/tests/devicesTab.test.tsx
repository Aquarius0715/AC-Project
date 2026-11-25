// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { iotModelRows, type ApiCalibration, type ApiDevice, type ApiDeviceDetail, type ApiDeviceOperation, type ApiIoTModel } from "@ac/web/lib/devices";
import {
  calibrationText, deviceTiles, eventRows, firmwareCard, newerFirmware, newerVersions, openFaults, operationRows, sensorRows, techDeviceRows, type ApiDeviceEventFull,
} from "@ac/web/lib/techDevices";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { PickerOption } from "@ac/web/lib/pickerOption";

// The HQ IoT devices tab (DD-A04, Figma Admin 246:2): the state filters, the server search and paging, the selected
// device (binding, MQTT node, actions, firmware card, sensors, history, events) and its dialogs — register (and bind),
// bind / rebind with the unit's device replaced, unbind and delete with a reason, calibrate, firmware update.
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return {
    calibrateSensor: fn(), checkDevice: fn(), deleteDevice: fn(), setMqttNode: fn(), unbindDevice: fn(), updateFirmware: fn(), bindDevice: fn(), deviceOnUnit: fn(),
    registerAndBind: fn(), searchUnits: fn(), searchDevices: fn(),
  };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[], searched: [] as string[] }));
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
// the unit picker searches on the server; here it offers one unit, or none, and reports what it searched
vi.mock("@ac/web/components/ui", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  AsyncSearchSelect: ({ onChange, label, search }: Readonly<{ onChange: (o: PickerOption | null) => void; label: string; search: (q: string) => Promise<PickerOption[]> }>) => (
    <span>
      <button type="button" onClick={() => { void search("hall").then((os) => { nav.searched.push(label); onChange(os[0] ?? null); }); }}>{`Pick ${label}`}</button>
      <button type="button" onClick={() => { onChange(null); }}>{`Clear ${label}`}</button>
    </span>
  ),
}));

import { DeviceTab } from "../app/admin/devices/_components/device-tab";
import type { DevicesLive } from "../app/admin/devices/_components/devices-view";

const NOW = Date.parse("2026-09-14T01:00:00Z");
const sensor = (id: string, metric: "temperature" | "humidity") => ({ id, metric, unit: metric === "temperature" ? "°C" as const : "%" as const, staleAfterSeconds: 300, calibratedAt: null });
const device = (id: string, over: Partial<ApiDevice> = {}): ApiDevice => ({
  id, version: 2, unitId: "u-1", serial: `AC-${id}`, connection: "online", lastSeenAt: "2026-09-14T00:59:00Z", firmwareVersion: "1.0.0", iotModelId: "i-1", mqttNode: null,
  powerSignal: "on", tamper: "clear", sensors: [sensor("s-1", "temperature"), sensor("s-2", "humidity")], ...over,
});
const iot: ApiIoTModel = { id: "i-1", version: 1, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "IoT-Co", model: "n1", sensors: [{ metric: "temperature", unit: "°C", staleAfterSeconds: 300, boundaryId: null }, { metric: "humidity", unit: "%", staleAfterSeconds: 300, boundaryId: null }], firmwareCandidates: ["1.0.0", "1.1.0", "1.2.0"] };
const event = (n: number): ApiDeviceEventFull => ({
  id: `e-${String(n)}`, version: 1, deviceId: "d-1", eventType: "communication_lost", evidenceSource: "heartbeat", occurredAt: `2026-09-13T${String(10 + n).padStart(2, "0")}:00:00Z`,
  restoredAt: null, alertIds: [], recovery: null, responseNotes: n === 0 ? [{ actorId: "tech-1", message: "Router rebooted", at: "2026-09-13T11:00:00Z" }] : [],
});
// unread: the server sent no registry page and no rows
interface Extra { unread?: boolean; ops?: ApiDeviceOperation[]; cals?: ApiCalibration[]; events?: ApiDeviceEventFull[]; unit?: DevicesLive["device"] extends infer D ? D extends { unit: infer U } ? U : never : never; active?: ApiDeviceOperation | null }
const names = new Map([["u-1", "Lobby AC"], ["u-9", "Hall AC"]]);
/** The live data the page builds for the devices tab, from the same helpers. */
function liveOf(list: ApiDevice[], open: ApiDevice | null, extra: Extra = {}, over: Partial<DevicesLive> = {}): DevicesLive {
  const rows = techDeviceRows(list, names, NOW);
  const row = open && rows.find((r) => r.id === open.id);
  const detail: ApiDeviceDetail | null = open && { ...open, calibrationRefs: [], activeOperation: extra.active ?? null };
  const events = extra.events ?? [];
  const candidates = iot.firmwareCandidates;
  return {
    tab: "devices", now: new Date(NOW).toISOString(), canAudit: true, canWrite: true, deviceTotal: list.length, models: [], iotModels: iotModelRows([iot], [], list), devices: [], units: [],
    ...(!extra.unread && { page: { q: "", next: null, cursor: null }, rows }),
    ...(detail && row && { device: {
      row, detail, tiles: deviceTiles(detail, newerFirmware(detail.firmwareVersion, candidates), NOW, openFaults(events)), sensors: sensorRows(detail),
      unit: extra.unit === undefined ? { name: "Lobby AC", place: "Home A › 1F", model: "AC-Co m1 v3" } : extra.unit,
      firmware: newerVersions(detail.firmwareVersion, candidates), firmwareCard: firmwareCard(detail, extra.ops ?? []), operations: operationRows(extra.ops ?? [], detail.firmwareVersion),
      calibrations: (extra.cals ?? []).map((c) => calibrationText(c)), events: eventRows(events, new Map(), NOW),
    } }),
    ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = []; nav.searched = [];
  for (const f of Object.values(actions)) f.mockReset();
  actions.searchUnits.mockResolvedValue({ ok: true, value: [{ id: "u-9", label: "Hall AC", detail: "Demo A · Home B" }] });
});
afterEach(() => { vi.useRealTimers(); cleanup(); });

describe("the device list", () => {
  it("filters by state, opening the filter's first device when the open one is hidden, and says when none match", () => {
    const list = [device("d-1"), device("d-2", { connection: "offline", tamper: "detected" }), device("d-3", { sensors: [] })];
    render(<DeviceTab live={liveOf(list, list[0] ?? null)} />);
    expect(button(/^All/).textContent).toContain("3");
    fireEvent.click(button(/^Offline/));
    expect(nav.patch).toHaveBeenLastCalledWith({ deviceId: "d-2" });
    cleanup();
    render(<DeviceTab live={liveOf(list, list[1] ?? null)} />);
    expect(screen.getByText("Detected").className).toBe("text-warn"); // the tamper tile stands out
    cleanup();
    render(<DeviceTab live={liveOf(list, list[0] ?? null)} />);
    expect(screen.getByText("Tamper", { selector: "span" })).toBeDefined();
    nav.patch.mockClear();
    fireEvent.click(button(/^Online/)); // the open device stays
    expect(nav.patch).not.toHaveBeenCalled();
    expect(screen.getByText("fw 1.0.0 · no sensors")).toBeDefined();
    fireEvent.click(button(/^AC-d-3/));
    expect(nav.patch).toHaveBeenLastCalledWith({ deviceId: "d-3" });
    fireEvent.click(button(/^Tamper/));
    fireEvent.click(button(/^No sensors/));
    cleanup();
    render(<DeviceTab live={liveOf([device("d-1")], null)} />);
    fireEvent.click(button(/^Offline/));
    expect(screen.getByText("No device matches.")).toBeDefined();
    expect(screen.getByText("Choose a device.")).toBeDefined();
    cleanup();
    render(<DeviceTab live={liveOf([], null, { unread: true })} />);
    expect(screen.getByText("Register the first device.")).toBeDefined();
    expect(screen.queryByText(/of 0/)).toBeNull(); // no paging without a page
  });

  it("searches on the server once typing pauses, and pages the registry", () => {
    const { rerender } = render(<DeviceTab live={liveOf([device("d-1")], null, {}, { deviceTotal: 250, page: { q: "", next: "cur-2", cursor: null } })} />);
    expect(screen.getByText("1 of 250")).toBeDefined();
    expect(screen.queryByText("First page")).toBeNull();
    fireEvent.click(screen.getByText("Next"));
    expect(nav.patch).toHaveBeenLastCalledWith({ cursor: "cur-2", deviceId: null });
    nav.patch.mockClear();
    vi.useFakeTimers();
    fireEvent.change(screen.getByPlaceholderText("Search serial, device or unit…"), { target: { value: " node_3 " } });
    act(() => { vi.advanceTimersByTime(399); });
    expect(nav.patch).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(1); });
    expect(nav.patch).toHaveBeenLastCalledWith({ q: "node_3", cursor: null, deviceId: null });
    rerender(<DeviceTab live={liveOf([], null, {}, { page: { q: "node_3", next: null, cursor: "cur-2" } })} />);
    fireEvent.change(screen.getByPlaceholderText("Search serial, device or unit…"), { target: { value: "" } });
    act(() => { vi.advanceTimersByTime(400); });
    expect(nav.patch).toHaveBeenLastCalledWith({ q: null, cursor: null, deviceId: null });
    vi.useRealTimers();
    fireEvent.click(screen.getByText("First page"));
    expect(nav.patch).toHaveBeenLastCalledWith({ cursor: null, deviceId: null });
  });
});

describe("a bound device", () => {
  const bound = device("d-1", { mqttNode: "node_1", firmwareVersion: "1.1.0" });

  it("shows its unit, tiles, firmware card, sensors, history and events, the latest ten first", () => {
    const ops: ApiDeviceOperation[] = [
      { id: "op-1", kind: "firmware", status: "failed", targetVersion: "1.2.0", failureCode: "TIMEOUT", createdAt: "2026-09-12T00:00:00Z", finishedAt: "2026-09-12T00:01:00Z" },
      { id: "op-2", kind: "check", status: "succeeded", targetVersion: null, failureCode: null, createdAt: "2026-09-11T00:00:00Z", finishedAt: "2026-09-11T00:00:05Z" },
    ];
    const cals: ApiCalibration[] = [{ id: "c-1", sensorId: "s-1", metric: "temperature", unit: "°C", referenceValue: 24, measuredValue: 24.4, calibratedAt: "2026-09-12T00:00:00Z", actorId: "tech-1" }];
    render(<DeviceTab live={liveOf([bound], bound, { ops, cals, events: Array.from({ length: 12 }, (_, n) => event(n)) })} />);
    expect(screen.getByText("Bound to unit")).toBeDefined();
    expect(screen.getByText("Home A › 1F · model AC-Co m1 v3")).toBeDefined();
    expect(screen.getByText("node_1")).toBeDefined();
    expect(screen.getByText("Firmware update to 1.2.0 failed")).toBeDefined();
    expect(screen.getByText("Sensors (2) · from the IoT model")).toBeDefined();
    expect(screen.getByText("heartbeat confirmed")).toBeDefined();
    expect(screen.getByText(/ref 24.0 °C \/ measured 24.4 °C · offset −0.4 °C/)).toBeDefined();
    expect(screen.getByText("A bound device cannot be deleted: unbind it first.")).toBeDefined();
    expect(button("Delete…").disabled).toBe(true);
    const table = () => screen.getByText("Recovery / response").closest("table");
    expect(within(table() ?? document.body).getAllByRole("row")).toHaveLength(11); // header + 10
    fireEvent.click(button("Show all 12"));
    expect(within(table() ?? document.body).getAllByRole("row")).toHaveLength(13);
    expect(screen.getByText(/Router rebooted/)).toBeDefined(); // the response note
    fireEvent.click(button("Show the latest 10"));
    expect(screen.getByRole("link", { name: /Full history in Audit/ }).getAttribute("href")).toBe("/admin/audit?tab=devices&deviceId=d-1");
  });

  it("queues a connection check and a firmware update, and records a calibration", async () => {
    actions.checkDevice.mockResolvedValue(ok); actions.updateFirmware.mockResolvedValue(ok); actions.calibrateSensor.mockResolvedValue(ok);
    render(<DeviceTab live={liveOf([bound], bound)} />);
    expect(screen.getAllByText("1.2.0 available")).toHaveLength(2); // the tile and the action
    fireEvent.click(button(/^Check connection/));
    await flush();
    expect(actions.checkDevice).toHaveBeenCalledWith("d-1", 2);
    fireEvent.click(button(/^Update firmware/));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Target version/).value).toBe("1.2.0"); // the newest by default
    fireEvent.change(dialog().getByLabelText(/^Target version/), { target: { value: "" } });
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Queue update" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^Target version/), { target: { value: "1.2.0" } });
    fireEvent.click(dialog().getByRole("button", { name: "Queue update" }));
    await flush();
    expect(actions.updateFirmware).toHaveBeenCalledWith("d-1", 2, "1.2.0");
    fireEvent.click(button(/^Calibrate sensor/));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Sensor/).value).toBe("s-1");
    fireEvent.change(dialog().getByLabelText(/^Reference/), { target: { value: "abc" } });
    fireEvent.click(dialog().getByRole("button", { name: "Record" }));
    expect(dialog().getAllByText("Enter a number")).toHaveLength(2);
    expect(actions.calibrateSensor).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Sensor/), { target: { value: "s-2" } });
    fireEvent.change(dialog().getByLabelText(/^Reference/), { target: { value: "55" } });
    fireEvent.change(dialog().getByLabelText(/^Measured/), { target: { value: "57.5" } });
    fireEvent.click(dialog().getByRole("button", { name: "Record" }));
    await flush();
    expect(actions.calibrateSensor).toHaveBeenCalledWith("d-1", 2, "s-2", "humidity", "%", 55, 57.5);
    fireEvent.click(screen.getAllByRole("button", { name: "Calibrate…" })[0] ?? document.body);
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Sensor/).value).toBe("s-1"); // the row's sensor
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.toasts).toStrictEqual(["Connection check queued", "Firmware update to 1.2.0 queued", "Calibration recorded (demo)"]);
  });

  it("edits the MQTT node ID, checking its format, and clears it", async () => {
    actions.setMqttNode.mockResolvedValue(ok);
    render(<DeviceTab live={liveOf([bound], bound)} />);
    fireEvent.click(button("Edit…"));
    const input = dialog().getByLabelText<HTMLInputElement>(/^Node ID/);
    expect(input.value).toBe("node_1");
    fireEvent.change(input, { target: { value: "node 1!" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    expect(dialog().getByText("Use 1–64 letters, digits, hyphens or underscores")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Node ID/), { target: { value: "node_7" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.setMqttNode).toHaveBeenLastCalledWith("d-1", 2, "node_7");
    fireEvent.click(button("Edit…"));
    fireEvent.change(dialog().getByLabelText(/^Node ID/), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    await flush();
    expect(actions.setMqttNode).toHaveBeenLastCalledWith("d-1", 2, null);
    expect(nav.toasts).toStrictEqual(["MQTT node ID saved", "MQTT node ID cleared"]);
  });

  it("unbinds with a reason, and rebinds to a unit whose device it replaces", async () => {
    actions.unbindDevice.mockResolvedValue(ok); actions.bindDevice.mockResolvedValue(ok);
    actions.deviceOnUnit.mockResolvedValue({ ok: true, value: { id: "d-other", serial: "AC-OTHER" } });
    render(<DeviceTab live={liveOf([bound], bound)} />);
    fireEvent.click(button("Unbind…"));
    fireEvent.click(dialog().getByRole("button", { name: "Unbind" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Moved " } });
    fireEvent.click(dialog().getByRole("button", { name: "Unbind" }));
    await flush();
    expect(actions.unbindDevice).toHaveBeenCalledWith("d-1", 2, "Moved");
    expect(nav.patch).not.toHaveBeenCalled(); // the device stays open
    fireEvent.click(button("Rebind…"));
    expect(dialog().getByText("Lobby AC")).toBeDefined(); // from
    fireEvent.click(dialog().getByRole("button", { name: "Rebind" }));
    expect(dialog().getByText("Choose a unit")).toBeDefined();
    expect(dialog().getAllByText("A reason is required").length).toBeGreaterThan(0);
    fireEvent.click(dialog().getByRole("button", { name: "Pick AC unit" }));
    await flush();
    expect(dialog().getByText("Hall AC · Demo A · Home B")).toBeDefined();
    expect(dialog().getByText(/AC-OTHER is on this unit now/)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Swap" } });
    fireEvent.click(dialog().getByRole("button", { name: "Rebind" }));
    await flush();
    expect(actions.bindDevice).toHaveBeenCalledWith("d-1", 2, "u-9", "Swap", "d-other");
    expect(nav.toasts).toStrictEqual(["Device unbound — it stays registered", "Device connected — the previous device on the unit was unbound"]);
  });

  it("rebinds to an empty unit, ignores the device itself, and clears the choice", async () => {
    actions.bindDevice.mockResolvedValue(ok);
    actions.deviceOnUnit.mockResolvedValueOnce({ ok: true, value: { id: "d-1", serial: "AC-d-1" } }).mockResolvedValueOnce({ ok: true, value: null });
    render(<DeviceTab live={liveOf([bound], bound)} />);
    fireEvent.click(button("Rebind…"));
    fireEvent.click(dialog().getByRole("button", { name: "Pick AC unit" }));
    await flush();
    expect(dialog().queryByText(/is on this unit now/)).toBeNull(); // the device on the unit is this one
    fireEvent.click(dialog().getByRole("button", { name: "Clear AC unit" }));
    expect(actions.deviceOnUnit).toHaveBeenCalledOnce();
    fireEvent.click(dialog().getByRole("button", { name: "Pick AC unit" }));
    await flush();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Moved" } });
    fireEvent.click(dialog().getByRole("button", { name: "Rebind" }));
    await flush();
    expect(actions.bindDevice).toHaveBeenCalledWith("d-1", 2, "u-9", "Moved", undefined);
    expect(nav.toasts).toStrictEqual(["Device bound — new sensor IDs issued"]);
  });

  it("locks the actions while an operation runs, and shows it on the firmware card", () => {
    const active: ApiDeviceOperation = { id: "op-9", kind: "firmware", status: "running", targetVersion: "1.2.0", failureCode: null, createdAt: "2026-09-14T00:50:00Z", finishedAt: null };
    render(<DeviceTab live={liveOf([bound], bound, { active })} />);
    for (const name of [/^Check connection/, /^Calibrate sensor/, /^Update firmware/, "Unbind…", "Delete…"]) expect(button(name).disabled).toBe(true);
    expect(screen.getByText("running…")).toBeDefined();
    expect(screen.getByText(/One operation at a time/)).toBeDefined();
    expect(screen.getByText("Firmware update 1.1.0 → 1.2.0")).toBeDefined();
  });

  it("offers no actions to a role that may only read, and no audit link without audit.read", () => {
    render(<DeviceTab live={liveOf([bound], bound, { events: [event(0), { ...event(1), eventType: "operation_failed" }] }, { canWrite: false, canAudit: false })} />);
    expect(screen.getByText("Operation failed")).toBeDefined(); // a muted event beside the faults
    for (const name of ["Register device", "Unbind…", "Rebind…", "Edit…", "Delete…", /^Check connection/, "Calibrate…"]) expect(screen.queryByRole("button", { name })).toBeNull();
    expect(screen.queryByRole("link", { name: /Full history in Audit/ })).toBeNull();
  });
});

describe("a device not bound yet", () => {
  const spare = device("d-5", { unitId: null, targetUnitId: "u-9", sensors: [], firmwareVersion: "1.2.0" });

  it("binds it to its unit with the default reason, and deletes it with a reason", async () => {
    actions.bindDevice.mockResolvedValue(ok); actions.deleteDevice.mockResolvedValue(ok);
    actions.deviceOnUnit.mockResolvedValue({ ok: false, messageKey: "error.forbidden" });
    render(<DeviceTab live={liveOf([spare], spare, { unit: null })} />);
    expect(screen.getByText("Not bound yet")).toBeDefined();
    expect(screen.getByText("Not bound yet — planned for Hall AC", { selector: "b" })).toBeDefined();
    expect(screen.getByText("Not set")).toBeDefined();
    expect(screen.getByText("This device has no sensors.")).toBeDefined();
    expect(screen.getByText("No operations yet.")).toBeDefined();
    expect(screen.getByText("No calibrations recorded.")).toBeDefined();
    expect(screen.getByText("No device events.")).toBeDefined();
    expect(button(/^Calibrate sensor/).disabled).toBe(true);
    expect(button(/^Update firmware/).disabled).toBe(true); // it runs the newest
    fireEvent.click(button("Edit…"));
    expect(dialog().getByLabelText<HTMLInputElement>(/^Node ID/).value).toBe(""); // no node ID yet
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Bind…"));
    expect(dialog().getByText("Not connected")).toBeDefined();
    expect(dialog().getByLabelText<HTMLTextAreaElement>(/^Reason/).value).toBe("Installed on site");
    fireEvent.click(dialog().getByRole("button", { name: "Pick AC unit" }));
    await flush();
    fireEvent.click(dialog().getByRole("button", { name: "Bind" }));
    await flush();
    expect(actions.bindDevice).toHaveBeenCalledWith("d-5", 2, "u-9", "Installed on site", undefined);
    expect(nav.toasts).toStrictEqual(["Device bound to the unit — it is connected now"]);
    fireEvent.click(button("Delete…"));
    expect(dialog().getByText(/^Delete AC-d-5\?/)).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Broken" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteDevice).toHaveBeenCalledWith("d-5", 2, "Broken");
    expect(nav.patch).toHaveBeenLastCalledWith({ deviceId: null });
    fireEvent.click(button("Delete…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("registering a device", () => {
  it("checks the fields, starts from the IoT model's sensors, and registers it on its own", async () => {
    actions.registerAndBind.mockResolvedValue({ ok: true, value: { id: "d-new", bindError: null } });
    render(<DeviceTab live={liveOf([], null)} />);
    fireEvent.click(button(/Register device/));
    fireEvent.click(dialog().getByRole("button", { name: "Register" }));
    for (const text of ["Serial is required", "Choose an IoT model"]) expect(dialog().getByText(text)).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Device serial/), { target: { value: " AC-0009 " } });
    fireEvent.change(dialog().getByLabelText(/^MQTT node ID/), { target: { value: "bad id" } });
    fireEvent.change(dialog().getByLabelText(/^IoT model/), { target: { value: "i-1" } });
    fireEvent.click(dialog().getByRole("checkbox", { name: "temperature (°C)" }));
    expect(dialog().getByText("No temperature sensor: temperature cannot be measured for this device.")).toBeDefined();
    fireEvent.click(dialog().getByRole("checkbox", { name: "temperature (°C)" }));
    fireEvent.click(dialog().getByRole("button", { name: "Register" }));
    expect(actions.registerAndBind).not.toHaveBeenCalled(); // the node ID
    fireEvent.change(dialog().getByLabelText(/^MQTT node ID/), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Register" }));
    await flush();
    expect(actions.registerAndBind).toHaveBeenCalledWith({ serial: "AC-0009", sensorTypes: ["humidity", "temperature"], mqttNode: null, iotModelId: "i-1", unitId: null });
    expect(nav.toasts).toStrictEqual(["Device registered — connect it to a unit when it is installed"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ deviceId: "d-new" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("registers it with a unit, and says when the bind was refused", async () => {
    actions.registerAndBind.mockResolvedValueOnce({ ok: true, value: { id: "d-a", bindError: null } }).mockResolvedValueOnce({ ok: true, value: { id: "d-b", bindError: "error.conflict" } });
    render(<DeviceTab live={liveOf([], null)} />);
    for (const serial of ["AC-A", "AC-B"]) {
      fireEvent.click(button(/Register device/));
      fireEvent.change(dialog().getByLabelText(/^Device serial/), { target: { value: serial } });
      fireEvent.change(dialog().getByLabelText(/^MQTT node ID/), { target: { value: "node_4" } });
      fireEvent.change(dialog().getByLabelText(/^IoT model/), { target: { value: "i-1" } });
      fireEvent.click(dialog().getByRole("button", { name: "Pick AC unit" }));
      await flush();
      fireEvent.click(dialog().getByRole("button", { name: "Register" }));
      await flush();
    }
    expect(actions.registerAndBind).toHaveBeenLastCalledWith(expect.objectContaining({ serial: "AC-B", mqttNode: "node_4", unitId: "u-9" }));
    expect(actions.searchUnits).toHaveBeenCalledWith("hall", true); // only units without a device
    expect(nav.toasts).toStrictEqual(["Device registered and connected to the unit", "Device registered, but not connected to the unit — connect it from its page"]);
    fireEvent.click(button(/Register device/));
    fireEvent.change(dialog().getByLabelText(/^IoT model/), { target: { value: "" } });
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("turns a failed picker search into an error for the picker to show", async () => {
    actions.searchUnits.mockResolvedValue({ ok: false, messageKey: "error.unavailable" });
    const { unitsWithoutDevice, allUnits, freeDevices } = await import("../app/admin/_components/device-fields");
    actions.searchDevices.mockResolvedValue({ ok: true, value: [{ id: "d-1", label: "AC-1" }] });
    await expect(unitsWithoutDevice("x")).rejects.toThrow("error.unavailable");
    await expect(allUnits("x")).rejects.toThrow("error.unavailable");
    expect(await freeDevices("AC")).toStrictEqual([{ id: "d-1", label: "AC-1" }]);
    expect(actions.searchDevices).toHaveBeenCalledWith("AC", false, true);
  });
});
