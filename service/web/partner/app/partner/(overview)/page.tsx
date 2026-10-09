// /partner (FR-P01, FR-P08, SCR-P01): in API mode a Server Component reads the company's jobs, summary, members,
// capacity and job events for the period of the URL (from/to, this week by default) through the DAL. The Phase 1A
// demo keeps the fixture dashboard.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { OverviewDemo } from "./_components/overview-demo";
import { OverviewView } from "./_components/overview-view";
import { loadOverview } from "./_lib/load";

export default async function PartnerOverviewPage({ searchParams }: PageProps<"/partner">) {
  await connection();
  if (!apiMode()) return <OverviewDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  return <OverviewView live={await loadOverview({ from: one("from"), to: one("to") })} />;
}
