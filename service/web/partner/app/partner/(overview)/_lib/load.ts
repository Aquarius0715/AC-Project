// The reads of the contractor overview (FR-P01, DD-P01) through the DAL: summaries.get (kind partner) and jobs.list
// with the same period (from/to on the requested slot, IR74; each summary names the technician of its active
// assignment, IR225), members.list (names), units.list (the delegated units inside their access window),
// members.capacity per day of the period (team capacity) and for today (timeline), and jobs.events of the recent
// jobs (activity). The counts and the list are the authority; the optional reads degrade to empty sections.
import "server-only";
import { coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import {
  actions, activity, capacity, hhmm, kpis, partnerJob, period, progress, timeline, timelinePct, weekOf, shift,
  type ApiCapacity, type ApiCounts, type ApiJobEvent, type ApiMember, type ApiPartnerJob,
} from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback; // not readable now: empty section
  throw e;
});

export async function loadOverview(sp: { from?: string; to?: string }) {
  const now = await coreNow();
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
  const week = weekOf(nowIso);
  const ms = now.getTime();
  return {
    updated: hhmm(nowIso),
    period: { from: p.from, to: p.to, label: p.label },
    periods: [
      { label: "Last week", ...shift(week, -7) }, { label: "This week", ...week }, { label: "Next week", ...shift(week, 7) },
    ],
    today: { date: today, weekday: new Date(`${today}T00:00:00Z`).toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" }), nowPct: timelinePct(nowIso, today), now: hhmm(nowIso) },
    kpis: kpis(summary.counts, jobs, ms, names, { from: p.from, to: p.to }),
    progress: progress(jobs, ms),
    actions: actions(jobs, ms, names),
    timeline: timeline(todayCapacity ?? days[p.days.indexOf(today)] ?? [], members, today),
    capacity: capacity(days, members),
    activity: activity(events, new Map(jobs.map((j) => [j.id, j])), members),
    jobCount: jobs.length,
  };
}

export type OverviewLive = Awaited<ReturnType<typeof loadOverview>>;
