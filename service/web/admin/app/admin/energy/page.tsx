// /admin/energy (FR-A13, SCR-A13): in API mode a Server Component reads customers, properties, units and baselines, and
// on the analysis tab energy.summary for the URL scope (customerId, unitIds, from/to in Kuala Lumpur time, baselineId;
// a selected baseline supplies the default unit set and period); on the baselines tab baselines.list for the scope
// customerId → propertyId → unitId and period. Saving a baseline version is a Server Action (actions.ts). The Phase 1A
// demo keeps the fixture baselines.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp, CoreError, corePermissions } from "@ac/web/lib/dal";
import { actionMessage } from "@ac/web/lib/actionMessage";
import type { ApiCustomer, ApiProperty } from "@ac/web/lib/billing";
import { baselineRows, klInstant, klLocal, summaryView, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import { EnergyDemo } from "./_components/energy-demo";
import { EnergyView, type EnergyLive } from "./_components/energy-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; customerOrgId: string; propertyId: string; archived: boolean };
const minute = (d: Date) => new Date(Math.floor(d.getTime() / 60000) * 60000);

export default async function AdminEnergyPage({ searchParams }: PageProps<"/admin/energy">) {
  await connection();
  if (!apiMode()) return <EnergyDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "baselines" ? "baselines" : "analysis";
  const [now, perms, customers, properties, units, all] = await Promise.all([
    coreNow(), corePermissions(),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiProperty>>("properties.list", { limit: 100 }),
    coreOp<Page<Unit>>("units.list", { limit: 100 }),
    coreOp<Page<ApiBaseline>>("baselines.list", { limit: 100 }),
  ]);
  const customerOfOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  const unitOptions = units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, customerId: customerOfOrg.get(u.customerOrgId) ?? "", propertyId: u.propertyId }));
  const scope = { customerId: one("customerId"), propertyId: one("propertyId"), unitId: one("unitId") };
  const live: EnergyLive = {
    tab, canWrite: perms.has("energy.write"), scope, units: unitOptions,
    customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
    properties: properties.items.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name, customerId: customerOfOrg.get(p.customerOrgId) ?? "" })),
    baselines: baselineRows(all.items),
  };
  if (tab === "analysis") {
    const baseline = all.items.find((b) => b.id === one("baselineId"));
    const unitIds = one("unitIds")?.split(",").filter(Boolean) ?? baseline?.unitIds ?? unitOptions.filter((u) => !scope.customerId || u.customerId === scope.customerId).map((u) => u.id);
    const today = klLocal(now.toISOString()).slice(0, 10);
    const from = one("from") ? klInstant(one("from")!) : baseline?.period.from ?? klInstant(`${today}T00:00`);
    const to = one("to") ? klInstant(one("to")!) : baseline?.period.to ?? minute(now).toISOString();
    let summary: ApiEnergySummary | null = null;
    let error: string | undefined;
    if (unitIds.length > 0) {
      try {
        summary = await coreOp<ApiEnergySummary>("energy.summary", { from, to, unitIds, ...(baseline ? { baselineId: baseline.id } : {}) });
      } catch (e) {
        if (!(e instanceof CoreError) || e.error.code !== "VALIDATION") throw e; // an unusable period or unit set shows next to the form
        error = actionMessage({ code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors ?? {} });
      }
    }
    live.analysis = { unitIds, from: klLocal(from), to: klLocal(to), baselineId: baseline?.id ?? null, summary: summary ? summaryView(summary) : null, error };
  } else {
    const filters = {
      ...(scope.customerId && { customerId: scope.customerId }), ...(scope.propertyId && { propertyId: scope.propertyId }), ...(scope.unitId && { unitId: scope.unitId }),
      ...(one("from") && { from: klInstant(one("from")!) }), ...(one("to") && { to: klInstant(one("to")!) }),
    };
    const scoped = Object.keys(filters).length ? (await coreOp<Page<ApiBaseline>>("baselines.list", { limit: 100, filters })).items : all.items;
    const rows = baselineRows(scoped);
    live.list = { rows, from: one("from") ?? "", to: one("to") ?? "", selectedId: rows.find((r) => r.id === one("baselineId"))?.id ?? rows[0]?.id ?? null };
  }
  return <EnergyView live={live} />;
}
