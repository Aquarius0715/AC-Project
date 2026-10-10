"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Input, Kpi, Page, Select, TextLink, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { axisRows, billingView, forecastView, jobRows, kpisFrom, overviewLinks, type AdminSummary, type AxisRow } from "@ac/web/lib/adminSummary";

type Live = {
  period: { kind: string; from: string; to: string; error: string | null; label: string }; custom: { from: string; to: string };
  customers: { id: string; name: string }[] | null; customerId: string | null; properties: { id: string; name: string }[] | null; propertyId: string | null;
  summary: AdminSummary | null; asOf: string | null;
};
const PERIODS = [{ id: "today", label: "Today" }, { id: "7d", label: "Last 7 days" }, { id: "30d", label: "Last 30 days" }, { id: "custom", label: "Custom" }];
const barTone: Record<AxisRow["tone"], string> = { primary: "bg-primary", muted: "bg-muted/50", warn: "bg-warn", ok: "bg-ok", crit: "bg-crit" };

/** The HQ dashboard (FR-A01, DD-A01, Figma Admin 01) from one admin.summary: scope and period in the URL. Texts in the
 * user's display language; the as-of time and the period come formatted from the page (IR289). */
export function AdminOverviewView({ live }: { live: Live }) {
  const t = useT();
  const router = useRouter();
  const patch = useUrlPatch();
  const s = live.summary;
  const links = overviewLinks({ customerId: live.customerId, propertyId: live.propertyId, period: live.period.error ? undefined : { from: live.period.from, to: live.period.to } });
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          {live.customers && (
            <Select aria-label={t("Customer")} className="w-auto text-xs font-semibold" value={live.customerId ?? ""} onChange={(e) => patch({ customerId: e.target.value || null, propertyId: null })}>
              <option value="">{t("Customer: All")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{t("Customer: {name}", { name: c.name })}</option>)}
            </Select>
          )}
          {live.customers && (
            <Select aria-label={t("Property")} className="w-auto text-xs font-semibold" disabled={!live.properties} value={live.propertyId ?? ""} onChange={(e) => patch({ propertyId: e.target.value || null })}>
              <option value="">{t("Property: All")}</option>{(live.properties ?? []).map((p) => <option key={p.id} value={p.id}>{t("Property: {name}", { name: p.name })}</option>)}
            </Select>
          )}
          <Select aria-label={t("Period")} className="w-auto text-xs font-semibold" value={live.period.kind} onChange={(e) => patch({ period: e.target.value === "today" ? null : e.target.value, ...(e.target.value === "custom" ? {} : { from: null, to: null }) })}>
            {PERIODS.map((p) => <option key={p.id} value={p.id}>{t("Period: {period}", { period: t(p.label) })}</option>)}
          </Select>
          {live.period.kind === "custom" && (
            <span className="flex items-center gap-1 text-xs">
              <Input aria-label={t("From")} type="datetime-local" className="w-auto text-xs" defaultValue={live.custom.from} onBlur={(e) => patch({ from: e.target.value || null })} />–
              <Input aria-label={t("To")} type="datetime-local" className="w-auto text-xs" defaultValue={live.custom.to} onBlur={(e) => patch({ to: e.target.value || null })} />
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-muted">{live.asOf && <span>{live.asOf}</span>}<Btn size="sm" onClick={() => router.refresh()}>{t("↻ Refresh")}</Btn></div>
      </div>
      {live.period.error && <Banner tone="warn">{t("{error} — choose another period.", { error: live.period.error })}</Banner>}
      {s && <Sections s={s} links={links} />}
    </Page>
  );
}

function Sections({ s, links }: { s: AdminSummary; links: ReturnType<typeof overviewLinks> }) {
  const t = useT();
  const k = kpisFrom(s, t);
  const f = forecastView(s.energyForecast, t);
  const axes = axisRows(s, links, t);
  const bill = billingView(s);
  const half = Math.ceil(10 / 2);
  const jobs = jobRows(s.jobCounts, links, t);
  return (
    <>
      {s.total === 0 && <Banner tone="warn">{t("Zero units in scope — nothing to show yet.")}</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Kpi label={t("Active customers")} value={k.customers} sub={t("Customer active (IR40)")} />
        <Kpi label={t("Target units")} value={k.units} sub={t("Running {on} · Stopped {off} · unknown {unknown} · archived excluded", { on: k.on, off: k.off, unknown: k.unknown })} href={links.units} link={t("Open units →")} />
        <Kpi label={t("Operation rate")} value={k.rate} sub={t("Running {on} / known {known} · unknown {unknown} not in denominator", { on: k.on, known: k.on + k.off, unknown: k.unknown })} href={links.power("on")} link={t("Units running →")} />
        <Kpi label={t("Unresolved alerts")} value={k.alerts} tone={k.alerts ? "warn" : undefined} sub={t("open/acknowledged · critical and warning (IR51)")} href={links.alerts} link={t("View alerts →")} />
        <Kpi label={t("Energy used (actual)")} value={k.kwh} sub={k.kwhSub} href={links.energy} link={t("Energy analysis →")} />
        <Kpi label={t("Overdue billing")} value={k.billing} tone={bill.forbidden ? undefined : bill.overdue ? "crit" : undefined} sub={k.billingSub} href={bill.forbidden ? undefined : links.billing} link={bill.forbidden ? undefined : t("Open overdue →")} />
        <Kpi label={t("Maintenance jobs (period)")} value={k.jobs} sub={jobs.filter((j) => j.count).map((j) => `${j.label} ${j.count}`).join(" · ") || t("no jobs in the period")} href={links.jobs()} link={t("Open jobs →")} />
        <Kpi label={t("Connection")} value={t("{n} online", { n: k.online })} sub={t("offline {offline} · unknown/connecting/error {unknown}", { offline: k.offline, unknown: k.connUnknown })} href={links.connection("online")} link={t("Open units →")} />
      </div>
      <Card title={<span className="flex flex-wrap items-center gap-2">{t("Energy-saving forecast")} <Badge tone="warn">{t("Forecast (prorated assumed baseline, demo)")}</Badge></span>} action={<TextLink href={links.energy}>{t("Energy analysis →")}</TextLink>}>
        {f.state === "ok" ? (
          <>
            <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
              <Tile label={t("Forecast savings")} value={f.saved.text} tone={f.saved.tone} sub={t("forecastSavedKWh · this period")} />
              <Tile label={t("Forecast saving rate")} value={f.rate.text} tone={f.rate.tone} sub={t("forecastSavingPercentage = saved ÷ predicted baseline")} />
              <Tile label={t("Predicted baseline")} value={f.baseline.value} sub={f.baseline.sub} />
              <Tile label={t("Predicted actual")} value={f.actual.value} sub={f.actual.sub} />
            </div>
            <p className="mt-2 flex flex-wrap items-center gap-2 text-xs">{t("Coverage {coverage} (validUnitMinutes / expectedUnitMinutes)", { coverage: f.coverage })}{f.warnings.map((w) => <Badge key={w} tone="primary">{w}</Badge>)}</p>
          </>
        ) : <p className="flex flex-wrap items-center gap-2 text-[13px] font-semibold">{f.text}{f.warnings.map((w) => <Badge key={w} tone="muted">{w}</Badge>)}</p>}
        <p className="mt-2 text-[11px] text-muted">{t("A forecast, not a measured saving. Actual results for the period are in the “Energy used” KPI; baseline comparison lives in Energy analysis (A13).")}</p>
      </Card>
      <div className="grid-fluid" style={{ ["--min" as string]: "420px" }}>
        <Card title={t("Operation (power state)")} action={<TextLink href={links.units}>{t("Open units →")}</TextLink>}><Axis rows={axes.power} /><p className="mt-2 text-[11px] text-muted">{axes.rateNote}</p></Card>
        <Card title={t("Connection")} action={<TextLink href={links.units}>{t("Open units →")}</TextLink>}><Axis rows={axes.connection} /><p className="mt-2 text-[11px] text-muted">{t("Connection is a separate axis from power state: an online unit can still have unknown power (no power sensor).")}</p></Card>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "420px" }}>
        <Card title={t("Maintenance jobs by status — this period")} action={<TextLink href={links.jobs()}>{t("Open jobs →")}</TextLink>}>
          <div className="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
            {[jobs.slice(0, half), jobs.slice(half)].map((col, i) => (
              <div key={i} className="rounded-xl border border-line">{col.map((j) => <Link key={j.status} href={j.href} className="flex justify-between border-t border-line px-3 py-1.5 text-[13px] first:border-0 hover:bg-surface2"><span className="text-muted">{j.label}</span><b>{j.count}</b></Link>)}</div>
            ))}
          </div>
          <p className="mt-2 text-[11px] text-muted">{t("Jobs whose requested time starts in the period. The status rows open the Jobs tab with that stage, the scope and the period (IR245).")}</p>
        </Card>
        <Card title={t("Billing — unpaid by currency")} action={!bill.forbidden && <TextLink href={links.billing}>{t("Open overdue →")}</TextLink>}>
          {bill.forbidden ? <p className="text-[13px] text-muted">{t("Hidden — billing.read is required.")}</p> : (
            <div className="rounded-xl border border-line">
              <div className="flex justify-between px-3 py-1.5 text-[13px]"><span className="text-muted">{t("Overdue invoices")}</span><b>{bill.overdue}</b></div>
              {bill.rows.map((r) => <div key={r.currency} className="flex justify-between border-t border-line px-3 py-1.5 text-[13px]"><span className="text-muted">{t("{currency} (unpaid)", { currency: r.currency })}</span><b className="text-crit">{r.text}</b></div>)}
              {bill.rows.length === 0 && <div className="border-t border-line px-3 py-1.5 text-[13px] text-muted">{t("Nothing unpaid past due.")}</div>}
            </div>
          )}
          <p className="mt-2 text-[11px] text-muted">{t("amountsByCurrency is listed per currency and never summed across currencies. The link opens /admin/billing?overdueOnly=true — no period (IR50, SR06).")}</p>
        </Card>
      </div>
      <p className="text-[11px] text-muted">{t("KPI → list links pass the same customer / property but not the period for current-state KPIs (units, alerts, unpaid); Back restores this URL including the period (IR50, D13).")}</p>
    </>
  );
}

function Tile({ label, value, sub, tone }: { label: string; value: string; sub: string; tone?: "ok" | "crit" | "muted" }) {
  return <div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{label}</div><b className={cx(tone === "ok" && "text-ok", tone === "crit" && "text-crit")}>{value}</b><div className="text-[10px] text-muted">{sub}</div></div>;
}

function Axis({ rows }: { rows: AxisRow[] }) {
  return (
    <>
      <div className="mb-2 flex h-2.5 w-full overflow-hidden rounded-full bg-surface2" role="img" aria-label={rows.map((r) => `${r.label} ${r.count}`).join(", ")}>
        {rows.map((r) => (r.pct > 0 ? <div key={r.key} className={barTone[r.tone]} style={{ width: `${r.pct}%` }} /> : null))}
      </div>
      {rows.map((r) => (
        <div key={r.key} className="flex items-center justify-between gap-2 py-1 text-[13px]">
          <span className="flex items-center gap-2"><i className={cx("inline-block h-2 w-2 rounded-full", barTone[r.tone])} />{r.label}</span>
          <span className="flex items-center gap-3"><Link href={r.href} className="text-[11px] text-primary hover:underline">{r.key} →</Link><b>{r.count}</b></span>
        </div>
      ))}
    </>
  );
}
