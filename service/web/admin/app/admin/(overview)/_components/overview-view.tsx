"use client";

import { useState } from "react";
import { Banner, Btn, Card, ErrorState, Kpi, Page, TextLink } from "@ac/web/components/ui";
import { mockKpis, type Kpis } from "@ac/web/lib/adminSummary";

/** AdminSummary of service-contracts.ts (fields shown on the KPI row). */
const jobs = [["requested", 1], ["offered", 0], ["accepted", 0], ["assigned", 1], ["in_progress", 0], ["submitted", 0], ["completed", 0], ["rework_requested", 0], ["on_hold", 0], ["cancelled", 0]] as const;
/** Admin dashboard. In API mode the Server Component passes the KPIs; the demo uses the fixture KPIs. */
export function AdminOverviewView({ kpis = mockKpis }: { kpis?: Kpis }) {
  const [state, setState] = useState<"ok" | "loading" | "error" | "empty">("ok");
  const k = kpis;
  if (state === "error") return <Page><ErrorState title="Couldn’t load the dashboard" onRetry={() => setState("ok")}>Initial load failed — nothing is shown as zero.</ErrorState></Page>;
  if (state === "loading") return <Page><div className="grid-fluid" style={{ ["--min"as string]: "200px" }}>{Array.from({ length: 8 }).map((_, i) => <div key={i} className="h-28 animate-pulse rounded-2xl bg-surface2" />)}</div><Btn onClick={() => setState("ok")}>Finish loading</Btn></Page>;
  const empty = state === "empty";
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted"><span>As of 09:00 MYT · 2026-09-14 00:00–09:00 (Asia/Kuala_Lumpur)</span><span className="flex gap-3"><button className="underline" onClick={() => setState("loading")}>skeleton</button><button className="underline" onClick={() => setState("error")}>error</button><button className="underline" onClick={() => setState(empty ? "ok" : "empty")}>{empty ? "normal" : "zero units"}</button></span></div>
      {empty && <Banner tone="warn">Zero units in scope — nothing to show yet.</Banner>}
      <div className="grid-fluid" style={{ ["--min"as string]: "200px" }}>
        <Kpi label="Active customers" value={empty ? 0 : k.customers} sub="Customer active (IR40)" href="/admin/units" link="Open →" />
        <Kpi label="Target units" value={empty ? 0 : k.units} sub={`Running ${k.on} · Stopped ${k.off} · unknown ${k.unknown} · archived excluded`} href="/admin/units" link="Open →" />
        <Kpi label="Operation rate" value={empty ? "—" : k.rate} sub={`Running ${k.on} / known ${k.on + k.off} · unknown ${k.unknown} not in denominator`} href="/admin/units?powerState=on" link="Open →" />
        <Kpi label="Unresolved alerts" value={empty ? 0 : k.alerts} tone="warn" sub="open/acknowledged · critical and warning (IR51)" href="/admin/alerts" link="Open →" />
        <Kpi label="Energy used (actual)" value={empty ? "—" : k.kwh} sub={k.kwhSub} href="/admin/energy" link="Open →" />
        <Kpi label="Overdue billing" value={k.billing} tone="crit" sub={k.billingSub} href="/admin/billing" link="Open →" />
        <Kpi label="Maintenance jobs (period)" value={k.jobs} sub="all statuses in the period" href="/admin/jobs" link="Open →" />
        <Kpi label="Connection" value={`${k.online} online`} sub={`offline ${k.offline} · unknown/connecting/error ${k.connUnknown}`} href="/admin/units?connections=online" link="Open →" />
      </div>
      <Card title="Energy-saving forecast" sub="Forecast (prorated assumed baseline, demo)" action={<TextLink href="/admin/energy">Energy analysis →</TextLink>}>
        <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}>
          {[["Forecast savings", "Expected reduction 9.9 kWh", "forecastSavedKWh · this period"], ["Forecast saving rate", "Expected reduction 30.6%", "saved ÷ predicted baseline"], ["Predicted baseline", "32.4 kWh", "baseline-demo-tenant-a v1 · demo_fixed"], ["Predicted actual", "22.5 kWh", "9.0 kWh on valid slots ÷ 1,080 × 2,700 unit-min"]].map(([a, b, c]) => <div key={a} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{a}</div><b>{b}</b><div className="text-[10px] text-muted">{c}</div></div>)}
        </div>
        <p className="mt-2 text-[11px] text-muted">Coverage 1,080 / 2,700 unit-min. A forecast, not a measured saving.</p>
      </Card>
      <div className="grid-fluid" style={{ ["--min"as string]: "300px" }}>
        <Card title="Operation (power state)" action={<TextLink href="/admin/units">Open units →</TextLink>}>
          {[["Running", "powerState=on", 2], ["Stopped", "powerState=off", 2], ["Unknown (stale / no valid power signal)", "powerState=unknown", 1]].map(([a, b, n]) => <div key={a as string} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span className="text-[13px]">{a}<span className="block text-[10px] text-muted">{b} →</span></span><b className="text-lg">{empty ? 0 : n}</b></div>)}
          <p className="text-[11px] text-muted">Operation rate = ON ÷ (ON + OFF) = 2 ÷ 4</p>
        </Card>
        <Card title="Connection" action={<TextLink href="/admin/units">Open units →</TextLink>}>
          {[["Online", "connections=online", 5], ["Offline", "connections=offline", 0], ["Unknown / connecting / error", "connections=connecting,error,unknown", 0]].map(([a, b, n]) => <div key={a as string} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span className="text-[13px]">{a}<span className="block text-[10px] text-muted">{b} →</span></span><b className="text-lg">{empty ? 0 : n}</b></div>)}
          <p className="text-[11px] text-muted">Connection is a separate axis from power</p>
        </Card>
        <Card title="Maintenance jobs by status — this period" action={<TextLink href="/admin/jobs">Open jobs →</TextLink>}>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(110px,1fr))] gap-x-4">{jobs.map(([s, n]) => <div key={s} className="flex justify-between border-t border-line py-1.5 text-[13px]"><span className="text-muted">{s}</span><b>{n}</b></div>)}</div>
        </Card>
        <Card title="Billing — unpaid by currency" action={<TextLink href="/admin/billing">Open overdue →</TextLink>}>
          <div className="flex justify-between border-t border-line py-2 text-[13px]"><span>Overdue invoices</span><b>1</b></div><div className="flex justify-between border-t border-line py-2 text-[13px]"><span>MYR (unpaid)</span><b>120.00 MYR</b></div>
          <p className="text-[11px] text-muted">amountsByCurrency is listed per currency — never converted.</p>
        </Card>
      </div>
    </Page>
  );
}
