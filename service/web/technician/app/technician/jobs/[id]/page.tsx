// /technician/jobs/[id] (FR-T04–T06, T08, T09, T13–T15, SCR-T04): in API mode a Server Component reads the job, its
// unit, the open draft or latest report and the parts catalog through the DAL; the workspace edits the draft on the page
// and writes through Server Actions (assignment, check-in, pause, draft, photos, sign-off, submit, rework). A job that is
// not the technician's is not found. URL key: tab (checklist group, kept in the page). The Phase 1A demo keeps the
// fixture view.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode } from "@ac/web/lib/dal";
import { loadWorkspace } from "./_lib/load";
import { WorkspaceDemo } from "./_components/workspace-demo";
import { WorkspaceView } from "./_components/workspace-view";

export default async function TechnicianJobPage({ params }: PageProps<"/technician/jobs/[id]">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <WorkspaceDemo id={id} />;
  const live = await loadWorkspace(id);
  if (live === "not_found") notFound();
  // a new job or report version starts the editor from the server state (key); unsaved edits are saved before transitions
  return <WorkspaceView key={`${live.job.version}:${live.report?.id ?? "-"}:${live.job.draftReportRef?.reportVersion ?? 0}`} live={live} />;
}
