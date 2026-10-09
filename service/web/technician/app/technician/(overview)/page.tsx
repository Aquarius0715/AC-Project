// /technician (FR-T01, DD-T01, SCR-T01): in API mode a Server Component reads the user's assignments, the tiles and
// the alerts on the assigned units through the DAL; ?tab=all lists every assigned job (Sidebar › Assigned jobs).
// The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { OverviewDemo } from "./_components/overview-demo";
import { OverviewView } from "./_components/overview-view";
import { loadOverview } from "./_lib/load";

export default async function TechnicianOverviewPage({ searchParams }: PageProps<"/technician">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  if (!apiMode()) return <OverviewDemo tab={one(sp.tab)} />;
  const live = await loadOverview({ tab: one(sp.tab), sort: one(sp.sort) });
  return <OverviewView live={live} />;
}
