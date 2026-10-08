// /customer/alerts (FR-C05, FR-C15): in API mode a Server Component reads alerts.list through the DAL; alert
// policies stay on the client view. The Phase 1A demo keeps the seed rows.
import { connection } from "next/server";
import { apiMode, coreOp } from "@/lib/dal";
import { alertRow, type ApiAlert } from "@/lib/alerts";
import { AlertsView } from "./_components/alerts-view";

export default async function CustomerAlertsPage() {
  await connection();
  if (!apiMode()) return <AlertsView />;
  const page = await coreOp<{ items: ApiAlert[] }>("alerts.list", { limit: 100 });
  return <AlertsView rows={page.items.map(alertRow)} />;
}
