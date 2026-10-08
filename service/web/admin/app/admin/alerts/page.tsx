// /admin/alerts (FR-A05): in API mode a Server Component reads alerts.list, policies.list and the names of units,
// customers and members through the DAL; acknowledging, resolving and saving a policy are Server Actions
// (actions.ts). The Phase 1A demo keeps the fixture rows.
import { connection } from "next/server";
import { apiMode, coreOp } from "@ac/web/lib/dal";
import { adminAlertRows, policyRows, type ApiAdminAlert, type ApiCustomerName, type ApiMemberName, type ApiPolicy, type ApiUnitName } from "@ac/web/lib/adminAlerts";
import { AdminAlertsView } from "./_components/alerts-view";

type Page<T> = { items: T[] };

export default async function AdminAlertsPage({ searchParams }: PageProps<"/admin/alerts">) {
  await connection();
  if (!apiMode()) return <AdminAlertsView />;
  const sp = await searchParams;
  const [alerts, units, customers, policies, members] = await Promise.all([
    coreOp<Page<ApiAdminAlert>>("alerts.list", { limit: 100 }),
    coreOp<Page<ApiUnitName>>("units.list", { limit: 100 }),
    coreOp<Page<ApiCustomerName>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiPolicy>>("policies.list", { limit: 100 }),
    coreOp<Page<ApiMemberName>>("members.list", { limit: 100 }),
  ]);
  return (
    <AdminAlertsView
      live={{
        tab: sp.tab === "policies" ? "policies" : "alerts",
        alerts: adminAlertRows(alerts.items, units.items, customers.items, policies.items),
        policies: policyRows(policies.items, customers.items, members.items),
      }}
    />
  );
}
