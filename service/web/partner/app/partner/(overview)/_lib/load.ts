// The reads of the contractor overview (FR-P01, DD-P01) through the DAL: summaries.get (kind partner) and jobs.list
// with the same period (from/to on the requested slot, IR74; each summary names the technician of its active
// assignment, IR225), members.list (names), units.list (the delegated units inside their access window),
// members.capacity per day of the period (team capacity) and for today (timeline), and jobs.events of the recent
// jobs (activity). The counts and the list are the authority; the optional reads degrade to empty sections. Texts in the
// user's display language; the period and today's timeline stay Kuala Lumpur days and hours (IR270).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showClock } from "@ac/web/lib/i18n";
import {
  actions, activity, capacity, hhmm, klDay, kpis, partnerJob, period, periodChoice, progress, timeline, timelinePct,
  type ApiCapacity, type ApiCounts, type ApiJobEvent, type ApiMember, type ApiPartnerJob,
} from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[] };
const KL = "Asia/Kuala_Lumpur";
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback; // not readable now: empty section
  throw e;
});

export async function loadOverview(sp: { from?: string; to?: string }) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const loc = display.locale;
  const nowIso = now.toISOString();
  const p = period(nowIso, sp.from, sp.to);
  const filters = { from: p.fromAt, to: p.toAt };
  const [summary, rows, members, units] = await Promise.all([
    coreOp<{ counts: ApiCounts }>("summaries.get", { kind: "partner", filters }),
    coreAll<ApiPartnerJob>("jobs.list", { filters }),
    optional(coreAll<ApiMember>("members.list"), []),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
  ]);
  const jobs = rows.map((r) => partnerJob(r, new Map(units.map((u) => [u.id, u.displayName]))));
  const today = new Date(now.getTime() + 8 * 3600_000).toISOString().slice(0, 10);
  const capacityOf = (date: string) => optional(coreOp<Page<ApiCapacity>>("members.capacity", { date, query: { limit: 100 } }).then((r) => r.items), []);
  const [days, todayCapacity] = await Promise.all([
    Promise.all(p.days.map(capacityOf)),
    p.days.includes(today) ? Promise.resolve(null) : capacityOf(today),
  ]);
  // activity: the events of the jobs that moved last (open ones first), newest first
  const recent = [...jobs].sort((a, b) => Number(a.status === "completed") - Number(b.status === "completed") || Date.parse(b.dueAt ?? "") - Date.parse(a.dueAt ?? "")).slice(0, 8);
  const events = (await Promise.all(recent.map((j) => optional(coreOp<Page<ApiJobEvent>>("jobs.events", { jobId: j.id, query: { limit: 100 } }).then((r) => r.items), [])))).flat();
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const ms = now.getTime();
  return {
    updated: showClock(nowIso, display),
    otherZone: display.timeZone !== KL,
    ...periodChoice(p, nowIso, i), // period + periods
    today: { title: t("Today · {day}", { day: klDay(today, loc, true) }), nowPct: timelinePct(nowIso, today), now: hhmm(nowIso) },
    kpis: kpis(summary.counts, jobs, ms, names, { from: p.from, to: p.to }, i),
    progress: progress(jobs, ms),
    actions: actions(jobs, ms, names, i),
    timeline: timeline(todayCapacity ?? days[p.days.indexOf(today)] ?? [], members, today, t),
    capacity: capacity(days, members, t),
    activity: activity(events, new Map(jobs.map((j) => [j.id, j])), members, ms, 6, i),
    jobCount: jobs.length,
  };
}

export type OverviewLive = Awaited<ReturnType<typeof loadOverview>>;
