// HQ maintenance plans (FR-A06 Plans tab, DD-A06 item 9, D16, IR130, Figma Admin 06 283:2 / 360:414) from the Core
// API: one row per plan, the plan form (monthly, every 1–12 months, the next date entered in the display zone and
// stored in UTC), the next-occurrence box with Generate job, the generated occurrences and the refusals. Pure code shared
// by the server loader and the client view; Vitest covers it. Texts in the display language (`i` / `t`, IR291); a
// plan's dates are its instants in the user's display time zone, the zone its next date is typed in (D16, NFR-08).
import { EN, showDate, translator, type Display, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

export type ApiPlan = {
  id: string; version: number; unitId: string; recurrence: { kind: "monthly"; intervalMonths: number }; timezone: string; anchorDay: number; nextDueAt: string;
  generatedOccurrences: { occurrenceAt: string; jobId: string }[]; createdAt: string; updatedAt: string;
};

/** “Every 3 months” / “Every month”. */
export const everyText = (n: number, t: T = en) => (n === 1 ? t("Every month") : t("Every {n} months", { n }));
/** The plan's cadence in the title: monthly, quarterly, half-yearly, yearly or every N months. */
export const cadence = (n: number, t: T = en) => t(({ 1: "monthly", 3: "quarterly", 6: "half-yearly", 12: "yearly" } as Record<number, string>)[n] ?? "every {n} months", { n });
/** The date of a plan instant in the display zone: “8 Dec 2026”. */
export const planDate = (iso: string, d?: Display) => showDate(iso, d);

export type PlanRow = { id: string; short: string; unitId: string; unit: string; customer: string; line: string; next: string };
/** One plan of the list (Figma “Living room AC · customer-a / Every 3 months · day 8 · Next 2026-12-08”), next due first. */
export function planRows(plans: ApiPlan[], units: Map<string, string>, customerOfUnit: Map<string, string>, i: I18n = EN): PlanRow[] {
  const { t, display } = i;
  return [...plans].sort((a, b) => Date.parse(a.nextDueAt) - Date.parse(b.nextDueAt) || a.id.localeCompare(b.id)).map((p) => ({
    id: p.id, short: p.id.slice(0, 8), unitId: p.unitId, unit: units.get(p.unitId) ?? t("Unit"), customer: customerOfUnit.get(p.unitId) ?? t("customer"),
    line: `${everyText(p.recurrence.intervalMonths, t)} · ${t("day {n}", { n: p.anchorDay })}`, next: t("Next {date}", { date: planDate(p.nextDueAt, display) }),
  }));
}

/** The occurrence after `iso` (D16): add the months to the UTC year and month, day = min(anchor day, days in that
 * month), the same UTC time — 2027-01-31T10:00Z → 2027-02-28T10:00Z → 2027-03-31T10:00Z. */
export function nextOccurrence(iso: string, months: number, anchorDay: number): string {
  const t = new Date(iso);
  const first = Date.UTC(t.getUTCFullYear(), t.getUTCMonth() + months, 1, t.getUTCHours(), t.getUTCMinutes(), t.getUTCSeconds(), t.getUTCMilliseconds());
  const f = new Date(first);
  const last = new Date(Date.UTC(f.getUTCFullYear(), f.getUTCMonth() + 1, 0)).getUTCDate();
  return new Date(first + (Math.min(anchorDay, last) - 1) * 86_400_000).toISOString();
}
/** The anchor day a save stores: the UTC day of a changed next date, otherwise the plan's (IR130 item 1). */
export const anchorOf = (p: Pick<ApiPlan, "anchorDay" | "nextDueAt"> | null, nextDueAt: string) => (p && p.nextDueAt === nextDueAt ? p.anchorDay : new Date(nextDueAt).getUTCDate());

/** The plan form checks: every 1–12 months, the next date in the future (the unit only when creating). */
export function planErrors(f: { unitId: string; every: number; nextDueAt: string | null }, now: number, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.unitId) e.unitId = t("Choose the unit.");
  if (!Number.isInteger(f.every) || f.every < 1 || f.every > 12) e.every = t("Every 1–12 months.");
  if (!f.nextDueAt || Number.isNaN(Date.parse(f.nextDueAt))) e.nextDueAt = t("Enter the next date and time.");
  else if (Date.parse(f.nextDueAt) <= now) e.nextDueAt = t("The next date must be in the future.");
  return e;
}

/** The next-occurrence box (Figma 283:2 / 360:414) and whether Generate job is allowed now. `next` is the saved next
 * date as text: the page passes the one the server formatted, so the first render matches it (IR282). */
export function nextBox(p: ApiPlan, now: number, dirty: boolean, generated: { occurrenceAt: string; jobId: string } | null, i: I18n = EN, next = planDate(p.nextDueAt, i.display)): { title: string; text: string; generate: boolean; why: string | null } {
  const { t } = i;
  const past = Date.parse(p.nextDueAt) <= now;
  const text = generated ? t("{date} was just generated as {job}. Next due advanced to {next}.", { date: planDate(generated.occurrenceAt, i.display), job: generated.jobId.slice(0, 8), next })
    : past ? t("The next date has passed — move it forward and save the plan first (no automatic catch-up, D16).")
    : t("Not generated yet. Generating creates one periodic job (requested) for this date — generating the same date again is rejected.");
  const why = past ? t("The next date has passed.") : dirty ? t("Save the plan first.") : null;
  return { title: t("Next occurrence · {date}", { date: next }), text, generate: !why, why };
}

/** The CONFLICT banner after a refused Generate job (Figma 360:414): the date that already has its job, or the plan
 * that changed meanwhile. */
export function generateConflict(p: ApiPlan, attempted: string, i: I18n = EN): { title: string; text: string } {
  const { t } = i;
  const date = planDate(attempted, i.display);
  const done = p.generatedOccurrences.find((o) => o.occurrenceAt === attempted);
  return done
    ? { title: t("CONFLICT · {date} already has a job", { date }), text: t("A second “Generate job” for the same plan and date (another tab / double submit) was rejected. Still one job: {job}.", { job: done.jobId.slice(0, 8) }) }
    : { title: t("CONFLICT · the plan changed meanwhile"), text: t("Nothing was generated for {date}; the latest plan is shown — check the next date and try again.", { date }) };
}

/** Readable refusals of plans.save / plans.generateNext. */
export function planRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string {
  const keys: Record<string, string> = {
    "errors.occurrence_generated": "this date already has a job",
    "errors.occurrence_mismatch": "the plan’s next date changed meanwhile — the latest plan is shown",
    "errors.next_date_past": "the next date has passed — move it forward and save the plan first",
    "error.versionConflict": "the plan changed meanwhile — the latest version is shown",
    "error.unitArchived": "the unit is archived", "error.unitFixed": "a plan’s unit cannot change",
    "error.past": "must be in the future", "error.invalid": "not a valid value (monthly, every 1–12 months)", "error.required": "required",
  };
  const word = (k: string) => (keys[k] ? t(keys[k]) : k);
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${word(v)}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return f.code === "CONFLICT" ? t("Not saved (CONFLICT): {reason}.", { reason: word(f.messageKey) }) : `${word(f.messageKey)}.`;
  if (f.code === "NOT_FOUND") return t("The plan or its unit no longer exists in your scope.");
  return `${f.code} — ${f.messageKey}`;
}
