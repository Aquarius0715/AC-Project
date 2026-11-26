// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { Children, isValidElement, type ReactNode } from "react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { PickerOption } from "@ac/web/lib/pickerOption";

// The IoT device of a unit on HQ customers & units (DEC-105): the device section of a new unit (none, a registered one,
// a new one, and what it hands to the form), and the unit's device panel: connect a registered or a new device,
// replace the one on the unit in the same write, unbind it, each with a reason. The Phase 1A demo of the screen too.
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return { bindDevice: fn(), registerAndBind: fn(), unbindDevice: fn(), searchDevices: fn(), searchUnits: fn() };
});
const nav = vi.hoisted(() => ({ toasts: [] as string[] }));
vi.mock("server-only", () => ({}));
vi.mock("../app/admin/devices/actions", () => actions);
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
// the device picker searches on the server; here it offers the first free device, or clears the choice
vi.mock("@ac/web/components/ui", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useToast: () => (m: string) => { nav.toasts.push(m); },
  ActionMenu: ({ onChoose, children }: Readonly<{ onChoose: (v: string) => void; children: ReactNode }>) => (
    <span>{Children.toArray(children).map((o) => (isValidElement<{ value: string; children: ReactNode }>(o) ? <button key={o.props.value} type="button" onClick={() => { onChoose(o.props.value); }}>{o.props.children}</button> : null))}</span>
  ),
  AsyncSearchSelect: ({ onChange, search }: Readonly<{ onChange: (o: PickerOption | null) => void; search: (q: string) => Promise<PickerOption[]> }>) => (
    <span>
      <button type="button" onClick={() => { void search("AC").then((os) => { onChange(os[0] ?? null); }); }}>Pick a free device</button>
      <button type="button" onClick={() => { onChange(null); }}>Clear the device</button>
    </span>
  ),
}));

import { NewUnitDeviceSection, UnitDevicePanel, type UnitDevice } from "../app/admin/units/_components/unit-device";
import { UnitsDemo } from "../app/admin/units/_components/units-demo";

const iotModels = [{ id: "i-1", label: "IoT-Co Node", sensors: ["temperature" as const, "humidity" as const] }];
const free: PickerOption = { id: "d-free", label: "AC-0009", version: 3, detail: "IoT-Co Node · node_9", unitId: null, blocked: null };
const device: UnitDevice = { id: "d-1", version: 2, serial: "AC-0001", connection: "online", iotModelId: "i-1", mqttNode: "node_1" };
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = { ok: true, value: null };

beforeEach(() => {
  nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
  actions.searchDevices.mockResolvedValue({ ok: true, value: [free] });
});
afterEach(cleanup);

describe("the device of a new unit", () => {
  it("hands the form no device, a registered one, or a complete new one, and says when one is unfinished", async () => {
    const onChange = vi.fn<(d: unknown, incomplete: boolean) => void>();
    render(<NewUnitDeviceSection iotModels={iotModels} onChange={onChange} />);
    const mode = () => screen.getByRole<HTMLSelectElement>("combobox", { name: "IoT device" }); // the section around it has the label too
    fireEvent.change(mode(), { target: { value: "existing" } });
    expect(onChange).toHaveBeenLastCalledWith(null, true); // a device to choose
    fireEvent.click(button("Pick a free device"));
    await flush();
    expect(actions.searchDevices).toHaveBeenCalledWith("AC", false, true); // free devices only
    expect(onChange).toHaveBeenLastCalledWith({ kind: "existing", deviceId: "d-free", version: 3 }, false);
    fireEvent.click(button("Clear the device"));
    expect(onChange).toHaveBeenLastCalledWith(null, true);
    fireEvent.change(mode(), { target: { value: "new" } });
    expect(onChange).toHaveBeenLastCalledWith(null, true);
    fireEvent.change(screen.getByLabelText(/^Device serial/), { target: { value: " AC-0100 " } });
    fireEvent.change(screen.getByLabelText(/^IoT model/), { target: { value: "i-1" } });
    expect(onChange).toHaveBeenLastCalledWith({ kind: "new", serial: "AC-0100", mqttNode: null, iotModelId: "i-1", sensorTypes: ["temperature", "humidity"] }, true);
    fireEvent.change(screen.getByLabelText(/^MQTT node ID/), { target: { value: "bad id" } });
    expect(onChange).toHaveBeenLastCalledWith(null, true); // an invalid node ID
    fireEvent.change(screen.getByLabelText(/^MQTT node ID/), { target: { value: "node_100" } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ mqttNode: "node_100" }), true);
    fireEvent.change(mode(), { target: { value: "bogus" } });
    expect(onChange).toHaveBeenLastCalledWith(null, false); // no device
  });
});

const panel = (over: Partial<{ device: UnitDevice | null; deviceKnown: boolean; canWrite: boolean }> = {}) =>
  render(<UnitDevicePanel unitId="u-1" unitName="Bedroom AC" device={over.device === undefined ? device : over.device} deviceKnown={over.deviceKnown ?? true} canWrite={over.canWrite ?? true} iotModels={iotModels} />);

describe("the unit's device panel", () => {

  it("shows the device with its IoT model and node, and what a reader sees", () => {
    panel();
    expect(screen.getByText("IoT-Co Node · node_1")).toBeDefined();
    expect(screen.getByRole("link", { name: /Devices/ }).getAttribute("href")).toBe("/admin/devices?tab=devices&deviceId=d-1");
    cleanup();
    panel({ device: { ...device, iotModelId: null, mqttNode: null } });
    expect(screen.getByText("No IoT model")).toBeDefined();
    cleanup();
    panel({ device: null, deviceKnown: false });
    expect(screen.getByText("Needs “View devices & models”")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Connect device…" })).toBeNull();
    cleanup();
    panel({ canWrite: false });
    expect(screen.queryByRole("button", { name: "Replace…" })).toBeNull();
  });

  it("connects a registered device with a reason, and checks both", async () => {
    actions.bindDevice.mockResolvedValue(ok);
    panel({ device: null });
    expect(screen.getByText("No device bound")).toBeDefined();
    fireEvent.click(button("Connect device…"));
    fireEvent.click(dialog().getByRole("button", { name: "Connect" }));
    expect(dialog().getByText("Choose a device")).toBeDefined();
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Pick a free device" }));
    await flush();
    expect(dialog().getByText("IoT-Co Node · node_9")).toBeDefined();
    expect(dialog().getByText("AC-0009")).toBeDefined(); // the After line
    fireEvent.click(dialog().getByRole("button", { name: "Connect" }));
    expect(actions.bindDevice).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Installed " } });
    fireEvent.click(dialog().getByRole("button", { name: "Connect" }));
    await flush();
    expect(actions.bindDevice).toHaveBeenCalledWith("d-free", 3, "u-1", "Installed", undefined);
    expect(nav.toasts).toStrictEqual(["Device connected"]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("replaces the unit's device with a registered one in the same write", async () => {
    actions.bindDevice.mockResolvedValue(ok);
    panel();
    fireEvent.click(button("Replace…"));
    expect(dialog().getByText(/^AC-0001 is on this unit now/)).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Pick a free device" }));
    await flush();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Faulty" } });
    fireEvent.click(dialog().getByRole("button", { name: "Replace" }));
    await flush();
    expect(actions.bindDevice).toHaveBeenCalledWith("d-free", 3, "u-1", "Faulty", "d-1");
    expect(nav.toasts).toStrictEqual(["Device replaced — the previous device was unbound"]);
  });

  it("registers a new device for the unit, replacing the old one, and says when the bind was refused", async () => {
    actions.registerAndBind.mockResolvedValueOnce({ ok: true, value: { id: "d-new", bindError: null } }).mockResolvedValueOnce({ ok: true, value: { id: "d-2", bindError: "error.conflict" } })
      .mockResolvedValueOnce({ ok: true, value: { id: "d-3", bindError: null } });
    panel();
    fireEvent.click(button("Replace…"));
    fireEvent.change(dialog().getByLabelText(/^Device$/), { target: { value: "new" } });
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Upgrade" } });
    fireEvent.click(dialog().getByRole("button", { name: "Replace" }));
    expect(dialog().getByText("Serial is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Device serial/), { target: { value: "AC-0200" } });
    fireEvent.change(dialog().getByLabelText(/^IoT model/), { target: { value: "i-1" } });
    fireEvent.change(dialog().getByLabelText(/^MQTT node ID/), { target: { value: "bad id" } });
    fireEvent.click(dialog().getByRole("button", { name: "Replace" }));
    expect(actions.registerAndBind).not.toHaveBeenCalled();
    fireEvent.change(dialog().getByLabelText(/^MQTT node ID/), { target: { value: "" } });
    expect(dialog().getByText("AC-0200")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Replace" }));
    await flush();
    expect(actions.registerAndBind).toHaveBeenCalledWith({ serial: "AC-0200", sensorTypes: ["temperature", "humidity"], mqttNode: null, iotModelId: "i-1", unitId: "u-1", reason: "Upgrade", replaceDeviceId: "d-1" });
    cleanup();
    panel({ device: null });
    for (let n = 0; n < 2; n += 1) {
      fireEvent.click(button("Connect device…"));
      fireEvent.change(dialog().getByLabelText(/^Device$/), { target: { value: "new" } });
      fireEvent.change(dialog().getByLabelText(/^Device serial/), { target: { value: `AC-03${String(n)}` } });
      fireEvent.change(dialog().getByLabelText(/^IoT model/), { target: { value: "i-1" } });
      fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "New" } });
      fireEvent.click(dialog().getByRole("button", { name: "Connect" }));
      await flush();
    }
    expect(Object.keys(actions.registerAndBind.mock.lastCall?.[0] ?? {})).not.toContain("replaceDeviceId"); // nothing to replace
    expect(nav.toasts).toStrictEqual(["New device registered and replaced the previous one", "Device registered, but not connected — connect it again from here", "New device registered and connected"]);
  });

  it("unbinds the device with a reason, and cancels both dialogs", async () => {
    actions.unbindDevice.mockResolvedValue(ok);
    panel();
    fireEvent.click(button("Unbind…"));
    fireEvent.click(dialog().getByRole("button", { name: "Unbind" }));
    expect(dialog().getByText("A reason is required")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Removed " } });
    fireEvent.click(dialog().getByRole("button", { name: "Unbind" }));
    await flush();
    expect(actions.unbindDevice).toHaveBeenCalledWith("d-1", 2, "Removed");
    expect(nav.toasts).toStrictEqual(["Device unbound — it stays registered"]);
    fireEvent.click(button("Unbind…"));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Replace…"));
    fireEvent.change(dialog().getByLabelText(/^Device$/), { target: { value: "new" } });
    fireEvent.change(dialog().getByLabelText(/^Device$/), { target: { value: "existing" } });
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("the units demo", () => {
  beforeEach(() => { history.replaceState(null, "", "/admin/units"); });

  it("searches the register, checks a new customer, and moves to the warranty tab and back", () => {
    render(<UnitsDemo />);
    expect(screen.getByText("2 of 3 · sorted by name", { exact: false })).toBeDefined();
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "all" } });
    expect(screen.getByText("3 of 3 · sorted by name", { exact: false })).toBeDefined();
    fireEvent.change(screen.getByPlaceholderText("Search customer name, ID, or property…"), { target: { value: "lim" } });
    expect(screen.getByText("1 of 3 · sorted by name", { exact: false })).toBeDefined();
    fireEvent.click(button(/New customer/));
    fireEvent.click(dialog().getByRole("button", { name: "Create" }));
    expect(dialog().getByText("Name is required (1–120 characters)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Customer name/), { target: { value: "Ng family" } });
    fireEvent.click(dialog().getByRole("button", { name: "Create" }));
    expect(nav.toasts).toStrictEqual(["Customer “Ng family” created"]);
    fireEvent.click(button(/Import CSV/));
    expect(screen.getByRole("dialog")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(screen.getByRole("tab", { name: /Warranty & coverage/ }));
    expect(new URLSearchParams(location.search).get("tab")).toBe("warranty");
    fireEvent.click(screen.getByRole("tab", { name: /Customers & units/ }));
    expect(new URLSearchParams(location.search).get("tab")).toBe("overview");
    fireEvent.click(screen.getAllByRole("row")[1] ?? document.body);
    expect(new URLSearchParams(location.search).get("customerId")).toBe("customer-b"); // the search still shows Lim Trading
  });

  it("opens a customer: its locations, units, a unit's edit and their demo dialogs", () => {
    history.replaceState(null, "", "/admin/units?customerId=customer-a");
    render(<UnitsDemo />);
    const save = (name: string, field: RegExp | null, value: string, toast: string) => {
      fireEvent.click(dialog().getByRole("button", { name }));
      if (field) fireEvent.change(dialog().getByLabelText(field), { target: { value } });
      fireEvent.click(dialog().getByRole("button", { name }));
      expect(nav.toasts.at(-1)).toBe(toast);
    };
    fireEvent.click(button(/^Add$/));
    save("Save", /^Name/, "Kitchen", "Location saved");
    fireEvent.click(screen.getAllByRole("button", { name: "Rename" })[0] ?? document.body);
    expect(dialog().getByLabelText<HTMLInputElement>(/^Name/).value).toBe("Home A"); // starts from the current name
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "Home Alpha" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save" }));
    expect(nav.toasts.at(-1)).toBe("Location saved");
    fireEvent.click(button(/Bedroom/));
    expect(screen.getByText("Bedroom — units")).toBeDefined();
    fireEvent.click(button("Delete location…"));
    expect(dialog().getByText(/^CONFLICT — this location still has units/)).toBeDefined();
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    fireEvent.click(button(/New unit/));
    save("Create", /^Unit name/, "Spare AC", "Unit created");
    fireEvent.click(screen.getAllByRole("row")[1] ?? document.body);
    expect(screen.getByText("Unit edit — Bedroom AC")).toBeDefined();
    fireEvent.click(button("Change location…"));
    save("Save", /^Reason/, "Moved", "Location changed");
    fireEvent.click(button(/Attach policy/));
    fireEvent.click(dialog().getByRole("button", { name: "Attach" }));
    fireEvent.click(button("Delete unit…"));
    expect(dialog().getByText(/^CONFLICT — unit is in use/)).toBeDefined();
    fireEvent.click(dialog().getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    fireEvent.click(button("Close"));
    expect(screen.queryByText("Unit edit — Bedroom AC")).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: "Alert policies" }));
    expect(screen.getByText("Bedroom too hot")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Units & locations" }));
    expect(new URLSearchParams(location.search).get("tab")).toBe("overview");
    fireEvent.click(button(/Customers & units/));
    expect(new URLSearchParams(location.search).get("customerId")).toBeNull();
    expect(nav.toasts).toContain("Policy attached");
  });

  it("adds and manages the demo client users", () => {
    history.replaceState(null, "", "/admin/units?customerId=customer-a&tab=users");
    render(<UnitsDemo />);
    fireEvent.click(button(/Add user/));
    fireEvent.click(dialog().getByRole("button", { name: "Add user" }));
    expect(dialog().getByText("A name of 1–120 characters")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "Siti" } });
    fireEvent.change(dialog().getByLabelText(/^Email/), { target: { value: "siti@example.com" } });
    fireEvent.change(dialog().getByLabelText(/^Role/), { target: { value: "owner" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add user" }));
    expect(nav.toasts.at(-1)).toMatch(/^Siti added/);
    for (const item of ["Reset password", "Change role (Owner / Member)", "Resend set-up e-mail"]) fireEvent.click(screen.getAllByRole("button", { name: item })[0] ?? document.body);
    fireEvent.click(screen.getAllByRole("button", { name: /^(Disable|Enable) sign-in$/ })[0] ?? document.body);
    fireEvent.click(screen.getAllByRole("button", { name: /^Remove/ })[0] ?? document.body);
    expect(nav.toasts.length).toBeGreaterThan(4);
    fireEvent.click(button(/Add user/));
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
