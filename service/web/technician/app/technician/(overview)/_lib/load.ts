// The reads of the technician overview (FR-T01, DD-T01, SCR-T01) through the DAL: summaries.get (kind=technician) for
// the tiles, jobs.list of the user's assignments with the sort (IR34; the projection shows the summary only while the
// viewing window is open, IR124), units.list for the unit names and alerts.list for the alerts on the assigned
// units. Assigned jobs before their work window are read-only; the job page enforces it (IR49).
import "server-only";
import { coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiAlert } from "@ac/web/lib/alerts";
import { alertRows, dayTimeline, kpis, reportRows, SORTS, sortOf, TABS, tabOf, techRow, type ApiTechCounts, type ApiTechJobRow, type TechJobRow } from "@ac/web/lib/techOverview";

type Page<T> = { items: T[]; total: number };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});

export async function loadOverview(sp: { tab?: string; sort?: string }) {
  const now = await coreNow();
  const nowMs = now.getTime();
  const tab = tabOf(sp.tab), sort = sortOf(sp.sort);
  const spec = SORTS.find((s) => s.id === sort)!;
  const [summary, list, units, alerts] = await Promise.all([
    coreOp<{ counts: ApiTechCounts }>("summaries.get", { kind: "technician", filters: {} }),
    coreOp<Page<ApiTechJobRow>>("jobs.list", { sort: { field: spec.field, direction: spec.direction }, limit: 100 }),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
    optional(coreOp<Page<ApiAlert>>("alerts.list", { sort: { field: "severity", direction: "desc" }, limit: 50 }).then((r) => r.items), [] as ApiAlert[]),
  ]);
  const names = new Map(units.map((u) => [u.id, u.displayName]));
  const rows = list.items.map((j) => techRow(j, nowMs, names)).filter((r): r is TechJobRow => !!r);
  const jobOfUnit = new Map<string, string>();
  for (const j of list.items) if (j.projection === "summary" && !jobOfUnit.has(j.unitId)) jobOfUnit.set(j.unitId, j.id);
  const today = rows.filter((r) => r.today);
  return {
    now: now.toISOString(), tab, sort, sorts: SORTS,
    tabs: TABS.map((t) => ({ ...t, count: t.id === "today" ? today.length : rows.length })),
    kpis: kpis(summary.counts, rows),
    pending: rows.filter((r) => r.ackPending),
    rows: tab === "today" ? today : rows,
    readOnly: today.filter((r) => r.opensAt),
    timeline: dayTimeline(rows, nowMs), todayRows: today,
    alerts: alertRows(alerts, names, jobOfUnit),
    reports: reportRows(rows),
  };
}

export type OverviewLive = Awaited<ReturnType<typeof loadOverview>>;
