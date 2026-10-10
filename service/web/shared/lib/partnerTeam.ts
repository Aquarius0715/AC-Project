// The contractor's team & capacity (FR-P06, DD-P06, Figma Contractor 04-1…04-5, 04-8) from the Core API: the URL's
// date / qualification / activeOnly, each technician's week and the chosen day (assigned and free time, utilization),
// the team's week of assigned / available hours, the qualification grants and the unavailable days form with the
// confirmed assignments it overlaps. Pure code shared by the server loader and the client view; Vitest covers it.
// Texts in the display language (`t` / `i`, IR276). Capacity is Kuala Lumpur working days and hours (members.capacity
// cuts days there), and so are the unavailable days; the screen says so in another display time zone.
import { EN, intlTag, translator, type I18n, type Locale, type T } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { hhmm, unavailabilityLabel, type ApiCapacity, type ApiMember, type Slot } from "@ac/web/lib/partnerOverview";
import { qualificationLabel, qualified } from "@ac/web/lib/partnerJobDetail";
import { freeSlots } from "@ac/web/lib/partnerSchedule";

const en = translator("en");
const DAY = 86_400_000, KL_OFFSET = 8 * 3600_000;
export const KL = "Asia/Kuala_Lumpur";

/** members.list rows of a contractor's technicians (Membership of service-contracts.ts). */
export type ApiTeamMember = ApiMember & { organizationId: string; employment: string | null; validFrom: string; validUntil: string | null };

/** The qualification register a filter offers (QualificationCode, SR13). */
export const QUALIFICATION_CODES = ["demo_indoor", "demo_outdoor", "demo_electrical"] as const;
const isDate = (d?: string) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(`${d}T00:00:00Z`));
/** The Kuala Lumpur date (YYYY-MM-DD) of an instant. */
export const klDate = (iso: string) => new Date(Date.parse(iso) + KL_OFFSET).toISOString().slice(0, 10);
/** The instant a Kuala Lumpur day starts. */
export const klStart = (date: string) => new Date(Date.parse(`${date}T00:00:00Z`) - KL_OFFSET).toISOString();

/** The URL's conditions (SCR-P06): a valid date (default today in Kuala Lumpur), a register qualification and
 * activeOnly (default true). */
export function teamQuery(sp: { date?: string; qualification?: string; activeOnly?: string }, nowIso: string) {
  const today = klDate(nowIso);
  const qualification = (QUALIFICATION_CODES as readonly string[]).includes(sp.qualification ?? "") ? sp.qualification! : null;
  return { date: isDate(sp.date) ? sp.date! : today, today, qualification, activeOnly: sp.activeOnly !== "false" };
}

/** Whether a membership is valid now: [validFrom, validUntil). */
export const activeNow = (m: ApiTeamMember, nowMs: number) => Date.parse(m.validFrom) <= nowMs && (!m.validUntil || nowMs < Date.parse(m.validUntil));
const hours = (min: number) => Math.round((min / 60) * 10) / 10;
const pct = (assigned: number, available: number) => (available > 0 ? Math.round((assigned / available) * 100) : null);
/** The short weekday of a Kuala Lumpur date in the user's language (“Tue”, “Sel”). */
export const weekday = (date: string, locale: Locale) => new Date(`${date}T04:00:00Z`).toLocaleDateString(intlTag(locale), { weekday: "short", timeZone: KL });
/** “Apr 2025”: the month a membership started, a Kuala Lumpur date in the user's language. */
const month = (iso: string, locale: Locale) => new Date(iso).toLocaleDateString(intlTag(locale), { month: "short", year: "numeric", timeZone: KL }).replace(/[   ]/g, " ");
const overlaps = (a: Slot, b: Slot) => Date.parse(a.startAt) < Date.parse(b.endAt) && Date.parse(b.startAt) < Date.parse(a.endAt);

/** A technician's job with a booked time (jobs.list per technician): its short ID, the slot and the slot as text. */
export type TeamJob = { jobId: string; short: string; technicianId: string; slot: Slot; text: string };

export type MemberRow = {
  id: string; name: string; sub: string; active: boolean;
  week: { text: string; pct: number | null }; day: { main: string; sub: string; tone?: "crit" | "warn" }; util: string;
};
/** The technicians card (Figma 04-1, 04-2, 04-3): qualifications and membership, the week's assigned / available
 * hours, and the chosen day's assigned and free time with its utilization — “—” without configured hours (DD-P06). */
export function memberRows(members: ApiTeamMember[], week: ApiCapacity[][], dayIndex: number, jobs: TeamJob[], nowMs: number, i: I18n = EN): MemberRow[] {
  const { t, display: { locale } } = i;
  return members.map((m): MemberRow => {
    const active = activeNow(m, nowMs);
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code, t)).join(" · ") || t("No qualifications");
    const since = active ? t("active since {month}", { month: month(m.validFrom, locale) }) : t("expired {date}", { date: m.validUntil ? businessDay(m.validUntil, locale) : "—" });
    const caps = week.map((d) => d.find((c) => c.membershipId === m.id));
    const assigned = caps.reduce((a, c) => a + (c?.assignedMinutes ?? 0), 0), available = caps.reduce((a, c) => a + (c?.availableMinutes ?? 0), 0);
    const row = { id: m.id, name: m.displayName, sub: `${quals} · ${since}`, active, week: { text: t("{a} h / {b} h", { a: hours(assigned), b: hours(available) }), pct: pct(assigned, available) } };
    if (!active) return { ...row, week: { text: "—", pct: null }, day: { main: t("Expired — cannot be assigned"), sub: "—", tone: "crit" }, util: "—" };
    const c = caps[dayIndex];
    if (c?.unavailability) return { ...row, day: { main: t("Unavailable · {reason}", { reason: unavailabilityLabel(c.unavailability, t) }), sub: "—", tone: "warn" }, util: "—" };
    if (!c || !c.availableMinutes) return { ...row, day: { main: t("No available hours set"), sub: "—" }, util: "—" };
    const mine = jobs.filter((j) => j.technicianId === m.id);
    const blocks = c.assignedSlots.map((s) => {
      const job = mine.find((j) => overlaps(j.slot, s));
      return job ? t("{slots} assigned ({job})", { slots: `${hhmm(s.startAt)}–${hhmm(s.endAt)}`, job: job.short }) : t("{slots} assigned", { slots: `${hhmm(s.startAt)}–${hhmm(s.endAt)}` });
    });
    const free = freeSlots(c);
    const util = pct(c.assignedMinutes, c.availableMinutes);
    return {
      ...row, util: util === null ? "—" : `${util}%`,
      day: { main: blocks.length ? blocks.join(", ") : t("No assignments"), sub: free.length ? t("{slots} free", { slots: free.join(", ") }) : t("fully booked") },
    };
  });
}

export type WeekCell = { text: string; kind: "busy" | "free" | "off" | "none" };
/** The team's week of assigned / available hours per technician and Kuala Lumpur day (Figma 04-1): “2 / 8 h”, “—”
 * on a day without configured hours, the unavailability on a day off. */
export function weekRows(members: ApiTeamMember[], week: ApiCapacity[][], t: T = en): { id: string; name: string; cells: WeekCell[] }[] {
  return members.map((m) => ({
    id: m.id, name: m.displayName,
    cells: week.map((d): WeekCell => {
      const c = d.find((x) => x.membershipId === m.id);
      if (c?.unavailability) return { text: unavailabilityLabel(c.unavailability, t), kind: "off" };
      if (!c || !c.availableMinutes) return { text: "—", kind: "none" };
      return { text: t("{a} / {b} h", { a: hours(c.assignedMinutes), b: hours(c.availableMinutes) }), kind: c.assignedMinutes > 0 ? "busy" : "free" };
    }),
  }));
}

/** The week's totals and the chosen day's free hours (Figma 04-1): assigned, available, team utilization. */
export function teamStats(members: ApiTeamMember[], week: ApiCapacity[][], dayIndex: number, t: T = en) {
  const ids = new Set(members.map((m) => m.id));
  const mine = (d: ApiCapacity[]) => d.filter((c) => ids.has(c.membershipId));
  const assigned = week.flatMap(mine).reduce((a, c) => a + c.assignedMinutes, 0), available = week.flatMap(mine).reduce((a, c) => a + (c.availableMinutes ?? 0), 0);
  const free = mine(week[dayIndex] ?? []).reduce((a, c) => a + Math.max(0, (c.availableMinutes ?? 0) - c.assignedMinutes), 0);
  const util = pct(assigned, available);
  return { assigned: t("{n} h", { n: hours(assigned) }), available: t("{n} h", { n: hours(available) }), utilization: util === null ? "—" : `${util}%`, free: t("{n} h", { n: hours(free) }) };
}

export type GrantRow = { key: string; technician: string; name: string; sub: string; until: string; badge: { text: string; tone: "ok" | "warn" | "muted" } };
/** The qualification grants of the listed technicians (Figma 04-1): valid, expiring within 30 days (IR133 item 2),
 * expired or revoked, and each register qualification nobody listed holds (“not held”). Dates are Kuala Lumpur days. */
export function grantRows(members: ApiTeamMember[], nowMs: number, i: I18n = EN): GrantRow[] {
  const { t, display: { locale } } = i;
  const rows: GrantRow[] = [];
  for (const m of members) {
    for (const q of m.qualifications ?? []) {
      const end = q.validUntil ? Date.parse(q.validUntil) : Infinity;
      const until = q.validUntil ? businessDay(q.validUntil, locale) : "—";
      const days = Math.ceil((end - nowMs) / DAY);
      const [sub, badge] = q.revokedAt ? [t("{name} · revoked {date}", { name: m.displayName, date: businessDay(q.revokedAt, locale) }), { text: t("Revoked"), tone: "muted" as const }]
        : end <= nowMs ? [t("{name} · expired {date}", { name: m.displayName, date: until }), { text: t("Expired"), tone: "muted" as const }]
        : days <= 30 ? [t("{name} · expires {date}", { name: m.displayName, date: until }), { text: t("Expiring · {n} d", { n: days }), tone: "warn" as const }]
        : [t("{name} · valid to {date}", { name: m.displayName, date: until }), { text: t("Valid"), tone: "ok" as const }];
      rows.push({ key: `${m.id}:${q.code}`, technician: m.displayName, name: qualificationLabel(q.code, t), sub, until, badge });
    }
  }
  const held = new Set(members.flatMap((m) => (qualifiedCodes(m, nowMs))));
  for (const code of QUALIFICATION_CODES) {
    if (!held.has(code)) rows.push({ key: `none:${code}`, technician: "—", name: qualificationLabel(code, t), sub: t("not held"), until: "—", badge: { text: "—", tone: "muted" } });
  }
  return rows.sort((a, b) => a.name.localeCompare(b.name) || a.technician.localeCompare(b.technician));
}
const qualifiedCodes = (m: ApiTeamMember, nowMs: number) => QUALIFICATION_CODES.filter((code) => qualified(m, [code], new Date(nowMs).toISOString()));

/** The technicians of the URL's conditions: activeOnly hides memberships that are not valid now, a qualification
 * keeps those holding it on the chosen day; `hidden` counts the expired memberships activeOnly hides. */
export function listed(members: ApiTeamMember[], q: { date: string; qualification: string | null; activeOnly: boolean }, nowMs: number) {
  const on = new Date(Date.parse(klStart(q.date)) + 9 * 3600_000).toISOString(); // 09:00 in Kuala Lumpur, the start of the working day
  const kept = members.filter((m) => !q.qualification || qualified(m, [q.qualification], on));
  const expired = kept.filter((m) => !activeNow(m, nowMs));
  return { rows: q.activeOnly ? kept.filter((m) => activeNow(m, nowMs)) : kept, hidden: q.activeOnly ? expired.length : 0, expired: q.activeOnly ? [] : expired.map((m) => m.displayName) };
}

export const UNAVAILABILITY_TYPES = [
  { id: "annual_leave", text: "Annual leave" }, { id: "training", text: "Training" }, { id: "public_holiday", text: "Public holiday" },
  { id: "sick", text: "Sick leave" }, { id: "other", text: "Other" },
] as const;
export type UnavailabilityForm = { membershipId: string; from: string; to: string; type: string; note: string };

/** The form's own checks before members.setUnavailability (DD-P06 Figma 04-8, IR111): dates, at most 31 days, a note
 * of at most 500 characters. */
export function unavailabilityErrors(f: UnavailabilityForm, t: T = en): Partial<Record<"from" | "to" | "note", string>> {
  const e: Partial<Record<"from" | "to" | "note", string>> = {};
  if (!isDate(f.from)) e.from = t("Choose the first day");
  if (!isDate(f.to)) e.to = t("Choose the last day");
  else if (isDate(f.from)) {
    const days = (Date.parse(`${f.to}T00:00:00Z`) - Date.parse(`${f.from}T00:00:00Z`)) / DAY;
    if (days < 0) e.to = t("The last day must be on or after the first day");
    else if (days > 30) e.to = t("At most 31 days at a time");
  }
  if (f.note.trim().length > 500) e.note = t("At most 500 characters");
  return e;
}

/** The confirmed assignments the days overlap (Kuala Lumpur days, the whole team when no technician is chosen):
 * shown before saving; saving keeps them (DD-P06). */
export function unavailabilityConflicts(jobs: TeamJob[], f: UnavailabilityForm): TeamJob[] {
  if (!isDate(f.from) || !isDate(f.to) || f.to < f.from) return [];
  const range = { startAt: klStart(f.from), endAt: new Date(Date.parse(klStart(f.to)) + DAY).toISOString() };
  return jobs.filter((j) => (!f.membershipId || j.technicianId === f.membershipId) && overlaps(j.slot, range));
}

const FIELD_TEXT: Record<string, Record<string, string>> = {
  to: { "error.range": "The last day must be on or after the first day, at most 31 days" },
  type: { "error.invalid": "Choose a type" },
  note: { "error.length": "At most 500 characters" },
};
/** Readable refusals of members.setUnavailability: the fields it names, or the reason the whole form was refused. */
export function unavailabilityRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): { fields: Record<string, string>; text: string | null } {
  if (f.code === "NOT_FOUND") return { fields: {}, text: t("That technician is no longer in your company — reload the page.") };
  if (f.code !== "VALIDATION") return { fields: {}, text: null }; // the toast says it (actionMessage)
  const fields = Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [k, FIELD_TEXT[k]?.[v] ? t(FIELD_TEXT[k][v]) : v]));
  return { fields, text: Object.keys(fields).length ? null : t("Check the input.") };
}
