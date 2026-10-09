// HQ maintenance plans (FR-A06 Plans tab, DD-A06 item 9, D16, IR130, Figma Admin 06 283:2 / 360:414) from the Core
// API: one row per plan, the plan form (monthly, every 1–12 months, the next date entered in the display zone and
// stored in UTC), the next-occurrence box with Generate job, the generated occurrences and the refusals. Pure code shared
// by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";

export type ApiPlan = {
  id: string; version: number; unitId: string; recurrence: { kind: "monthly"; intervalMonths: number }; timezone: string; anchorDay: number; nextDueAt: string;
  generatedOccurrences: { occurrenceAt: string; jobId: string }[]; createdAt: string; updatedAt: string;
};

/** “Every 3 months” / “Every month”. */
export const everyText = (n: number) => (n === 1 ? "Every month" : `Every ${n} months`);
/** The plan's cadence in the title: monthly, quarterly, half-yearly, yearly or every N months. */
export const cadence = (n: number) => ({ 1: "monthly", 3: "quarterly", 6: "half-yearly", 12: "yearly" } as Record<number, string>)[n] ?? `every ${n} months`;
/** The date part of an instant in the display zone (Asia/Kuala_Lumpur). */
export const klDate = (iso: string) => klTime(iso).slice(0, 10);
/** A datetime-local value in the display zone, and back to UTC (D16: the UI converts display-zone input to UTC). */
export const klInput = (iso: string) => klTime(iso).replace(" ", "T");
export const fromKlInput = (v: string) => new Date(`${v}:00+08:00`).toISOString();

export type PlanRow = { id: string; short: string; unitId: string; unit: string; customer: string; line: string; next: string };
/** One plan of the list (Figma “Living room AC · customer-a / Every 3 months · day 8 · Next 2026-12-08”), next due first. */
export function planRows(plans: ApiPlan[], units: Map<string, string>, customerOfUnit: Map<string, string>): PlanRow[] {
  return [...plans].sort((a, b) => Date.parse(a.nextDueAt) - Date.parse(b.nextDueAt) || a.id.localeCompare(b.id)).map((p) => ({
    id: p.id, short: p.id.slice(0, 8), unitId: p.unitId, unit: units.get(p.unitId) ?? "Unit", customer: customerOfUnit.get(p.unitId) ?? "customer",
    line: `${everyText(p.recurrence.intervalMonths)} · day ${p.anchorDay}`, next: `Next ${klDate(p.nextDueAt)}`,
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
export function planErrors(f: { unitId: string; every: number; nextDueAt: string | null }, now: number): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.unitId) e.unitId = "Choose the unit.";
  if (!Number.isInteger(f.every) || f.every < 1 || f.every > 12) e.every = "Every 1–12 months.";
  if (!f.nextDueAt || Number.isNaN(Date.parse(f.nextDueAt))) e.nextDueAt = "Enter the next date and time.";
  else if (Date.parse(f.nextDueAt) <= now) e.nextDueAt = "The next date must be in the future.";
  return e;
}

/** The next-occurrence box (Figma 283:2 / 360:414) and whether Generate job is allowed now. */
export function nextBox(p: ApiPlan, now: number, dirty: boolean, generated: { occurrenceAt: string; jobId: string } | null): { title: string; text: string; generate: boolean; why: string | null } {
  const title = `Next occurrence · ${klDate(p.nextDueAt)}`;
  const past = Date.parse(p.nextDueAt) <= now;
  const text = generated ? `${klDate(generated.occurrenceAt)} was just generated as ${generated.jobId.slice(0, 8)}. Next due advanced to ${klDate(p.nextDueAt)}.`
    : past ? "The next date has passed — move it forward and save the plan first (no automatic catch-up, D16)."
    : "Not generated yet. Generating creates one periodic job (requested) for this date — generating the same date again is rejected.";
  const why = past ? "The next date has passed." : dirty ? "Save the plan first." : null;
  return { title, text, generate: !why, why };
}

/** The CONFLICT banner after a refused Generate job (Figma 360:414): the date that already has its job, or the plan
 * that changed meanwhile. */
export function generateConflict(p: ApiPlan, attempted: string): { title: string; text: string } {
  const done = p.generatedOccurrences.find((o) => o.occurrenceAt === attempted);
  return done
    ? { title: `CONFLICT · ${klDate(attempted)} already has a job`, text: `A second “Generate job” for the same plan and date (another tab / double submit) was rejected. Still one job: ${done.jobId.slice(0, 8)}.` }
    : { title: "CONFLICT · the plan changed meanwhile", text: `Nothing was generated for ${klDate(attempted)}; the latest plan is shown — check the next date and try again.` };
}

/** Readable refusals of plans.save / plans.generateNext. */
export function planRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  const keys: Record<string, string> = {
    "errors.occurrence_generated": "this date already has a job",
    "errors.occurrence_mismatch": "the plan’s next date changed meanwhile — the latest plan is shown",
    "errors.next_date_past": "the next date has passed — move it forward and save the plan first",
    "error.versionConflict": "the plan changed meanwhile — the latest version is shown",
    "error.unitArchived": "the unit is archived", "error.unitFixed": "a plan’s unit cannot change",
    "error.past": "must be in the future", "error.invalid": "not a valid value (monthly, every 1–12 months)", "error.required": "required",
  };
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved (CONFLICT): " : ""}${keys[f.messageKey]}.`;
  if (f.code === "NOT_FOUND") return "The plan or its unit no longer exists in your scope.";
  return `${f.code} — ${f.messageKey}`;
}
