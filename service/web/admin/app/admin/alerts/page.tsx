// /admin/alerts (FR-A05): in API mode a Server Component reads alerts.list, policies.list and the names of units,
// customers and members through the DAL; acknowledging, resolving and saving a policy are Server Actions
// (actions.ts). The Phase 1A demo keeps the fixture rows. The rows' texts and times are formatted here in the user's
// language and display time zone, so the first render matches the browser's (IR292). The Alerts tab's scope (customer →
// property → unit), severity, status and selected alert live in the URL (DD-A05 item 5, Figma Admin 256:2, IR318).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { adminAlertRows, policyRows, type AlertState, type ApiAdminAlert, type ApiCustomerName, type ApiMemberName, type ApiPolicy, type ApiUnitName, type Severity } from "@ac/web/lib/adminAlerts";
import { AdminAlertsView } from "./_components/alerts-view";

type Page<T> = { items: T[] };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
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
  const filters = {
    ...(q.customerId ? { customerId: q.customerId } : {}), ...(q.propertyId ? { propertyId: q.propertyId } : {}), ...(q.unitId ? { unitId: q.unitId } : {}),
    ...(severity ? { severity } : {}),
  };
  const [now, display, alerts, units, customers, properties, policies, members] = await Promise.all([
    coreNow(), coreDisplay(),
    coreOp<Page<ApiAdminAlert>>("alerts.list", { limit: 100, filters }), // every status in the scope: the tiles count them
    coreOp<Page<ApiUnitName & { propertyId: string }>>("units.list", { limit: 100 }),
    coreOp<Page<ApiCustomerName>>("customers.list", { limit: 100 }),
    coreAll<{ id: string; name: string; customerOrgId: string }>("properties.list"),
    coreOp<Page<ApiPolicy>>("policies.list", { limit: 100 }),
    coreOp<Page<ApiMemberName>>("members.list", { limit: 100 }),
  ]);
  const i = i18nOf(display);
  const custOrg = customers.items.find((c) => c.id === q.customerId)?.organizationId;
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
        alerts: adminAlertRows(alerts.items, units.items, customers.items, policies.items, now.getTime(), i),
        policies: policyRows(policies.items, customers.items, members.items, i.t),
      }}
    />
  );
}
