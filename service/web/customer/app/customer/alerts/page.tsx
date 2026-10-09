// /customer/alerts (FR-C08, FR-C15, SCR-C08): in API mode a Server Component reads the alert inbox (_lib/load.ts) and
// opening an alert marks its notifications read (./actions.ts); the Alert policies tab is still illustrative in API mode
// (FR-C15, next). The Phase 1A demo keeps the seed rows.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { AlertsView } from "./_components/alerts-view";
import { AlertsLiveView } from "./_components/alerts-live";
import { loadAlerts } from "./_lib/load";

export default async function CustomerAlertsPage({ searchParams }: PageProps<"/customer/alerts">) {
  await connection();
  if (!apiMode()) return <AlertsView />;
  return <AlertsLiveView live={await loadAlerts(await searchParams)} />;
}
