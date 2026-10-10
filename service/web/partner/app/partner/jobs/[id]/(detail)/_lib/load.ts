// The reads of the contractor's offer / job detail (FR-P02, FR-P08, DD-P02, SCR-P02) through the DAL: jobs.get (the
// offer before the delegation period, the detail inside it, the history after it — another company's job is not
// found), members.list (the company's technicians and their qualifications) and members.capacity of the visit day and
// the next four days (who fits), jobs.events (status timeline, the offer's ordinal), and inside the delegation period units.get and
// alerts.get of the linked alerts. The optional reads leave their sections empty. Every time is formatted here, on the
// server (IR272): instants in the user's display time zone, the capacity days as Kuala Lumpur dates; the view only
// translates fixed texts.
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showClock, showSpan, showTime, zonedParts } from "@ac/web/lib/i18n";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { delegationLeft, detailBanner, eventRows, fits, ordinal, qualified, requiredFor, timeline, type ApiPartnerJobDetail } from "@ac/web/lib/partnerJobDetail";
import { until, type ApiCapacity, type ApiJobEvent, type ApiMember } from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const klDate = (iso: string) => new Date(Date.parse(iso) + 8 * 3600_000).toISOString().slice(0, 10);
const plusDays = (date: string, n: number) => new Date(Date.parse(`${date}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);

export async function loadJobDetail(jobId: string) {
  const job = await coreOp<ApiPartnerJobDetail>("jobs.get", { jobId }).catch((e) => {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN")) return null; // another company's job reads as absent (D01)
    throw e;
  });
  if (!job) return null;
  const [nowAt, display] = await Promise.all([coreNow(), coreDisplay()]);
  const now = nowAt.toISOString();
  const ms = nowAt.getTime();
  const i = i18nOf(display);
  const { t } = i;
  if (job.projection === "history") {
    return {
      kind: "history" as const, now, job,
      text: { completed: job.completedAt ? showTime(job.completedAt, display) : "—", snapshot: showTime(job.asOf, display), events: eventRows(job.ownDecisionEvents, i) },
    };
  }
  const members = await optional(coreAll<ApiMember>("members.list"), []);
  const capacityOf = (date: string) => optional(coreOp<Page<ApiCapacity>>("members.capacity", { date, query: { limit: 100 } }).then((r) => r.items), []);
  const events = optional(coreOp<Page<ApiJobEvent>>("jobs.events", { jobId, query: { limit: 100 } }).then((r) => r.items), []);
  const names = Object.fromEntries(members.map((m) => [m.id, m.displayName]));
  if (job.projection === "offer") {
    const first = klDate(job.visitSlot.startAt);
    const dates = [0, 1, 2, 3, 4].map((n) => plusDays(first, n));
    const [days, ev] = await Promise.all([Promise.all(dates.map(capacityOf)), events]);
    const technicians = members.filter((m) => m.role === "technician" && qualified(m, job.requiredQualifications, job.visitSlot.startAt)).map((m) => ({ id: m.id, name: m.displayName }));
    const p = job.partnerSlotProposal;
    const offers = Math.max(1, ev.filter((e) => e.action === "job.offered").length);
    const start = zonedParts(job.visitSlot.startAt, display.timeZone);
    return {
      kind: "offer" as const, now, job, names, technicians, events: ev,
      fits: fits(members, job.requiredQualifications, job.visitSlot, dates.map((date, k) => ({ date, capacity: days[k] })), "offer", undefined, i),
      text: {
        expires: showTime(job.offerExpiresAt, display), visit: showSpan(job.visitSlot.startAt, job.visitSlot.endAt, display),
        access: showSpan(job.accessValidFrom, job.accessValidUntil, display), accessFrom: showTime(job.accessValidFrom, display), offered: showTime(job.offeredAt, display),
        left: until(job.offerExpiresAt, ms, t), proposal: p ? showSpan(p.slot.startAt, p.slot.endAt, display) : null, offers, nth: ordinal(offers, display.locale),
        // Propose another time: typed in the display time zone (NFR-08), the day after the fixed visit by default
        propose: { zone: display.timeZone, date: zonedParts(new Date(Date.parse(job.visitSlot.startAt) + 86_400_000).toISOString(), display.timeZone).date, from: start.time, to: zonedParts(job.visitSlot.endAt, display.timeZone).time },
      },
    };
  }
  const visit = job.assignment ? { startAt: job.assignment.scheduledStart, endAt: job.assignment.scheduledEnd } : job.offer?.visitSlot ?? job.requestedSlot;
  const dates = [0, 1, 2, 3, 4].map((n) => plusDays(klDate(visit.startAt), n));
  const [unit, ev, alerts, days] = await Promise.all([
    optional(coreOp<ApiUnitDetail>("units.get", { id: job.unitId }), null),
    events,
    Promise.all(job.alertIds.map((id) => optional(coreOp<ApiAlert>("alerts.get", { id }), null))),
    Promise.all(dates.map(capacityOf)),
  ]);
  const required = requiredFor(unit?.serviceScope ?? []);
  const linked = alerts.filter((a): a is ApiAlert => !!a);
  const tech = job.assignment ? names[job.assignment.technicianMembershipId] ?? t("Technician") : null;
  return {
    kind: "detail" as const, now, job, names, unit, events: ev, alerts: linked, required,
    fits: fits(members, required, visit, dates.map((date, k) => ({ date, capacity: days[k] })), "detail", job.assignment?.technicianMembershipId, i),
    text: {
      tech, banner: detailBanner(job, tech, ms, i), steps: timeline(job.status, ev, !!job.assignment, job.origin, i),
      visit: job.offer ? showSpan(job.offer.visitSlot.startAt, job.offer.visitSlot.endAt, display) : showSpan(job.requestedSlot.startAt, job.requestedSlot.endAt, display),
      access: job.offer ? showSpan(job.offer.accessValidFrom, job.offer.accessValidUntil, display) : null,
      scheduled: job.assignment ? showSpan(job.assignment.scheduledStart, job.assignment.scheduledEnd, display) : null,
      seen: unit?.lastSeenAt ? showClock(unit.lastSeenAt, display) : null,
      raised: Object.fromEntries(linked.map((a) => [a.id, showTime(a.detectedAt, display)])),
      delegation: job.offer ? { left: delegationLeft(job.offer.accessValidUntil, ms, false, i), toSchedule: delegationLeft(job.offer.accessValidUntil, ms, true, i) } : null,
    },
  };
}

export type JobDetailLive = NonNullable<Awaited<ReturnType<typeof loadJobDetail>>>;
