// /admin/jobs (FR-A06, DD-A06, SCR-A06): in API mode a Server Component reads the Jobs tab — scope, pipeline counts,
// filters, list and the selected job — through the DAL; bookings, proposals, partner time changes, hold, cancel and
// follow-up classification are Server Actions (./actions.ts). Plans, Contractors and SLA still show illustrative data.
// The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { JobsDemo } from "./_components/jobs-demo";
import { JobsView } from "./_components/jobs-view";
import { loadJobs } from "./_lib/load";

export default async function AdminJobsPage({ searchParams }: PageProps<"/admin/jobs">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const tab = ["plans", "contractors", "sla"].includes(one(sp.tab) ?? "") ? one(sp.tab)! : "jobs";
  if (!apiMode()) return <JobsDemo jobId={one(sp.jobId)} tab={one(sp.tab)} />;
  const live = await loadJobs(sp);
  return <JobsView live={live} tab={tab} />;
}
