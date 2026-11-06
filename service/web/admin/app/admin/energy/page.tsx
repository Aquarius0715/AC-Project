// /admin/energy (FR-A13, SCR-A13): in API mode a Server Component reads customers, properties, units and baselines, and
// on the analysis tab energy.summary for the URL scope (customerId, unitIds, from/to in Kuala Lumpur time, baselineId;
// a selected baseline supplies the default unit set and period); on the baselines tab baselines.list for the scope
// customerId → propertyId → unitId and period. Saving a baseline version is a Server Action (actions.ts). Texts in the
// display language; the analysis and baseline periods are Kuala Lumpur business time and say so (IR296). The Phase 1A
// demo keeps the fixture baselines.
import { connection } from "next/server";
import { coreAll, apiMode, coreDisplay, coreNow, coreOp, CoreError, corePermissions, coreAllPage } from "@ac/web/lib/dal";
import { actionMessage } from "@ac/web/lib/actionMessage";
import type { ApiCustomer, ApiProperty } from "@ac/web/lib/billing";
import { baselineRows, klInstant, klLocal, summaryView, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import { i18nOf, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import { withAccess } from "@ac/web/lib/withAccess";
import { EnergyDemo } from "./_components/energy-demo";
import { EnergyView, type EnergyLive } from "./_components/energy-view";
import { queryReader } from "@ac/web/lib/searchParams";

const minute = (d: Date) => new Date(Math.floor(d.getTime() / 60_000) * 60_000);

// a role without the page's read permission sees which one it needs, not a failure (IR582)
/** energy.summary for the analysis form; an unusable period or unit set comes back as the message shown next to the form. */
async function summaryOf(input: OpInput<"energy.summary">, t: T): Promise<{ summary: ApiEnergySummary | null; error?: string }> {
  try {
    return { summary: await coreOp<ApiEnergySummary>("energy.summary", input) };
  } catch (caught) {
    if (!(caught instanceof CoreError) || caught.error.code !== "VALIDATION") throw caught;
    return { summary: null, error: actionMessage(caught.error, t) };
  }
}

export default async function AdminEnergyPage(props: PageProps<"/admin/energy">) {
  return withAccess("Energy analysis", ["energy.read", "asset.read"], () => AdminEnergyPageContent(props));
}

async function AdminEnergyPageContent({ searchParams }: Readonly<PageProps<"/admin/energy">>) {
  interface Unit { id: string; displayName: string; customerOrgId: string; propertyId: string; archived: boolean }
  await connection();
  if (!apiMode()) return <EnergyDemo />;
  const sp = await searchParams;
  const one = queryReader(sp);
  const fromQ = one("from"), toQ = one("to"); // the period asked in the URL
  const tab = one("tab") === "baselines" ? "baselines" : "analysis";
  const [now, perms, display, customers, properties, units, all] = await Promise.all([
    coreNow(), corePermissions(), coreDisplay(),
    coreAllPage<ApiCustomer>("customers.list"),
    coreAllPage<ApiProperty>("properties.list"),
    coreAllPage<Unit>("units.list"),
    coreAllPage<ApiBaseline>("baselines.list"),
  ]);
  const i = i18nOf(display);
  const customerOfOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  const unitOptions = units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, customerId: customerOfOrg.get(u.customerOrgId) ?? "", propertyId: u.propertyId }));
  const scope = { customerId: one("customerId"), propertyId: one("propertyId"), unitId: one("unitId") };
  const live: EnergyLive = {
    tab,
    scope,
    canWrite: perms.has("energy.write"),
    units: unitOptions,
    customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
    properties: properties.items.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name, customerId: customerOfOrg.get(p.customerOrgId) ?? "" })),
    baselines: baselineRows(all.items, i.t),
  };
  if (tab === "analysis") {
    const baseline = all.items.find((b) => b.id === one("baselineId"));
    const unitIds = one("unitIds")?.split(",").filter(Boolean) ?? baseline?.unitIds ?? unitOptions.filter((u) => !scope.customerId || u.customerId === scope.customerId).map((u) => u.id);
    const today = klLocal(now.toISOString()).slice(0, 10);
    const from = fromQ ? klInstant(fromQ) : baseline?.period.from ?? klInstant(`${today}T00:00`);
    const to = toQ ? klInstant(toQ) : baseline?.period.to ?? minute(now).toISOString();
    const { summary, error } = unitIds.length > 0 ? await summaryOf({ from, to, unitIds, ...(baseline && { baselineId: baseline.id }) }, i.t) : { summary: null };
    live.analysis = { from: klLocal(from), to: klLocal(to), baselineId: baseline?.id ?? null, summary: summary ? summaryView(summary, i) : null, unitIds, error };
  } else {
    const filters = {
      ...(scope.customerId && { customerId: scope.customerId }), ...(scope.propertyId && { propertyId: scope.propertyId }), ...(scope.unitId && { unitId: scope.unitId }),
      ...(fromQ && { from: klInstant(fromQ) }), ...(toQ && { to: klInstant(toQ) }),
    };
    const scoped = Object.keys(filters).length > 0 ? await coreAll<ApiBaseline>("baselines.list", { filters }) : all.items;
    const rows = baselineRows(scoped, i.t);
    live.list = { rows, from: one("from") ?? "", to: one("to") ?? "", selectedId: rows.find((r) => r.id === one("baselineId"))?.id ?? rows[0]?.id ?? null };
  }
  return <EnergyView live={live} />;
}
