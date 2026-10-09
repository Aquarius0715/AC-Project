// The reads of the contractor's job history (FR-P07, FR-P08, DD-P07, SCR-P07) through the DAL: jobs.list of the jobs
// after acceptance (the period filter reads the requested slot, histories their completion), jobs.events of each
// (the counts, the latest event and the timeline — only the company's own decisions after the delegation),
// units.list and members.list for the names, and for the open job notifications.recipients of the job (who may get
// a preview: HQ, the assigned technician, the customer contact). A job outside the period is looked up without it.
import "server-only";
import { coreAll, coreNow, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { byUser, eventItem, HISTORY_STATUSES, historyRow, noteMode, PERIODS, periodOf, SORTS, sortOf, tabCounts, TABS, tabOf, visibleRows, type ApiEvent, type HistoryRow } from "@ac/web/lib/partnerHistory";
import type { ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[]; total: number };
export type ApiRecipient = { id: string; role: string; displayLabel: string; allowedChannels: string[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const jobId = (j: ApiPartnerJob) => (j.projection === "summary" ? j.id : j.jobId);

export async function loadHistory(sp: { jobId?: string; tab?: string; sort?: string; period?: string }) {
  const now = await coreNow();
  const nowMs = now.getTime();
  const tab = tabOf(sp.tab), sort = sortOf(sp.sort), period = periodOf(sp.period);
  const [members, units] = await Promise.all([optional(coreAll<ApiMember>("members.list"), []), optional(coreAll<{ id: string; displayName: string }>("units.list"), [])]);
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const unitNames = new Map(units.map((u) => [u.id, u.displayName]));
  const collect = async (withPeriod: boolean) => {
    const filters = { statuses: [...HISTORY_STATUSES], ...(withPeriod ? { from: new Date(nowMs - 90 * 86_400_000).toISOString() } : {}) };
    const list = await coreOp<Page<ApiPartnerJob>>("jobs.list", { filters, sort: { field: "status", direction: "asc" }, limit: 100 });
    const jobs = list.items.filter((j) => j.projection !== "offer");
    const events = await Promise.all(jobs.map((j) => optional(coreOp<Page<ApiEvent>>("jobs.events", { jobId: jobId(j), query: { limit: 100 } }).then((r) => r.items), [] as ApiEvent[])));
    const rows = jobs.map((j, i) => ({ row: historyRow(j, events[i], nowMs, unitNames, names), events: events[i] })).filter((x): x is { row: HistoryRow; events: ApiEvent[] } => !!x.row);
    return { rows, total: list.total };
  };
  const listed = await collect(period === "90d");
  const counts = tabCounts(listed.rows.map((r) => r.row));
  const base = {
    now: now.toISOString(), tab, sort, period, sorts: SORTS, periods: PERIODS,
    tabs: TABS.map((t) => ({ ...t, count: counts[t.id] })), rows: visibleRows(listed.rows.map((r) => r.row), tab, sort), total: listed.total,
    options: listed.rows.map((r) => ({ id: r.row.id, label: `${r.row.short} · ${r.row.title}` })),
  };
  if (!sp.jobId) return { ...base, detail: null };
  let hit = listed.rows.find((r) => r.row.id === sp.jobId);
  if (!hit && period === "90d") hit = (await collect(false)).rows.find((r) => r.row.id === sp.jobId); // older than the period
  if (!hit) return { ...base, detail: { missing: true as const, id: sp.jobId } };
  const users = byUser(members, (await corePrincipal()).userId);
  const items = [...hit.events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt) || b.id.localeCompare(a.id)).map((e) => eventItem(e, users));
  const mode = noteMode(hit.row);
  const recipients = mode.kind === "closed" ? [] : await optional(coreOp<Page<ApiRecipient>>("notifications.recipients", { target: { kind: "job", id: hit.row.id }, templateKey: "schedule_change", channel: "inApp", query: { limit: 100 } }).then((r) => r.items.filter((x) => x.role !== "contractor")), []);
  return { ...base, detail: { missing: false as const, row: hit.row, items, mode, recipients } };
}

export type HistoryLive = Awaited<ReturnType<typeof loadHistory>>;
