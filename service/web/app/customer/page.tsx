"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Btn, Card, ConnBadge, Kpi, LineChart, BarChart, Page, PowerBadge, SeverityBadge, Tabs, TextLink, OnOffBadge, Banner, ErrorState, cx } from "@/components/ui";
import { units as mockUnits, week, unitRowFromApi, mockCounts, type ApiUnit, type CustomerCounts } from "@/lib/client";
import { useOp } from "@/lib/useOp";

export default function Overview() {
  const [range, setRange] = useState<"today" | "7d" | "30d">("today");
  const [stale, setStale] = useState(false);
  // DATA_SOURCE=api: units.list + summaries.get(kind=customer); the demo keeps the fixture rows
  const { data: units, error } = useOp<{ items: ApiUnit[] }, typeof mockUnits>("units.list", { limit: 100 }, mockUnits, (p) => p.items.map(unitRowFromApi));
  const { data: counts } = useOp<{ counts: CustomerCounts }, CustomerCounts>("summaries.get", { kind: "customer", filters: {} }, mockCounts, (s) => s.counts);
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-line bg-surface px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted">Viewing</span>
          <select aria-label="Property" className="rounded-control border border-line bg-surface px-2.5 py-1.5 text-xs font-semibold"><option>Property: Home A</option><option>Property: Office A</option></select>
          <select aria-label="Unit" className="rounded-control border border-line bg-surface px-2.5 py-1.5 text-xs font-semibold"><option>Unit: All units ({units.length})</option>{units.map((u) => <option key={u.id}>{u.name}</option>)}</select>
          <Tabs value={range} onChange={setRange} tabs={[{ id: "today", label: "Today" }, { id: "7d", label: "7d" }, { id: "30d", label: "30d" }]} />
        </div>
        <div className="text-right text-xs">
          <div className="font-semibold">Updated 09:12:30 <Badge tone={stale ? "unknown" : "ok"} icon="●">{stale ? "Stale" : "Live"}</Badge></div>
          <div className="text-muted">Today = 00:00–09:12 · Asia/Kuala_Lumpur</div>
        </div>
      </div>

      {error && <Banner tone="warn">Could not load units ({error.error.code}). Showing the last loaded values.</Banner>}
      {stale && <Banner tone="warn" action={<Btn size="sm" onClick={() => setStale(false)}>↻ Retry</Btn>}>Offline — showing values last loaded at 09:12. Remote actions are disabled until the connection returns.</Banner>}

      <div className="grid-fluid" style={{ ["--min" as string]: "210px" }}>
        <Kpi label="Running" badge={<PowerBadge s="running" />} value={counts.powerOn} sub={`of ${counts.total} units · latest observation per unit`} href="/customer/properties?powerState=on" link="View running units →" />
        <Kpi label="Stopped" badge={<PowerBadge s="stopped" />} value={counts.powerOff} sub="Stopped — confirmed by the device" href="/customer/properties?powerState=off" link="View stopped units →" />
        <Kpi label="Unknown" badge={<PowerBadge s="unknown" />} value={counts.powerUnknown} sub="Offline or missing data — counted here, never as running/normal" href="/customer/properties?connections=offline" link="View unknown units →" />
        <Kpi label="Needs attention" badge={<SeverityBadge s="warning" />} value={2} tone="warn" sub="Unresolved warnings · +2 reminders & info (not counted)" href="/customer/alerts" link="Open alerts →" />
      </div>

      <div className="split">
        <Card title="Units in Home A" sub="Latest value per unit — no averaged temperature across rooms (BR-C01)" action={<TextLink href="/customer/properties">View all →</TextLink>}>
          <div className="flex flex-col divide-y divide-line">
            <div className="hidden grid-cols-[minmax(0,1fr)_64px_64px_72px_110px_14px] gap-2 pb-2 text-[11px] text-muted @lg:grid">
              <span>Unit · location · observed</span><span>Room temp</span><span>Humidity</span><span>Power now</span><span>Status</span><span />
            </div>
            {units.map((u) => (
              <Link key={u.id} href={`/customer/units/${u.id}`} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-2 py-3 hover:bg-surface2/50 @lg:grid-cols-[minmax(0,1fr)_64px_64px_72px_120px_14px]">
                <div className="flex min-w-0 items-center gap-2.5"><span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-primary-soft text-primary">❄</span><div className="min-w-0"><div className="truncate font-bold">{u.name}</div><div className="truncate text-[11px] text-muted">{u.loc} · {u.obs}</div></div></div>
                <div className="col-span-2 row-start-2 flex gap-4 text-xs @lg:contents">
                  {([[u.temp, "°C"], [u.hum, "%"], [u.power, "W"]] as const).map(([v, unit], i) => (
                    <div key={i}><span className="text-[13px] font-bold">{v ?? "—"}</span> <span className="text-[11px] text-muted">{v === null ? "no data" : unit}</span></div>
                  ))}
                </div>
                <div className="col-start-2 row-start-1 flex flex-col items-end gap-1 @lg:col-auto @lg:row-auto @lg:items-start"><PowerBadge s={u.state} /><ConnBadge s={u.conn} /></div>
                <span className="hidden text-muted @lg:block">›</span>
              </Link>
            ))}
          </div>
        </Card>

        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Needs attention" action={<span className="font-bold text-warn">2</span>}>
            <ul className="flex flex-col divide-y divide-line">
              {[["Possible open window", "Bedroom AC · 09:12 today"], ["Compressor short-cycling", "Kitchen AC · Yesterday 22:40"]].map(([t, d]) => (
                <li key={t}><Link href="/customer/alerts" className="flex items-center justify-between gap-2 py-2.5"><div><div className="font-bold">{t}</div><div className="my-1"><SeverityBadge s="warning" /></div><div className="text-[11px] text-muted">{d}</div></div><span className="text-muted">›</span></Link></li>
              ))}
            </ul>
            <p className="mt-2 border-t border-line pt-2 text-[11px] text-muted">Reminders & info (2) — filter cleaning, insulation inspection record</p>
            <TextLink href="/customer/alerts" className="mt-1 inline-block">Open alerts →</TextLink>
          </Card>
          <Card title="Automations" action={<span className="text-xs text-muted">3 on · 1 off</span>}>
            {[["Weekday pre-cool", "Next run: today 18:00 · Bedroom AC"], ["Away power-save", "When everyone leaves · location consent: granted"]].map(([t, d]) => (
              <div key={t} className="flex items-center justify-between gap-2 py-2"><div className="min-w-0"><div className="font-bold">{t}</div><div className="text-[11px] text-muted">{d}</div></div><OnOffBadge on /></div>
            ))}
            <TextLink href="/customer/automations" className="mt-1 inline-block">Manage automations →</TextLink>
          </Card>
        </div>
      </div>

      <div className="grid-fluid" style={{ ["--min" as string]: "290px" }}>
        <Card title="Energy used" sub="This week · Sep 14–20 · all 5 units" action={<span className="text-xs text-muted">kWh</span>}>
          <div className="text-[28px] font-bold">18.6 <span className="text-sm font-medium text-muted">kWh</span></div>
          <div className="mb-2 text-xs font-semibold text-ok">↓ 8% vs last week (20.2 kWh)</div>
          <BarChart labels={week} series={[[3.1, 2.9, 2.4, 3.3, 2.9, 2.2, 1.8].map((v) => v + 0.4), [3.1, 2.9, 2.4, 3.3, 2.9, 2.2, 1.8]]} />
          <div className="mt-1 flex gap-4 text-[11px] text-muted"><span><i className="mr-1 inline-block h-2 w-2 bg-primary" />This week</span><span><i className="mr-1 inline-block h-2 w-2 bg-[#c9d8ee]" />Last week</span></div>
          <TextLink href="/customer/energy" className="mt-2 inline-block">Energy details & table view →</TextLink>
        </Card>
        <Card title="Estimated emissions" sub="This week · all 5 units · factor-demo-2026 (demo factor)" action={<span className="text-xs text-muted">kgCO2e</span>}>
          <div className="text-[28px] font-bold">13.2 <span className="text-sm font-medium text-muted">kgCO2e</span></div>
          <p className="mt-2 text-xs">Baseline <b>15.3 kgCO2e</b> (demo_fixed)</p>
          <p className="text-xs">Estimated savings <b className="text-ok">2.1 kgCO2e</b></p>
          <p className="mt-2 text-[11px] text-muted">Estimate only — savings are not a tradable balance.</p>
          <TextLink href="/customer/energy" className="mt-2 inline-block">Carbon impact →</TextLink>
        </Card>
        <Card title="Air quality · Bedroom" action={<Badge tone="ok" icon="●">Live</Badge>}>
          <div className="text-[28px] font-bold"><span className="text-xs font-normal text-muted">CO2 </span>1000 <span className="text-sm font-medium text-muted">ppm</span></div>
          <div className="my-1"><Badge tone="warn" icon="⚠">Ventilation recommended</Badge></div>
          <p className="mb-2 text-[11px] text-muted">Observed 09:12 · Bedroom AC sensor · threshold 1000 ppm</p>
          <LineChart points={[640, 650, 700, 760, 820, 880, 930, 960, 980, 1000]} min={400} max={1200} threshold={1000} height={90} />
          <p className="mt-1 text-xs">PM2.5 <b>12 µg/m³</b> <span className="text-muted">· No current advice</span></p>
          <TextLink href="/customer/air-quality" className="mt-1 inline-block">Air quality details →</TextLink>
        </Card>
      </div>
      <div className="flex justify-end"><button className="text-[11px] text-muted underline" onClick={() => setStale((s) => !s)}>{stale ? "Hide" : "Show"} offline/stale state</button></div>
    </Page>
  );
}
