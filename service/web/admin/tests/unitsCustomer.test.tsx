// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { locationTree, selection, unitRows, unitsAt, type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow, type CustomerRow } from "@ac/web/lib/assets";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { NewUnitDevice } from "../app/admin/units/actions";

// One customer on the HQ customers & units screen (FR-A02): the header (edit, delete, new unit), the KPIs and the
// contract standing, the location tree (select, rename, delete, add floor / room), the location card of a property, a
// space or the unassigned units, the units card (search, power and connection filters), and the modals: the customer,
// its deletion, a property with its pin and radius, a space, a rename, a location's deletion and a new unit with its
// optional device.
const actions = vi.hoisted(() => {
  const fn = () => vi.fn<(...args: unknown[]) => Promise<unknown>>();
  return { updateCustomer: fn(), deleteCustomer: fn(), saveProperty: fn(), saveSpace: fn(), archiveLocation: fn(), saveUnit: fn(), saveUnitWithDevice: fn(), searchAddress: fn() };
});
const nav = vi.hoisted(() => ({ patch: vi.fn<(p: Record<string, string | null>) => void>(), toasts: [] as string[] }));
vi.mock("server-only", () => ({}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }), usePathname: () => "/admin/units", useSearchParams: () => new URLSearchParams() }));
vi.mock("../app/admin/units/actions", () => actions);
// the device part of a new unit is unit-device's own test; here it hands back a device, an incomplete one or none
vi.mock("../app/admin/units/_components/unit-device", () => ({
  UnitDevicePanel: () => null,
  NewUnitDeviceSection: ({ onChange }: Readonly<{ onChange: (v: NewUnitDevice | null, incomplete: boolean) => void }>) => (
    <span>
      <button type="button" onClick={() => { onChange({ kind: "existing", deviceId: "d-1", version: 2 }, false); }}>Use device d-1</button>
      <button type="button" onClick={() => { onChange(null, true); }}>Start a device</button>
    </span>
  ),
}));
// the map picker: a valid pin, or coordinates out of range
vi.mock("@ac/web/components/LocationPicker", () => ({
  PinMap: ({ label }: Readonly<{ label: string }>) => <p>{label}</p>,
  LocationPicker: ({ onChange }: Readonly<{ onChange: (v: { latitude: number; longitude: number } | null, valid: boolean) => void }>) => (
    <span>
      <button type="button" onClick={() => { onChange({ latitude: 3.15, longitude: 101.7 }, true); }}>Drop pin</button>
      <button type="button" onClick={() => { onChange(null, false); }}>Bad coordinates</button>
    </span>
  ),
}));
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
// the dropdown of a tree row, as plain buttons
vi.mock("@ac/web/components/ui", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  ActionMenu: ({ label, onChoose }: Readonly<{ label: string; onChoose: (v: string) => void }>) => (
    <span>{([["rename", "Rename"], ["delete", "Delete…"]] as const).map(([v, text]) => <button key={v} type="button" aria-label={`${label}: ${text}`} onClick={() => { onChoose(v); }}>{text}</button>)}</span>
  ),
}));

import { UnitsView, type CustomerLive, type UnitsLive } from "../app/admin/units/_components/units-view";
import { registerKpis } from "@ac/web/lib/assets";

const property = (id: string, kind: "home" | "office", over: Partial<ApiPropertyRow> = {}): ApiPropertyRow => ({
  id, kind, version: 2, customerOrgId: "org-a", name: `${kind === "home" ? "Home" : "Office"} ${id}`, address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-01T00:00:00Z", ...over,
});
const space = (id: string, propertyId: string, kind: ApiSpaceRow["kind"], parentSpaceId: string | null = null): ApiSpaceRow => ({ id, propertyId, kind, parentSpaceId, version: 1, name: `${kind} ${id}`, archived: false });
const unit = (id: string, propertyId: string, spaceId: string | null, over: Partial<ApiUnitRow> = {}): ApiUnitRow => ({
  id, propertyId, spaceId, version: 1, customerOrgId: "org-a", displayName: `Unit ${id}`, modelId: "m-1", type: "split", installState: "installed", installedAt: null, serviceScope: ["indoor"], alertPolicyIds: [],
  archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, ...over,
});
const props = [property("p-home", "home", { address: "1 Jalan Demo", accessInstructions: "Gate code 1234", location: { latitude: 3.1, longitude: 101.6 }, geofenceRadiusMeters: 300 }), property("p-office", "office")];
const spaces = [space("s-floor", "p-home", "floor"), space("s-room", "p-home", "room", "s-floor"), space("s-area", "p-office", "area")];
const units = [unit("u-1", "p-home", "s-room", { activeAlertCount: 2 }), unit("u-2", "p-home", null, { effectivePowerState: "off", connection: "offline" }), unit("u-3", "p-office", "s-area")];
const tree = locationTree(props, spaces, units, "org-a");
const row = (over: Partial<CustomerRow> = {}): CustomerRow => ({
  id: "cust-a", version: 3, name: "Demo Customer A", billingName: "Demo Customer A Sdn Bhd", status: "active", orgId: "org-a", orgVersion: 4, orgStatus: "active", profile: "rto", since: "Jan 2026",
  properties: 2, propertyNames: ["Home p-home", "Office p-office"], units: 3, operation: "67%", running: 2, known: 3, alerts: 2, marks: [{ label: "Good standing", tone: "ok" }],
  standing: { plans: ["rto"], contracts: 1, contractUnits: 3, overdue: 0, overdueAmount: null, restriction: null }, ...over,
});
function customerOf(locationId: string | undefined, over: Partial<CustomerLive> = {}): CustomerLive {
  const sel = selection(tree, locationId, spaces);
  const at = sel ? unitsAt(sel, units) : [];
  return {
    row: row(), tab: "overview", users: [], tree, selection: sel, places: [], models: [{ id: "m-1", label: "AC-Co Split 1" }], iotModels: [], canRegisterDevice: true, perProperty: "Home p-home 2 · Office p-office 1",
    alertsSub: "2 warning", filter: { connections: [], search: "" }, total: at.length, units: unitRows(at, spaces, new Map([["m-1", "Split 1"]])), lastEdit: "1 Sep 2026 · HQ Ops", policies: [], rules: null,
    canRules: true, canAttach: true, ...over,
  };
}
function liveOf(c: CustomerLive, over: Partial<UnitsLive> = {}): UnitsLive {
  return {
    now: "2026-09-14T01:00:00Z", todayKL: "2026-09-14", canWrite: true, canForce: false, standingKnown: true, rows: [c.row], kpis: registerKpis([c.row], units), search: "", missing: false, top: "customers",
    activeCustomers: [], coverageAttention: null, map: { styleUrl: "https://tiles.example/style.json", search: true }, customer: c, ...over,
  };
}
const button = (name: string | RegExp) => screen.getByRole<HTMLButtonElement>("button", { name });
const dialog = () => within(screen.getByRole("dialog"));
const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const ok = (value: unknown = null) => ({ ok: true, value });
const treeCard = () => within(screen.getByText("Locations").closest("[data-slot=card]") ?? document.body);

beforeEach(() => {
  nav.patch.mockReset(); nav.toasts = [];
  for (const f of Object.values(actions)) f.mockReset();
});
afterEach(cleanup);

describe("the customer header and KPIs", () => {
  it("goes back to the register, moves between tabs, and shows the standing", () => {
    render(<UnitsView live={liveOf(customerOf(undefined))} />);
    expect(screen.getByText("Demo Customer A Sdn Bhd · customer since Jan 2026 · Rent-to-own (RTO)")).toBeDefined();
    expect(screen.getByText("Plan: Rent-to-own (RTO) · 3 units under contract")).toBeDefined();
    expect(screen.getByText("no overdue invoice")).toBeDefined();
    expect(screen.getByText("Has units — a customer with units cannot be deleted")).toBeDefined();
    expect(button("Delete customer…").disabled).toBe(true);
    fireEvent.click(button(/Customers/));
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, tab: null, locationId: null, propertyId: null, unitId: null, powerState: null, connections: null, search: null });
    fireEvent.click(screen.getByRole("tab", { name: /Users/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: "users", unitId: null });
    cleanup();
    render(<UnitsView live={liveOf(customerOf(undefined, { tab: "users" }))} />);
    fireEvent.click(screen.getByRole("tab", { name: /Units & locations/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ tab: null, unitId: null });
  });

  it("words overdue invoices and restrictions, and says what the KPIs need without billing or alerts", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf(undefined, { row: row({ standing: { plans: ["rto"], contracts: 1, contractUnits: 1, overdue: 2, overdueAmount: "MYR 200.00", restriction: null } }) }))} />);
    expect(screen.getByText("2 invoices overdue")).toBeDefined();
    expect(screen.getByRole("link", { name: /View invoices/ }).getAttribute("href")).toBe("/admin/billing?customerId=cust-a&overdueOnly=true");
    rerender(<UnitsView live={liveOf(customerOf(undefined, { row: row({ standing: { plans: [], contracts: 0, contractUnits: 0, overdue: 1, overdueAmount: "MYR 1.00", restriction: { id: "r-1", state: "release_requested", contractId: "k-1" } } }) }))} />);
    expect(screen.getByText("Restriction release requested")).toBeDefined();
    expect(screen.getByRole("link", { name: /View restriction/ }).getAttribute("href")).toBe("/admin/restrictions?restrictionId=r-1");
    expect(screen.getByText("No current contract")).toBeDefined();
    rerender(<UnitsView live={liveOf(customerOf(undefined, { row: row({ standing: { plans: [], contracts: 1, contractUnits: 1, overdue: 0, overdueAmount: null, restriction: { id: "r-2", state: "active", contractId: "k-1" } } }) }))} />);
    expect(screen.getByText("Restriction active")).toBeDefined();
    rerender(<UnitsView live={liveOf(customerOf(undefined, { row: row({ standing: null, units: 0 }), alertsSub: null }))} />);
    expect(screen.getByText("needs “View billing”")).toBeDefined();
    expect(screen.getByText("severity needs “View alerts”")).toBeDefined();
    expect(button("Delete customer…").disabled).toBe(false);
  });

  it("edits the customer and renames its billing organization, and deletes a customer without units with force", async () => {
    actions.updateCustomer.mockResolvedValue(ok());
    actions.deleteCustomer.mockResolvedValue(ok());
    const c = customerOf(undefined, { row: row({ units: 0 }) });
    render(<UnitsView live={liveOf(c, { canForce: true })} />);
    fireEvent.click(button("Edit customer"));
    expect(dialog().getByText("A customer that still has units, contracts or open jobs cannot be deactivated.")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Billing name/), { target: { value: "Demo A Holdings" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save customer" }));
    await flush();
    expect(actions.updateCustomer).toHaveBeenCalledWith("cust-a", 3, { id: "org-a", version: 4, name: "Demo Customer A Sdn Bhd", status: "active" }, { billingName: "Demo A Holdings", name: "Demo Customer A", serviceProfile: "rto", status: "active" });
    fireEvent.click(button("Delete customer…"));
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    expect(dialog().getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Duplicate " } });
    fireEvent.click(dialog().getByRole("checkbox", { name: /Force delete/ }));
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Delete" }).disabled).toBe(true); // the name typed first
    fireEvent.change(dialog().getByLabelText(/^Type Demo Customer A to confirm/), { target: { value: "Demo Customer A" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete" }));
    await flush();
    expect(actions.deleteCustomer).toHaveBeenCalledWith("cust-a", 3, { id: "org-a", version: 4 }, "Duplicate", true);
    expect(nav.patch).toHaveBeenLastCalledWith({ customerId: null, tab: null, locationId: null, propertyId: null, unitId: null, powerState: null, connections: null, search: null });
    expect(nav.toasts).toStrictEqual(["Customer saved", "Customer deleted"]);
  });

  it("offers no writes to a reader, and no new unit for an inactive customer", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf("s-room"), { canWrite: false })} />);
    for (const name of ["Edit customer", "Delete customer…", /New unit/, /Add property/, "Rename"]) expect(screen.queryByRole("button", { name })).toBeNull();
    rerender(<UnitsView live={liveOf(customerOf("s-room", { row: row({ status: "inactive" }) }))} />);
    expect(screen.queryByRole("button", { name: /New unit/ })).toBeNull();
  });
});

describe("the location tree and cards", () => {
  it("selects each location, shows the property's details and pin, and the unassigned units", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    expect(screen.getByText("1 Jalan Demo")).toBeDefined();
    expect(screen.getByText("Gate code 1234")).toBeDefined();
    expect(screen.getByText("300 m")).toBeDefined();
    expect(screen.getByText("Map of Home p-home with its pin")).toBeDefined();
    expect(screen.getByText(/^All units in Home p-home · 2$/)).toBeDefined();
    fireEvent.click(treeCard().getByRole("button", { name: /^Office p-office/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "p-office", unitId: null });
    fireEvent.click(treeCard().getByRole("button", { name: /^room s-room/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "s-room", unitId: null });
    fireEvent.click(treeCard().getByRole("button", { name: /^Unassigned/ }));
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "unassigned:p-home", unitId: null });
    rerender(<UnitsView live={liveOf(customerOf("unassigned:p-home"))} />);
    expect(screen.getByText("Home p-home · units without a room or floor")).toBeDefined();
    expect(screen.getByText(/^Unassigned units in Home p-home · 1$/)).toBeDefined();
    rerender(<UnitsView live={liveOf(customerOf("p-office"))} />);
    expect(screen.getByText("No pin yet")).toBeDefined();
    expect(screen.queryByText(/with its pin$/)).toBeNull();
    rerender(<UnitsView live={liveOf(customerOf("s-floor"))} />);
    expect(screen.getByText(/^Floor · .*floor s-floor · 1 unit$/)).toBeDefined();
    expect(screen.queryByRole("button", { name: /New unit in/ })).toBeNull(); // a floor holds rooms, not units
    rerender(<UnitsView live={liveOf(customerOf("s-room"))} />);
    expect(screen.getByRole("button", { name: /New unit in room s-room/ })).toBeDefined();
    expect(screen.getByText("none")).toBeDefined(); // no child locations
  });

  it("adds a floor, a room and an area from the tree and the cards, under the right parent", async () => {
    actions.saveSpace.mockResolvedValue(ok("s-new"));
    render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    fireEvent.click(treeCard().getAllByRole("button", { name: /Add floor$/ })[0] ?? document.body);
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Kind/).value).toBe("floor");
    fireEvent.click(dialog().getByRole("button", { name: "Add floor" }));
    expect(dialog().getByText("1–120 characters")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: " 2F " } });
    fireEvent.click(dialog().getByRole("button", { name: "Add floor" }));
    await flush();
    expect(actions.saveSpace).toHaveBeenLastCalledWith({ propertyId: "p-home", kind: "floor", parentSpaceId: null, name: "2F" });
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "s-new", unitId: null });
    fireEvent.click(treeCard().getAllByRole("button", { name: /Add room$/ })[0] ?? document.body); // under the floor
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Parent/).value).toBe("s-floor");
    fireEvent.change(dialog().getByLabelText(/^Property/), { target: { value: "p-office" } });
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Parent/).value).toBe(""); // the parent goes with the property
    fireEvent.change(dialog().getByLabelText(/^Parent/), { target: { value: "s-area" } });
    fireEvent.change(dialog().getByLabelText(/^Kind/), { target: { value: "space" } });
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "Server room" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add space" }));
    await flush();
    expect(actions.saveSpace).toHaveBeenLastCalledWith({ propertyId: "p-office", kind: "space", parentSpaceId: "s-area", name: "Server room" });
    fireEvent.click(treeCard().getByRole("button", { name: /Add floor \/ area$/ }));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Kind/).value).toBe("area");
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(nav.toasts).toStrictEqual(["Floor “2F” added", "Space “Server room” added"]);
  });

  it("renames a property and a space, only once the name changed", async () => {
    actions.saveProperty.mockResolvedValue(ok("p-home"));
    actions.saveSpace.mockResolvedValue(ok("s-room"));
    render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    fireEvent.click(treeCard().getAllByRole("button", { name: "Location actions: Rename" })[0] ?? document.body); // the property's
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Save name" }).disabled).toBe(true);
    fireEvent.change(dialog().getByLabelText(/^New name/), { target: { value: " " } });
    fireEvent.click(dialog().getByRole("button", { name: "Save name" }));
    expect(dialog().getByText("1–120 characters")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^New name/), { target: { value: "Main house" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save name" }));
    await flush();
    expect(actions.saveProperty).toHaveBeenCalledWith(expect.objectContaining({ id: "p-home", name: "Main house", customerOrgId: "org-a", address: "1 Jalan Demo" }), 2);
    cleanup();
    render(<UnitsView live={liveOf(customerOf("s-room"))} />);
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    expect(dialog().getByText("room s-room · Room")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^New name/), { target: { value: " Master bedroom " } });
    fireEvent.click(dialog().getByRole("button", { name: "Save name" }));
    await flush();
    expect(actions.saveSpace).toHaveBeenCalledWith({ id: "s-room", propertyId: "p-home", parentSpaceId: "s-floor", kind: "room", name: "Master bedroom" }, 1);
    expect(nav.toasts).toStrictEqual(["Name saved", "Name saved"]);
  });

  it("refuses to delete a location that still holds others or units, and deletes an empty one with a reason", async () => {
    actions.archiveLocation.mockResolvedValue(ok());
    const emptyTree = locationTree([property("p-empty", "home")], [space("s-empty", "p-empty", "room")], [], "org-a");
    render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(dialog().getByText("Home p-home still contains 1 location and 2 units. Move or delete them first (unit › Location).")).toBeDefined();
    expect(dialog().getByRole<HTMLButtonElement>("button", { name: "Delete property" }).disabled).toBe(true);
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    cleanup();
    render(<UnitsView live={liveOf({ ...customerOf(undefined), tree: emptyTree, selection: selection(emptyTree, "s-empty", [space("s-empty", "p-empty", "room")]) })} />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(dialog().getByRole("button", { name: "Delete room" }));
    expect(dialog().getByText("A reason is required (1–1000 characters)")).toBeDefined();
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: " Merged " } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete room" }));
    await flush();
    expect(actions.archiveLocation).toHaveBeenCalledWith("space", "s-empty", 1, "Merged");
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: null, unitId: null });
    fireEvent.click(treeCard().getAllByRole("button", { name: "Location actions: Delete…" })[1] ?? document.body); // the room's, from the tree
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Sold" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete room" }));
    await flush();
    expect(actions.archiveLocation).toHaveBeenLastCalledWith("space", "s-empty", 1, "Sold");
  });
});

describe("the location actions", () => {
  it("opens each action from the cards and the tree for the location it belongs to", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    const card = () => within(screen.getByText(/^Home · property ·/).closest("[data-slot=card]") ?? document.body);
    fireEvent.click(card().getByRole("button", { name: "Rename" }));
    expect(dialog().getByText("Home p-home · Home · property")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(card().getByRole("button", { name: /Add floor$/ }));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Kind/).value).toBe("floor");
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(treeCard().getAllByRole("button", { name: "Location actions: Delete…" })[0] ?? document.body);
    expect(dialog().getByText("Delete Home p-home?")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    fireEvent.click(treeCard().getAllByRole("button", { name: "Location actions: Rename" })[1] ?? document.body);
    expect(dialog().getByText("floor s-floor · Floor")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    rerender(<UnitsView live={liveOf(customerOf("p-office"))} />);
    fireEvent.click(within(screen.getByText(/^Office · property ·/).closest("[data-slot=card]") ?? document.body).getByRole("button", { name: /Add floor \/ area$/ }));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Kind/).value).toBe("area");
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    rerender(<UnitsView live={liveOf(customerOf("s-floor"))} />);
    fireEvent.click(within(screen.getByText(/^Floor · .*· 1 unit$/).closest("[data-slot=card]") ?? document.body).getByRole("button", { name: /Add room$/ }));
    expect([dialog().getByLabelText<HTMLSelectElement>(/^Kind/).value, dialog().getByLabelText<HTMLSelectElement>(/^Parent/).value]).toStrictEqual(["room", "s-floor"]);
  });

  it("deletes an empty property, and keeps one connection filter chosen", async () => {
    actions.archiveLocation.mockResolvedValue(ok());
    const emptyTree = locationTree([property("p-empty", "office")], [], [], "org-a");
    const { rerender } = render(<UnitsView live={liveOf({ ...customerOf(undefined), tree: emptyTree, selection: selection(emptyTree, "p-empty", []), units: [], total: 0 })} />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.change(dialog().getByLabelText(/^Reason/), { target: { value: "Closed" } });
    fireEvent.click(dialog().getByRole("button", { name: "Delete property" }));
    await flush();
    expect(actions.archiveLocation).toHaveBeenCalledWith("property", "p-empty", 2, "Closed");
    rerender(<UnitsView live={liveOf(customerOf("p-home", { filter: { connections: ["offline"], search: "" } }))} />);
    expect(screen.getByLabelText<HTMLSelectElement>("Connection").value).toBe("offline");
  });
});

describe("properties", () => {
  it("adds a property with its pin and radius, checking both", async () => {
    actions.saveProperty.mockResolvedValue(ok("p-new"));
    render(<UnitsView live={liveOf(customerOf(undefined))} />);
    fireEvent.click(button(/Add property/));
    fireEvent.click(dialog().getByRole("button", { name: "Office" }));
    fireEvent.change(dialog().getByLabelText(/^Name/), { target: { value: "HQ tower" } });
    fireEvent.click(dialog().getByRole("button", { name: "Bad coordinates" }));
    fireEvent.change(dialog().getByLabelText(/^Away\/home radius/), { target: { value: "10" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add property" }));
    expect(dialog().getByText("Latitude −90 to 90 and longitude −180 to 180, in degrees")).toBeDefined();
    expect(dialog().getByText("A whole number of metres, 50–2000")).toBeDefined();
    fireEvent.click(dialog().getByRole("button", { name: "Drop pin" }));
    fireEvent.change(dialog().getByLabelText(/^Away\/home radius/), { target: { value: "" } });
    fireEvent.change(dialog().getByLabelText(/^Address/), { target: { value: "9 Jalan Demo" } });
    fireEvent.change(dialog().getByLabelText(/^Access instructions/), { target: { value: "Lobby desk" } });
    fireEvent.click(dialog().getByRole("button", { name: "Add property" }));
    await flush();
    expect(actions.saveProperty).toHaveBeenCalledWith(expect.objectContaining({ customerOrgId: "org-a", kind: "office", name: "HQ tower", address: "9 Jalan Demo", accessInstructions: "Lobby desk", location: { latitude: 3.15, longitude: 101.7 }, geofenceRadiusMeters: null }), undefined);
    expect(nav.toasts).toStrictEqual(["Property added"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "p-new", unitId: null });
  });

  it("edits a property's details at its version, keeping its radius", async () => {
    actions.saveProperty.mockResolvedValue(ok("p-home"));
    render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    fireEvent.click(button("Edit details"));
    expect(dialog().getByLabelText<HTMLInputElement>(/^Away\/home radius/).value).toBe("300");
    fireEvent.change(dialog().getByLabelText(/^Away\/home radius/), { target: { value: "500" } });
    fireEvent.click(dialog().getByRole("button", { name: "Save property" }));
    await flush();
    expect(actions.saveProperty).toHaveBeenCalledWith(expect.objectContaining({ id: "p-home", geofenceRadiusMeters: 500 }), 2);
    expect(nav.toasts).toStrictEqual(["Property saved"]);
  });

  it("asks for the first property of a customer without one", () => {
    const empty = { ...customerOf(undefined), tree: [], selection: null };
    const { rerender } = render(<UnitsView live={liveOf(empty)} />);
    expect(screen.getByText("Add the customer’s home or office first.")).toBeDefined();
    expect(screen.queryByRole("button", { name: /New unit/ })).toBeNull(); // nowhere to put one
    rerender(<UnitsView live={liveOf(empty, { canWrite: false })} />);
    expect(screen.getByText("HQ has not registered a property yet.")).toBeDefined();
  });
});

describe("the units card", () => {
  it("searches the units, filters power and connection in the URL, clears the filters, and opens a unit", () => {
    const { rerender } = render(<UnitsView live={liveOf(customerOf("p-home"))} />);
    fireEvent.change(screen.getByPlaceholderText("Search unit name or ID…"), { target: { value: "u-2" } });
    expect(screen.getByText(/^All units in Home p-home · 1 of 2$/)).toBeDefined();
    fireEvent.change(screen.getByLabelText("Power"), { target: { value: "off" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ powerState: "off" });
    fireEvent.change(screen.getByLabelText("Connection"), { target: { value: "offline" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ connections: "offline" });
    fireEvent.change(screen.getByLabelText("Power"), { target: { value: "" } });
    expect(nav.patch).toHaveBeenLastCalledWith({ powerState: null });
    fireEvent.click(screen.getAllByRole("row")[1] ?? document.body);
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: "u-2" });
    rerender(<UnitsView live={liveOf(customerOf("p-home", { filter: { powerState: "off", connections: ["offline", "error"], search: "" } }))} />);
    expect(screen.getByText("Filtered by Power: Stopped · Connection: Offline, Error — clear the filter to see all 2 units.")).toBeDefined();
    fireEvent.click(button("Clear filters"));
    expect(nav.patch).toHaveBeenLastCalledWith({ powerState: null, connections: null });
  });

  it("says when a location has no units", () => {
    const emptyTree = locationTree([property("p-empty", "home")], [], [], "org-a");
    const c = { ...customerOf(undefined), tree: emptyTree, selection: selection(emptyTree, "p-empty", []), units: [], total: 0 };
    const { rerender } = render(<UnitsView live={liveOf(c)} />);
    expect(screen.getByText("Register a unit for this location with + New unit.")).toBeDefined();
    rerender(<UnitsView live={liveOf(c, { canWrite: false })} />);
    expect(screen.getByText("No unit is registered at this location.")).toBeDefined();
  });

  it("closes the notice of a unit that is gone", () => {
    render(<UnitsView live={liveOf(customerOf("p-home", { unitMissing: true }))} />);
    expect(screen.getByText("This unit no longer exists or belongs to another customer.")).toBeDefined();
    fireEvent.click(button("Close"));
    expect(nav.patch).toHaveBeenLastCalledWith({ unitId: null });
  });
});

describe("a new unit", () => {
  it("checks and registers a unit without a device in the selected room", async () => {
    actions.saveUnit.mockResolvedValue(ok("u-new"));
    render(<UnitsView live={liveOf(customerOf("s-room"))} />);
    fireEvent.click(button(/New unit in room s-room/));
    expect(dialog().getByLabelText<HTMLSelectElement>(/^Room \/ space/).value).toBe("s-room");
    fireEvent.click(dialog().getByRole("checkbox", { name: "indoor" }));
    fireEvent.click(dialog().getByRole("checkbox", { name: "outdoor" })); // indoor and outdoor are the default
    fireEvent.click(dialog().getByRole("button", { name: "Register unit" }));
    expect(dialog().getByText("1–120 characters")).toBeDefined();
    expect(dialog().getByText("Choose at least one inspection group")).toBeDefined();
    fireEvent.click(dialog().getByRole("checkbox", { name: "outdoor" }));
    fireEvent.change(dialog().getByLabelText(/^Aircon model/), { target: { value: "m-1" } });
    fireEvent.change(dialog().getByLabelText(/^Display name/), { target: { value: " Bedroom AC " } });
    fireEvent.change(dialog().getByLabelText(/^Installed at/), { target: { value: "2026-09-01" } });
    fireEvent.change(dialog().getByLabelText(/^Warranty end/), { target: { value: "2027-09-01" } });
    fireEvent.click(dialog().getByRole("button", { name: "Register unit" }));
    await flush();
    expect(actions.saveUnit).toHaveBeenCalledWith(expect.objectContaining({ customerOrgId: "org-a", propertyId: "p-home", spaceId: "s-room", displayName: "Bedroom AC", modelId: "m-1", serviceScope: ["outdoor"] }));
    expect(nav.toasts).toStrictEqual(["Unit “Bedroom AC” registered"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "s-room", unitId: "u-new" });
  });

  it("plans an installation from the header, with a device, and says when the device was not connected", async () => {
    actions.saveUnitWithDevice.mockResolvedValueOnce(ok({ id: "u-a", deviceError: null })).mockResolvedValueOnce(ok({ id: "u-b", deviceError: "serial: taken" }));
    render(<UnitsView live={liveOf(customerOf(undefined))} />);
    for (const name of ["Office AC", "Spare AC"]) {
      fireEvent.click(button(/New unit/));
      fireEvent.change(dialog().getByLabelText(/^Location/), { target: { value: "p-office" } });
      fireEvent.change(dialog().getByLabelText(/^Room \/ space/), { target: { value: "s-area" } });
      fireEvent.click(dialog().getByRole("button", { name: "Installation planned" }));
      expect(dialog().getByLabelText<HTMLInputElement>(/^Installed at/).disabled).toBe(true);
      fireEvent.change(dialog().getByLabelText(/^Display name/), { target: { value: name } });
      fireEvent.click(dialog().getByRole("button", { name: "Start a device" }));
      fireEvent.click(dialog().getByRole("button", { name: "Register unit" }));
      await flush();
      expect(actions.saveUnitWithDevice).toHaveBeenCalledTimes(name === "Office AC" ? 0 : 1); // a device started but not chosen stops the save
      fireEvent.click(dialog().getByRole("button", { name: "Use device d-1" }));
      fireEvent.click(dialog().getByRole("button", { name: "Register unit" }));
      await flush();
    }
    expect(actions.saveUnitWithDevice).toHaveBeenLastCalledWith(expect.objectContaining({ propertyId: "p-office", spaceId: "s-area", displayName: "Spare AC" }), { kind: "existing", deviceId: "d-1", version: 2 });
    expect(nav.toasts).toStrictEqual(["Unit “Office AC” registered with its device", "Unit “Spare AC” registered, but its device was not connected: serial: taken — connect it from the unit’s page"]);
    expect(nav.patch).toHaveBeenLastCalledWith({ locationId: "s-area", unitId: "u-b" });
  });

  it("says when no model can be read, and offers no device without device.write", () => {
    render(<UnitsView live={liveOf(customerOf("unassigned:p-home", { models: [], canRegisterDevice: false }))} />);
    fireEvent.click(button(/^New unit$/));
    expect(dialog().getByText("Models need “View devices & models”")).toBeDefined();
    expect(dialog().getByRole("option", { name: "no model available" })).toBeDefined();
    expect(dialog().queryByRole("button", { name: "Use device d-1" })).toBeNull();
    fireEvent.click(dialog().getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
