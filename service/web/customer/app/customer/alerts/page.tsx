// /customer/alerts (FR-C08, FR-C15, SCR-C08): in API mode a Server Component reads the alert inbox and the Alert
// policies tab (_lib/load.ts); opening an alert marks its notifications read, and the policies tab saves, deletes and
// switches rules (./actions.ts). The Phase 1A demo keeps the seed rows.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { AlertsView } from "./_components/alerts-view";
import { AlertsLiveView } from "./_components/alerts-live";
import { loadAlerts } from "./_lib/load";

export default async function CustomerAlertsPage({ searchParams }: PageProps<"/customer/alerts">) {
  await connection();
  return apiMode() ? <AlertsLiveView live={await loadAlerts(await searchParams)} /> : <AlertsView />;
}
