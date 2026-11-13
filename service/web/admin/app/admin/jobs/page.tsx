// /admin/jobs (FR-A06, DD-A06, SCR-A06): in API mode a Server Component reads the Jobs tab — scope, pipeline counts,
// filters, list and the selected job — the Plans tab — the plans in the scope and the selected plan with its
// generated occurrences — or the Contractors tab — the contractor register with KPIs, rate cards and certificates —
// — or the SLA tab — the scorecard by customer with the targets per plan — or the Installation requests tab — the
// customers' requests for new aircons (FR-C20) — through the DAL; their writes are Server
// Actions (./actions.ts). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { withAccess } from "@ac/web/lib/withAccess";
import { JobsDemo } from "./_components/jobs-demo";
import { JobsView } from "./_components/jobs-view";
import { PlansView } from "./_components/plans-view";
import { ContractorsView } from "./_components/contractors-view";
import { SlaView } from "./_components/sla-view";
import { InstallationsView } from "./_components/installations-view";
import { loadContractors, loadInstallations, loadJobs, loadPlans, loadSla } from "./_lib/load";
import { firstOf as one } from "@ac/web/lib/searchParams";

// a role without the page's read permission sees which one it needs, not a failure (IR582)
export default async function AdminJobsPage(props: PageProps<"/admin/jobs">) {
  return withAccess("Maintenance jobs", ["job.read", "asset.read"], () => AdminJobsPageContent(props));
}

async function AdminJobsPageContent({ searchParams }: Readonly<PageProps<"/admin/jobs">>) {
  await connection();
  const sp = await searchParams;
  if (!apiMode()) return <JobsDemo jobId={one(sp.jobId)} tab={one(sp.tab)} />;
  const asked = one(sp.tab);
  const tab = asked && ["plans", "contractors", "sla", "installations"].includes(asked) ? asked : "jobs";
  if (tab === "plans") return <PlansView live={await loadPlans(sp)} />;
  if (tab === "contractors") return <ContractorsView live={await loadContractors(sp)} />;
  if (tab === "sla") return <SlaView live={await loadSla(sp)} />;
  if (tab === "installations") return <InstallationsView live={await loadInstallations(sp)} />;
  return <JobsView live={await loadJobs(sp)} />;
}
