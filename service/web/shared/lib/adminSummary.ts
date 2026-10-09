// Admin dashboard KPIs (FR-A01, IR115): admin.summary projected into the KPI tiles. Pure code shared by the
// Server Component that reads it and the client view.
export type ApiForecast = {
  period: { from: string; to: string }; unitIds: string[]; baselineRef: { id: string; version: number } | null;
  baselineSnapshot: { id: string; version: number; method: string; baselineKWh: number | null; period: { from: string; to: string } } | null;
  expectedUnitMinutes: number; validUnitMinutes: number; actualKWhOnValidSlots: number | null; predictedBaselineKWh: number | null; predictedActualKWh: number | null;
  forecastSavedKWh: number | null; forecastSavingPercentage: number | null; qualityWarnings: string[];
};
export type AdminSummary = {
  customerCount: number; total: number; online: number; offline: number; unknown: number; powerOn: number; powerOff: number; powerUnknown: number;
  operatingRate: number | null; alertCount: number; jobCounts: Record<string, number>; overdueInvoiceCount: number | null;
  amountsByCurrency: { amountMinor: number; currency: string }[] | null; billingVisibility: "allowed" | "forbidden";
  energySummary: { totals: { kWh: number | null; amountMinor: number | null }; coverage: number | null } | null;
  energyForecast?: ApiForecast | null; asOf?: string;
};
export type Kpis = { customers: number; units: number; on: number; off: number; unknown: number; rate: string; alerts: number; kwh: string; kwhSub: string; billing: string; billingSub: string; jobs: number; online: number; offline: number; connUnknown: number };
export const mockKpis: Kpis = { customers: 2, units: 5, on: 2, off: 2, unknown: 1, rate: "50.0%", alerts: 1, kwh: "9.0 kWh", kwhSub: "measured · coverage 40% · cost 3.29 MYR", billing: "120.00 MYR", billingSub: "1 overdue invoice · unpaid, per currency", jobs: 2, online: 5, offline: 0, connUnknown: 0 };
const money = (m: { amountMinor: number; currency: string }) => `${(m.amountMinor / 100).toFixed(2)} ${m.currency}`;
export function kpisFrom(a: AdminSummary): Kpis {
  const e = a.energySummary;
  return {
    customers: a.customerCount, units: a.total, on: a.powerOn, off: a.powerOff, unknown: a.powerUnknown,
    rate: a.operatingRate === null ? "—" : `${a.operatingRate.toFixed(1)}%`, alerts: a.alertCount,
    kwh: e?.totals.kWh == null ? "—" : `${e.totals.kWh.toFixed(1)} kWh`,
    kwhSub: `measured · coverage ${e?.coverage == null ? "—" : `${Math.round(e.coverage * 100)}%`} · cost ${e?.totals.amountMinor == null ? "—" : `${(e.totals.amountMinor / 100).toFixed(2)} MYR`}`,
    billing: a.billingVisibility === "forbidden" ? "Not permitted" : (a.amountsByCurrency ?? []).map(money).join(" · ") || "0.00 MYR",
    billingSub: a.billingVisibility === "forbidden" ? "billing.read required" : `${a.overdueInvoiceCount ?? 0} overdue invoice(s) · unpaid, per currency`,
    jobs: Object.values(a.jobCounts).reduce((x, y) => x + y, 0), online: a.online, offline: a.offline, connUnknown: a.unknown,
  };
}


/** Today in Asia/Kuala_Lumpur on UTC minute boundaries, ending at the business clock (admin.summary range). */
export function todayRange(clock: Date): { from: string; to: string } {
  const now = new Date(Math.floor(clock.getTime() / 60000) * 60000);
  const kl = new Date(now.getTime() + 8 * 3600000);
  const from = new Date(Date.UTC(kl.getUTCFullYear(), kl.getUTCMonth(), kl.getUTCDate()) - 8 * 3600000);
  return { from: from.toISOString(), to: (now > from ? now : new Date(from.getTime() + 60000)).toISOString() };
}

// ---- the HQ overview sections from the same admin.summary (DD-A01 steps 5–7, Figma Admin 01, IR244) ----

const KL = "Asia/Kuala_Lumpur";
const stamp = (iso: string) => new Date(iso).toLocaleString("en-CA", { timeZone: KL, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", "");
const kwh = (v: number) => `${(Math.round(v * 10) / 10).toFixed(1)} kWh`;
const thousands = (v: number) => v.toLocaleString("en-US");

/** “As of 09:00 MYT · 2026-09-14 00:00–09:00 (Asia/Kuala_Lumpur)”; a range over several days names both dates. */
export function asOfText(asOf: string, range: { from: string; to: string }): string {
  const f = stamp(range.from), t = stamp(range.to);
  const span = f.slice(0, 10) === t.slice(0, 10) ? `${f.slice(0, 10)} ${f.slice(11)}–${t.slice(11)}` : `${f} – ${t}`;
  return `As of ${stamp(asOf).slice(11)} MYT · ${span} (${KL})`;
}

/** The drill-down links (IR50, SR06): current-state lists carry the customer / property but not the period; the job
 * links carry the period too (the counts are the jobs whose requested time starts in it, IR245). */
export function overviewLinks(q: { customerId: string | null; propertyId: string | null; period?: { from: string; to: string } }) {
  const scope = (extra: Record<string, string> = {}) => {
    const p = new URLSearchParams({ ...(q.customerId ? { customerId: q.customerId } : {}), ...(q.propertyId ? { propertyId: q.propertyId } : {}), ...extra });
    return p.size ? `?${p}` : "";
  };
  return {
    units: `/admin/units${scope()}`, power: (state: "on" | "off" | "unknown") => `/admin/units${scope({ powerState: state })}`,
    connection: (c: string) => `/admin/units${scope({ connections: c })}`, alerts: "/admin/alerts", billing: "/admin/billing?overdueOnly=true",
    jobs: (stage?: string) => `/admin/jobs${scope({ ...(stage ? { stage } : {}), ...(q.period ? { from: q.period.from, to: q.period.to } : {}) })}`, energy: "/admin/energy",
  };
}

export type ForecastView =
  | { state: "ok"; saved: { text: string; tone: "ok" | "crit" | "muted" }; rate: { text: string; tone: "ok" | "crit" | "muted" }; baseline: { value: string; sub: string };
      actual: { value: string; sub: string }; coverage: string; warnings: string[] }
  | { state: "none"; text: string; warnings: string[] };
const direction = (v: number, unit: string) => (v > 0 ? { text: `Expected reduction ${unit === "%" ? `${v.toFixed(1)}%` : kwh(v)}`, tone: "ok" as const }
  : v < 0 ? { text: `Expected increase ${unit === "%" ? `${Math.abs(v).toFixed(1)}%` : kwh(Math.abs(v))}`, tone: "crit" as const } : { text: "No change 0.0", tone: "muted" as const });
/** The energy-saving forecast card (IR78): a forecast from a prorated demo_fixed baseline, never a measured saving. */
export function forecastView(f: ApiForecast | null | undefined): ForecastView {
  if (!f || f.forecastSavedKWh === null || f.predictedBaselineKWh === null || f.predictedActualKWh === null) {
    const w = f?.qualityWarnings ?? [];
    return { state: "none", text: w.includes("no_units") ? "No target equipment" : w.includes("baseline_unavailable") ? "Baseline not set" : "Cannot calculate", warnings: w };
  }
  const b = f.baselineSnapshot;
  return {
    state: "ok", saved: direction(f.forecastSavedKWh, "kWh"), rate: f.forecastSavingPercentage === null ? { text: "—", tone: "muted" } : direction(f.forecastSavingPercentage, "%"),
    baseline: { value: kwh(f.predictedBaselineKWh), sub: b ? `${f.baselineRef?.id.slice(0, 8) ?? b.id.slice(0, 8)} v${f.baselineRef?.version ?? b.version} · ${b.method}${b.baselineKWh === null ? "" : ` · ${thousands(b.baselineKWh)} kWh`}` : "baseline" },
    actual: { value: kwh(f.predictedActualKWh), sub: `${f.actualKWhOnValidSlots === null ? "—" : kwh(f.actualKWhOnValidSlots)} on valid slots ÷ ${thousands(f.validUnitMinutes)} × ${thousands(f.expectedUnitMinutes)} unit-min` },
    coverage: `${thousands(f.validUnitMinutes)} / ${thousands(f.expectedUnitMinutes)} unit-min`, warnings: f.qualityWarnings,
  };
}

export type AxisRow = { label: string; count: number; pct: number; tone: "primary" | "muted" | "warn" | "ok" | "crit"; href: string; key: string };
/** The power-state and connection cards (SR27): one row per class with its share of the bar and its list link. */
export function axisRows(s: AdminSummary, links: ReturnType<typeof overviewLinks>) {
  const pct = (n: number, of: number) => (of ? (n / of) * 100 : 0);
  const conn = s.online + s.offline + s.unknown;
  return {
    power: [
      { label: "Running", key: "powerState=on", count: s.powerOn, pct: pct(s.powerOn, s.total), tone: "primary", href: links.power("on") },
      { label: "Stopped", key: "powerState=off", count: s.powerOff, pct: pct(s.powerOff, s.total), tone: "muted", href: links.power("off") },
      { label: "Unknown (stale / no valid power signal)", key: "powerState=unknown", count: s.powerUnknown, pct: pct(s.powerUnknown, s.total), tone: "warn", href: links.power("unknown") },
    ] as AxisRow[],
    connection: [
      { label: "Online", key: "connections=online", count: s.online, pct: pct(s.online, conn), tone: "ok", href: links.connection("online") },
      { label: "Offline", key: "connections=offline", count: s.offline, pct: pct(s.offline, conn), tone: "crit", href: links.connection("offline") },
      { label: "Unknown / connecting / error", key: "connections=connecting,error,unknown", count: s.unknown, pct: pct(s.unknown, conn), tone: "warn", href: links.connection("connecting,error,unknown") },
    ] as AxisRow[],
    rateNote: `Operation rate = Running ÷ (Running + Stopped) = ${s.powerOn} ÷ ${s.powerOn + s.powerOff} = ${s.operatingRate === null ? "—" : `${s.operatingRate.toFixed(1)}%`}. total ${s.total} = Running + Stopped + unknown. Unknown is never counted as Stopped. Archived units are excluded (IR39).`,
  };
}

/** All ten job statuses in pipeline order, zeros included (DD-A01); each links to the Jobs tab stage when it has one. */
export const JOB_STATUSES = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed", "rework_requested", "on_hold", "cancelled"] as const;
export function jobRows(counts: Record<string, number>, links: ReturnType<typeof overviewLinks>) {
  return JOB_STATUSES.map((st) => ({ status: st, count: counts[st] ?? 0, href: links.jobs(st === "cancelled" ? undefined : st) }));
}

/** The billing card: overdue invoices and one row per currency, never summed across currencies; forbidden without billing.read. */
export function billingView(s: AdminSummary) {
  if (s.billingVisibility === "forbidden") return { forbidden: true as const };
  return { forbidden: false as const, overdue: s.overdueInvoiceCount ?? 0, rows: (s.amountsByCurrency ?? []).map((m) => ({ currency: m.currency, text: money(m) })) };
}
