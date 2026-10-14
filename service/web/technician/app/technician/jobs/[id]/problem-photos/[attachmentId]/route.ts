// GET /technician/jobs/[id]/problem-photos/[attachmentId] (FR-T04, IR437): a photo the client sent with the problem
// report this follow-up job answers; the Core API decides as for the job.
import { jobPhotoResponse } from "@ac/web/lib/jobPhotoResponse";

export async function GET(_request: Request, ctx: RouteContext<"/technician/jobs/[id]/problem-photos/[attachmentId]">) {
  const { id, attachmentId } = await ctx.params;
  return jobPhotoResponse(id, attachmentId);
}
