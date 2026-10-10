// /admin/alerts (FR-A05): in API mode a Server Component reads alerts.list, policies.list and the names of units,
// customers and members through the DAL; acknowledging, resolving and saving a policy are Server Actions
// (actions.ts). The Phase 1A demo keeps the fixture rows. The rows' texts and times are formatted here in the user's
// language and display time zone, so the first render matches the browser's (IR292).
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { adminAlertRows, policyRows, type ApiAdminAlert, type ApiCustomerName, type ApiMemberName, type ApiPolicy, type ApiUnitName } from "@ac/web/lib/adminAlerts";
import { AdminAlertsView } from "./_components/alerts-view";

type Page<T> = { items: T[] };

export default async function AdminAlertsPage({ searchParams }: PageProps<"/admin/alerts">) {
  await connection();
  if (!apiMode()) return <AdminAlertsView />;
  const sp = await searchParams;
  const [now, display, alerts, units, customers, policies, members] = await Promise.all([
    coreNow(), coreDisplay(),
    coreOp<Page<ApiAdminAlert>>("alerts.list", { limit: 100 }),
    coreOp<Page<ApiUnitName>>("units.list", { limit: 100 }),
    coreOp<Page<ApiCustomerName>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiPolicy>>("policies.list", { limit: 100 }),
    coreOp<Page<ApiMemberName>>("members.list", { limit: 100 }),
  ]);
  const i = i18nOf(display);
  return (
    <AdminAlertsView
      live={{
        tab: sp.tab === "policies" ? "policies" : "alerts",
        alerts: adminAlertRows(alerts.items, units.items, customers.items, policies.items, now.getTime(), i),
        policies: policyRows(policies.items, customers.items, members.items, i.t),
      }}
    />
  );
}
