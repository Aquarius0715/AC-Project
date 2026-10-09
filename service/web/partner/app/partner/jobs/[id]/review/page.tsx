// /partner/jobs/[id]/review (FR-P05, SCR-P05): in API mode a Server Component reads the delegated job, its submitted
// report with photos, the author and contributor names, the linked alerts and the unit through the DAL; accepting or
// returning the report is a Server Action (jobs.review). Outside the company's delegation the job is not found. The
// Phase 1A demo keeps the fixture view.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, CoreError } from "@ac/web/lib/dal";
import { loadReview } from "./_lib/load";
import { ReviewDemo } from "./_components/review-demo";
import { ReviewView } from "./_components/review-view";

export default async function PartnerReviewPage({ params }: PageProps<"/partner/jobs/[id]/review">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <ReviewDemo id={id} />;
  const live = await loadReview(id).catch((e) => {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || e.error.fieldErrors.jobId)) return null;
    throw e;
  });
  if (!live) notFound();
  return <ReviewView live={live} />;
}
