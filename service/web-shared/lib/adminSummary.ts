// Admin dashboard KPIs (FR-A01, IR115): admin.summary projected into the KPI tiles. Pure code shared by the
// Server Component that reads it and the client view.
export type AdminSummary = {
  customerCount: number; total: number; online: number; offline: number; unknown: number; powerOn: number; powerOff: number; powerUnknown: number;
  operatingRate: number | null; alertCount: number; jobCounts: Record<string, number>; overdueInvoiceCount: number | null;
  amountsByCurrency: { amountMinor: number; currency: string }[] | null; billingVisibility: "allowed" | "forbidden";
  energySummary: { totals: { kWh: number | null; amountMinor: number | null }; coverage: number | null } | null;
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
