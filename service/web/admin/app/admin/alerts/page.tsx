// /admin/alerts (FR-A05): in API mode a Server Component reads alerts.list, policies.list and the names of units,
// customers and members through the DAL; acknowledging, resolving and saving a policy are Server Actions
// (actions.ts). The Phase 1A demo keeps the fixture rows. The rows' texts and times are formatted here in the user's
// language and display time zone, so the first render matches the browser's (IR292). The Alerts tab's scope (customer →
// property → unit), severity, status and selected alert live in the URL (DD-A05 item 5, Figma Admin 256:2, IR318); so do
// the Policies tab's selected policy (`policyId`) and the owner customer of a new one (`new`, DD-A05 item 8, IR345).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { adminAlertRows, policyRows, type AlertState, type ApiAdminAlert, type ApiCustomerName, type ApiMemberName, type ApiPolicy, type ApiUnitName, type Severity } from "@ac/web/lib/adminAlerts";
import { AdminAlertsView } from "./_components/alerts-view";

type Page<T> = { items: T[] };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => { // names the session may not read (no identity.read or job.read)
  if (e instanceof CoreError && e.error.code === "FORBIDDEN") return fallback;
  throw e;
});
const SEVERITIES: Severity[] = ["critical", "warning", "normal"];
const STATES: AlertState[] = ["open", "acknowledged", "resolved"];

export default async function AdminAlertsPage({ searchParams }: PageProps<"/admin/alerts">) {
  await connection();
  if (!apiMode()) return <AdminAlertsView />;
  const sp = await searchParams;
  const severity = SEVERITIES.find((s) => s === one(sp.severity)) ?? null;
  const q = {
    customerId: one(sp.customerId), propertyId: one(sp.propertyId), unitId: one(sp.unitId), severity,
    status: STATES.find((s) => s === one(sp.status)) ?? "open", alertId: one(sp.alertId) ?? null,
  };
  const scope = { ...(q.customerId ? { customerId: q.customerId } : {}), ...(q.propertyId ? { propertyId: q.propertyId } : {}), ...(q.unitId ? { unitId: q.unitId } : {}) };
  const filters = { ...scope, ...(severity ? { severity } : {}) };
  // the policies in the same scope (DD-A05 item 8); a property or unit scope lists the attached policies only, so the
  // default policy — on every unit — is read on its own and listed first (Figma 268:2)
  const byUnits = !!(q.propertyId || q.unitId);
  const [now, perms, display, alerts, units, customers, properties, policies, members, defaults] = await Promise.all([
    coreNow(), corePermissions(), coreDisplay(),
    coreOp<Page<ApiAdminAlert>>("alerts.list", { limit: 100, filters }), // every status in the scope: the tiles count them
    coreOp<Page<ApiUnitName & { propertyId: string; archived?: boolean }>>("units.list", { limit: 100 }),
    coreOp<Page<ApiCustomerName & { status?: "active" | "inactive" }>>("customers.list", { limit: 100 }),
    coreAll<{ id: string; name: string; customerOrgId: string }>("properties.list"),
    coreOp<Page<ApiPolicy>>("policies.list", { limit: 100, filters: scope }),
    optional(coreOp<Page<ApiMemberName>>("members.list", { limit: 100 }), { items: [] }),
    byUnits ? coreOp<Page<ApiPolicy>>("policies.list", { limit: 1, filters: { kind: "default_alert" } }) : Promise.resolve({ items: [] as ApiPolicy[] }),
  ]);
  const scoped = [...defaults.items, ...policies.items];
  const i = i18nOf(display);
  const custOrg = customers.items.find((c) => c.id === q.customerId)?.organizationId;
  const custByOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  return (
    <AdminAlertsView
      live={{
        tab: sp.tab === "policies" ? "policies" : "alerts", q,
        scope: {
          customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
          properties: properties.filter((p) => !custOrg || p.customerOrgId === custOrg).map((p) => ({ id: p.id, name: p.name })),
          units: units.items.filter((u) => (!custOrg || u.customerOrgId === custOrg) && (!q.propertyId || u.propertyId === q.propertyId)).map((u) => ({ id: u.id, name: u.displayName })),
        },
        scopeName: customers.items.find((c) => c.id === q.customerId)?.name ?? null,
        alerts: adminAlertRows(alerts.items, units.items, customers.items, scoped, now.getTime(), i),
        policies: policyRows(scoped, customers.items, members.items, i.t, units.items),
        pol: {
          policyId: one(sp.policyId) ?? null, newFor: one(sp.new) ?? null, canWrite: perms.has("alert.policy.write"), timezone: display.timeZone,
          customers: customers.items.filter((c) => c.status !== "inactive").map((c) => ({ id: c.id, name: c.name })),
          units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, customerId: custByOrg.get(u.customerOrgId) ?? null })),
        },
      }}
    />
  );
}
