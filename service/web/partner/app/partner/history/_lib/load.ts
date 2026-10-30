// The reads of the contractor's job history (FR-P07, FR-P08, DD-P07, SCR-P07) through the DAL: jobs.list of the jobs
// after acceptance (the period filter reads the requested slot, histories their completion) with each job's event
// counts and latest event (eventSummary; a history's own decision events, IR506), units.list and members.list for the
// names, and for the open job its timeline (jobs.events, newest first — only the company's own decisions after the
// delegation) and notifications.recipients (who may get a preview: HQ, the assigned technician, the customer contact).
// A job outside the period is looked up without it.
// Texts in the user's display language; every time is formatted here, in their display time zone (IR278).
import "server-only";
import { coreAll, coreDisplay, coreIdentity, coreNow, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { byUser, eventItem, HISTORY_STATUSES, historyRow, noteMode, PERIODS, periodOf, SORTS, sortOf, tabCounts, TABS, tabOf, visibleRows, type ApiEvent, type HistoryRow } from "@ac/web/lib/partnerHistory";
import type { ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[]; total: number };
export type ApiRecipient = { id: string; role: string; displayLabel: string; allowedChannels: string[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
export async function loadHistory(sp: { jobId?: string; tab?: string; sort?: string; period?: string }) {
  const [now, display, who] = await Promise.all([coreNow(), coreDisplay(), coreIdentity()]);
  const nowMs = now.getTime();
  const i = i18nOf(display);
  const { t } = i;
  const tab = tabOf(sp.tab), sort = sortOf(sp.sort), period = periodOf(sp.period);
  const [members, units] = await Promise.all([optional(coreAll<ApiMember>("members.list"), []), optional(coreAll<{ id: string; displayName: string }>("units.list"), [])]);
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const unitNames = new Map(units.map((u) => [u.id, u.displayName]));
  const collect = async (withPeriod: boolean) => {
    const filters = { statuses: [...HISTORY_STATUSES], ...(withPeriod ? { from: new Date(nowMs - 90 * 86_400_000).toISOString() } : {}) };
    // the latest 100 jobs: IDs are UUIDv7, so the newest first by ID; the page then sorts them as asked (IR494)
    const list = await coreOp<Page<ApiPartnerJob>>("jobs.list", { filters, sort: { field: "id", direction: "desc" }, limit: 100 });
    const rows = list.items.map((j) => historyRow(j, nowMs, unitNames, names, i)).filter((r): r is HistoryRow => !!r); // offers drop out
    return { rows, total: list.total };
  };
  const listed = await collect(period === "90d");
  const counts = tabCounts(listed.rows);
  const base = {
    now: now.toISOString(), tab, sort, period, company: who.organizationName || t("your company"),
    sorts: SORTS.map((s) => ({ id: s.id, text: t("Sort: {sort}", { sort: t(s.text) }) })), periods: PERIODS.map((p) => ({ id: p.id, text: t("Period: {period}", { period: t(p.label) }) })),
    tabs: TABS.map((x) => ({ id: x.id, label: t(x.label), count: counts[x.id] })), rows: visibleRows(listed.rows, tab, sort), total: listed.total,
    options: listed.rows.map((r) => ({ id: r.id, label: `${r.short} · ${r.title}` })),
  };
  if (!sp.jobId) return { ...base, detail: null };
  let hit = listed.rows.find((r) => r.id === sp.jobId);
  if (!hit && period === "90d") hit = (await collect(false)).rows.find((r) => r.id === sp.jobId); // older than the period
  if (!hit) return { ...base, detail: { missing: true as const, id: sp.jobId } };
  const [principal, events] = await Promise.all([ // the job's newest 100 events, newest first
    corePrincipal(),
    optional(coreOp<Page<ApiEvent>>("jobs.events", { jobId: hit.id, query: { limit: 100, sort: { field: "occurredAt", direction: "desc" } } }).then((r) => r.items), [] as ApiEvent[]),
  ]);
  const users = byUser(members, principal.userId, t);
  const items = [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt) || b.id.localeCompare(a.id)).map((e) => eventItem(e, users, i));
  const mode = noteMode(hit, t);
  const recipients = mode.kind === "closed" ? [] : await optional(coreOp<Page<ApiRecipient>>("notifications.recipients", { target: { kind: "job", id: hit.id }, templateKey: "schedule_change", channel: "inApp", query: { limit: 100 } }).then((r) => r.items.filter((x) => x.role !== "contractor")), []);
  return { ...base, detail: { missing: false as const, row: hit, items, mode, recipients } };
}

export type HistoryLive = Awaited<ReturnType<typeof loadHistory>>;
