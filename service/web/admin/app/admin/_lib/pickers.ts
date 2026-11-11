import "server-only";
import { coreAll, coreOp } from "@ac/web/lib/dal";
import type { ApiCustomerRow, ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import type { PickerOption } from "@ac/web/lib/pickerOption";
import type { ApiCapability, ApiDevice, ApiIoTModel } from "@ac/web/lib/devices";

const LIMIT = 20;

interface Page<T> { items: T[] }

const names = <T extends { id: string }>(rows: T[], name: (r: T) => string) => new Map(rows.map((r) => [r.id, name(r)]));

/** Units matching `search` (name, customer, property, room, aircon model or the serial of its device) in HQ's scope, a
page at a time: the customer, the place and the aircon model are written beside each name, so units of the same name
in different places are told apart. With `withoutDevice` only the units that have no device bound are listed. */
export async function unitOptions(search: string, withoutDevice: boolean): Promise<PickerOption[]> {
  const filters = { ...(search.trim() && { search: search.trim() }), ...(withoutDevice && { withoutDevice: true }) };
  const page = await coreOp<Page<ApiUnitRow>>("units.list", { limit: LIMIT, sort: { field: "id", direction: "asc" }, filters });
  const units = page.items.filter((u) => !u.archived);
  const [customers, caps, props, spaces] = await Promise.all([
    coreAll<ApiCustomerRow>("customers.list"),
    coreAll<ApiCapability>("capabilities.list"),
    Promise.all([...new Set(units.map((u) => u.customerOrgId))].map((id) => coreAll<ApiPropertyRow>("properties.list", { filters: { customerId: id } }))).then((x) => x.flat()),
    Promise.all([...new Set(units.map((u) => u.propertyId))].map((id) => coreAll<ApiSpaceRow>("spaces.list", { filters: { propertyId: id } }))).then((x) => x.flat()),
  ]);
  const customerName = new Map(customers.map((c) => [c.organizationId, c.name]));
  const modelName = names(caps, (c) => `${c.manufacturer} ${c.model}`);
  const propertyName = names(props, (p) => p.name);
  const spaceName = names(spaces, (s) => s.name);
  return units.map((u) => ({
    id: u.id, version: u.version, unitId: u.id, blocked: null, label: u.displayName,
    detail: [customerName.get(u.customerOrgId), [propertyName.get(u.propertyId), u.spaceId ? spaceName.get(u.spaceId) : null].filter(Boolean).join(" › "), modelName.get(u.modelId)].filter(Boolean).join(" · "),
  }));
}

/** Devices matching `search` (serial, MQTT node ID, IoT model, or the unit's name, customer, property, room or aircon
model), a page at a time. `bound` narrows to the devices on a unit (true) or not on any (false). The serial, the IoT
model, the node ID and the unit a device is on are written beside it; one that is on a unit says so (`blocked`) when
`free` asks for devices that are not. */
export async function deviceOptions(search: string, bound: boolean | null, free: boolean): Promise<PickerOption[]> {
  const filters = { ...(search.trim() && { search: search.trim() }), ...(bound !== null && { bound }) };
  const page = await coreOp<Page<ApiDevice>>("devices.list", { limit: LIMIT, sort: { field: "serial", direction: "asc" }, filters });
  const unitIds = [...new Set(page.items.map((d) => d.unitId).filter((id): id is string => id !== null))];
  const [models, units] = await Promise.all([
    coreAll<ApiIoTModel>("iotModels.list"),
    unitIds.length > 0 ? coreOp<Page<ApiUnitRow>>("units.list", { limit: 100, filters: { unitIds } }).then((p) => p.items) : Promise.resolve([] as ApiUnitRow[]),
  ]);
  const modelName = names(models, (m) => `${m.manufacturer} ${m.model}`);
  const unitName = names(units, (u) => u.displayName);
  return page.items.map((d) => {
    const on = d.unitId ? unitName.get(d.unitId) ?? d.unitId : null;
    return {
      id: d.id, version: d.version, unitId: d.unitId, label: d.serial,
      detail: [d.iotModelId ? modelName.get(d.iotModelId) : null, d.mqttNode, on].filter(Boolean).join(" · "),
      blocked: free && on ? on : null,
    };
  });
}
