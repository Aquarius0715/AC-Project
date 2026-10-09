// /customer/properties (FR-C02, FR-C14, SCR-C02): in API mode a Server Component reads the customer's properties.list,
// spaces.list and units.list into the read-only tree (IR109) and, for the selected room, area or unassigned units, each
// unit's units.get (setting, room temperature, power draw, capability and restriction for group control). KPI links
// (powerState, connections) list the matching units. Renaming is locations.rename; group control sends commands.create
// per AC and changed setting and follows each command with commands.get through the BFF. URL keys: propertyId,
// spaceId, unassignedOnly, mode, unitIds, powerState, connections. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreOp, corePrincipal } from "@ac/web/lib/dal";
import { klStamp } from "@ac/web/lib/energy";
import {
  connections, filterUnits, flatten, locationTree, powerStates, selection, spacePath, unitsAt,
  type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow, type Connection,
} from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { roomUnits } from "@ac/web/lib/clientProperties";
import { PropertiesDemo } from "./_components/properties-demo";
import { PropertiesView, type PropertiesLive } from "./_components/properties-view";

export default async function CustomerPropertiesPage({ searchParams }: PageProps<"/customer/properties">) {
  await connection();
  if (!apiMode()) return <PropertiesDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [me, properties, spaces, units] = await Promise.all([
    corePrincipal(), coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"), coreAll<ApiUnitRow>("units.list"),
  ]);
  const tree = locationTree(properties, spaces, units, properties[0]?.customerOrgId ?? "");
  const powerState = powerStates.find((p) => p === one("powerState"));
  const conns = (one("connections") ?? "").split(",").filter((c): c is Connection => (connections as string[]).includes(c));
  const filtered = !one("spaceId") && !one("propertyId") && (!!powerState || conns.length > 0);
  const sel = one("spaceId") ? selection(tree, one("spaceId"), spaces)
    : one("unassignedOnly") === "true" && one("propertyId") ? selection(tree, `unassigned:${one("propertyId")}`, spaces)
    : selection(tree, one("propertyId"), spaces);
  // the units whose detail the panel needs: the KPI filter, a room / area (descendants included) or the property's unassigned units
  const shown = filtered ? filterUnits(units, { powerState, connections: conns, search: "" })
    : !sel ? [] : sel.kind === "property" ? units.filter((u) => u.propertyId === sel.property.id && u.spaceId === null) : unitsAt(sel, units);
  const details = await Promise.all(shown.map((u) => coreOp<ApiUnitDetail>("units.get", { id: u.id })));
  const place = (u: ApiUnitRow) => [tree.find((p) => p.id === u.propertyId)?.name ?? "", spacePath(u.spaceId, spaces)].filter(Boolean).join(" › ");
  const p = sel?.property;
  const live: PropertiesLive = {
    tree, selection: sel, owner: me.clientRole === "owner", filter: filtered ? { powerState: powerState ?? null, connections: conns, total: units.length } : null,
    units: roomUnits(details).map((r) => ({ ...r, place: place(units.find((u) => u.id === r.id)!) })), details,
    summary: p ? {
      kind: p.kind, floors: flatten(p.spaces).filter((s) => s.kind === "floor").map((s) => s.name), rooms: p.rooms, inRooms: p.units - p.unassigned, unassigned: p.unassigned,
      edited: `${klStamp(p.updatedAt).slice(0, 10)} · version ${p.version}`,
    } : null,
    mode: one("mode") === "group" ? "group" : "single", selectedUnits: (one("unitIds") ?? "").split(",").filter(Boolean),
  };
  return <PropertiesView live={live} />;
}
