// /admin/jobs (FR-A06, DD-A06, SCR-A06): in API mode a Server Component reads the Jobs tab — scope, pipeline counts,
// filters, list and the selected job — or the Plans tab — the plans in the scope and the selected plan with its
// generated occurrences — through the DAL; their writes are Server Actions (./actions.ts). Contractors and SLA still
// show illustrative data. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { JobsDemo } from "./_components/jobs-demo";
import { JobsView } from "./_components/jobs-view";
import { PlansView } from "./_components/plans-view";
import { loadJobs, loadPlans } from "./_lib/load";

export default async function AdminJobsPage({ searchParams }: PageProps<"/admin/jobs">) {
  await connection();
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const tab = ["plans", "contractors", "sla"].includes(one(sp.tab) ?? "") ? one(sp.tab)! : "jobs";
  if (!apiMode()) return <JobsDemo jobId={one(sp.jobId)} tab={one(sp.tab)} />;
  if (tab === "plans") return <PlansView live={await loadPlans(sp)} />;
  return <JobsView live={await loadJobs(sp)} tab={tab} />;
}
