/** The HQ scope selects (Jobs, Plans, Alerts, Alert policies): customers and properties by name, and the units grouped
"customer › property" in that order, so same-named units in different places stay apart — as the automation policy
form groups them (UI/UX loop R127). The lists narrow to the chosen customer and property. */
export interface ScopeCustomer { id: string; name: string; organizationId: string }
export interface ScopeProperty { id: string; name: string; customerOrgId: string }
export interface ScopeUnit { id: string; displayName: string; customerOrgId: string; propertyId: string }
interface UnitGroup { key: string; label: string; units: { id: string; name: string }[] }
export interface ScopeLists { customers: { id: string; name: string }[]; properties: { id: string; name: string }[]; unitGroups: UnitGroup[] }

const byName = <T extends { name: string }>(a: T, b: T) => a.name.localeCompare(b.name);

export function scopeLists(customers: ScopeCustomer[], properties: ScopeProperty[], units: ScopeUnit[], q: { customerId?: string | null; propertyId?: string | null }): ScopeLists {
  const custOrg = customers.find((c) => c.id === q.customerId)?.organizationId;
  const customerName = new Map(customers.map((c) => [c.organizationId, c.name]));
  const propertyName = new Map(properties.map((p) => [p.id, p.name]));
  const groups = new Map<string, UnitGroup>();
  for (const u of units) {
    if ((custOrg && u.customerOrgId !== custOrg) || (q.propertyId && u.propertyId !== q.propertyId)) continue;
    const g = groups.get(u.propertyId) ?? { key: u.propertyId, label: [customerName.get(u.customerOrgId), propertyName.get(u.propertyId)].filter(Boolean).join(" › "), units: [] };
    g.units.push({ id: u.id, name: u.displayName });
    groups.set(u.propertyId, g);
  }
  return {
    customers: customers.map((c) => ({ id: c.id, name: c.name })).toSorted(byName),
    properties: properties.filter((p) => !custOrg || p.customerOrgId === custOrg).map((p) => ({ id: p.id, name: p.name })).toSorted(byName),
    unitGroups: [...groups.values()].toSorted((a, b) => a.label.localeCompare(b.label)),
  };
}
