// Customer energy & cost and carbon offsets (FR-C06, FR-C13, FR-C16, DATA_SOURCE=API): the period choices, the daily
// buckets of energy.summary, the unit comparison rows and the selection carried to the offsets page. Pure code shared
// by the Server Components and the client views. The periods are Kuala Lumpur days (REV18-035); their labels follow the
// display language (`i`, IR265).
import { coveragePct, klInstant, klLocal, klStamp, one, saving, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import { EN, intlTag, showDay, translator, type I18n, type Locale, type T } from "@ac/web/lib/i18n";
import { filled } from "@ac/web/lib/filled";

const ASIA_KUALA_LUMPUR = "Asia/Kuala_Lumpur";

export type PeriodKind = "today" | "7d" | "30d" | "custom";
export const periodKinds: PeriodKind[] = ["today", "7d", "30d", "custom"];
const dayMs = 86_400_000;
const addDays = (ymd: string, n: number) => new Date(Date.parse(`${ymd}T00:00:00Z`) + n * dayMs).toISOString().slice(0, 10);
const fmt = (iso: string, locale: Locale) => new Date(iso).toLocaleString(intlTag(locale), { timeZone: ASIA_KUALA_LUMPUR, month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).replaceAll(/[\u{A0}\u{2009}\u{202F}]/gu, " ");

// the days of a preset period; the month is the remaining preset
const SPAN_DAYS = new Map<PeriodKind, number>([["today", 1], ["7d", 7]]);
export interface Period { kind: PeriodKind; from: string; to: string; label: string; days: number; error?: string }
/** [from, to) of a period choice: whole Kuala Lumpur days up to the current minute of the business clock (today, the
last 7 or 30 days), or the custom range (KL datetime-local values, at most 366 days, DD-C06). */
export function periodRange(kind: PeriodKind, now: Date, custom: { from?: string; to?: string } = {}, i: I18n = EN): Period {
  const minute = new Date(Math.floor(now.getTime() / 60_000) * 60_000).toISOString();
  let from: string, to: string;
  if (kind === "custom") {
    const today = klLocal(now.toISOString()).slice(0, 10);
    from = klInstant(filled(custom.from) ?? `${addDays(today, -6)}T00:00`);
    to = custom.to ? klInstant(custom.to) : minute;
  } else {
    const days = SPAN_DAYS.get(kind) ?? 30;
    from = klInstant(`${addDays(klLocal(now.toISOString()).slice(0, 10), -(days - 1))}T00:00`);
    to = minute;
  }
  const span = Date.parse(to) - Date.parse(from);
  const days = Math.max(1, Math.ceil(span / dayMs));
  const tooLong = span > 366 * dayMs ? i.t("At most 366 days") : undefined;
  const error = span > 0 ? tooLong : i.t("The end must be after the start");
  const l = i.display.locale;
  return { kind, from, to, days, error, label: i.t(days === 1 ? "{from} – {to} ({n} day) · {zone}" : "{from} – {to} ({n} days) · {zone}", { from: fmt(from, l), to: fmt(to, l), n: days, zone: ASIA_KUALA_LUMPUR }) };
}
/** The Kuala Lumpur days of [from, to) (the last one ends at `to`), at most 31 for the daily chart, labelled “Mon 14”
in the display language (Figma Client 04a). */
export function dayRanges(from: string, to: string, locale: Locale = "en"): { label: string; from: string; to: string }[] {
  const out: { label: string; from: string; to: string }[] = [];
  let start = from;
  while (Date.parse(start) < Date.parse(to) && out.length < 31) {
    const next = klInstant(`${addDays(klLocal(start).slice(0, 10), 1)}T00:00`);
    const end = Date.parse(next) < Date.parse(to) ? next : to;
    out.push({ label: showDay(start, { locale, timeZone: ASIA_KUALA_LUMPUR }), from: start, to: end }); // "Mon 14" (Figma Client 04a)
    start = next;
  }
  return out;
}
/** Baselines that can be compared with the unit set (D07: the same units); the backend still checks period and coverage. */
export const baselinesFor = (bs: ApiBaseline[], unitIds: string[]) =>
  bs.filter((b) => b.unitIds.length === unitIds.length && unitIds.every((u) => b.unitIds.includes(u)) && b.baselineKWh !== null);
export const baselineLabel = (b: ApiBaseline) => `${b.method} · ${one(b.baselineKWh ?? 0)} kWh · ${klStamp(b.period.from).slice(0, 10)} → ${klStamp(b.period.to).slice(0, 10)}`;

/** One unit of the comparison table (Figma Client 04b): actual, baseline, IR68 difference and coverage. */
export interface CompareRow { unitId: string; name: string; actual: string; baseline: string; difference: string; tone: "ok" | "warn" | "muted"; coverage: string }
/** The saving against the unit's baseline, or why there is none to show. */
function baselineDifference(s: ApiEnergySummary, tr: T): string {
  const b = s.baselineSnapshot;
  if (!b) return tr("No baseline for this unit");
  if (s.totals.savedKWh === null) return tr("Cannot calculate — baseline not comparable");
  if (b.baselineKWh === 0) return tr("Cannot calculate — baseline is 0");
  return `${saving(s.totals.savedKWh, " kWh", one, tr)} · ${saving(s.totals.savingPercentage, "%", one, tr)}`;
}
/** A saving is ok, a higher use than the baseline a warning, and no comparison or no change muted. */
function savingTone(saved: number | null): "ok" | "warn" | "muted" {
  if (saved === null || saved === 0) return "muted";
  return saved > 0 ? "ok" : "warn";
}
export function compareRow(unitId: string, name: string, s: ApiEnergySummary, tr: T = translator("en")): CompareRow {
  const t = s.totals;
  const baselineKWh = s.baselineSnapshot?.baselineKWh ?? null;
  const difference = baselineDifference(s, tr);
  return {
    unitId,
    name,
    difference,
    actual: t.kWh === null ? "—" : one(t.kWh),
    baseline: baselineKWh === null ? "—" : one(baselineKWh),
    tone: savingTone(t.savedKWh),
    coverage: s.coverage === null ? "—" : `${coveragePct(s.coverage)}%`,
  };
}
/** The URL's units (up to 4 of the customer's), else the first unit by name — the same default on Energy & cost and
Offsets (IR265). */
export function pickUnits(units: { id: string; displayName: string }[], parameter: string | undefined = ""): string[] {
  const picked = [...new Set(parameter.split(","))].filter((id) => units.some((u) => u.id === id)).slice(0, 4);
  return picked.length > 0 ? picked : [...units].toSorted((a, b) => a.displayName.localeCompare(b.displayName)).slice(0, 1).map((u) => u.id);
}
/** The Kuala Lumpur month before the one `now` is in, as "YYYY-MM": the first month of the report export's list (IR679). It comes from the Kuala Lumpur date, not the UTC one, which is still in the old month for eight hours at the end of every month. */
export function previousMonth(now: Date): string {
  const [year = 0, month = 1] = klLocal(now.toISOString()).slice(0, 7).split("-").map(Number);
  const before = month === 1 ? { year: year - 1, month: 12 } : { year, month: month - 1 };
  return `${String(before.year)}-${String(before.month).padStart(2, "0")}`;
}
/** The energy screen's selection as URL parameters (also passed to the offsets page, FR-C13 “Based on Energy & cost”). */
export function selectionQuery(p: { unitIds: string[]; period: PeriodKind; from?: string; to?: string; baselineId?: string | null }): string {
  const q = new URLSearchParams({ unitIds: p.unitIds.join(","), period: p.period });
  if (p.period === "custom" && p.from && p.to) {
    q.set("from", p.from);
    q.set("to", p.to);
  }
  if (p.baselineId) q.set("baselineId", p.baselineId);
  return q.toString();
}
