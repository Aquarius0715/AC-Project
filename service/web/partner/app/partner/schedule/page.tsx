// /partner/schedule (FR-P03, DD-P03, SCR-P03): in API mode a Server Component reads the company's jobs that can be
// (re)assigned, the selected job's slot, candidates and the team's week through the DAL; the assignment is a Server
// Action (./actions.ts). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { ScheduleDemo } from "./_components/schedule-demo";
import { ScheduleView } from "./_components/schedule-view";
import { loadSchedule } from "./_lib/load";

export default async function PartnerSchedulePage({ searchParams }: PageProps<"/partner/schedule">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  if (!apiMode()) return <ScheduleDemo jobId={one(sp.jobId)} />;
  const live = await loadSchedule({ jobId: one(sp.jobId), sort: one(sp.sort) });
  return <ScheduleView live={live} />;
}
