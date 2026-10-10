// The reads of the contractor's team & capacity (FR-P06, DD-P06, SCR-P06) through the DAL: session.get (the own
// company, so another orgId in the URL reads as NOT_FOUND, AT-P06-E①), members.list (the company's technicians with
// their qualification grants and membership periods), members.capacity for each Kuala Lumpur day of the chosen date's
// week, and jobs.list per technician (the job of a booked block and the confirmed assignments unavailable days would
// overlap). Texts in the user's display language; every date and time is formatted here, on the server (IR276).
import "server-only";
import { coreAll, coreDisplay, coreIdentity, coreNow, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showDay, showSpan } from "@ac/web/lib/i18n";
import { klDay, weekOf, type ApiCapacity, type ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import { qualificationLabel } from "@ac/web/lib/partnerJobDetail";
import { activeNow, grantRows, KL, listed, memberRows, QUALIFICATION_CODES, teamQuery, teamStats, UNAVAILABILITY_TYPES, weekday, weekRows, type ApiTeamMember, type TeamJob } from "@ac/web/lib/partnerTeam";

type Page<T> = { items: T[]; total: number };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const plusDays = (date: string, n: number) => new Date(Date.parse(`${date}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
const noon = (date: string) => `${date}T04:00:00Z`; // 12:00 in Kuala Lumpur

export async function loadTeam(sp: { date?: string; qualification?: string; activeOnly?: string; orgId?: string }) {
  const [now, display, who, principal] = await Promise.all([coreNow(), coreDisplay(), coreIdentity(), corePrincipal()]);
  const i = i18nOf(display);
  const { t } = i;
  const { locale } = display;
  const nowMs = now.getTime();
  const company = who.organizationName || t("your company");
  if (sp.orgId && sp.orgId !== principal.organizationId) return { notFound: true as const, company };
  const q = teamQuery(sp, now.toISOString());
  const week = weekOf(noon(q.date));
  const dates = [0, 1, 2, 3, 4, 5, 6].map((n) => plusDays(week.from, n));
  const dayIndex = dates.indexOf(q.date);
  const members = (await coreAll<ApiTeamMember>("members.list", { filters: { role: "technician" } })).filter((m) => m.role === "technician");
  const current = members.filter((m) => activeNow(m, nowMs)); // capacity and assignments exist only for these
  const shown = listed(members, q, nowMs);
  const [capacity, booked] = await Promise.all([
    Promise.all(dates.map((date) => optional(coreOp<Page<ApiCapacity>>("members.capacity", { date, query: { limit: 100 } }).then((r) => r.items), []))),
    Promise.all(current.map((m) => optional(coreOp<Page<ApiPartnerJob>>("jobs.list", { filters: { membershipId: m.id }, limit: 100 }), { items: [], total: 0 })
      .then((r) => r.items.flatMap((j): TeamJob[] => (j.projection === "summary" && j.scheduledSlot && j.status !== "completed" && j.status !== "cancelled" // completion releases the assignment (IR234)
        ? [{ jobId: j.id, short: j.id.slice(0, 8), technicianId: m.id, slot: j.scheduledSlot, text: showSpan(j.scheduledSlot.startAt, j.scheduledSlot.endAt, display) }] : []))))),
  ]);
  const jobs = booked.flat();
  const inWeek = shown.rows.filter((m) => activeNow(m, nowMs));
  const from = klDay(week.from, locale);
  return {
    notFound: false as const, company, query: q, otherZone: display.timeZone !== KL,
    technicians: current.map((m) => ({ id: m.id, name: m.displayName })), total: members.length, activeCount: current.length,
    rows: memberRows(shown.rows, capacity, dayIndex, jobs, nowMs, i), hidden: shown.hidden, expired: shown.expired,
    week: {
      title: dayIndex >= 5 ? t("Week of {day} — hours ({selected} selected)", { day: from, selected: showDay(noon(q.date), { locale, timeZone: KL }) }) : t("Week of {day} — assigned / available hours", { day: from }),
      short: t("Week of {day}", { day: from }), dates, dayIndex, days: dates.map((d) => showDay(noon(d), { locale, timeZone: KL })),
      rows: weekRows(inWeek, capacity, t), stats: teamStats(inWeek, capacity, dayIndex, t), freeLabel: t("Free hours {day}", { day: weekday(q.date, locale) }),
    },
    grants: grantRows(shown.rows, nowMs, i),
    qualifications: QUALIFICATION_CODES.map((code) => ({ code, text: qualificationLabel(code, t) })),
    types: UNAVAILABILITY_TYPES.map((x) => ({ id: x.id, text: t(x.text) })),
    jobs,
  };
}

export type TeamLive = Awaited<ReturnType<typeof loadTeam>>;
