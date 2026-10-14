// GET /partner/certificates/[id]/document (FR-P09, IR436): the uploaded document of one of the company's
// certificates; another company's is 404.
import { documentResponse } from "@ac/web/lib/documentResponse";

export async function GET(_request: Request, ctx: RouteContext<"/partner/certificates/[id]/document">) {
  const { id } = await ctx.params;
  return documentResponse(id);
}
