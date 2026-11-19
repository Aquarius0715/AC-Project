// /customer/properties (FR-C02, FR-C14, SCR-C02): in API mode a Server Component reads the customer's properties.list,
// spaces.list and units.list into the read-only tree (IR109) and, for the selected room, area or unassigned units, each
// unit's units.get (setting, room temperature, power draw, capability and restriction for group control). KPI links
// (powerState, connections) list the matching units. Renaming is locations.rename; group control sends commands.create
// per AC and changed setting and follows each command with commands.get through the BFF. URL keys: propertyId,
// spaceId, unassignedOnly, mode, unitIds, powerState, connections, search (a unit's name or place, IR801). Texts and times in the user's display language and
// time zone (IR263). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf, showDate } from "@ac/web/lib/i18n";
import {
  connections, filterUnits, flatten, unitsMatching, locationTree, powerStates, selection, spacePath, unitsAt,
  type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow, type Connection,
} from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { roomUnits } from "@ac/web/lib/clientProperties";
import { PropertiesDemo } from "./_components/properties-demo";
import { PropertiesView, type PropertiesLive } from "./_components/properties-view";
import { queryReader } from "@ac/web/lib/searchParams";

const PROPERTYID = "propertyId";

export default async function CustomerPropertiesPage({ searchParams }: Readonly<PageProps<"/customer/properties">>) {
  await connection();
  if (!apiMode()) return <PropertiesDemo />;
  const sp = await searchParams;
  const one = queryReader(sp);
  const [me, properties, spaces, units, display, now] = await Promise.all([
    corePrincipal(), coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"), coreAll<ApiUnitRow>("units.list"), coreDisplay(), coreNow(),
  ]);
  const i = i18nOf(display);
  const tree = locationTree(properties, spaces, units, properties[0]?.customerOrgId ?? "");
  const powerState = powerStates.find((p) => p === one("powerState"));
  const conns = (one("connections") ?? "").split(",").filter((c): c is Connection => (connections as string[]).includes(c));
  const search = (one("search") ?? "").trim();
  const isFiltered = !one("spaceId") && !one(PROPERTYID) && (!!powerState || conns.length > 0 || search.length > 0);
  const propertySel = one("unassignedOnly") === "true" && one(PROPERTYID) ? selection(tree, `unassigned:${one(PROPERTYID) ?? ""}`, spaces) : selection(tree, one(PROPERTYID), spaces);
  const sel = one("spaceId") ? selection(tree, one("spaceId"), spaces) : propertySel;
  // the units whose detail the panel needs: the KPI filter, a room / area (descendants included) or the property's units
  const selectedUnits = () => {
    if (!sel) return [];
    // a property lists all its units (those in rooms and the unassigned ones), as a floor or room lists those beneath it
    return sel.kind === "property" ? units.filter((u) => u.propertyId === sel.property.id) : unitsAt(sel, units);
  };
  const place = (u: ApiUnitRow) => [tree.find((p) => p.id === u.propertyId)?.name ?? "", spacePath(u.spaceId, spaces)].filter(Boolean).join(" › ");
  // the search matches the place too (IR801)
  const shown = isFiltered ? unitsMatching(filterUnits(units, { powerState, connections: conns, search: "" }), search, place) : selectedUnits();
  const details = await Promise.all(shown.map((u) => coreOp<ApiUnitDetail>("units.get", { id: u.id })));
  const placeOf = (id: string) => {
    const unit = units.find((u) => u.id === id);
    return unit ? place(unit) : "";
  };
  const p = sel?.property;
  const live: PropertiesLive = {
    tree,
    details,
    selection: sel,
    owner: me.clientRole === "owner",
    filter: isFiltered ? { search, powerState: powerState ?? null, connections: conns, total: units.length } : null,
    units: roomUnits(details, i, now.getTime()).map((r) => ({ ...r, place: placeOf(r.id) })),
    now: now.toISOString(),
    summary: p ? {
      kind: p.kind, floors: flatten(p.spaces).filter((s) => s.kind === "floor").map((s) => s.name), rooms: p.rooms, inRooms: p.units - p.unassigned, unassigned: p.unassigned,
      edited: i.t("{date} · version {n}", { date: showDate(p.updatedAt, display), n: p.version }),
    } : null,
    mode: one("mode") === "group" ? "group" : "single",
    selectedUnits: (one("unitIds") ?? "").split(",").filter(Boolean),
  };
  return <PropertiesView live={live} />;
}
