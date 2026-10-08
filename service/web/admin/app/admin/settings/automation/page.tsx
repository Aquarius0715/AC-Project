// /admin/settings/automation (FR-A11, SCR-A11): in API mode a Server Component reads policies.list (kind automation)
// for the URL scope customerId → propertyId → unitId, the policyId editor from policies.get (AT-A11-R01), units,
// customers, properties and capabilities. Saving, simulating and firing are Server Actions (actions.ts). The Phase 1A
// demo keeps the fixture policies.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp, corePermissions } from "@ac/web/lib/dal";
import type { ApiCustomer, ApiProperty } from "@ac/web/lib/billing";
import type { ApiCapability } from "@ac/web/lib/devices";
import { policyGroups, type ApiAutoPolicy } from "@ac/web/lib/automation";
import { AutomationDemo } from "./_components/automation-demo";
import { AutomationView } from "./_components/automation-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; customerOrgId: string; propertyId: string; modelId: string; archived: boolean };

export default async function AdminAutomationPage({ searchParams }: PageProps<"/admin/settings/automation">) {
  await connection();
  if (!apiMode()) return <AutomationDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const scope = { customerId: one("customerId"), propertyId: one("propertyId"), unitId: one("unitId") };
  const filters = { kind: "automation", ...(scope.customerId && { customerId: scope.customerId }), ...(scope.propertyId && { propertyId: scope.propertyId }), ...(scope.unitId && { unitId: scope.unitId }) };
  const [now, perms, policies, units, customers, properties, caps] = await Promise.all([
    coreNow(), corePermissions(),
    coreOp<Page<ApiAutoPolicy>>("policies.list", { limit: 100, filters }),
    coreOp<Page<Unit>>("units.list", { limit: 100 }),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiProperty>>("properties.list", { limit: 100 }),
    coreOp<Page<ApiCapability>>("capabilities.list", { limit: 100 }),
  ]);
  const customerOfOrg = new Map(customers.items.map((c) => [c.organizationId, c]));
  const customerOfUnit = new Map(units.items.map((u) => [u.id, customerOfOrg.get(u.customerOrgId)?.name ?? "unknown customer"]));
  const capOf = new Map(caps.items.map((k) => [k.id, k]));
  const id = one("policyId") === "new" ? "new" : policies.items.find((p) => p.id === one("policyId"))?.id ?? policies.items[0]?.id ?? "new";
  const selected = id === "new" ? null : await coreOp<ApiAutoPolicy>("policies.get", { policyId: id });
  return (
    <AutomationView live={{
      now: now.toISOString(), canWrite: perms.has("automation.policy.write"), scope, groups: policyGroups(policies.items, customerOfUnit), selected, isNew: id === "new",
      customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
      properties: properties.items.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name, customerId: customerOfOrg.get(p.customerOrgId)?.id ?? "" })),
      units: units.items.filter((u) => !u.archived).map((u) => {
        const k = capOf.get(u.modelId);
        return { id: u.id, name: u.displayName, customerId: customerOfOrg.get(u.customerOrgId)?.id ?? "", propertyId: u.propertyId, temperature: k?.temperature ?? null, ventilation: k?.ventilation ?? false };
      }),
    }} />
  );
}
