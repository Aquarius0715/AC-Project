// One stored report photo or signature (attachments.getContent, FR-T09), sent as the image itself so a page does not
// carry the files in its payload. The API's NOT_FOUND / FORBIDDEN pass on as 404 / 403. The file's name goes into the
// header the RFC 5987 way, so a name in any script opens (IR368). Not stored by the browser: a sign-off carries the
// customer's signature, personal data like every gateway answer (IR407, IR434).
import "server-only";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import { contentDisposition } from "@ac/web/lib/files";

export async function attachmentResponse(input: { jobId: string; reportId: string; reportVersion: number; attachmentId: string }): Promise<Response> {
  try {
    const blob = await coreOp<{ name: string; mime: string; bytes: string }>("attachments.getContent", input);
    return new Response(Buffer.from(blob.bytes, "base64"), {
      headers: { "Content-Type": blob.mime, "Cache-Control": "no-store", "Content-Disposition": contentDisposition(blob.name) },
    });
  } catch (e) {
    if (e instanceof CoreError) return new Response(e.error.messageKey, { status: e.error.code === "FORBIDDEN" ? 403 : e.error.code === "NOT_FOUND" ? 404 : 502 });
    throw e;
  }
}
