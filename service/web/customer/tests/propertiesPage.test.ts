import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";

// Units & locations of the customer app (FR-C02, FR-C14): the demo outside API mode; otherwise the customer's tree with
// the selected property, space (descendants included) or unassigned units and each one's units.get, or — with no
// location chosen — the KPI filter (power, connection, a search of name or place, IR801). renameLocation trims the
// name and re-reads the page also when refused; sendCommand creates each command at the unit's current version.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return { coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(), apiMode: vi.fn(), coreAll: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), coreNow: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/properties/_components/properties-demo", () => ({ PropertiesDemo: () => null }));
vi.mock("../app/customer/properties/_components/properties-view", () => ({ PropertiesView: () => null }));

import { renameLocation, sendCommand } from "../app/customer/properties/actions";
import CustomerPropertiesPage from "../app/customer/properties/page";
import { PropertiesDemo } from "../app/customer/properties/_components/properties-demo";
import { PropertiesView } from "../app/customer/properties/_components/properties-view";
import PropertiesLoading from "../app/customer/properties/loading";
import PropertiesErrorState from "../app/customer/properties/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const NOW = new Date("2026-09-15T01:12:30Z");
const property = (id: string, name: string, kind: "home" | "office", customerOrgId = "org-a"): ApiPropertyRow => ({
  id, name, kind, customerOrgId, version: 3, address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-12T02:00:00Z",
});
const space = (id: string, name: string, kind: ApiSpaceRow["kind"], parentSpaceId: string | null): ApiSpaceRow => ({ id, propertyId: "p-home", name, version: 1, parentSpaceId, kind, archived: false });
const unit = (id: string, displayName: string, spaceId: string | null, over: Partial<ApiUnitRow> = {}): ApiUnitRow => ({
  id, displayName, spaceId, version: 1, customerOrgId: "org-a", propertyId: "p-home", modelId: "cap-1", type: "split", installState: "installed", installedAt: null, serviceScope: [],
  alertPolicyIds: [], archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0, ...over,
});
const detailOf = (u: ApiUnitRow): ApiUnitDetail => ({
  id: u.id, displayName: u.displayName, version: 4, spaceId: u.spaceId, alertPolicyIds: [], connection: u.connection, effectivePowerState: u.effectivePowerState,
  observedState: { power: true, celsius: 24, mode: null, fanLevel: null, observedAt: null }, lastSeenAt: null, latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKF", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: [], address: null, accessInstructions: null }, installedAt: null, installState: "installed", modelId: "cap-1", capabilityVersion: 1, serviceScope: [], components: [],
});
// Home: 1F › Bedroom with two ACs, a hall area on 1F with one, and an unassigned porch AC; Office has no unit, and
// another organization's property is left out of the tree, so its unit has no place
const units = [
  unit("u-bed1", "Bedroom AC", "s-bed"), unit("u-bed2", "Bedroom AC 2", "s-bed", { effectivePowerState: "off" }),
  unit("u-hall", "Hall AC", "s-hall", { connection: "offline", effectivePowerState: "unknown" }), unit("u-porch", "Porch AC", null, { connection: "error", effectivePowerState: "unknown" }),
  unit("u-old", "Old shop AC", null, { propertyId: "p-other", connection: "unknown", effectivePowerState: "off" }),
];
const spaces = [space("s-1f", "1F", "floor", null), space("s-bed", "Bedroom", "room", "s-1f"), space("s-hall", "Hall", "area", "s-1f")];

/** the id of an op's input ({ id }), "" for another input */
const idOf = (input: unknown) => (typeof input === "object" && input !== null && "id" in input && typeof input.id === "string" ? input.id : "");
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof PropertiesView>>(element) || element.type !== PropertiesView) throw new TypeError("not PropertiesView");
  return element.props.live;
};
const open = async (searchParams: Record<string, string>) => viewProps(await CustomerPropertiesPage({ searchParams: Promise.resolve(searchParams), params: Promise.resolve({}) }));
/** the ids of the units the page read units.get for */
const read = () => dal.coreOp.mock.calls.filter((c) => c[0] === "units.get").map((c) => idOf(c[1]));
const refusal = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, messageKey, fieldErrors, retryAfterSeconds: null });

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.corePrincipal.mockResolvedValue({ membershipId: "m-me", clientRole: "owner" });
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  const lists = new Map<string, unknown[]>([
    ["properties.list", [property("p-office", "Office", "office"), property("p-home", "Home", "home"), property("p-other", "Another", "home", "org-b")]],
    ["spaces.list", spaces], ["units.list", units],
  ]);
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
  dal.coreOp.mockImplementation((op: string, input: unknown) => {
    const u = units.find((x) => x.id === idOf(input));
    return op === "units.get" && u ? Promise.resolve(detailOf(u)) : Promise.reject(new Error(`unexpected ${op}`));
  });
});

describe("renameLocation", () => {
  it("sends the trimmed name at the row's version and re-reads; a refusal comes back with its fields, also after a re-read", async () => {
    dal.coreOp.mockResolvedValueOnce({});
    await expect(renameLocation("space", "s-bed", 2, "  Main bedroom ")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["locations.rename", { target: { kind: "space", id: "s-bed" }, name: "Main bedroom" }, { write: true, expectedVersion: 2 }]);
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT", "error.duplicateSiblingName", { name: "duplicate" }));
    await expect(renameLocation("property", "p-home", 3, "Office")).resolves.toMatchObject({ ok: false, code: "CONFLICT", messageKey: "error.duplicateSiblingName", fieldErrors: { name: "duplicate" } });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("sendCommand", () => {
  it("creates the command at the unit's current version; a refused read sends nothing", async () => {
    const command = { id: "c-1", status: "pending" };
    dal.coreOp.mockResolvedValueOnce(detailOf(units[0] ?? unit("x", "x", null))).mockResolvedValueOnce(command);
    await expect(sendCommand("u-bed1", { kind: "set_power", power: false })).resolves.toStrictEqual({ ok: true, value: command });
    expect(dal.coreOp.mock.calls).toStrictEqual([["units.get", { id: "u-bed1" }], ["commands.create", { unitId: "u-bed1", action: { kind: "set_power", power: false }, expectedUnitVersion: 4 }, { write: true }]]);
    dal.coreOp.mockClear();
    dal.coreOp.mockRejectedValueOnce(refusal("NOT_FOUND", "error.notFound"));
    await expect(sendCommand("u-gone", { kind: "set_power", power: true })).resolves.toMatchObject({ ok: false, code: "NOT_FOUND" });
    expect(dal.coreOp.mock.calls.map((c) => c[0])).toStrictEqual(["units.get"]);
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("properties page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await CustomerPropertiesPage({ searchParams: Promise.resolve({}), params: Promise.resolve({}) });
    expect(isValidElement(page) && page.type).toBe(PropertiesDemo);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("without a location, selects the first property by name with all of its units and their places", async () => {
    const live = await open({});
    expect(live.tree.map((p) => p.name)).toStrictEqual(["Home", "Office"]);
    expect(live.selection).toMatchObject({ kind: "property", property: { id: "p-home" } });
    expect(read()).toStrictEqual(["u-bed1", "u-bed2", "u-hall", "u-porch"]);
    expect(live.summary).toStrictEqual({ kind: "home", floors: ["1F"], rooms: 1, inRooms: 3, unassigned: 1, edited: "12 Sept 2026 · version 3" });
    expect(live.units.map((u) => [u.name, u.place])).toStrictEqual([["Bedroom AC", "Home › 1F › Bedroom"], ["Bedroom AC 2", "Home › 1F › Bedroom"], ["Hall AC", "Home › 1F › Hall"], ["Porch AC", "Home"]]);
    expect(live).toMatchObject({ owner: true, filter: null, mode: "single", selectedUnits: [], now: NOW.toISOString() });
  });

  it("a floor lists the units beneath it, in group mode with the chosen units; a member is no owner", async () => {
    dal.corePrincipal.mockResolvedValue({ membershipId: "m-me", clientRole: "member" });
    const live = await open({ spaceId: "s-1f", propertyId: "p-office", mode: "group", unitIds: "u-bed1,,u-bed2" });
    expect(live.selection).toMatchObject({ kind: "space", space: { id: "s-1f" }, path: "Home › 1F" });
    expect(read()).toStrictEqual(["u-bed1", "u-bed2", "u-hall"]);
    expect(live).toMatchObject({ owner: false, mode: "group", selectedUnits: ["u-bed1", "u-bed2"], filter: null });
    expect(live.summary?.kind).toBe("home");
  });

  it("unassignedOnly lists a property's units outside rooms, and only with its property; an unknown mode is single", async () => {
    const live = await open({ propertyId: "p-home", unassignedOnly: "true", mode: "grid" });
    expect(live.selection).toMatchObject({ kind: "unassigned", property: { id: "p-home" } });
    expect(read()).toStrictEqual(["u-porch"]);
    expect(live.mode).toBe("single");
    dal.coreOp.mockClear();
    expect((await open({ unassignedOnly: "true" })).selection).toMatchObject({ kind: "property", property: { id: "p-home" } });
    expect((await open({ propertyId: "p-office" })).units).toStrictEqual([]);
  });

  it("the KPI filter lists the matching units across properties; a search matches a place; a location turns it off", async () => {
    const running = await open({ powerState: "on" });
    expect(running.filter).toStrictEqual({ search: "", powerState: "on", connections: [], total: 5 });
    expect(read()).toStrictEqual(["u-bed1"]);
    dal.coreOp.mockClear();
    const conns = await open({ connections: "offline,bogus,error", powerState: "sideways" });
    expect(conns.filter).toStrictEqual({ search: "", powerState: null, connections: ["offline", "error"], total: 5 });
    expect(read()).toStrictEqual(["u-hall", "u-porch"]);
    dal.coreOp.mockClear();
    const found = await open({ search: "  1f " });
    expect(found.filter?.search).toBe("1f");
    expect(read()).toStrictEqual(["u-bed1", "u-bed2", "u-hall"]);
    dal.coreOp.mockClear();
    expect((await open({ search: "old shop" })).units.map((u) => [u.id, u.place])).toStrictEqual([["u-old", ""]]);
    dal.coreOp.mockClear();
    expect((await open({ search: "porch", spaceId: "s-bed" })).filter).toBeNull();
    expect(read()).toStrictEqual(["u-bed1", "u-bed2"]);
  });

  it("an empty tree selects nothing and reads no unit", async () => {
    dal.coreAll.mockResolvedValue([]);
    const live = await open({});
    expect(live).toMatchObject({ tree: [], selection: null, summary: null, units: [], details: [] });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = PropertiesLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "units & locations" });
    const failed = PropertiesErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Units & locations", error, retry });
  });
});
