// The alerts of one unit for the unit pages (technician unit and alert evidence, contractor unit): every unresolved
// alert, and the latest resolved ones up to `resolved` (IR712). alerts.list sorts the most severe first across the
// statuses, so one page of every status lost the unresolved alerts behind resolved ones of a higher severity (the class
// of IR710 and IR711). The Core API scopes each read; a refusal of one fails the whole read, as the single read did.
import { coreAll, coreItems } from "@ac/web/lib/dal";

const LIST = "alerts.list";

export async function unitAlerts<T>(unitId: string, resolved: number): Promise<T[]> {
  const [open, acknowledged, done] = await Promise.all([
    coreAll<T>(LIST, { filters: { unitId, status: "open" } }),
    coreAll<T>(LIST, { filters: { unitId, status: "acknowledged" } }),
    coreItems<T>(LIST, { limit: resolved, filters: { unitId, status: "resolved" }, sort: { field: "updatedAt", direction: "desc" } }),
  ]);
  return [...open, ...acknowledged, ...done];
}
