// GET /technician/jobs/[id]/photos/[attachmentId]?reportId=&reportVersion= (FR-T09): one report photo or signature
// through the DAL (attachments.getContent), streamed as the image so the workspace does not carry the files in its
// page payload. Outside the technician's job or window the API answers NOT_FOUND / FORBIDDEN, passed on as 404 / 403.
import { attachmentResponse } from "@ac/web/lib/attachmentResponse";

export async function GET(request: Request, ctx: RouteContext<"/technician/jobs/[id]/photos/[attachmentId]">) {
  const { id, attachmentId } = await ctx.params;
  const url = new URL(request.url);
  const reportId = url.searchParams.get("reportId");
  const reportVersion = Number(url.searchParams.get("reportVersion"));
  return !reportId || !Number.isSafeInteger(reportVersion) || reportVersion < 1 ? new Response("reportId and reportVersion are required", { status: 400 }) : attachmentResponse({ jobId: id, reportId, reportVersion, attachmentId });
}
