// /customer/maintenance (FR-C09, FR-C17, FR-C18, SCR-C09): in API mode a Server Component reads the customer's
// requests (jobs.list) and, for jobId, the request with its notes, history and accepted report (_lib/load.ts); new
// requests, answers to a proposed time, plan-visit reschedules, cancellation, notes, ratings and problem reports are
// Server Actions (./actions.ts). URL keys: jobId, tab (filter-care). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { MaintenanceDemo } from "./_components/maintenance-demo";
import { MaintenanceView } from "./_components/maintenance-view";
import { loadMaintenance } from "./_lib/load";

export default async function CustomerMaintenancePage({ searchParams }: PageProps<"/customer/maintenance">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  if (!apiMode()) return <MaintenanceDemo jobId={one(sp.jobId)} initialTab={one(sp.tab)} />;
  return <MaintenanceView live={await loadMaintenance(sp)} />;
}
