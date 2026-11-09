import "server-only";
import { coreAll, coreOp } from "@ac/web/lib/dal";
import type { ApiAdminAlert } from "@ac/web/lib/adminAlerts";
import type { OpInput } from "@ac/web/lib/opTypes";

type Filters = Omit<NonNullable<OpInput<"alerts.list">["filters"]>, "status">;

/** The alerts of the HQ alerts screen in its scope (FR-A05), each status read on its own (IR710). The list sorts the
most severe first across the statuses, so a single page of every status lost the open alerts behind 100 resolved ones
of a higher severity, from the list and from the tiles. The open and acknowledged alerts are read in full; the resolved
ones are their first page, most severe first. */
export async function scopeAlerts(filters: Filters): Promise<{ items: ApiAdminAlert[] }> {
  const list = "alerts.list";
  const [open, acknowledged, resolved] = await Promise.all([
    coreAll<ApiAdminAlert>(list, { filters: { ...filters, status: "open" } }),
    coreAll<ApiAdminAlert>(list, { filters: { ...filters, status: "acknowledged" } }),
    coreOp<{ items: ApiAdminAlert[] }>(list, { limit: 100, filters: { ...filters, status: "resolved" } }),
  ]);
  return { items: [...open, ...acknowledged, ...resolved.items] };
}
