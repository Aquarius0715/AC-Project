// A problem photo of a follow-up job (jobs.getAttachment, IR437) sent as the image itself. The API's NOT_FOUND /
// FORBIDDEN pass on as 404 / 403; the name goes into the header the RFC 5987 way (IR368), and the browser keeps no
// copy (IR434).
import "server-only";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import { contentDisposition } from "@ac/web/lib/files";

export async function jobPhotoResponse(jobId: string, attachmentId: string): Promise<Response> {
  try {
    const blob = await coreOp<{ name: string; mime: string; bytes: string }>("jobs.getAttachment", { jobId, attachmentId });
    return new Response(Buffer.from(blob.bytes, "base64"), {
      headers: { "Content-Type": blob.mime, "Cache-Control": "no-store", "Content-Disposition": contentDisposition(blob.name) },
    });
  } catch (e) {
    if (e instanceof CoreError) return new Response(e.error.messageKey, { status: e.error.code === "FORBIDDEN" ? 403 : e.error.code === "NOT_FOUND" ? 404 : 502 });
    throw e;
  }
}
