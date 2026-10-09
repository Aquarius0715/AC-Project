// /partner/jobs/[id] (FR-P02, FR-P08, SCR-P02): in API mode a Server Component reads the offer, the job detail inside
// the delegation period or the history after it through the DAL; another company's job is not found. Accept,
// decline, propose another time and withdraw are Server Actions (../actions.ts). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode } from "@ac/web/lib/dal";
import { JobDemo } from "./_components/job-demo";
import { JobDetailView } from "./_components/job-view";
import { loadJobDetail } from "./_lib/load";

export default async function PartnerJobPage({ params }: PageProps<"/partner/jobs/[id]">) {
  await connection();
  const { id } = await params;
  if (!apiMode()) return <JobDemo id={id} />;
  const live = await loadJobDetail(id);
  if (!live) notFound();
  return <JobDetailView live={live} />;
}
