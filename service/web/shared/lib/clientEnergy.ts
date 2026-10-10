// Customer energy & cost and carbon offsets (FR-C06, FR-C13, FR-C16, DATA_SOURCE=api): the period choices, the daily
// buckets of energy.summary, the unit comparison rows and the selection carried to the offsets page. Pure code shared
// by the Server Components and the client views.
import { klInstant, klLocal, klStamp, one, saving, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import { intlTag, type Locale } from "@ac/web/lib/i18n";

export type PeriodKind = "today" | "7d" | "30d" | "custom";
export const periodKinds: PeriodKind[] = ["today", "7d", "30d", "custom"];
const dayMs = 86_400_000;
const addDays = (ymd: string, n: number) => new Date(Date.parse(`${ymd}T00:00:00Z`) + n * dayMs).toISOString().slice(0, 10);
const fmt = (iso: string) => new Date(iso).toLocaleString("en-US", { timeZone: "Asia/Kuala_Lumpur", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });

export type Period = { kind: PeriodKind; from: string; to: string; label: string; days: number; error?: string };
/** [from, to) of a period choice: whole Kuala Lumpur days up to the current minute of the business clock (today, the
 * last 7 or 30 days), or the custom range (KL datetime-local values, at most 366 days, DD-C06). */
export function periodRange(kind: PeriodKind, now: Date, custom: { from?: string; to?: string } = {}): Period {
  const minute = new Date(Math.floor(now.getTime() / 60000) * 60000).toISOString();
  let from: string, to: string;
  if (kind === "custom") {
    const today = klLocal(now.toISOString()).slice(0, 10);
    from = custom.from ? klInstant(custom.from) : klInstant(`${addDays(today, -6)}T00:00`);
    to = custom.to ? klInstant(custom.to) : minute;
  } else {
    const days = kind === "today" ? 1 : kind === "7d" ? 7 : 30;
    from = klInstant(`${addDays(klLocal(now.toISOString()).slice(0, 10), -(days - 1))}T00:00`);
    to = minute;
  }
  const span = Date.parse(to) - Date.parse(from);
  const days = Math.max(1, Math.ceil(span / dayMs));
  const error = !(span > 0) ? "The end must be after the start" : span > 366 * dayMs ? "At most 366 days" : undefined;
  return { kind, from, to, days, error, label: `${fmt(from)} – ${fmt(to)} (${days} day${days === 1 ? "" : "s"}) · Asia/Kuala_Lumpur` };
}
/** The Kuala Lumpur days of [from, to) (the last one ends at `to`), at most 31 for the daily chart, labelled “Mon 14”
 * in the display language (Figma Client 04a). */
export function dayRanges(from: string, to: string, locale: Locale = "en"): { label: string; from: string; to: string }[] {
  const out: { label: string; from: string; to: string }[] = [];
  let start = from;
  while (Date.parse(start) < Date.parse(to) && out.length < 31) {
    const next = klInstant(`${addDays(klLocal(start).slice(0, 10), 1)}T00:00`);
    const end = Date.parse(next) < Date.parse(to) ? next : to;
    const day = (o: Intl.DateTimeFormatOptions) => new Date(start).toLocaleDateString(intlTag(locale), { timeZone: "Asia/Kuala_Lumpur", ...o });
    out.push({ label: `${day({ weekday: "short" })} ${day({ day: "numeric" })}`, from: start, to: end }); // "Mon 14" (Figma Client 04a)
    start = next;
  }
  return out;
}
/** Baselines that can be compared with the unit set (D07: the same units); the backend still checks period and coverage. */
export const baselinesFor = (bs: ApiBaseline[], unitIds: string[]) =>
  bs.filter((b) => b.unitIds.length === unitIds.length && unitIds.every((u) => b.unitIds.includes(u)) && b.baselineKWh !== null);
export const baselineLabel = (b: ApiBaseline) => `${b.method} · ${one(b.baselineKWh ?? 0)} kWh · ${klStamp(b.period.from).slice(0, 10)} → ${klStamp(b.period.to).slice(0, 10)}`;

/** One unit of the comparison table (Figma Client 04b): actual, baseline, IR68 difference and coverage. */
export type CompareRow = { unitId: string; name: string; actual: string; baseline: string; difference: string; tone: "ok" | "warn" | "muted"; coverage: string };
export function compareRow(unitId: string, name: string, s: ApiEnergySummary): CompareRow {
  const t = s.totals;
  const b = s.baselineSnapshot;
  const comparable = t.savedKWh !== null;
  const difference = !b ? "No baseline for this unit" : !comparable ? "Cannot calculate — baseline not comparable" : b.baselineKWh === 0 ? "Cannot calculate — baseline is 0"
    : `${saving(t.savedKWh, " kWh")} · ${saving(t.savingPercentage, "%")}`;
  return {
    unitId, name, actual: t.kWh === null ? "—" : one(t.kWh), baseline: b?.baselineKWh === null || !b ? "—" : one(b.baselineKWh), difference,
    tone: comparable && (t.savedKWh ?? 0) > 0 ? "ok" : comparable && (t.savedKWh ?? 0) < 0 ? "warn" : "muted",
    coverage: s.coverage === null ? "—" : `${one(s.coverage * 100)}%`,
  };
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
