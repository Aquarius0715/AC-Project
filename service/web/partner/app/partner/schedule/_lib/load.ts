// The reads of the contractor's schedule & assignments (FR-P03, DD-P03, SCR-P03) through the DAL: jobs.list of the
// states that can be (re)assigned with the sort (IR34; each summary carries its delegation window, IR227), units.list
// and members.list for the names, jobs.get of the selected job (form mode, current assignment and acknowledgement),
// units.get for its required qualifications, members.eligible for its slot, members.capacity of the slot's week and
// jobs.list per technician (the job of each booked block). Candidate search does not authorize: jobs.assign rechecks.
// Texts in the user's display language; the job's instants are formatted here in their display time zone and the
// team's week keeps Kuala Lumpur days and hours (IR275).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError, orWhenRefused, coreItems } from "@ac/web/lib/dal";
import { i18nOf, showDay, showSpan, showTime, zonedParts } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { requiredFor, type ApiPartnerJobDetail } from "@ac/web/lib/partnerJobDetail";
import { klDay, weekOf, type ApiCapacity, type ApiMember, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";
import { candidates, formMode, SCHEDULE_STATUSES, scheduleRow, scheduleTeam, sortOf, SORTS, type ScheduleRow } from "@ac/web/lib/partnerSchedule";
import { recover } from "@ac/web/lib/promises";

const KL = "Asia/Kuala_Lumpur";

const optional = orWhenRefused;
const plusDays = (date: string, n: number) => new Date(Date.parse(`${date}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
const klDate = (iso: string) => new Date(Date.parse(iso) + 8 * 3_600_000).toISOString().slice(0, 10);

export async function loadSchedule(sp: { jobId?: string; sort?: string }) {
  interface Page<T> { items: T[]; total: number }
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const nowMs = now.getTime();
  const i = i18nOf(display);
  const { t } = i;
  const sort = sortOf(sp.sort);
  const [jobs, members, units] = await Promise.all([
    // every open job: the schedule assigns them all, so a busy company sees past the first page too (IR494)
    coreAll<ApiPartnerJob>("jobs.list", { filters: { statuses: [...SCHEDULE_STATUSES] }, sort: { field: sort, direction: "asc" } }),
    optional(coreAll<ApiMember>("members.list"), []),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
  ]);
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const unitNames = new Map(units.map((u) => [u.id, u.displayName]));
  const rows = jobs.map((j) => scheduleRow(j, nowMs, unitNames, names, i)).filter((r): r is ScheduleRow => !!r);
  // a job whose delegation has ended reads as history and is not listed: nothing to assign there (IR624)
  const base = { now: now.toISOString(), sorts: SORTS.map((s) => ({ id: s.id, text: t(s.text) })), zone: display.timeZone, otherZone: display.timeZone !== KL, sort, rows, members };
  const picked = rows.find((r) => r.id === sp.jobId) ?? rows[0];
  if (!picked) return { ...base, selected: null, week: null };
  const job = await recover(coreOp<ApiPartnerJobDetail>("jobs.get", { jobId: picked.id }), (error: unknown) => {
    if (error instanceof CoreError && (error.error.code === "NOT_FOUND" || error.error.code === "FORBIDDEN")) return null; // delegation ended meanwhile
    throw error;
  });
  if (!job || job.projection === "history") return { ...base, selected: null, week: null };
  const label = `${picked.short} · ${picked.title}`;
  const mode = formMode(job, label, nowMs, names, i);
  const slot: Slot | null = mode.slot;
  const unit = job.projection === "detail" ? await optional(coreOp<ApiUnitDetail>("units.get", { id: job.unitId }), null) : null;
  const required = job.projection === "offer" ? job.requiredQualifications : requiredFor(unit?.serviceScope ?? []);
  const week = weekOf(slot?.startAt ?? now.toISOString());
  const dates = [0, 1, 2, 3, 4, 5, 6].map((n) => plusDays(week.from, n));
  const currentId = job.projection === "detail" ? job.assignment?.technicianMembershipId ?? null : null;
  const technicians = scheduleTeam(members, nowMs, currentId); // the choice and the week leave former technicians out (IR624)
  const [capacity, eligible, booked] = await Promise.all([
    Promise.all(dates.map((date) => optional(coreItems<ApiCapacity>("members.capacity", { date, query: { limit: 100 } }), []))),
    slot && mode.kind !== "blocked" ? optional(coreOp<Page<ApiMember>>("members.eligible", { jobId: picked.id, startAt: slot.startAt, endAt: slot.endAt, query: { limit: 100 } }).then((r) => r.items.map((m) => m.id)), []) : Promise.resolve([] as string[]),
    Promise.all(technicians.map(async (t) => { const r = await optional(coreOp<Page<ApiPartnerJob>>("jobs.list", { filters: { membershipId: t.id }, limit: 100 }), { items: [], total: 0 }); return [t.id, r.items.flatMap((j) => (j.projection === "summary" && j.scheduledSlot && j.status !== "completed" && j.status !== "cancelled" ? { short: j.id.slice(0, 8), slot: j.scheduledSlot } : [])) /* completion released them (IR234) */] as const; })),
  ]);
  const day = slot ? capacity[dates.indexOf(klDate(slot.startAt))] ?? [] : [];
  const alternative = job.projection === "detail" ? job.assignment?.alternativeSlot ?? null : null;
  const local = (iso: string) => { const p = zonedParts(iso, display.timeZone); return `${p.date}T${p.time}`; }; // datetime-local in the display zone
  return {
    ...base,
    members: technicians, // the week's rows (IR624)
    selected: {
      id: picked.id,
      short: picked.short,
      version: job.projection === "detail" ? job.version : job.jobVersion,
      ack: job.projection === "detail" && job.assignment ? { status: job.assignment.acknowledgement, reason: job.assignment.cantMakeReason, alternative } : null,
      window: picked.window,
      candidates: slot ? candidates(technicians, new Set(eligible), required, slot, day, currentId, t) : [],
      text: {
        window: picked.window ? showSpan(picked.window.from, picked.window.until, display) : null,
        slot: slot ? showSpan(slot.startAt, slot.endAt, display) : null, start: slot ? showTime(slot.startAt, display) : null,
        end: slot ? local(slot.endAt) : "", alternative: alternative ? showSpan(alternative.startAt, alternative.endAt, display) : null,
      },
      label,
      mode,
      currentId,
      required,
    },
    week: {
      from: week.from,
      jobsOf: Object.fromEntries(booked) as Record<string, { short: string; slot: Slot }[]>,
      title: t("Team schedule — week of {day}", { day: klDay(week.from, display.locale) }),
      today: klDate(now.toISOString()),
      days: dates.map((d) => showDay(`${d}T04:00:00Z`, { locale: display.locale, timeZone: KL })),
      dates,
      capacity,
    },
  };
}

export type ScheduleLive = Awaited<ReturnType<typeof loadSchedule>>;
