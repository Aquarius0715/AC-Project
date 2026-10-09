"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Badge, Banner, Btn, Card, Input, Kpi, Page, Select, TextLink, cx } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { asOfText, axisRows, billingView, forecastView, jobRows, kpisFrom, overviewLinks, type AdminSummary, type AxisRow } from "@ac/web/lib/adminSummary";

type Live = {
  now: string; period: { kind: string; from: string; to: string; error: string | null; label: string }; custom: { from: string; to: string };
  customers: { id: string; name: string }[] | null; customerId: string | null; properties: { id: string; name: string }[] | null; propertyId: string | null;
  summary: AdminSummary | null;
};
const PERIODS = [{ id: "today", label: "Today" }, { id: "7d", label: "Last 7 days" }, { id: "30d", label: "Last 30 days" }, { id: "custom", label: "Custom" }];
const barTone: Record<AxisRow["tone"], string> = { primary: "bg-primary", muted: "bg-muted/50", warn: "bg-warn", ok: "bg-ok", crit: "bg-crit" };

/** The HQ dashboard (FR-A01, DD-A01, Figma Admin 01) from one admin.summary: scope and period in the URL. */
export function AdminOverviewView({ live }: { live: Live }) {
  const router = useRouter();
  const patch = useUrlPatch();
  const s = live.summary;
  const links = overviewLinks({ customerId: live.customerId, propertyId: live.propertyId });
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          {live.customers && (
            <Select aria-label="Customer" className="w-auto text-xs font-semibold" value={live.customerId ?? ""} onChange={(e) => patch({ customerId: e.target.value || null, propertyId: null })}>
              <option value="">Customer: All</option>{live.customers.map((c) => <option key={c.id} value={c.id}>Customer: {c.name}</option>)}
            </Select>
          )}
          {live.customers && (
            <Select aria-label="Property" className="w-auto text-xs font-semibold" disabled={!live.properties} value={live.propertyId ?? ""} onChange={(e) => patch({ propertyId: e.target.value || null })}>
              <option value="">Property: All</option>{(live.properties ?? []).map((p) => <option key={p.id} value={p.id}>Property: {p.name}</option>)}
            </Select>
          )}
          <Select aria-label="Period" className="w-auto text-xs font-semibold" value={live.period.kind} onChange={(e) => patch({ period: e.target.value === "today" ? null : e.target.value, ...(e.target.value === "custom" ? {} : { from: null, to: null }) })}>
            {PERIODS.map((p) => <option key={p.id} value={p.id}>Period: {p.label}</option>)}
          </Select>
          {live.period.kind === "custom" && (
            <span className="flex items-center gap-1 text-xs">
              <Input aria-label="From" type="datetime-local" className="w-auto text-xs" defaultValue={live.custom.from} onBlur={(e) => patch({ from: e.target.value || null })} />–
              <Input aria-label="To" type="datetime-local" className="w-auto text-xs" defaultValue={live.custom.to} onBlur={(e) => patch({ to: e.target.value || null })} />
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-muted">{s && <span>{asOfText(s.asOf ?? live.now, live.period)}</span>}<Btn size="sm" onClick={() => router.refresh()}>↻ Refresh</Btn></div>
      </div>
      {live.period.error && <Banner tone="warn">{live.period.error} — choose another period.</Banner>}
      {s && <Sections s={s} links={links} />}
    </Page>
  );
}

function Sections({ s, links }: { s: AdminSummary; links: ReturnType<typeof overviewLinks> }) {
  const k = kpisFrom(s);
  const f = forecastView(s.energyForecast);
  const axes = axisRows(s, links);
  const bill = billingView(s);
  const half = Math.ceil(10 / 2);
  const jobs = jobRows(s.jobCounts, links);
  return (
    <>
      {s.total === 0 && <Banner tone="warn">Zero units in scope — nothing to show yet.</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Kpi label="Active customers" value={k.customers} sub="Customer active (IR40)" />
        <Kpi label="Target units" value={k.units} sub={`Running ${k.on} · Stopped ${k.off} · unknown ${k.unknown} · archived excluded`} href={links.units} link="Open units →" />
        <Kpi label="Operation rate" value={k.rate} sub={`Running ${k.on} / known ${k.on + k.off} · unknown ${k.unknown} not in denominator`} href={links.power("on")} link="Units running →" />
        <Kpi label="Unresolved alerts" value={k.alerts} tone={k.alerts ? "warn" : undefined} sub="open/acknowledged · critical and warning (IR51)" href={links.alerts} link="View alerts →" />
        <Kpi label="Energy used (actual)" value={k.kwh} sub={k.kwhSub} href={links.energy} link="Energy analysis →" />
        <Kpi label="Overdue billing" value={k.billing} tone={bill.forbidden ? undefined : bill.overdue ? "crit" : undefined} sub={k.billingSub} href={bill.forbidden ? undefined : links.billing} link={bill.forbidden ? undefined : "Open overdue →"} />
        <Kpi label="Maintenance jobs (period)" value={k.jobs} sub={jobs.filter((j) => j.count).map((j) => `${j.status} ${j.count}`).join(" · ") || "no jobs in the period"} href={links.jobs()} link="Open jobs →" />
        <Kpi label="Connection" value={`${k.online} online`} sub={`offline ${k.offline} · unknown/connecting/error ${k.connUnknown}`} href={links.connection("online")} link="Open units →" />
      </div>
      <Card title={<span className="flex flex-wrap items-center gap-2">Energy-saving forecast <Badge tone="warn">Forecast (prorated assumed baseline, demo)</Badge></span>} action={<TextLink href={links.energy}>Energy analysis →</TextLink>}>
        {f.state === "ok" ? (
          <>
            <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
              <Tile label="Forecast savings" value={f.saved.text} tone={f.saved.tone} sub="forecastSavedKWh · this period" />
              <Tile label="Forecast saving rate" value={f.rate.text} tone={f.rate.tone} sub="forecastSavingPercentage = saved ÷ predicted baseline" />
              <Tile label="Predicted baseline" value={f.baseline.value} sub={f.baseline.sub} />
              <Tile label="Predicted actual" value={f.actual.value} sub={f.actual.sub} />
            </div>
            <p className="mt-2 flex flex-wrap items-center gap-2 text-xs">Coverage {f.coverage} (validUnitMinutes / expectedUnitMinutes){f.warnings.map((w) => <Badge key={w} tone="primary">{w}</Badge>)}</p>
          </>
        ) : <p className="flex flex-wrap items-center gap-2 text-[13px] font-semibold">{f.text}{f.warnings.map((w) => <Badge key={w} tone="muted">{w}</Badge>)}</p>}
        <p className="mt-2 text-[11px] text-muted">A forecast, not a measured saving. Actual results for the period are in the “Energy used” KPI; baseline comparison lives in Energy analysis (A13).</p>
      </Card>
      <div className="grid-fluid" style={{ ["--min" as string]: "420px" }}>
        <Card title="Operation (power state)" action={<TextLink href={links.units}>Open units →</TextLink>}><Axis rows={axes.power} /><p className="mt-2 text-[11px] text-muted">{axes.rateNote}</p></Card>
        <Card title="Connection" action={<TextLink href={links.units}>Open units →</TextLink>}><Axis rows={axes.connection} /><p className="mt-2 text-[11px] text-muted">Connection is a separate axis from power state: an online unit can still have unknown power (no power sensor).</p></Card>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "420px" }}>
        <Card title="Maintenance jobs by status — this period" action={<TextLink href={links.jobs()}>Open jobs →</TextLink>}>
          <div className="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
            {[jobs.slice(0, half), jobs.slice(half)].map((col, i) => (
              <div key={i} className="rounded-xl border border-line">{col.map((j) => <Link key={j.status} href={j.href} className="flex justify-between border-t border-line px-3 py-1.5 text-[13px] first:border-0 hover:bg-surface2"><span className="text-muted">{j.status}</span><b>{j.count}</b></Link>)}</div>
            ))}
          </div>
          <p className="mt-2 text-[11px] text-muted">Jobs whose requested time starts in the period. The status rows open the Jobs tab for that stage and scope — it lists every job of the stage, not only this period’s (IR244).</p>
        </Card>
        <Card title="Billing — unpaid by currency" action={!bill.forbidden && <TextLink href={links.billing}>Open overdue →</TextLink>}>
          {bill.forbidden ? <p className="text-[13px] text-muted">Hidden — billing.read is required.</p> : (
            <div className="rounded-xl border border-line">
              <div className="flex justify-between px-3 py-1.5 text-[13px]"><span className="text-muted">Overdue invoices</span><b>{bill.overdue}</b></div>
              {bill.rows.map((r) => <div key={r.currency} className="flex justify-between border-t border-line px-3 py-1.5 text-[13px]"><span className="text-muted">{r.currency} (unpaid)</span><b className="text-crit">{r.text}</b></div>)}
              {bill.rows.length === 0 && <div className="border-t border-line px-3 py-1.5 text-[13px] text-muted">Nothing unpaid past due.</div>}
            </div>
          )}
          <p className="mt-2 text-[11px] text-muted">amountsByCurrency is listed per currency and never summed across currencies. The link opens /admin/billing?overdueOnly=true — no period (IR50, SR06).</p>
        </Card>
      </div>
      <p className="text-[11px] text-muted">KPI → list links pass the same customer / property but not the period for current-state KPIs (units, alerts, unpaid); Back restores this URL including the period (IR50, D13).</p>
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
