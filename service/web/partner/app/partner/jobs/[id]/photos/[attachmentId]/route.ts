// GET /partner/jobs/[id]/photos/[attachmentId] (FR-P02, IR437): a photo the client sent with the problem report this
// follow-up job answers; the Core API decides as for the job (another company's is 404).
import { jobPhotoResponse } from "@ac/web/lib/jobPhotoResponse";

export async function GET(_request: Request, ctx: RouteContext<"/partner/jobs/[id]/photos/[attachmentId]">) {
  const { id, attachmentId } = await ctx.params;
  return jobPhotoResponse(id, attachmentId);
}
