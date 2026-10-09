// The reads of the contractor's offer / job detail (FR-P02, FR-P08, DD-P02, SCR-P02) through the DAL: jobs.get (the
// offer before the delegation period, the detail inside it, the history after it — another company's job is not
// found), members.list (the company's technicians and their qualifications) and members.capacity of the visit day and
// the next four days (who fits), jobs.events (status timeline, the offer's ordinal), and inside the delegation period units.get and
// alerts.get of the linked alerts. The optional reads leave their sections empty.
import "server-only";
import { coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { fits, qualified, type ApiPartnerJobDetail } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiJobEvent, ApiMember } from "@ac/web/lib/partnerOverview";

type Page<T> = { items: T[] };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const SCOPE_QUALIFICATION: Record<string, string> = { indoor: "demo_indoor", outdoor: "demo_outdoor", electrical: "demo_electrical" }; // fixture qualificationRequirements
const klDate = (iso: string) => new Date(Date.parse(iso) + 8 * 3600_000).toISOString().slice(0, 10);
const plusDays = (date: string, n: number) => new Date(Date.parse(`${date}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);

export async function loadJobDetail(jobId: string) {
  const job = await coreOp<ApiPartnerJobDetail>("jobs.get", { jobId }).catch((e) => {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN")) return null; // another company's job reads as absent (D01)
    throw e;
  });
  if (!job) return null;
  const now = (await coreNow()).toISOString();
  if (job.projection === "history") return { kind: "history" as const, now, job };
  const members = await optional(coreAll<ApiMember>("members.list"), []);
  const capacityOf = (date: string) => optional(coreOp<Page<ApiCapacity>>("members.capacity", { date, query: { limit: 100 } }).then((r) => r.items), []);
  const events = optional(coreOp<Page<ApiJobEvent>>("jobs.events", { jobId, query: { limit: 100 } }).then((r) => r.items), []);
  const names = Object.fromEntries(members.map((m) => [m.id, m.displayName]));
  if (job.projection === "offer") {
    const first = klDate(job.visitSlot.startAt);
    const dates = [0, 1, 2, 3, 4].map((n) => plusDays(first, n));
    const [days, ev] = await Promise.all([Promise.all(dates.map(capacityOf)), events]);
    const technicians = members.filter((m) => m.role === "technician" && qualified(m, job.requiredQualifications, job.visitSlot.startAt)).map((m) => ({ id: m.id, name: m.displayName }));
    return {
      kind: "offer" as const, now, job, names, technicians, events: ev,
      fits: fits(members, job.requiredQualifications, job.visitSlot, dates.map((date, i) => ({ date, capacity: days[i] }))),
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
  const required = (unit?.serviceScope ?? []).map((s) => SCOPE_QUALIFICATION[s]).filter(Boolean);
  return {
    kind: "detail" as const, now, job, names, unit, events: ev, alerts: alerts.filter((a): a is ApiAlert => !!a), required,
    fits: fits(members, required, visit, dates.map((date, i) => ({ date, capacity: days[i] })), "detail", job.assignment?.technicianMembershipId),
  };
}

export type JobDetailLive = NonNullable<Awaited<ReturnType<typeof loadJobDetail>>>;
