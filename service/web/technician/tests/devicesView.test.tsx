// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiDeviceDetail } from "@ac/web/lib/devices";
import type { EventRow, TechDeviceRow } from "@ac/web/lib/techDevices";

// the technician's device list and detail (FR-T11) and one device's events (FR-T12) in API mode (review round 1084)
type Answer = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
const store = vi.hoisted((): { next: Answer; toasts: string[]; refresh: ReturnType<typeof vi.fn>; push: ReturnType<typeof vi.fn>; patch: ReturnType<typeof vi.fn> } => ({
  next: { ok: true, value: null }, toasts: [], refresh: vi.fn(), push: vi.fn(), patch: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: store.refresh, push: store.push }) }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => store.patch }));
vi.mock("../app/technician/devices/actions", () => ({
  registerDevice: vi.fn(() => Promise.resolve(store.next)),
  bindDevice: vi.fn(() => Promise.resolve(store.next)),
  checkDevice: vi.fn(() => Promise.resolve(store.next)),
  calibrateSensor: vi.fn(() => Promise.resolve(store.next)),
  updateFirmware: vi.fn(() => Promise.resolve(store.next)),
  addResponseNote: vi.fn(() => Promise.resolve(store.next)),
  acknowledgeAlert: vi.fn(() => Promise.resolve(store.next)),
}));
// the action runner without the transition and the refresh: the success text, then `after`, or `failed`
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Answer>, ok: string, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { store.toasts.push(ok); after?.(r.value); } else failed?.(r);
    });
  }],
}));

import { acknowledgeAlert, addResponseNote, bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../app/technician/devices/actions";
import { TechDevicesView, type TechDevicesLive } from "../app/technician/devices/_components/devices-view";

type Shown = NonNullable<TechDevicesLive["device"]>;
const deviceRow = (over: Partial<TechDeviceRow> = {}): TechDeviceRow => ({
  id: "dev-11111111aaaa", serial: "AC-001", mqttNode: null, unitId: "unit-1", targetUnitId: null, unit: "Living AC", conn: "online", connText: "12 s ago", fw: "1.0.0", tamper: false, sensors: 2, ...over,
});
const detail = (over: Partial<ApiDeviceDetail> = {}): ApiDeviceDetail => ({
  id: "dev-11111111aaaa", version: 4, unitId: "unit-1", serial: "AC-001", connection: "online", lastSeenAt: "2026-11-20T02:00:00Z", firmwareVersion: "1.0.0",
  powerSignal: "on", tamper: "clear", calibrationRefs: [], activeOperation: null,
  sensors: [{ id: "sen-1", metric: "temperature", unit: "°C", staleAfterSeconds: 300, calibratedAt: null }, { id: "sen-2", metric: "humidity", unit: "%", staleAfterSeconds: 300, calibratedAt: null }],
  ...over,
});
const event = (over: Partial<EventRow> = {}): EventRow => ({ id: "evt-1", version: 2, time: "11-20 10:00", type: "Tamper", tone: "warn", detail: "Case opened", recovery: "Still open", notes: [], alerts: [], ...over });
const shown = (over: Partial<Shown> = {}): Shown => ({
  detail: detail(), row: deviceRow(), jobId: "job-12345678abcd",
  tiles: [{ label: "Connection", value: "Online", dot: "ok", sub: "12 s ago" }, { label: "Tamper", value: "Detected", dot: null, sub: "since 10:00", warn: true }],
  sensors: [{ id: "sen-1", metric: "Temperature", unit: "°C", stale: "5 min", calibrated: "never" }, { id: "sen-2", metric: "Humidity", unit: "%", stale: "5 min", calibrated: "11-01" }],
  unit: { name: "Living AC", place: "Home · Living room", model: "Daikin X" }, firmware: ["1.1.0", "1.2.0"], firmwareCard: null,
  operations: [], calibrations: [], events: null, eventsLocked: false, ...over,
});
const OTHER = deviceRow({ id: "dev-22222222bbbb", serial: "AC-002", unitId: "unit-2", unit: "Office AC", conn: "offline", connText: null, tamper: true });
const live = (over: Partial<TechDevicesLive> = {}): TechDevicesLive => ({
  rows: [deviceRow(), OTHER], canMaintain: true, canAcknowledge: true, events: false,
  register: [
    { unitId: "unit-3", label: "Bedroom AC", model: "Daikin Y", sensors: ["temperature", "co2"], jobs: [{ id: "job-3a", label: "Install 3a" }, { id: "job-3b", label: "Install 3b" }] },
    { unitId: "unit-4", label: "Kitchen AC", model: "Daikin Z", sensors: [], jobs: [] },
  ],
  rebind: [{ unitId: "unit-1", label: "Living AC", jobId: "job-12345678abcd" }, { unitId: "unit-2", label: "Office AC", jobId: "job-2" }],
  device: shown(), ...over,
});
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const select = (name: RegExp) => screen.getByRole<HTMLSelectElement>("combobox", { name });
const dialog = () => screen.getByRole("dialog");
const refused = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}): Answer => ({ ok: false, code, messageKey, fieldErrors });

beforeEach(() => {
  store.next = { ok: true, value: null }; store.toasts = [];
  store.refresh.mockReset(); store.push.mockReset(); store.patch.mockReset();
  for (const f of [registerDevice, bindDevice, checkDevice, calibrateSensor, updateFirmware, addResponseNote, acknowledgeAlert]) vi.mocked(f).mockClear();
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("TechDevicesView list", () => {
  it("counts each filter, and a filter that hides the open device opens its first one", () => {
    render(<TechDevicesView live={live()} />);
    expect(button(/^All 2/).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(button(/^Offline 1/));
    expect(store.patch).toHaveBeenCalledExactlyOnceWith({ deviceId: "dev-22222222bbbb" });
    expect(screen.queryByText("AC-001", { selector: "b" })).toBeNull();
    fireEvent.click(button(/^Online 1/));
    expect(store.patch.mock.calls).toHaveLength(1); // the open device is online: it stays
    fireEvent.click(button(/^Tamper 1/));
    expect(store.patch).toHaveBeenLastCalledWith({ deviceId: "dev-22222222bbbb" });
  });
  it("searches serial, id and unit, says when nothing matches, and opens a device through the URL", () => {
    render(<TechDevicesView live={live()} />);
    const search = screen.getByPlaceholderText("Search serial, device or unit…");
    fireEvent.change(search, { target: { value: "office" } });
    expect(screen.queryByRole("button", { name: /^AC-001/ })).toBeNull();
    fireEvent.click(button(/^AC-002/));
    expect(store.patch).toHaveBeenCalledWith({ deviceId: "dev-22222222bbbb" });
    fireEvent.change(search, { target: { value: "nothing" } });
    expect(screen.getByText("No device matches.")).toBeDefined();
  });
  it("on the events page a filter never moves the device, and a row opens its events page", () => {
    render(<TechDevicesView live={live({ events: true, device: shown({ events: [] }) })} />);
    fireEvent.click(button(/^Offline 1/));
    expect(store.patch).not.toHaveBeenCalled();
    fireEvent.click(button(/^All 2/));
    fireEvent.click(button(/^AC-002/));
    expect(store.push).toHaveBeenCalledWith("/technician/devices/dev-22222222bbbb");
    expect(screen.getByRole("link", { name: /Device detail/ }).getAttribute("href")).toBe("/technician/devices?deviceId=dev-11111111aaaa");
    expect(screen.getByText("No device events.")).toBeDefined();
  });
  it("without devices: the empty list, no device selected, and the register button off with its reason", () => {
    render(<TechDevicesView live={live({ rows: [], device: null, register: [] })} />);
    expect(screen.getByText("No device on your assigned units yet.")).toBeDefined();
    expect(screen.getByText("Choose a device from the list.")).toBeDefined();
    expect(button(/Register device/).disabled).toBe(true);
    expect(screen.getByText("Every assigned unit already has a device")).toBeDefined();
  });
  it("without device.maintain: no register, bind or device work, and the history says whose job it shows", () => {
    render(<TechDevicesView live={live({ canMaintain: false })} />);
    expect(screen.queryByRole("button", { name: /Register device/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rebind…" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Check connection/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Calibrate…" })).toBeNull();
    expect(screen.getByText("Shown for your assigned job on this unit.")).toBeDefined();
  });
});

describe("TechDevicesView detail", () => {
  it("shows the tiles, the binding, the sensors, the history and the link to the events", () => {
    render(<TechDevicesView live={live({ device: shown({ operations: [{ id: "op-1", kind: "Check", status: "failed", statusText: "Failed", time: "10:00", detail: "No answer" }], calibrations: ["11-01 · temperature 24 → 24.3"] }) })} />);
    expect(screen.getByText("Detected").className).toContain("text-warn");
    expect(screen.getByText("Bound to unit")).toBeDefined();
    expect(screen.getByText(/Home · Living room · model Daikin X · job job-1234/)).toBeDefined();
    expect(screen.getByText("Sensors (2) · from the IoT model")).toBeDefined();
    expect(screen.getByText("No answer")).toBeDefined();
    expect(screen.getByText("11-01 · temperature 24 → 24.3")).toBeDefined();
    expect(screen.getByRole("link", { name: /Device events/ }).getAttribute("href")).toBe("/technician/devices/dev-11111111aaaa");
  });
  it("empty sensors, operations and calibrations each say so; without a unit the row's unit is named", () => {
    render(<TechDevicesView live={live({ device: shown({ sensors: [], detail: detail({ sensors: [] }), unit: null }) })} />);
    expect(screen.getByText("This device has no sensors.")).toBeDefined();
    expect(screen.getByText("No operations yet.")).toBeDefined();
    expect(screen.getByText("No calibrations recorded.")).toBeDefined();
    expect(button(/Calibrate sensor/).disabled).toBe(true);
    expect(screen.getAllByText("Living AC").some((n) => n.tagName === "B")).toBe(true);
  });
  it("without an assigned job the device is read-only", () => {
    render(<TechDevicesView live={live({ device: shown({ jobId: null }) })} />);
    expect(screen.getByText(/there is none now, so the device is read-only/)).toBeDefined();
    expect(screen.getByText(/no assigned job on this unit/)).toBeDefined();
    expect(button(/Check connection/).disabled).toBe(true);
    expect(button("Rebind…").disabled).toBe(true);
    expect(button("Calibrate…", screen.getAllByRole("row")[1]).disabled).toBe(true);
  });
  it("while an operation runs: the others wait, the page reads again every 4 s, and the firmware card shows the run", () => {
    vi.useFakeTimers();
    const op = { id: "op-1", kind: "firmware" as const, status: "running" as const, targetVersion: "1.2.0", failureCode: null, createdAt: "2026-11-20T02:00:00Z", finishedAt: null };
    const { unmount } = render(<TechDevicesView live={live({ device: shown({ detail: detail({ activeOperation: op }), firmwareCard: { tone: "primary", title: "Updating to 1.2.0", status: "Running", text: "Keep it powered" } }) })} />);
    expect(screen.getByText(/One device operation at a time \(Firmware update · Running\)/)).toBeDefined();
    expect(button(/Check connection/).disabled).toBe(true);
    expect(button(/Update firmware/).textContent).toContain("running…");
    expect(screen.getByText("Updating to 1.2.0").closest("div.rounded-xl")?.querySelector(".bg-primary")).not.toBeNull();
    act(() => { vi.advanceTimersByTime(8000); });
    expect(store.refresh).toHaveBeenCalledTimes(2);
    unmount();
    act(() => { vi.advanceTimersByTime(8000); });
    expect(store.refresh).toHaveBeenCalledTimes(2);
  });
  it("a finished firmware card has no progress bar", () => {
    render(<TechDevicesView live={live({ device: shown({ firmwareCard: { tone: "crit", title: "Update failed", status: "Failed", text: "1.0.0 kept" } }) })} />);
    expect(screen.getByText("Update failed").closest("div.rounded-xl")?.querySelector(".bg-primary")).toBeNull();
  });
});

describe("TechDevicesView device writes", () => {
  it("checks the connection under the assigned job with the device's version", async () => {
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Check connection/));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Connection check requested"]); });
    expect(checkDevice).toHaveBeenCalledWith("dev-11111111aaaa", 4, "job-12345678abcd");
  });
  it("calibrates the sensor of its row, refuses values that are not numbers, and sends numbers", async () => {
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button("Calibrate…", screen.getAllByRole("row")[2]));
    expect(select(/^Sensor/).value).toBe("sen-2");
    fireEvent.change(screen.getByRole("textbox", { name: /^Reference value/ }), { target: { value: "abc" } });
    fireEvent.click(button("Record calibration"));
    expect(screen.getByRole("textbox", { name: /^Reference value/ }).getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByRole("textbox", { name: /^Measured value/ }).getAttribute("aria-invalid")).toBe("true");
    expect(calibrateSensor).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("textbox", { name: /^Reference value/ }), { target: { value: "24" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Measured value/ }), { target: { value: " 24.3 " } });
    fireEvent.click(button("Record calibration"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Calibration recorded — earlier readings unchanged"]); });
    expect(calibrateSensor).toHaveBeenCalledWith("dev-11111111aaaa", 4, "sen-2", "humidity", "%", 24, 24.3, "job-12345678abcd");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
  it("the calibrate action opens on the first sensor, and a field error of the API names the field", async () => {
    store.next = refused("VALIDATION", "errors.validation", { metric: "error.invalid" });
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Calibrate sensor/));
    expect(select(/^Sensor/).value).toBe("sen-1");
    fireEvent.change(select(/^Sensor/), { target: { value: "sen-2" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Reference value/ }), { target: { value: "1" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Measured value/ }), { target: { value: "2" } });
    fireEvent.click(button("Record calibration"));
    await waitFor(() => { expect(select(/^Sensor/).getAttribute("aria-invalid")).toBe("true"); });
    expect(select(/^Sensor/).closest("label")?.textContent).toContain("Not valid");
    expect(screen.queryByRole("alert")).toBeNull(); // a VALIDATION that only marks fields has no banner
  });
  it("updates the firmware to the newest supported version; an offline refusal stays as a banner", async () => {
    store.next = refused("OFFLINE", "errors.offline");
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Update firmware/));
    expect(select(/^Target version/).value).toBe("1.2.0");
    fireEvent.change(select(/^Target version/), { target: { value: "1.1.0" } });
    fireEvent.click(button("Start update"));
    await waitFor(() => { expect(screen.getByText(/The device is not reachable/)).toBeDefined(); });
    expect(updateFirmware).toHaveBeenCalledWith("dev-11111111aaaa", 4, "1.1.0", "job-12345678abcd");
    store.next = { ok: true, value: null };
    fireEvent.click(button("Start update"));
    await waitFor(() => { expect(screen.queryByText(/The device is not reachable/)).toBeNull(); });
    expect(store.toasts).toStrictEqual(["Firmware update to 1.1.0 requested"]);
  });
});

describe("TechDevicesView after the page reads again with a form open", () => {
  const READ_ONLY = /there is none now, so the device is read-only/;
  it("a calibration whose assignment ended is not sent and the banner says why", () => {
    const { rerender } = render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Calibrate sensor/));
    fireEvent.change(screen.getByRole("textbox", { name: /^Reference value/ }), { target: { value: "1" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Measured value/ }), { target: { value: "2" } });
    rerender(<TechDevicesView live={live({ device: shown({ jobId: null }) })} />);
    fireEvent.click(button("Record calibration"));
    expect(calibrateSensor).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getAllByText(READ_ONLY)).toHaveLength(2); // the banner and the note under the actions
  });
  it("a firmware update whose assignment ended is not sent and the banner says why", () => {
    const { rerender } = render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Update firmware/));
    rerender(<TechDevicesView live={live({ device: shown({ jobId: null }) })} />);
    fireEvent.click(button("Start update"));
    expect(updateFirmware).not.toHaveBeenCalled();
    expect(screen.getAllByText(READ_ONLY)).toHaveLength(2);
  });
  it("a sensor the device no longer has (a rebind issues new IDs) must be chosen again", () => {
    const { rerender } = render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Calibrate sensor/));
    rerender(<TechDevicesView live={live({ device: shown({ detail: detail({ sensors: [{ id: "sen-9", metric: "temperature", unit: "°C", staleAfterSeconds: 300, calibratedAt: null }] }) }) })} />);
    fireEvent.click(button("Record calibration"));
    expect(screen.getByText("Choose a sensor")).toBeDefined();
    expect(calibrateSensor).not.toHaveBeenCalled();
  });
  it("without an open device a filter changes only the list", () => {
    render(<TechDevicesView live={live({ device: null })} />);
    fireEvent.click(button(/^Offline 1/));
    expect(store.patch).not.toHaveBeenCalled();
    expect(screen.getByText("No device selected")).toBeDefined();
  });
});

describe("TechDevicesView register", () => {
  it("a sensor-type refusal of the API is shown under the sensor choices", async () => {
    store.next = refused("VALIDATION", "errors.validation", { sensorTypes: "error.invalid" });
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Register device/));
    fireEvent.change(screen.getByRole("textbox", { name: /^Serial number/ }), { target: { value: "AC-011" } });
    fireEvent.click(button("Register"));
    await waitFor(() => { expect(within(dialog()).getByText("Not valid")).toBeDefined(); });
  });
  it("opens on the first free unit, checks the serial, unit and job, and sends the trimmed serial with the picked sensors", async () => {
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Register device/));
    expect(select(/^Planned unit/).value).toBe("unit-3");
    expect(select(/^Job · required/).value).toBe("job-3a");
    fireEvent.click(button("Register"));
    expect(screen.getByText("3–64 letters, digits or hyphens")).toBeDefined();
    expect(registerDevice).not.toHaveBeenCalled();
    fireEvent.change(select(/^Planned unit/), { target: { value: "unit-4" } });
    expect(screen.getByText("The model defines no sensors.")).toBeDefined();
    fireEvent.change(screen.getByRole("textbox", { name: /^Serial number/ }), { target: { value: " ac-009 " } });
    fireEvent.click(button("Register"));
    expect(screen.getByText("Choose the job")).toBeDefined(); // the unit has no job
    expect(screen.getByText(/→ AC-009\./)).toBeDefined(); // the hint is back once the serial is valid
    fireEvent.change(select(/^Planned unit/), { target: { value: "unit-3" } });
    fireEvent.change(select(/^Job · required/), { target: { value: "job-3b" } });
    fireEvent.click(within(dialog()).getByRole("checkbox", { name: "CO₂ (ppm)" }));
    fireEvent.click(within(dialog()).getByRole("checkbox", { name: "Temperature" }));
    fireEvent.click(within(dialog()).getByRole("checkbox", { name: "CO₂ (ppm)" }));
    fireEvent.click(button("Register"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Device registered — bind it to the unit once it is installed"]); });
    expect(registerDevice).toHaveBeenCalledWith("ac-009", ["temperature"], "unit-3", "job-3b");
  });
  it("a unit that left the list after a refresh is not sent", () => {
    const { rerender } = render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Register device/));
    fireEvent.change(screen.getByRole("textbox", { name: /^Serial number/ }), { target: { value: "AC-010" } });
    rerender(<TechDevicesView live={live({ register: [] })} />);
    expect(select(/^Planned unit/).textContent).toBe("No unit without a device");
    fireEvent.click(button("Register"));
    expect(screen.getByText("Choose a unit")).toBeDefined();
    expect(registerDevice).not.toHaveBeenCalled();
  });
  it("a serial already registered is refused in words and the form stays open", async () => {
    store.next = refused("CONFLICT", "errors.conflict", { serial: "error.duplicate" });
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button(/Register device/));
    fireEvent.change(screen.getByRole("textbox", { name: /^Serial number/ }), { target: { value: "AC-001" } });
    fireEvent.click(button("Register"));
    await waitFor(() => { expect(screen.getByRole("textbox", { name: /^Serial number/ }).getAttribute("aria-invalid")).toBe("true"); });
    expect(screen.getByText("Already registered")).toBeDefined();
    fireEvent.click(button("Cancel"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("This serial is already registered.")).toBeDefined();
  });
});

describe("TechDevicesView bind and rebind", () => {
  it("a registered device is bound to its planned unit with a ready reason", async () => {
    const row = deviceRow({ unitId: null, targetUnitId: "unit-2", unit: "Office AC (planned)" });
    render(<TechDevicesView live={live({ rows: [row], device: shown({ row, detail: detail({ unitId: null }), unit: null }) })} />);
    expect(screen.getByText("Not bound yet")).toBeDefined();
    expect(button(/Check connection/).disabled).toBe(true); // the device writes wait for the binding
    fireEvent.click(button("Bind…"));
    expect(select(/^Planned unit/).value).toBe("unit-2");
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: /^Reason/ }).value).toBe("Installed on site");
    fireEvent.click(button("Bind"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Device bound to the unit — it is connected now"]); });
    expect(bindDevice).toHaveBeenCalledWith("dev-11111111aaaa", 4, "unit-2", "job-2", "Installed on site");
  });
  it("a registered device without a planned unit opens Bind with no unit chosen", () => {
    const row = deviceRow({ unitId: null, targetUnitId: null });
    render(<TechDevicesView live={live({ rows: [row], device: shown({ row }) })} />);
    fireEvent.click(button("Bind…"));
    expect(select(/^Planned unit/).value).toBe("");
  });
  it("a rebind offers only the other units and needs a unit and a reason of 1–1000 characters", async () => {
    render(<TechDevicesView live={live()} />);
    fireEvent.click(button("Rebind…"));
    expect([...select(/^New unit/).options].map((o) => o.value)).toStrictEqual(["", "unit-2"]);
    fireEvent.click(button("Rebind"));
    expect(screen.getByText("Choose a unit")).toBeDefined();
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(select(/^New unit/), { target: { value: "unit-2" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^Reason/ }), { target: { value: "x".repeat(1001) } });
    fireEvent.click(button("Rebind"));
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeDefined();
    expect(bindDevice).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("textbox", { name: /^Reason/ }), { target: { value: "Moved to the office" } });
    fireEvent.click(button("Rebind"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Device rebound — new sensor IDs issued"]); });
    expect(bindDevice).toHaveBeenCalledWith("dev-11111111aaaa", 4, "unit-2", "job-2", "Moved to the office");
  });
});

describe("TechDevicesView events", () => {
  const alert = { id: "alr-12345678abcd", version: 3, status: "open" as const, type: "tamper", severity: "warning" };
  const events = [event({ notes: ["Checked the case"], alerts: [alert] }), event({ id: "evt-2", version: 5, time: "11-20 09:00", type: "Offline", tone: "muted", alerts: [alert, { ...alert, id: "alr-done", status: "resolved" }] })];
  it("lists the events with their notes, adds a response note to the chosen event, and acknowledges each open alert once", async () => {
    render(<TechDevicesView live={live({ events: true, device: shown({ events }) })} />);
    expect(screen.getByText("Checked the case")).toBeDefined();
    expect(screen.queryByText("Sensors (2) · from the IoT model")).toBeNull();
    expect(screen.getAllByRole("button", { name: /^Acknowledge/ })).toHaveLength(1);
    fireEvent.click(button(/Add response note/));
    expect(select(/^Event/).value).toBe("evt-1");
    fireEvent.click(button("Add note"));
    expect(screen.getByText("A note of 1–1000 characters")).toBeDefined();
    fireEvent.change(select(/^Event/), { target: { value: "evt-2" } });
    fireEvent.change(screen.getByRole("textbox", { name: /^What you checked/ }), { target: { value: "Power restored" } });
    fireEvent.click(button("Add note"));
    await waitFor(() => { expect(store.toasts).toStrictEqual(["Response note added — the device state is unchanged"]); });
    expect(addResponseNote).toHaveBeenCalledWith("dev-11111111aaaa", "evt-2", 5, "Power restored");
    fireEvent.click(button("Acknowledge tamper alert alr-1234"));
    await waitFor(() => { expect(store.toasts).toHaveLength(2); });
    expect(acknowledgeAlert).toHaveBeenCalledWith("alr-12345678abcd", 3);
  });
  it("a note longer than 1000 characters is refused", () => {
    render(<TechDevicesView live={live({ events: true, device: shown({ events }) })} />);
    fireEvent.click(button(/Add response note/));
    fireEvent.change(screen.getByRole("textbox", { name: /^What you checked/ }), { target: { value: "x".repeat(1001) } });
    fireEvent.click(button("Add note"));
    expect(addResponseNote).not.toHaveBeenCalled();
  });
  it("only the acknowledge links without device.maintain, and neither without both permissions", () => {
    const { rerender } = render(<TechDevicesView live={live({ events: true, canMaintain: false, device: shown({ events }) })} />);
    expect(screen.queryByRole("button", { name: /Add response note/ })).toBeNull();
    expect(screen.getAllByRole("button", { name: /^Acknowledge/ })).toHaveLength(1);
    rerender(<TechDevicesView live={live({ events: true, canMaintain: false, canAcknowledge: false, device: shown({ events }) })} />);
    expect(screen.queryByRole("button", { name: /^Acknowledge/ })).toBeNull();
  });
  it("events without an assignment on the unit are locked, naming the unit or this unit", () => {
    const { rerender } = render(<TechDevicesView live={live({ events: true, device: shown({ events: null, eventsLocked: true }) })} />);
    expect(screen.getByText(/Device events open while you have an assignment on Living AC/)).toBeDefined();
    expect(screen.queryByText("Operation history")).toBeNull();
    rerender(<TechDevicesView live={live({ events: true, device: shown({ events: null, eventsLocked: true, unit: null }) })} />);
    expect(screen.getByText(/assignment on this unit\./)).toBeDefined();
  });
});
