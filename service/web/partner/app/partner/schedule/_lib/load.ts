// The reads of the contractor's schedule & assignments (FR-P03, DD-P03, SCR-P03) through the DAL: jobs.list of the
// states that can be (re)assigned with the sort (IR34; each summary carries its delegation window, IR227), units.list
// and members.list for the names, jobs.get of the selected job (form mode, current assignment and acknowledgement),
// units.get for its required qualifications, members.eligible for its slot, members.capacity of the slot's week and
// jobs.list per technician (the job of each booked block). Candidate search does not authorize: jobs.assign rechecks.
// Texts in the user's display language; the job's instants are formatted here in their display time zone and the
// team's week keeps Kuala Lumpur days and hours (IR275).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showDay, showSpan, showTime, zonedParts } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { requiredFor, type ApiPartnerJobDetail } from "@ac/web/lib/partnerJobDetail";
import { klDay, weekOf, type ApiCapacity, type ApiMember, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";
import { candidates, formMode, SCHEDULE_STATUSES, scheduleRow, sortOf, SORTS, type ScheduleRow } from "@ac/web/lib/partnerSchedule";

type Page<T> = { items: T[]; total: number };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const plusDays = (date: string, n: number) => new Date(Date.parse(`${date}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
const klDate = (iso: string) => new Date(Date.parse(iso) + 8 * 3600_000).toISOString().slice(0, 10);
const KL = "Asia/Kuala_Lumpur";

export async function loadSchedule(sp: { jobId?: string; sort?: string }) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const nowMs = now.getTime();
  const i = i18nOf(display);
  const { t } = i;
  const sort = sortOf(sp.sort);
  const [list, members, units] = await Promise.all([
    coreOp<Page<ApiPartnerJob>>("jobs.list", { filters: { statuses: [...SCHEDULE_STATUSES] }, sort: { field: sort, direction: "asc" }, limit: 100 }),
    optional(coreAll<ApiMember>("members.list"), []),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
  ]);
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const unitNames = new Map(units.map((u) => [u.id, u.displayName]));
  const rows = list.items.map((j) => scheduleRow(j, nowMs, unitNames, names, i)).filter((r): r is ScheduleRow => !!r);
  const base = { now: now.toISOString(), sort, sorts: SORTS.map((s) => ({ id: s.id, text: t(s.text) })), rows, total: list.total, members, zone: display.timeZone, otherZone: display.timeZone !== KL };
  const picked = rows.find((r) => r.id === sp.jobId) ?? rows[0];
  if (!picked) return { ...base, selected: null, week: null };
  const job = await coreOp<ApiPartnerJobDetail>("jobs.get", { jobId: picked.id }).catch((e) => {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN")) return null; // delegation ended meanwhile
    throw e;
  });
  if (!job || job.projection === "history") return { ...base, selected: null, week: null };
  const label = `${picked.short} · ${picked.title}`;
  const mode = formMode(job, label, nowMs, names, i);
  const slot: Slot | null = mode.slot;
  const unit = job.projection === "detail" ? await optional(coreOp<ApiUnitDetail>("units.get", { id: job.unitId }), null) : null;
  const required = job.projection === "offer" ? job.requiredQualifications : requiredFor(unit?.serviceScope ?? []);
  const week = weekOf(slot?.startAt ?? now.toISOString());
  const dates = [0, 1, 2, 3, 4, 5, 6].map((n) => plusDays(week.from, n));
  const technicians = members.filter((m) => m.role === "technician");
  const [capacity, eligible, booked] = await Promise.all([
    Promise.all(dates.map((date) => optional(coreOp<Page<ApiCapacity>>("members.capacity", { date, query: { limit: 100 } }).then((r) => r.items), []))),
    slot && mode.kind !== "blocked" ? optional(coreOp<Page<ApiMember>>("members.eligible", { jobId: picked.id, startAt: slot.startAt, endAt: slot.endAt, query: { limit: 100 } }).then((r) => r.items.map((m) => m.id)), []) : Promise.resolve([] as string[]),
    Promise.all(technicians.map((t) => optional(coreOp<Page<ApiPartnerJob>>("jobs.list", { filters: { membershipId: t.id }, limit: 100 }), { items: [], total: 0 }).then((r) => [t.id, r.items.flatMap((j) => (j.projection === "summary" && j.scheduledSlot && j.status !== "completed" && j.status !== "cancelled" ? [{ short: j.id.slice(0, 8), slot: j.scheduledSlot }] : [])) /* completion released them (IR234) */] as const))),
  ]);
  const currentId = job.projection === "detail" ? job.assignment?.technicianMembershipId ?? null : null;
  const day = slot ? capacity[dates.indexOf(klDate(slot.startAt))] ?? [] : [];
  const alternative = job.projection === "detail" ? job.assignment?.alternativeSlot ?? null : null;
  const local = (iso: string) => { const p = zonedParts(iso, display.timeZone); return `${p.date}T${p.time}`; }; // datetime-local in the display zone
  return {
    ...base,
    selected: {
      id: picked.id, short: picked.short, label, version: job.projection === "detail" ? job.version : job.jobVersion, mode, currentId,
      ack: job.projection === "detail" && job.assignment ? { status: job.assignment.acknowledgement, reason: job.assignment.cantMakeReason, alternative } : null,
      window: picked.window, required, candidates: slot ? candidates(members, new Set(eligible), required, slot, day, currentId, t) : [],
      text: {
        window: picked.window ? showSpan(picked.window.from, picked.window.until, display) : null,
        slot: slot ? showSpan(slot.startAt, slot.endAt, display) : null, start: slot ? showTime(slot.startAt, display) : null,
        end: slot ? local(slot.endAt) : "", alternative: alternative ? showSpan(alternative.startAt, alternative.endAt, display) : null,
      },
    },
    week: {
      from: week.from, dates, capacity, jobsOf: Object.fromEntries(booked) as Record<string, { short: string; slot: Slot }[]>,
      title: t("Team schedule — week of {day}", { day: klDay(week.from, display.locale) }), today: klDate(now.toISOString()),
      days: dates.map((d) => showDay(`${d}T04:00:00Z`, { locale: display.locale, timeZone: KL })), // noon in Kuala Lumpur
    },
  };
}

export type ScheduleLive = Awaited<ReturnType<typeof loadSchedule>>;
