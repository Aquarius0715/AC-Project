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
import { codePoints } from "@ac/web/lib/text";
import { isAfter, isBefore } from "@ac/web/lib/order";

const en = translator("en");
const DAY = 86_400_000, KL_OFFSET = 8 * 3_600_000;
export const KL = "Asia/Kuala_Lumpur";

/** members.list rows of a contractor's technicians (Membership of service-contracts.ts). */
export type ApiTeamMember = ApiMember & { organizationId: string; employment: string | null; validFrom: string; validUntil: string | null };

/** The qualification register a filter offers (QualificationCode, SR13). */
export const QUALIFICATION_CODES = ["demo_indoor", "demo_outdoor", "demo_electrical"] as const;
const isDate = (d?: string): d is string => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(`${d}T00:00:00Z`));
/** The Kuala Lumpur date (YYYY-MM-DD) of an instant. */
export const klDate = (iso: string) => new Date(Date.parse(iso) + KL_OFFSET).toISOString().slice(0, 10);
/** The instant a Kuala Lumpur day starts. */
export const klStart = (date: string) => new Date(Date.parse(`${date}T00:00:00Z`) - KL_OFFSET).toISOString();

/** The URL's conditions (SCR-P06): a valid date (default today in Kuala Lumpur), a register qualification and
activeOnly (default true). */
export function teamQuery(sp: { date?: string; qualification?: string; activeOnly?: string }, nowIso: string) {
  const today = klDate(nowIso);
  const asked = sp.qualification;
  const qualification = asked && (QUALIFICATION_CODES as readonly string[]).includes(asked) ? asked : null;
  return { date: isDate(sp.date) ? sp.date : today, activeOnly: sp.activeOnly !== "false", today, qualification };
}

/** Whether a membership is valid now: [validFrom, validUntil). */
export const activeNow = (m: ApiTeamMember, nowMs: number) => Date.parse(m.validFrom) <= nowMs && (!m.validUntil || nowMs < Date.parse(m.validUntil));
const hours = (min: number) => Math.round((min / 60) * 10) / 10;
const pct = (assigned: number, available: number) => (available > 0 ? Math.round((assigned / available) * 100) : null);
/** The short weekday of a Kuala Lumpur date in the user's language (“Tue”, “Sel”). */
export const weekday = (date: string, locale: Locale) => new Date(`${date}T04:00:00Z`).toLocaleDateString(intlTag(locale), { weekday: "short", timeZone: KL });
/** “Apr 2025”: the month a membership started, a Kuala Lumpur date in the user's language. */
const month = (iso: string, locale: Locale) => new Date(iso).toLocaleDateString(intlTag(locale), { month: "short", year: "numeric", timeZone: KL }).replaceAll(/[\u{A0}\u{2009}\u{202F}]/gu, " ");
const overlaps = (a: Slot, b: Slot) => Date.parse(a.startAt) < Date.parse(b.endAt) && Date.parse(b.startAt) < Date.parse(a.endAt);

/** A technician's job with a booked time (jobs.list per technician): its short ID, the slot and the slot as text. */
export interface TeamJob { jobId: string; short: string; technicianId: string; slot: Slot; text: string }

export interface MemberRow {
  id: string; name: string; sub: string; active: boolean;
  week: { text: string; pct: number | null }; day: { main: string; sub: string; tone?: "crit" | "warn" }; util: string;
}
/** The technicians card (Figma 04-1, 04-2, 04-3): qualifications and membership, the week's assigned / available
hours, and the chosen day's assigned and free time with its utilization — “—” without configured hours (DD-P06). */
export function memberRows(members: ApiTeamMember[], week: ApiCapacity[][], dayIndex: number, jobs: TeamJob[], nowMs: number, i: I18n = EN): MemberRow[] {
  const { t, display: { locale } } = i;
  return members.map((m): MemberRow => {
    const isActive = activeNow(m, nowMs);
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code, t)).join(" · ") || t("No qualifications");
    const since = isActive ? t("active since {month}", { month: month(m.validFrom, locale) }) : t("expired {date}", { date: m.validUntil ? businessDay(m.validUntil, locale) : "—" });
    const caps = week.map((d) => d.find((c) => c.membershipId === m.id));
    const assigned = caps.reduce((a, c) => a + (c?.assignedMinutes ?? 0), 0), available = caps.reduce((a, c) => a + (c?.availableMinutes ?? 0), 0);
    const row = { id: m.id, name: m.displayName, sub: `${quals} · ${since}`, active: isActive, week: { text: t("{a} h / {b} h", { a: hours(assigned), b: hours(available) }), pct: pct(assigned, available) } };
    if (!isActive) return { ...row, week: { text: "—", pct: null }, day: { main: t("Expired — cannot be assigned"), sub: "—", tone: "crit" }, util: "—" };
    const c = caps[dayIndex];
    if (c?.unavailability) return { ...row, day: { main: t("Unavailable · {reason}", { reason: unavailabilityLabel(c.unavailability, t) }), sub: "—", tone: "warn" }, util: "—" };
    if (!c?.availableMinutes) return { ...row, day: { main: t("No available hours set"), sub: "—" }, util: "—" };
    const mine = jobs.filter((j) => j.technicianId === m.id);
    const blocks = c.assignedSlots.map((s) => {
      const job = mine.find((j) => overlaps(j.slot, s));
      return job ? t("{slots} assigned ({job})", { slots: `${hhmm(s.startAt)}–${hhmm(s.endAt)}`, job: job.short }) : t("{slots} assigned", { slots: `${hhmm(s.startAt)}–${hhmm(s.endAt)}` });
    });
    const free = freeSlots(c);
    const usage = pct(c.assignedMinutes, c.availableMinutes);
    return {
      ...row, util: usage === null ? "—" : `${String(usage)}%`,
      day: { main: blocks.length > 0 ? blocks.join(", ") : t("No assignments"), sub: free.length > 0 ? t("{slots} free", { slots: free.join(", ") }) : t("fully booked") },
    };
  });
}

export interface WeekCell { text: string; kind: "busy" | "free" | "off" | "none" }
/** The team's week of assigned / available hours per technician and Kuala Lumpur day (Figma 04-1): “2 / 8 h”, “—”
on a day without configured hours, the unavailability on a day off. */
export function weekRows(members: ApiTeamMember[], week: ApiCapacity[][], t: T = en): { id: string; name: string; cells: WeekCell[] }[] {
  return members.map((m) => {
    return {
      id: m.id, name: m.displayName,
      cells: week.map((d): WeekCell => {
        const c = d.find((x) => x.membershipId === m.id);
        if (c?.unavailability) return { text: unavailabilityLabel(c.unavailability, t), kind: "off" };
        if (!c?.availableMinutes) return { text: "—", kind: "none" };
        return { text: t("{a} / {b} h", { a: hours(c.assignedMinutes), b: hours(c.availableMinutes) }), kind: c.assignedMinutes > 0 ? "busy" : "free" };
      }),
    };
  });
}

/** The week's totals and the chosen day's free hours (Figma 04-1): assigned, available, team utilization. */
export function teamStats(members: ApiTeamMember[], week: ApiCapacity[][], dayIndex: number, t: T = en) {
  const ids = new Set(members.map((m) => m.id));
  const mine = (d: ApiCapacity[]) => d.filter((c) => ids.has(c.membershipId));
  const assigned = week.flatMap((item) => mine(item)).reduce((a, c) => a + c.assignedMinutes, 0), available = week.flatMap((item) => mine(item)).reduce((a, c) => a + (c.availableMinutes ?? 0), 0);
  const free = mine(week[dayIndex] ?? []).reduce((a, c) => a + Math.max(0, (c.availableMinutes ?? 0) - c.assignedMinutes), 0);
  const usage = pct(assigned, available);
  return { assigned: t("{n} h", { n: hours(assigned) }), available: t("{n} h", { n: hours(available) }), utilization: usage === null ? "—" : `${String(usage)}%`, free: t("{n} h", { n: hours(free) }) };
}

export interface GrantRow { key: string; technician: string; name: string; sub: string; until: string; badge: { text: string; tone: "ok" | "warn" | "muted" } }
/** The qualification grants of the listed technicians (Figma 04-1): valid, expiring within 30 days (IR133 item 2),
expired or revoked, and each register qualification nobody listed holds (“not held”). Dates are Kuala Lumpur days. */
export function grantRows(members: ApiTeamMember[], nowMs: number, i: I18n = EN): GrantRow[] {
  const { t, display: { locale } } = i;
  type Grant = NonNullable<ApiTeamMember["qualifications"]>[number];
  // a grant's line and badge: revoked, expired, expiring within 30 days, or valid
  const standing = (name: string, q: Grant, until: string): [string, GrantRow["badge"]] => {
    if (q.revokedAt) return [t("{name} · revoked {date}", { name, date: businessDay(q.revokedAt, locale) }), { text: t("Revoked"), tone: "muted" }];
    const end = q.validUntil ? Date.parse(q.validUntil) : Infinity;
    if (end <= nowMs) return [t("{name} · expired {date}", { name, date: until }), { text: t("Expired"), tone: "muted" }];
    const days = Math.ceil((end - nowMs) / DAY);
    return days <= 30
      ? [t("{name} · expires {date}", { name, date: until }), { text: t("Expiring · {n} d", { n: days }), tone: "warn" }]
      : [t("{name} · valid to {date}", { name, date: until }), { text: t("Valid"), tone: "ok" }];
  };
  const rows: GrantRow[] = [];
  for (const m of members) {
    if ((m.qualifications) != null) {
      for (const q of m.qualifications) {
        const until = q.validUntil ? businessDay(q.validUntil, locale) : "—";
        const [sub, badge] = standing(m.displayName, q, until);
        rows.push({ key: `${m.id}:${q.code}`, technician: m.displayName, name: qualificationLabel(q.code, t), sub, until, badge });
      }
    }
  }
  const held = new Set(members.flatMap((m) => (qualifiedCodes(m, nowMs))));
  for (const code of QUALIFICATION_CODES) {
    if (!held.has(code)) rows.push({ key: `none:${code}`, technician: "—", name: qualificationLabel(code, t), sub: t("not held"), until: "—", badge: { text: "—", tone: "muted" } });
  }
  return rows.toSorted((a, b) => a.name.localeCompare(b.name) || a.technician.localeCompare(b.technician));
}
const qualifiedCodes = (m: ApiTeamMember, nowMs: number) => QUALIFICATION_CODES.filter((code) => qualified(m, [code], new Date(nowMs).toISOString()));

/** The technicians of the URL's conditions: activeOnly hides memberships that are not valid now, a qualification
keeps those holding it on the chosen day; `hidden` counts the expired memberships activeOnly hides. */
export function listed(members: ApiTeamMember[], q: { date: string; qualification: string | null; activeOnly: boolean }, nowMs: number) {
  const on = new Date(Date.parse(klStart(q.date)) + 9 * 3_600_000).toISOString(); // 09:00 in Kuala Lumpur, the start of the working day
  const kept = members.filter((m) => !q.qualification || qualified(m, [q.qualification], on));
  const expired = kept.filter((m) => !activeNow(m, nowMs));
  return { rows: q.activeOnly ? kept.filter((m) => activeNow(m, nowMs)) : kept, hidden: q.activeOnly ? expired.length : 0, expired: q.activeOnly ? [] : expired.map((m) => m.displayName) };
}

export const UNAVAILABILITY_TYPES = [
  { id: "annual_leave", text: "Annual leave" }, { id: "training", text: "Training" }, { id: "public_holiday", text: "Public holiday" },
  { id: "sick", text: "Sick leave" }, { id: "other", text: "Other" },
] as const;
export interface UnavailabilityForm { membershipId: string; from: string; to: string; type: (typeof UNAVAILABILITY_TYPES)[number]["id"]; note: string }

/** What is wrong with the last day of an unavailability, or undefined. */
function lastDayError(f: UnavailabilityForm, t: T): string | undefined {
  if (!isDate(f.to)) return t("Choose the last day");
  if (!isDate(f.from)) return undefined;
  const days = (Date.parse(`${f.to}T00:00:00Z`) - Date.parse(`${f.from}T00:00:00Z`)) / DAY;
  if (days < 0) return t("The last day must be on or after the first day");
  return days > 30 ? t("At most 31 days at a time") : undefined;
}
/** The form's own checks before members.setUnavailability (DD-P06 Figma 04-8, IR111): dates, at most 31 days, a note
of at most 500 characters. */
export function unavailabilityErrors(f: UnavailabilityForm, t: T = en): Partial<Record<"from" | "to" | "note", string>> {
  const e: Partial<Record<"from" | "to" | "note", string>> = {};
  if (!isDate(f.from)) e.from = t("Choose the first day");
  const last = lastDayError(f, t);
  if (last) e.to = last;
  if (codePoints(f.note.trim()) > 500) e.note = t("At most 500 characters");
  return e;
}

/** The confirmed assignments the days overlap (Kuala Lumpur days, the whole team when no technician is chosen):
shown before saving; saving keeps them (DD-P06). */
export function unavailabilityConflicts(jobs: TeamJob[], f: UnavailabilityForm): TeamJob[] {
  if (!isDate(f.from) || !isDate(f.to) || isBefore(f.to, f.from)) return [];
  const range = { startAt: klStart(f.from), endAt: new Date(Date.parse(klStart(f.to)) + DAY).toISOString() };
  return jobs.filter((j) => (!f.membershipId || j.technicianId === f.membershipId) && overlaps(j.slot, range));
}

const FIELD_TEXT: Record<string, Record<string, string>> = {
  to: { "error.range": "The last day must be on or after the first day, at most 31 days" },
  type: { "error.invalid": "Choose a type" },
  note: { "error.length": "At most 500 characters" },
};
/** Readable refusals of members.setUnavailability: the fields it names, or the reason the whole form was refused. */
export function unavailabilityRefusal(f: { code: string; fieldErrors: Record<string, string> }, t: T = en): { fields: Record<string, string>; text: string | null } {
  if (f.code === "NOT_FOUND") return { fields: {}, text: t("That technician is no longer in your company — reload the page.") };
  if (f.code !== "VALIDATION") return { fields: {}, text: null }; // the toast says it (actionMessage)
  const fields = Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [k, FIELD_TEXT[k]?.[v] ? t(FIELD_TEXT[k][v]) : v]));
  return { fields, text: Object.keys(fields).length > 0 ? null : t("Check the input.") };
}

/** members.unavailability rows (Unavailability of service-contracts.ts, IR612). */
export interface ApiUnavailability { id: string; version: number; membershipId: string | null; from: string; to: string; type: string; note: string | null }

/** The technicians as the select and the unavailable days show them (IR612): a name two technicians share gets each
one's e-mail (their partner user), so the right one is picked; without an e-mail the name stays as it is. */
export function technicianNames(members: { id: string; displayName: string }[], emails: Map<string, string>): Map<string, string> {
  const count = new Map<string, number>();
  for (const m of members) count.set(m.displayName, (count.get(m.displayName) ?? 0) + 1);
  return new Map(members.map((m) => {
    const email = emails.get(m.id);
    return [m.id, email && (count.get(m.displayName) ?? 0) > 1 ? `${m.displayName} · ${email}` : m.displayName];
  }));
}

/** Kuala Lumpur days as text in the user's language: “Wed 16 Sept” or “Wed 16 – Fri 18 Sept”. */
export function dayRange(from: string, to: string, locale: Locale): string {
  const noon = (d: string) => new Date(`${d}T04:00:00Z`); // 12:00 in Kuala Lumpur
  const f = new Intl.DateTimeFormat(intlTag(locale), { weekday: "short", day: "numeric", month: "short", timeZone: KL });
  return (from === to ? f.format(noon(from)) : f.formatRange(noon(from), noon(to))).replaceAll(/[\u{A0}\u{2009}\u{202F}]/gu, " ");
}

/** The day before a date (YYYY-MM-DD). */
export const dayBefore = (date: string) => new Date(Date.parse(`${date}T00:00:00Z`) - DAY).toISOString().slice(0, 10);

export interface UnavailabilityRow {
  id: string; version: number; who: string; type: string; note: string | null; days: string;
  from: string; to: string; first: string; started: boolean;
}
/** The upcoming unavailable days on Team & capacity (IR612): who — “All technicians” for the whole company — the
type, the Kuala Lumpur days and the note, first day first. `first` is the earliest day that can be removed: a row
that started before today keeps its past days. */
export function unavailabilityRows(rows: ApiUnavailability[], names: Map<string, string>, today: string, i: I18n = EN): UnavailabilityRow[] {
  const { t, display: { locale } } = i;
  return [...rows].toSorted((a, b) => a.from.localeCompare(b.from) || a.id.localeCompare(b.id)).map((u) => {
    return {
      id: u.id, version: u.version, who: u.membershipId ? names.get(u.membershipId) ?? t("Technician") : t("All technicians"),
      type: t(UNAVAILABILITY_TYPES.find((x) => x.id === u.type)?.text ?? "Other"), note: u.note, days: dayRange(u.from, u.to, locale),
      from: u.from, to: u.to, first: isBefore(u.from, today) ? today : u.from, started: isBefore(u.from, today),
    };
  });
}

/** The remove dialog's own check before members.clearUnavailability: a day from `first` to the row's last day. */
export function clearError(row: Pick<UnavailabilityRow, "first" | "to">, from: string, locale: Locale, t: T = en): string | null {
  return isDate(from) && !isBefore(from, row.first) && !isAfter(from, row.to) ? null : t("Choose a day from {first} to {last}", { first: dayRange(row.first, row.first, locale), last: dayRange(row.to, row.to, locale) });
}

/** What removing from `from` does to the row, said before the user confirms: all of it, or the days that stay. */
export function clearEffect(row: Pick<UnavailabilityRow, "from" | "started">, from: string, locale: Locale, t: T = en): string {
  const past = row.started ? ` ${t("Days before today stay recorded.")}` : "";
  return isAfter(from, row.from) ? `${t("Still unavailable: {days}; the days from {day} on are removed.", { days: dayRange(row.from, dayBefore(from), locale), day: dayRange(from, from, locale) })}${past}` : t("All of these days are removed.");
}
