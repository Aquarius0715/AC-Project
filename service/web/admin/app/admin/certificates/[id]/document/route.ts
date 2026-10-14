// GET /admin/certificates/[id]/document (FR-A21, IR436): the uploaded document of a certificate waiting for
// verification, opened from Verify uploads in a new tab.
import { documentResponse } from "@ac/web/lib/documentResponse";

export async function GET(_request: Request, ctx: RouteContext<"/admin/certificates/[id]/document">) {
  const { id } = await ctx.params;
  return documentResponse(id);
}
