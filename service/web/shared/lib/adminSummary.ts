// Admin dashboard KPIs (FR-A01, IR115): admin.summary projected into the KPI tiles. Pure code shared by the
// Server Component that reads it and the client view. Texts in the display language (`t` / `i`, IR289); the as-of time
// in the user's display time zone (IR44), the period in Kuala Lumpur days (named so).
import { isNone } from "@ac/web/lib/isNone";
import { byCount, EN, showClock, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { statusWord } from "@ac/web/lib/partnerJobDetail";
import { coveragePct, warningShort } from "@ac/web/lib/energy";
import { money as formatMoney } from "@ac/web/lib/money";
import { lookup } from "@ac/web/lib/lookup";

const en = translator("en");
export interface ApiForecast {
  period: { from: string; to: string }; unitIds: string[]; baselineRef: { id: string; version: number } | null;
  baselineSnapshot: { id: string; version: number; method: string; baselineKWh: number | null; period: { from: string; to: string } } | null;
  expectedUnitMinutes: number; validUnitMinutes: number; actualKWhOnValidSlots: number | null; predictedBaselineKWh: number | null; predictedActualKWh: number | null;
  forecastSavedKWh: number | null; forecastSavingPercentage: number | null; qualityWarnings: string[];
}
export interface AdminSummary {
  customerCount: number; total: number; online: number; offline: number; unknown: number; powerOn: number; powerOff: number; powerUnknown: number;
  operatingRate: number | null; alertCount: number; jobCounts: Record<string, number>; overdueInvoiceCount: number | null;
  amountsByCurrency: { amountMinor: number; currency: string }[] | null; billingVisibility: "allowed" | "forbidden";
  energySummary: { totals: { kWh: number | null; amountMinor: number | null }; coverage: number | null } | null;
  energyForecast?: ApiForecast | null; asOf?: string;
}
export interface Kpis { customers: number; units: number; on: number; off: number; unknown: number; rate: string; alerts: number; kwh: string; kwhSub: string; billing: string; billingSub: string; jobs: number; online: number; offline: number; connUnknown: number }
export const mockKpis: Kpis = { customers: 2, units: 5, on: 2, off: 2, unknown: 1, rate: "50.0%", alerts: 1, kwh: "9.0 kWh", kwhSub: "measured · coverage 40% · cost 3.29 MYR", billing: "120.00 MYR", billingSub: "1 overdue invoice · unpaid, per currency", jobs: 2, online: 5, offline: 0, connUnknown: 0 };
const money = (m: { amountMinor: number; currency: string }) => formatMoney(m.amountMinor, m.currency);
export function kpisFrom(a: AdminSummary, t: T = en): Kpis {
  const e = a.energySummary;
  const overdue = a.overdueInvoiceCount ?? 0;
  return {
    customers: a.customerCount, units: a.total, on: a.powerOn, off: a.powerOff, unknown: a.powerUnknown,
    rate: a.operatingRate === null ? "—" : `${a.operatingRate.toFixed(1)}%`, alerts: a.alertCount,
    kwh: isNone(e?.totals.kWh) ? "—" : `${e.totals.kWh.toFixed(1)} kWh`,
    kwhSub: t("measured · coverage {coverage} · cost {cost}", { coverage: isNone(e?.coverage) ? "—" : `${coveragePct(e.coverage, 0)}%`, cost: isNone(e?.totals.amountMinor) ? "—" : formatMoney(e.totals.amountMinor, "MYR") }),
    billing: a.billingVisibility === "forbidden" ? t("Not permitted") : (a.amountsByCurrency ?? []).map((item) => money(item)).join(" · ") || "0.00 MYR",
    billingSub: a.billingVisibility === "forbidden" ? t("billing.read required") : byCount(overdue, { one: t("1 overdue invoice · unpaid, per currency"), many: t("{n} overdue invoices · unpaid, per currency", { n: overdue }) }),
    jobs: Object.values(a.jobCounts).reduce((x, y) => x + y, 0), online: a.online, offline: a.offline, connUnknown: a.unknown,
  };
}



// ---- the HQ overview sections from the same admin.summary (DD-A01 steps 5–7, Figma Admin 01, IR244) ----

const kwh = (v: number) => `${(Math.round(v * 10) / 10).toFixed(1)} kWh`;
const thousands = (v: number) => v.toLocaleString("en-US");

/** “As of 9:00 am MYT · 14 Sept, 00:00 – 14 Sept, 09:00 (1 day) · Asia/Kuala_Lumpur”: the summary's time in the display
time zone and the period's Kuala Lumpur days (periodRange labels them). */
export const asOfText = (asOf: string, period: string, i: I18n = EN) => i.t("As of {time} · {period}", { time: showClock(asOf, i.display), period });

/** The drill-down links (IR50, SR06): current-state lists carry the customer / property but not the period; the job
links carry the period too (the counts are the jobs whose requested time starts in it, IR245). */
export function overviewLinks(q: { customerId: string | null; propertyId: string | null; period?: { from: string; to: string } }) {
  const scope = (extra: Record<string, string> = {}) => {
    const p = new URLSearchParams({ ...(q.customerId && { customerId: q.customerId }), ...(q.propertyId && { propertyId: q.propertyId }), ...extra });
    return p.size > 0 ? `?${p}` : "";
  };
  return {
    units: `/admin/units${scope()}`, power: (state: "on" | "off" | "unknown") => `/admin/units${scope({ powerState: state })}`,
    connection: (c: string) => `/admin/units${scope({ connections: c })}`, alerts: "/admin/alerts", billing: "/admin/billing?overdueOnly=true",
    jobs: (stage?: string) => `/admin/jobs${scope({ ...(stage && { stage }), ...(q.period && { from: q.period.from, to: q.period.to }) })}`, energy: "/admin/energy",
    // the requests waiting for HQ, whatever their requested time (DEC-90, IR611)
    waiting: `/admin/jobs${scope({ stage: "requested" })}`,
  };
}

export type ForecastView =
  | { state: "ok"; saved: { text: string; tone: "ok" | "crit" | "muted" }; rate: { text: string; tone: "ok" | "crit" | "muted" }; baseline: { value: string; sub: string };
      actual: { value: string; sub: string }; coverage: string; warnings: string[] }
  | { state: "none"; text: string; warnings: string[] };
const direction = (v: number, unit: string, t: T) => {
  const size = unit === "%" ? `${Math.abs(v).toFixed(1)}%` : kwh(Math.abs(v));
  if (v > 0) return { text: t("Expected reduction {value}", { value: size }), tone: "ok" as const };
  if (v < 0) return { text: t("Expected increase {value}", { value: size }), tone: "crit" as const };
  return { text: t("No change 0.0"), tone: "muted" as const };
};
// the warnings that say why there is no forecast at all, in the order the title names them
const NONE_REASONS = ["no_units", "baseline_unavailable"] as const;
function noneTitle(why: string | null, t: T): string {
  if (why === "no_units") return t("No target equipment");
  if (why === "baseline_unavailable") return t("Baseline not set");
  return t("Cannot calculate");
}
/** The baseline under the forecast: its ID and version, method and total, or the word alone without one. */
function baselineSub(f: ApiForecast, t: T): string {
  const b = f.baselineSnapshot;
  if (!b) return t("baseline");
  const total = b.baselineKWh === null ? "" : ` · ${thousands(b.baselineKWh)} kWh`;
  return `${f.baselineRef?.id.slice(0, 8) ?? b.id.slice(0, 8)} v${String(f.baselineRef?.version ?? b.version)} · ${b.method}${total}`;
}
/** The forecast card without a forecast: why there is none, and the other warnings. */
function noForecast(w: string[], t: T): ForecastView {
  const why = NONE_REASONS.find((x) => w.includes(x)) ?? null;
  // the badges name the other warnings in words; the one the title already says is not repeated (IR577)
  return { state: "none", text: noneTitle(why, t), warnings: w.filter((x) => x !== why).map((x) => warningShort(x, t)) };
}
/** The energy-saving forecast card (IR78): a forecast from a prorated demo_fixed baseline, never a measured saving. */
export function forecastView(f: ApiForecast | null | undefined, t: T = en): ForecastView {
  if (!f) return noForecast([], t);
  if (f.forecastSavedKWh === null || f.predictedBaselineKWh === null || f.predictedActualKWh === null) return noForecast(f.qualityWarnings, t);
  return {
    state: "ok", saved: direction(f.forecastSavedKWh, "kWh", t), rate: f.forecastSavingPercentage === null ? { text: "—", tone: "muted" } : direction(f.forecastSavingPercentage, "%", t),
    baseline: { value: kwh(f.predictedBaselineKWh), sub: baselineSub(f, t) },
    actual: { value: kwh(f.predictedActualKWh), sub: t("{actual} on valid slots ÷ {valid} × {expected} unit-min", { actual: f.actualKWhOnValidSlots === null ? "—" : kwh(f.actualKWhOnValidSlots), valid: thousands(f.validUnitMinutes), expected: thousands(f.expectedUnitMinutes) }) },
    coverage: t("{valid} / {expected} unit-min", { valid: thousands(f.validUnitMinutes), expected: thousands(f.expectedUnitMinutes) }), warnings: f.qualityWarnings.map((x) => warningShort(x, t)),
  };
}

export interface AxisRow { label: string; count: number; pct: number; tone: "primary" | "muted" | "warn" | "ok" | "crit"; href: string; key: string }
/** The power-state and connection cards (SR27): one row per class with its share of the bar and its list link. */
const pct = (n: number, of: number) => (of ? (n / of) * 100 : 0);
export function axisRows(s: AdminSummary, links: ReturnType<typeof overviewLinks>, t: T = en) {
  const conn = s.online + s.offline + s.unknown;
  return {
    power: [
      { label: t("Running"), key: "powerState=on", count: s.powerOn, pct: pct(s.powerOn, s.total), tone: "primary", href: links.power("on") },
      { label: t("Stopped"), key: "powerState=off", count: s.powerOff, pct: pct(s.powerOff, s.total), tone: "muted", href: links.power("off") },
      { label: t("Unknown (stale / no valid power signal)"), key: "powerState=unknown", count: s.powerUnknown, pct: pct(s.powerUnknown, s.total), tone: "warn", href: links.power("unknown") },
    ] as AxisRow[],
    connection: [
      { label: t("Online"), key: "connections=online", count: s.online, pct: pct(s.online, conn), tone: "ok", href: links.connection("online") },
      { label: t("Offline"), key: "connections=offline", count: s.offline, pct: pct(s.offline, conn), tone: "crit", href: links.connection("offline") },
      { label: t("Unknown / connecting / error"), key: "connections=connecting,error,unknown", count: s.unknown, pct: pct(s.unknown, conn), tone: "warn", href: links.connection("connecting,error,unknown") },
    ] as AxisRow[],
    rateNote: t("Operation rate = Running ÷ (Running + Stopped) = {on} ÷ {known} = {rate}. total {total} = Running + Stopped + unknown. Unknown is never counted as Stopped. Archived units are excluded.", {
      on: s.powerOn, known: s.powerOn + s.powerOff, rate: s.operatingRate === null ? "—" : `${s.operatingRate.toFixed(1)}%`, total: s.total,
    }),
  };
}

/** All ten job statuses in pipeline order, zeros included (DD-A01); each links to the Jobs tab stage when it has one. */
export const JOB_STATUSES = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed", "rework_requested", "on_hold", "cancelled"] as const;
export function jobRows(counts: Record<string, number>, links: ReturnType<typeof overviewLinks>, t: T = en) {
  return JOB_STATUSES.map((st) => ({ status: st, label: statusWord(st, t), count: lookup(counts, st) ?? 0, href: links.jobs(st === "cancelled" ? undefined : st) }));
}

/** The billing card: overdue invoices and one row per currency, never summed across currencies; forbidden without billing.read. */
export function billingView(s: AdminSummary) {
  return s.billingVisibility === "forbidden" ? { forbidden: true as const } : { forbidden: false as const, overdue: s.overdueInvoiceCount ?? 0, rows: (s.amountsByCurrency ?? []).map((m) => ({ currency: m.currency, text: money(m) })) };
}
