// /customer overview (FR-C01, DD-C01, SCR-C01, IR240): in API mode a Server Component reads the units with their latest
// readings, the tiles, the unresolved alerts, the automations, the energy of the period and the air-quality unit's CO2
// (_lib/load.ts) and the client view keeps the property, unit and period in the URL. The Phase 1A demo keeps the
// fixture rows.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { OverviewDemo } from "./_components/overview-demo";
import { OverviewView } from "./_components/overview-view";
import { loadOverview } from "./_lib/load";

export default async function CustomerOverviewPage({ searchParams }: PageProps<"/customer">) {
  await connection(); // DATA_SOURCE is a runtime setting of the image
  if (!apiMode()) return <OverviewDemo />;
  return <OverviewView live={await loadOverview(await searchParams)} />;
}
