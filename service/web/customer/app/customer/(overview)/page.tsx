// /customer overview (FR-C01): in API mode a Server Component reads units.list and summaries.get(kind=customer)
// through the DAL and passes plain rows to the client view; the Phase 1A demo keeps the fixture rows.
import { connection } from "next/server";
import { apiMode, coreOp } from "@ac/web/lib/dal";
import { unitRowFromApi, type ApiUnit, type CustomerCounts } from "@ac/web/lib/client";
import { OverviewView } from "./_components/overview-view";

export default async function CustomerOverviewPage() {
  await connection(); // DATA_SOURCE is a runtime setting of the image
  if (!apiMode()) return <OverviewView />;
  const [units, summary] = await Promise.all([
    coreOp<{ items: ApiUnit[] }>("units.list", { limit: 100 }),
    coreOp<{ counts: CustomerCounts }>("summaries.get", { kind: "customer", filters: {} }),
  ]);
  return <OverviewView units={units.items.map(unitRowFromApi)} counts={summary.counts} />;
}
