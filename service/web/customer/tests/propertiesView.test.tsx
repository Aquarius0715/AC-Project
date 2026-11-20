// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { TreeProperty, TreeSpace } from "@ac/web/lib/assets";
import type { RoomUnit } from "@ac/web/lib/clientProperties";
import type { ApiCommand, ApiUnitDetail, UnitAction } from "@ac/web/lib/units";
import type { Followed } from "@ac/web/lib/unitCommands";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { unreachableFailure } from "@ac/web/lib/unreachable";

// Units & locations (FR-C02, FR-C14) in API mode: the tree marks the clicked location at once while the server renders
// it; the property panel splits units in rooms from unassigned ones; a space lists its units and, for an owner with two
// ACs directly in it, switches to Group control, which plans one command per AC and setting, waits out the write limit,
// stops an AC at its first failure and retries only the failed ACs the list still holds; the KPI filter lists the
// matching units and, for the Running list, turns the running ACs off (IR798); the search box waits before it asks (IR801).
type Sent = { ok: true; value: ApiCommand } | { ok: false; code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };
const actions = vi.hoisted(() => ({ sendCommand: vi.fn<(unitId: string, action: UnitAction) => Promise<Sent>>() }));
vi.mock("../app/customer/properties/actions", () => actions);
const commands = vi.hoisted(() => ({ followCommand: vi.fn<(first: ApiCommand, onUpdate: (c: ApiCommand) => void) => Promise<Followed>>() }));
vi.mock("@ac/web/lib/unitCommands", () => commands);
// the write limit without the real pause: one wait, then the command again
vi.mock("@ac/web/lib/writeLimit", () => ({
  sendPaced: async (send: () => Promise<Sent>, onWait: (seconds: number) => void) => {
    const r = await send();
    if (r.ok || r.code !== "RATE_LIMITED") return r;
    onWait(r.retryAfterSeconds ?? 5);
    return send();
  },
}));
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), pending: false }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatchPending: () => [nav.patch, nav.pending] }));
const router = vi.hoisted(() => ({ refresh: vi.fn(), params: new URLSearchParams() }));
vi.mock("next/navigation", () => ({ useRouter: () => router, useSearchParams: () => router.params }));
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));
vi.mock("../app/customer/_components/rename-modal", () => ({
  RenameModal: ({ r, onClose }: Readonly<{ r: { kind: string; id: string; version: number; where: string }; onClose: () => void }>) => (
    <div role="dialog" aria-label="Rename">{`${r.kind}:${r.id}:${String(r.version)}:${r.where}`}<button type="button" onClick={onClose}>Close rename</button></div>
  ),
}));

import { PropertiesView, type PropertiesLive } from "../app/customer/properties/_components/properties-view";

const RESET = { propertyId: null, spaceId: null, unassignedOnly: null, mode: null, unitIds: null, powerState: null, connections: null, search: null };
const sp = (id: string, name: string, kind: TreeSpace["kind"], parentSpaceId: string | null, direct: number, children: TreeSpace[] = []): TreeSpace => ({
  id, kind, name, parentSpaceId, direct, children, version: 2, propertyId: "p-home", units: direct,
});
const bedroom = sp("s-bed", "Bedroom", "room", "s-1f", 3);
const hall = sp("s-hall", "Hall", "area", "s-1f", 0);
const floor = sp("s-1f", "1F", "floor", null, 0, [bedroom, hall]);
const property = (id: string, name: string, kind: "home" | "office", spaces: TreeSpace[], unassigned: number): TreeProperty => ({
  id, kind, name, spaces, unassigned, version: 5, address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, updatedAt: "2026-09-11T20:00:00Z",
  units: 4, floors: 1, rooms: 1,
});
const home = property("p-home", "Home", "home", [floor], 1);
const office = property("p-office", "Office", "office", [], 0);
const ru = (id: string, name: string, over: Partial<RoomUnit & { place: string }> = {}): RoomUnit & { place: string } => ({
  id, version: 1, name, set: "Cool 24 °C", temp: "26.0 °C", watts: "500 W", power: "running", conn: "online", seen: null, online: true, place: "Home › 1F › Bedroom", ...over,
});
const detail = (id: string, name: string, spaceId: string | null, over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id, displayName: name, version: 4, spaceId, alertPolicyIds: [], connection: "online", effectivePowerState: "on",
  observedState: { power: true, celsius: 24, mode: "cool", fanLevel: "low", observedAt: null }, lastSeenAt: null, latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKF", control: true, modeControl: true, fanControl: true, temperature: { min: 18, max: 30, step: 1 }, modes: ["cool", "dry", "fan"], fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [] },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: [], address: null, accessInstructions: null }, installedAt: null, installState: "installed", modelId: "cap-1", capabilityVersion: 1, serviceScope: [], components: [],
  ...over,
});
const command = (status: ApiCommand["status"], failureCode: string | null = null): ApiCommand => ({ status, failureCode, id: "cmd-0001-xyz", action: { kind: "set_power", power: false }, requestedAt: "2026-09-15T01:12:00Z" });
const created = (c: ApiCommand): Sent => ({ ok: true, value: c });

// Bedroom: three ACs directly in it (the third offline, seen at 08:10); the hall is empty; the porch AC is unassigned
const bedUnits = [ru("u-a", "Bedroom AC"), ru("u-b", "Bedroom AC 2", { temp: "—", watts: "—", power: "stopped" }), ru("u-c", "Bedroom AC 3", { conn: "offline", online: false, seen: "08:10", power: "unknown" })];
const bedDetails = [detail("u-a", "Bedroom AC", "s-bed"), detail("u-b", "Bedroom AC 2", "s-bed", { observedState: { power: false, celsius: 22, mode: "dry", fanLevel: "mid", observedAt: null } }), detail("u-c", "Bedroom AC 3", "s-bed", { connection: "offline", lastSeenAt: "2026-09-15T00:10:00Z" })];
const base = (over: Partial<PropertiesLive> = {}): PropertiesLive => ({
  tree: [home, office], selection: { kind: "property", property: home }, owner: true, filter: null,
  units: [...bedUnits, ru("u-p", "Porch AC", { place: "Home" })], details: [...bedDetails, detail("u-p", "Porch AC", null)], now: "2026-09-15T01:12:30Z",
  summary: { kind: "home", floors: ["1F"], rooms: 1, inRooms: 3, unassigned: 1, edited: "Sep 12 · 04:00" }, mode: "single", selectedUnits: [], ...over,
});
const atBedroom = (over: Partial<PropertiesLive> = {}): PropertiesLive => base({
  selection: { kind: "space", property: home, space: bedroom, path: "Home › 1F › Bedroom" }, summary: null, units: bedUnits, details: bedDetails, ...over,
});
const tree = () => within(screen.getByText("My properties").closest("[data-slot=card]") ?? document.body);
// a tree row by the name its accessible name starts with (the type and unit count follow it)
const pressed = (name: string) => screen.getByRole("button", { name: (accessible) => accessible.startsWith(name) }).getAttribute("aria-pressed");
// an AC card of Group control, by its name
const cardOf = (name: string): HTMLElement => {
  const b = screen.getByText(name, { selector: "b" }).closest("button");
  if (!b) throw new Error(`no card ${name}`);
  return b;
};
// a choice of a settings group (Power, Mode, Fan)
const opt = (group: string, name: string) => within(screen.getByRole("group", { name: group })).getByRole("button", { name });
const flush = () => act(async () => { await Promise.resolve(); });

beforeEach(() => {
  nav.patch.mockReset();
  nav.pending = false;
  router.refresh.mockReset();
  router.params = new URLSearchParams();
  toast.mockReset();
  actions.sendCommand.mockReset().mockResolvedValue(created(command("requested")));
  // each poll reports its state, as followCommand does; the card keeps saying Sending meanwhile
  commands.followCommand.mockReset().mockImplementation((first, onUpdate) => { onUpdate(first); return Promise.resolve({ answered: true, command: command("acknowledged") }); });
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("PropertiesView: the tree", () => {
  it("without a property, says HQ registers them", () => {
    render(<PropertiesView live={base({ tree: [] })} />);
    expect(screen.getByText("No properties yet")).toBeDefined();
    expect(screen.queryByText("My properties")).toBeNull();
  });

  it("lists the properties, their spaces and unassigned units, and marks the clicked row at once while its panel is read", () => {
    const { rerender } = render(<PropertiesView live={base()} />);
    expect(tree().getByText("2")).toBeDefined();
    expect(pressed("Home")).toBe("true");
    expect(pressed("Bedroom")).toBe("false");
    expect(tree().getByText("· 3 units")).toBeDefined();
    expect(tree().getByText("· 1 unit · placed by HQ")).toBeDefined();
    // the hall has no unit of its own, and the office no unassigned row
    expect(screen.getByRole("button", { name: /^Hall/ }).textContent).toBe("HallArea");
    expect(tree().getAllByText("Unassigned units")).toHaveLength(1);
    // the panel is not first on a phone until a location is chosen
    expect(screen.getByText("Air conditioners in this property (3)").closest("[aria-busy]")?.className).not.toContain("order-first");

    fireEvent.click(screen.getByRole("button", { name: /^Bedroom/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, spaceId: "s-bed" });
    nav.pending = true;
    rerender(<PropertiesView live={base()} />);
    expect(pressed("Bedroom")).toBe("true");
    expect(pressed("Home")).toBe("false");
    expect(screen.getByText("Air conditioners in this property (3)").closest("[aria-busy]")?.getAttribute("aria-busy")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: /^Unassigned units/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, propertyId: "p-home", unassignedOnly: "true" });
    expect(pressed("Unassigned units")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: /^Office/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, propertyId: "p-office" });
    expect(pressed("Office")).toBe("true");

    // the server's render arrives: the selection shows again, and a chosen location comes first on a phone
    nav.pending = false;
    router.params = new URLSearchParams("propertyId=p-office");
    rerender(<PropertiesView live={base()} />);
    expect(pressed("Home")).toBe("true");
    expect(screen.getByText("Air conditioners in this property (3)").closest("[aria-busy]")?.className).toContain("order-first");
  });

  it("renames a property or a space from its edit button", () => {
    render(<PropertiesView live={base()} />);
    fireEvent.click(screen.getByRole("button", { name: "Rename Home" }));
    expect(screen.getByRole("dialog", { name: "Rename" }).textContent).toContain("property:p-home:5:Home · Home (property)");
    fireEvent.click(screen.getByRole("button", { name: "Close rename" }));
    expect(screen.queryByRole("dialog", { name: "Rename" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Rename Hall" }));
    expect(screen.getByRole("dialog", { name: "Rename" }).textContent).toContain("space:s-hall:2:Hall · Area in Home");
  });

  it("searches after a pause, not for the same text, and follows a search the URL cleared", () => {
    vi.useFakeTimers();
    const live = base({ filter: { powerState: null, connections: [], search: "bed", total: 4 }, units: [bedUnits[0] ?? ru("x", "x")] });
    const { rerender } = render(<PropertiesView live={live} />);
    const box = screen.getByPlaceholderText<HTMLInputElement>("Find a room or AC…");
    expect(box.value).toBe("bed");
    fireEvent.change(box, { target: { value: " bed " } });
    act(() => { vi.advanceTimersByTime(400); });
    expect(nav.patch).not.toHaveBeenCalled();
    fireEvent.change(box, { target: { value: "porch" } });
    act(() => { vi.advanceTimersByTime(299); });
    expect(nav.patch).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(1); });
    expect(nav.patch).toHaveBeenCalledWith({ ...RESET, search: "porch" });
    // emptied: the search leaves the URL
    fireEvent.change(box, { target: { value: "" } });
    act(() => { vi.advanceTimersByTime(300); });
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, search: null });
    // a tree row cleared the search: the box follows
    nav.patch.mockReset();
    rerender(<PropertiesView live={base()} />);
    expect(box.value).toBe("");
    act(() => { vi.advanceTimersByTime(300); });
    expect(nav.patch).not.toHaveBeenCalled();
  });
});

describe("PropertiesView: the panels", () => {
  it("a property: its summary, the ACs in rooms and the unassigned ones apart", () => {
    render(<PropertiesView live={base()} />);
    expect(screen.getByText("Home", { selector: "b" })).toBeDefined();
    expect(screen.getByText("1 (1F)")).toBeDefined();
    expect(screen.getByText("3 in rooms · 1 unassigned")).toBeDefined();
    expect(screen.getByText("Sep 12 · 04:00")).toBeDefined();
    const rooms = screen.getByText("Air conditioners in this property (3)").parentElement;
    expect(within(rooms ?? document.body).getAllByRole("link").map((a) => a.getAttribute("href"))).toStrictEqual(["/customer/units/u-a", "/customer/units/u-b", "/customer/units/u-c"]);
    expect(screen.getByText("Unassigned units (1)").nextElementSibling?.textContent).toContain("Porch AC");
    // the readings: only what was read, and the time an offline AC was last seen
    expect(screen.getAllByText("26.0 °C · 500 W").length).toBeGreaterThan(0);
    expect(screen.getAllByText("no recent reading").length).toBeGreaterThan(0);
    expect(screen.getByText("seen 08:10")).toBeDefined();
    fireEvent.click(screen.getAllByRole("button", { name: "Rename" })[0] ?? document.body);
    expect(screen.getByRole("dialog", { name: "Rename" }).textContent).toContain("property:p-home:5:");
  });

  it("a property without floors or ACs in rooms says so, and without a summary shows no panel", () => {
    const { rerender } = render(<PropertiesView live={base({ selection: { kind: "property", property: office }, units: [], details: [], summary: { kind: "office", floors: [], rooms: 0, inRooms: 0, unassigned: 0, edited: "—" } })} />);
    expect(screen.getByText("none")).toBeDefined();
    expect(screen.getByText("No air conditioner in a room of this property yet.")).toBeDefined();
    expect(screen.queryByText(/^Unassigned units \(/)).toBeNull();
    rerender(<PropertiesView live={base({ summary: null })} />);
    expect(screen.queryByText("Last edited")).toBeNull();
  });

  it("a property's unassigned units", () => {
    render(<PropertiesView live={base({ selection: { kind: "unassigned", property: home }, units: [ru("u-p", "Porch AC")], summary: null })} />);
    expect(screen.getByText("Unassigned units (1)")).toBeDefined();
    expect(screen.getByText("Home · not in any room — placed by HQ")).toBeDefined();
    expect(screen.getByRole("link", { name: /Porch AC/ }).getAttribute("href")).toBe("/customer/units/u-p");
  });

  it("a space: its path and ACs; Group control only for an owner with two ACs directly in it", () => {
    const { rerender } = render(<PropertiesView live={atBedroom()} />);
    expect(screen.getByText("Bedroom", { selector: "b" }).parentElement?.parentElement?.textContent).toBe("My properties › Home › 1F › Bedroom");
    expect(screen.getByText("Air conditioners in this room (3)")).toBeDefined();
    expect(screen.getByText("3 ACs in Bedroom ·")).toBeDefined();
    expect(screen.getByText(/Group control sends one setting/)).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    expect(screen.getByRole("dialog", { name: "Rename" }).textContent).toContain("space:s-bed:2:Bedroom · Room in Home");

    // a member: no switch, and a group URL still lists the ACs
    rerender(<PropertiesView live={atBedroom({ owner: false, mode: "group" })} />);
    expect(screen.queryByRole("button", { name: "Group control" })).toBeNull();
    expect(screen.getByText("Each AC has its own control screen. Unit membership is managed in HQ’s unit register.")).toBeDefined();
    // the floor lists its rooms' ACs, but none is directly in it
    rerender(<PropertiesView live={atBedroom({ selection: { kind: "space", property: home, space: floor, path: "Home › 1F" } })} />);
    expect(screen.getByText("Air conditioners in this floor (3)")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Group control" })).toBeNull();
    rerender(<PropertiesView live={atBedroom({ selection: { kind: "space", property: home, space: hall, path: "Home › 1F › Hall" }, units: [], details: [] })} />);
    expect(screen.getByText("No air conditioner in this area.")).toBeDefined();
  });

  it("the mode switch shows the chosen mode at once, busy until the server renders it", () => {
    const { rerender } = render(<PropertiesView live={atBedroom()} />);
    expect(screen.getByRole("button", { name: "Single AC" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Group control" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, spaceId: "s-bed", mode: "group" });
    nav.pending = true;
    rerender(<PropertiesView live={atBedroom()} />);
    const group = screen.getByRole("button", { name: "Group control" });
    expect(group.getAttribute("aria-pressed")).toBe("true");
    expect(group.getAttribute("aria-busy")).toBe("true");
    nav.pending = false;
    rerender(<PropertiesView live={atBedroom({ mode: "group" })} />);
    expect(screen.getByRole("button", { name: "Group control" }).getAttribute("aria-busy")).toBe("false");
    expect(screen.getByText("2 selected")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Single AC" }));
    expect(nav.patch).toHaveBeenLastCalledWith({ ...RESET, spaceId: "s-bed", mode: null });
  });
});

describe("PropertiesView: Group control", () => {
  const card = cardOf;

  it("selects the online ACs, toggles, selects all and clears; chooses the settings within the ACs' range", () => {
    render(<PropertiesView live={atBedroom({ mode: "group" })} />);
    expect(screen.getByText("2 selected")).toBeDefined();
    expect(card("Bedroom AC 3").textContent).toContain("last seen 08:10");
    const all = screen.getByRole("button", { name: "Select all online (2)" });
    expect(all.hasAttribute("disabled")).toBe(true);
    fireEvent.click(card("Bedroom AC 2"));
    expect(card("Bedroom AC 2").getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("Apply to 1 selected AC")).toBeDefined();
    fireEvent.click(all);
    expect(screen.getByText("Apply to 2 selected ACs")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.getByText("0 selected")).toBeDefined();
    expect(screen.getByRole("button", { name: "Clear" }).hasAttribute("disabled")).toBe(true);
    fireEvent.click(card("Bedroom AC"));

    // every setting kept: nothing to review
    const reviewBtn = screen.getByRole("button", { name: "Review & send" });
    expect(reviewBtn.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Every setting is kept — choose what to change.")).toBeDefined();
    // the first step gives 24 °C, and the steps stop at the ACs' maximum; Keep goes back
    fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    expect(screen.getByText("24 °C")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    expect(screen.getByText("25 °C")).toBeDefined();
    for (let n = 0; n < 7; n++) fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    expect(screen.getByText("30 °C")).toBeDefined();
    fireEvent.click(opt("Set temperature", "Keep"));
    expect(within(screen.getByRole("group", { name: "Set temperature" })).getByText("Keep", { selector: "b" })).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Lower the temperature" }));
    expect(screen.getByText("24 °C")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Lower the temperature" }));
    expect(screen.getByText("23 °C")).toBeDefined();
    expect(reviewBtn.hasAttribute("disabled")).toBe(false);
    // Off leaves only the power
    fireEvent.click(opt("Power", "Off"));
    expect(screen.queryByRole("button", { name: "Raise the temperature" })).toBeNull();
    fireEvent.click(opt("Power", "Keep"));
    expect(screen.getByRole("button", { name: "Raise the temperature" })).toBeDefined();
  });

  it("starts from the URL's units that are online, and without a temperature capability steps within 16–30 °C", () => {
    const flat = bedDetails.map((d) => ({ ...d, capabilities: { ...d.capabilities, temperature: null } }));
    render(<PropertiesView live={atBedroom({ mode: "group", selectedUnits: ["u-b", "u-c"], details: flat })} />);
    expect(card("Bedroom AC 2").getAttribute("aria-pressed")).toBe("true");
    expect(card("Bedroom AC").getAttribute("aria-pressed")).toBe("false");
    expect(card("Bedroom AC 3").getAttribute("aria-pressed")).toBe("false");
    for (let n = 0; n < 10; n++) fireEvent.click(screen.getByRole("button", { name: "Lower the temperature" }));
    expect(screen.getByText("16 °C")).toBeDefined();
  });

  it("reviews the plan, sends each AC's commands, waits out the write limit and reports each AC's result", async () => {
    // AC 1: power on is already set, so 27 °C then Cool are not needed — the mode is cool; it gets the temperature and fan
    // after a write-limit wait; AC 2 is refused at once
    actions.sendCommand.mockImplementation((id) => Promise.resolve(id === "u-b" ? { ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {} } : created(command("requested"))));
    actions.sendCommand.mockResolvedValueOnce({ ok: false, code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 7 });
    const { rerender } = render(<PropertiesView live={atBedroom({ mode: "group" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    fireEvent.click(opt("Fan", "High"));
    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Send to 2 selected ACs in Bedroom?")).toBeDefined();
    expect(dialog.getByText("25 °C · Fan high")).toBeDefined();
    expect(dialog.getByText("24 → 25 °C, Fan high")).toBeDefined();
    expect(dialog.getByText("22 → 25 °C, Fan high")).toBeDefined();
    expect(dialog.getAllByText("Will send")).toHaveLength(2);
    fireEvent.click(dialog.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 2 ACs" }));
    await flush();
    await flush();
    expect(actions.sendCommand.mock.calls).toStrictEqual([
      ["u-a", { kind: "set_temperature", celsius: 25 }], ["u-b", { kind: "set_temperature", celsius: 25 }],
      ["u-a", { kind: "set_temperature", celsius: 25 }], ["u-a", { kind: "set_fan", fanLevel: "high" }],
    ]);
    expect(card("Bedroom AC").textContent).toContain("Confirmed");
    expect(card("Bedroom AC 2").textContent).toContain("Failed");
    expect(card("Bedroom AC 2").textContent).toContain(actionMessage({ code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {} }));
    expect(toast).toHaveBeenCalledWith("Group change finished — results are shown per AC");
    expect(router.refresh).toHaveBeenCalledOnce();

    // Retry plans only the failed AC
    actions.sendCommand.mockReset().mockResolvedValue(created(command("requested")));
    fireEvent.click(screen.getByRole("button", { name: "Retry failed ACs" }));
    expect(within(screen.getByRole("dialog")).getByText("Send to 1 selected AC in Bedroom?")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 1 AC" }));
    await flush();
    await flush();
    expect(actions.sendCommand.mock.calls.map(([id]) => id)).toStrictEqual(["u-b", "u-b"]);
    expect(card("Bedroom AC 2").textContent).toContain("Confirmed");
    expect(screen.queryByRole("button", { name: "Retry failed ACs" })).toBeNull();
    rerender(<PropertiesView live={atBedroom({ mode: "group" })} />);
    expect(card("Bedroom AC 2").textContent).toContain("Confirmed");
  });

  it("shows the write-limit wait on the card while the command waits", async () => {
    const second = Promise.withResolvers<Sent>();
    actions.sendCommand.mockResolvedValueOnce({ ok: false, code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 7 })
      .mockReturnValueOnce(second.promise);
    render(<PropertiesView live={atBedroom({ mode: "group", selectedUnits: ["u-a"] })} />);
    fireEvent.click(opt("Mode", "Dry"));
    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 1 AC" }));
    await flush();
    expect(card("Bedroom AC").textContent).toContain("Write limit reached — sending again in 7 s");
    expect(card("Bedroom AC").hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: "Review & send" }).hasAttribute("disabled")).toBe(true);
    second.resolve(created(command("requested")));
    await flush();
    await flush();
    expect(card("Bedroom AC").textContent).toContain("Confirmed");
  });

  it("an unread answer is Unknown, a failed command and an action without an answer are Failed, and an AC with nothing to change is not sent", async () => {
    const third = detail("u-d", "Bedroom AC 4", "s-bed");
    const fourth = detail("u-e", "Bedroom AC 5", "s-bed", { observedState: { power: false, celsius: 20, mode: "cool", fanLevel: "low", observedAt: null } });
    const units = [...bedUnits.slice(0, 2), ru("u-d", "Bedroom AC 4"), ru("u-e", "Bedroom AC 5")];
    commands.followCommand.mockImplementation((first) => Promise.resolve(first.id === "unread" ? { answered: false, command: first } : { answered: true, command: command("failed", "DEVICE_REJECTED") }));
    actions.sendCommand.mockImplementation((id) => {
      if (id === "u-a") return Promise.resolve(created({ ...command("requested"), id: "unread" }));
      if (id === "u-b") return Promise.resolve(created(command("requested")));
      return Promise.reject(new Error("offline"));
    });
    const details = [...bedDetails.slice(0, 2), third, fourth];
    const { rerender } = render(<PropertiesView live={atBedroom({ mode: "group", units, details })} />);
    fireEvent.click(opt("Power", "Off"));
    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Send to 4 selected ACs in Bedroom?")).toBeDefined();
    expect(dialog.getByText("Power Off")).toBeDefined();
    expect(dialog.getAllByText("No change")).toHaveLength(2);
    fireEvent.click(dialog.getByRole("button", { name: "Send to 2 ACs" }));
    await flush();
    await flush();
    expect(actions.sendCommand.mock.calls.map(([id]) => id)).toStrictEqual(["u-a", "u-d"]);
    expect(card("Bedroom AC").textContent).toContain("Unknown");
    expect(card("Bedroom AC").textContent).toContain("Sent — its answer could not be read. Refresh to see the AC's state.");
    expect(card("Bedroom AC 4").textContent).toContain(actionMessage(unreachableFailure()));
    expect(card("Bedroom AC 2").textContent).not.toContain("Failed");

    // a command the device refused: failed with its history text
    actions.sendCommand.mockReset().mockResolvedValue(created(command("requested")));
    fireEvent.click(screen.getByRole("button", { name: "Retry failed ACs" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 1 AC" }));
    await flush();
    await flush();
    expect(card("Bedroom AC 4").textContent).toContain("FailedSet power OFF — failed: the AC refused it");
    expect(screen.getByRole("button", { name: "Retry failed ACs" })).toBeDefined();
    // the failed AC left the re-read list: nothing is left to retry
    rerender(<PropertiesView live={atBedroom({ mode: "group", units: units.filter((u) => u.id !== "u-d"), details })} />);
    expect(screen.queryByRole("button", { name: "Retry failed ACs" })).toBeNull();
  });

  it("clamps the setpoint to each AC's range, says an AC never connected, and Escape closes the review", () => {
    const wide = detail("u-a", "Bedroom AC", "s-bed");
    const narrow = detail("u-b", "Bedroom AC 2", "s-bed", { capabilities: { ...wide.capabilities, temperature: { min: 18, max: 26, step: 1 } } });
    const units = [ru("u-a", "Bedroom AC"), ru("u-b", "Bedroom AC 2"), ru("u-n", "New AC", { conn: "unknown", online: false, seen: null })];
    const details = [wide, narrow, detail("u-n", "New AC", "s-bed", { connection: "unknown" })];
    render(<PropertiesView live={atBedroom({ units, details, mode: "group" })} />);
    expect(card("New AC").textContent).toContain("no connection yet");
    for (let n = 0; n < 8; n++) fireEvent.click(screen.getByRole("button", { name: "Raise the temperature" }));
    expect(screen.getByText("30 °C")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Clamped")).toBeDefined();
    expect(dialog.getByText("18–26 °C on this model")).toBeDefined();
    expect(dialog.getByText("24 → 26 °C")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an AC with nothing to send keeps the send button off", () => {
    render(<PropertiesView live={atBedroom({ mode: "group", selectedUnits: ["u-a"] })} />);
    fireEvent.click(opt("Power", "On"));
    fireEvent.click(screen.getByRole("button", { name: "Review & send" }));
    expect(within(screen.getByRole("dialog")).getByText("Send to 1 selected AC in Bedroom?")).toBeDefined();
    expect(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 0 ACs" }).hasAttribute("disabled")).toBe(true);
    expect(within(screen.getByRole("dialog")).getByText("already set")).toBeDefined();
  });
});

// the KPI filter of the Running list, or another one
const running = (over: Partial<NonNullable<PropertiesLive["filter"]>> = {}): NonNullable<PropertiesLive["filter"]> => ({ powerState: "on", connections: [], search: "", total: 4, ...over });

describe("PropertiesView: the KPI filter", () => {

  it("names the filter, counts the matches, says when none matches and clears", () => {
    const { rerender } = render(<PropertiesView live={base({ filter: running({ search: "bed", connections: ["offline", "error"] }), units: bedUnits })} />);
    expect(screen.getByText("Air conditioners · “bed” · Power: Running · Connection: Offline, Error")).toBeDefined();
    expect(screen.getByText("3 of 4")).toBeDefined();
    // the filter list marks no tree row
    expect(pressed("Home")).toBe("false");
    expect(screen.queryByRole("button", { name: /Turn off running ACs/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Clear filter" }));
    expect(nav.patch).toHaveBeenLastCalledWith(RESET);
    rerender(<PropertiesView live={base({ filter: running({ powerState: null, search: "zzz" }), units: [] })} />);
    expect(screen.getByText("No room or AC name contains “zzz”. Check the spelling, or clear the filter to see every unit.")).toBeDefined();
    rerender(<PropertiesView live={base({ filter: running({ powerState: "off" }), units: [] })} />);
    expect(screen.getByText("Air conditioners · Power: Stopped")).toBeDefined();
    expect(screen.getByText("Clear the filter to see every unit.")).toBeDefined();
  });

  it("turns off running ACs only for an owner's Running list of two or more", () => {
    const { rerender } = render(<PropertiesView live={base({ filter: running(), units: bedUnits.slice(0, 1) })} />);
    expect(screen.queryByRole("button", { name: /Turn off running ACs/ })).toBeNull();
    rerender(<PropertiesView live={base({ filter: running(), units: bedUnits, owner: false })} />);
    expect(screen.queryByRole("button", { name: /Turn off running ACs/ })).toBeNull();
    rerender(<PropertiesView live={base({ filter: running({ connections: ["online"] }), units: bedUnits })} />);
    expect(screen.queryByRole("button", { name: /Turn off running ACs/ })).toBeNull();
    rerender(<PropertiesView live={base({ filter: running(), units: bedUnits })} />);
    expect(screen.getByRole("button", { name: /Turn off running ACs/ })).toBeDefined();
  });

  it("turns the running ACs off, keeps its results when they leave the list, and goes back", async () => {
    const offList = [ru("u-a", "Bedroom AC"), ru("u-b", "Bedroom AC 2")];
    const offDetails = [detail("u-a", "Bedroom AC", "s-bed"), detail("u-b", "Bedroom AC 2", "s-hall")];
    const { rerender } = render(<PropertiesView live={base({ filter: running(), units: offList, details: offDetails })} />);
    fireEvent.click(screen.getByRole("button", { name: /Turn off running ACs/ }));
    expect(screen.getByText("Turn off 2 selected ACs")).toBeDefined();
    expect(screen.getAllByText("Home › 1F › Bedroom")).toHaveLength(2);
    expect(screen.getByRole("link", { name: /Turn these off automatically/ }).getAttribute("href")).toBe("/customer/automations?automationId=new&preset=off&unitIds=u-a,u-b");
    fireEvent.click(card("Bedroom AC 2"));
    expect(screen.getByText("Turn off 1 selected AC")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Review & turn off" }));
    expect(within(screen.getByRole("dialog")).getByText("Turn off 1 selected AC?")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 1 AC" }));
    await flush();
    await flush();
    expect(actions.sendCommand).toHaveBeenCalledWith("u-a", { kind: "set_power", power: false });

    // the re-read list: one AC left, the panel stays with its result
    rerender(<PropertiesView live={base({ filter: running(), units: offList.slice(1), details: offDetails })} />);
    expect(screen.getByText("1 AC turned off — it has left the Running list")).toBeDefined();
    rerender(<PropertiesView live={base({ filter: running(), units: [], details: offDetails })} />);
    expect(screen.getByText("No AC is running any more.")).toBeDefined();
    expect(screen.getByText("Turn off 0 selected ACs")).toBeDefined();
    expect(screen.queryByRole("link", { name: /Turn these off automatically/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to the list" }));
    expect(screen.getByText("No air conditioner matches")).toBeDefined();
  });

  it("names the turned-off ACs in the plural", async () => {
    const offList = [ru("u-a", "Bedroom AC"), ru("u-b", "Bedroom AC 2")];
    const offDetails = [detail("u-a", "Bedroom AC", "s-bed"), detail("u-b", "Bedroom AC 2", "s-bed")];
    const { rerender } = render(<PropertiesView live={base({ filter: running(), units: offList, details: offDetails })} />);
    fireEvent.click(screen.getByRole("button", { name: /Turn off running ACs/ }));
    fireEvent.click(screen.getByRole("button", { name: "Review & turn off" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Send to 2 ACs" }));
    await flush();
    await flush();
    rerender(<PropertiesView live={base({ filter: running(), units: [], details: offDetails })} />);
    expect(screen.getByText("2 ACs turned off — they have left the Running list")).toBeDefined();
  });

  const card = cardOf;
});
