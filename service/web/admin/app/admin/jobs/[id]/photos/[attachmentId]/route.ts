// GET /admin/jobs/[id]/photos/[attachmentId] (FR-A06, IR437): a photo the client sent with a problem report, opened
// from the follow-up card while HQ classifies it.
import { jobPhotoResponse } from "@ac/web/lib/jobPhotoResponse";

export async function GET(_request: Request, ctx: RouteContext<"/admin/jobs/[id]/photos/[attachmentId]">) {
  const { id, attachmentId } = await ctx.params;
  return jobPhotoResponse(id, attachmentId);
}
