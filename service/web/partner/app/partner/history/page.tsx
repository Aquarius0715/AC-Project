// /partner/history (FR-P07, FR-P08, DD-P07, SCR-P07): in API mode a Server Component reads the company's jobs after
// acceptance with their events and, with ?jobId=, one job's timeline and its recipients through the DAL; the note and
// the preview are a Server Action (./actions.ts). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { HistoryDemo } from "./_components/history-demo";
import { HistoryView } from "./_components/history-view";
import { loadHistory } from "./_lib/load";

export default async function PartnerHistoryPage({ searchParams }: PageProps<"/partner/history">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  if (!apiMode()) return <HistoryDemo />;
  const live = await loadHistory({ jobId: one(sp.jobId), tab: one(sp.tab), sort: one(sp.sort), period: one(sp.period) });
  return <HistoryView live={live} />;
}
