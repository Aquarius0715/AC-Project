// The reads of the contractor job list (FR-P01, DD-P01 job list, SCR-P09) through the DAL: jobs.list of the selected
// tab and period with the sort (IR34) and the page cursor, one total per tab (jobs.list with limit 1), units.list for
// the unit names (delegated units inside their access window) and members.list for the technicians' names (each
// summary names its active assignment's technician, IR225). A cursor that no longer fits the list starts over at
// the first page.
import "server-only";
import { coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { jobRow, pages, PAGE_SIZE, sortOf, sortSpec, SORTS, TABS, tabFilters, tabOf } from "@ac/web/lib/partnerJobs";
import { period, shift, weekOf, type ApiMember, type ApiPartnerJob } from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[]; nextCursor: string | null; total: number };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const stale = (e: unknown) => e instanceof CoreError && (!!e.error.fieldErrors.cursor || e.error.messageKey === "error.cursorExpired");

export async function loadJobs(sp: { tab?: string; sort?: string; from?: string; to?: string; cursor?: string; page?: string }) {
  const now = await coreNow();
  const nowIso = now.toISOString();
  const p = period(nowIso, sp.from, sp.to);
  const tab = tabOf(sp.tab), sort = sortOf(sp.sort);
  const query = (cursor?: string) => coreOp<Page<ApiPartnerJob>>("jobs.list", { filters: tabFilters(tab, p.fromAt, p.toAt), sort: sortSpec(sort), limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) });
  let restarted = false;
  const list = await query(sp.cursor).catch((e) => {
    if (!sp.cursor || !stale(e)) throw e;
    restarted = true; // the list changed since the cursor was issued
    return query();
  });
  const [counts, units, members] = await Promise.all([
    Promise.all(TABS.map((t) => coreOp<Page<unknown>>("jobs.list", { filters: tabFilters(t.id, p.fromAt, p.toAt), limit: 1 }).then((r) => r.total))),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
    optional(coreAll<ApiMember>("members.list"), []),
  ]);
  const unitNames = new Map(units.map((u) => [u.id, u.displayName]));
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const week = weekOf(nowIso);
  const page = restarted ? 1 : Number(sp.page) || (sp.cursor ? 2 : 1);
  return {
    tab, sort, sortText: SORTS.find((s) => s.id === sort)!.text, restarted,
    tabs: TABS.map((t, i) => ({ id: t.id, label: t.label, count: counts[i] })),
    period: { from: p.from, to: p.to, label: p.label },
    periods: [{ label: "Last week", ...shift(week, -7) }, { label: "This week", ...week }, { label: "Next week", ...shift(week, 7) }],
    rows: list.items.map((j) => jobRow(j, now.getTime(), unitNames, names)),
    total: list.total, nextCursor: list.nextCursor, paging: pages(list.total, page),
  };
}

export type JobsLive = Awaited<ReturnType<typeof loadJobs>>;
