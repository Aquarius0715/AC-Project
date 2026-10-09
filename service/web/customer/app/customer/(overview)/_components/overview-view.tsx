"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, BarChart, Btn, Card, ConnBadge, Kpi, LineChart, Page, PowerBadge, Select, SeverityBadge, Tabs, TextLink } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { hms, OVERVIEW_PERIODS, type OverviewPeriod } from "@ac/web/lib/customerOverview";
import type { OverviewLive } from "../_lib/load";

/** Customer overview (FR-C01, DD-C01, Figma Client 01a) from the Core API: the scope and period live in the URL. */
export function OverviewView({ live }: { live: OverviewLive }) {
  const router = useRouter();
  const patch = useUrlPatch();
  const c = live.counts;
  const where = live.propertyName ? ` in ${live.propertyName}` : "";
  const a = live.attention;
  // the values were read when the page rendered: two minutes later they read as stale until a refresh (DD-C01)
  const [staleFor, setStaleFor] = useState<string | null>(null);
  useEffect(() => {
    const t = setTimeout(() => setStaleFor(live.asOf), 120_000);
    return () => clearTimeout(t);
  }, [live.asOf]);
  const stale = staleFor === live.asOf;
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-line bg-surface px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted">Viewing</span>
          <Select aria-label="Property" className="w-auto text-xs font-semibold" value={live.propertyId ?? ""} onChange={(e) => patch({ propertyId: e.target.value || null, unitId: null })}>
            <option value="">Property: All properties</option>{live.properties.map((p) => <option key={p.id} value={p.id}>Property: {p.name}</option>)}
          </Select>
          <Select aria-label="Unit" className="w-auto text-xs font-semibold" value={live.unitId ?? ""} onChange={(e) => patch({ unitId: e.target.value || null })}>
            <option value="">Unit: All units ({live.units.length})</option>{live.units.map((u) => <option key={u.id} value={u.id}>Unit: {u.name}</option>)}
          </Select>
          <Tabs value={live.period} onChange={(v: OverviewPeriod) => patch({ period: v === "today" ? null : v })} tabs={OVERVIEW_PERIODS} />
        </div>
        <div className="text-right text-xs">
          <div className="flex items-center justify-end gap-2 font-semibold">Updated {hms(live.asOf)} <Badge tone={stale ? "unknown" : "ok"} icon="●">{stale ? "Stale" : "Live"}</Badge><Btn size="sm" onClick={() => router.refresh()}>↻ Refresh</Btn></div>
          <div className="text-muted">{live.note}</div>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,2fr)_repeat(3,minmax(0,1fr))]">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Kpi label="Running" badge={<PowerBadge s="running" />} value={c.powerOn} sub={`of ${c.total} unit${c.total === 1 ? "" : "s"}${where} · latest observation`} href="/customer/properties?powerState=on" link="View running units →" />
          <Kpi label="Stopped" badge={<PowerBadge s="stopped" />} value={c.powerOff} sub="Stopped — confirmed by the device" href="/customer/properties?powerState=off" link="View stopped units →" />
          <Kpi label="Unknown" badge={<PowerBadge s="unknown" />} value={c.powerUnknown} sub="Offline or missing data — counted here, never as running/normal" href="/customer/properties?connections=offline" link="View unknown units →" />
          <Kpi label="Needs attention" badge={<SeverityBadge s="warning" />} value={c.alertCount} tone={c.alertCount ? "warn" : undefined} sub={`Unresolved warnings${a.info.count ? ` · +${a.info.count} reminder${a.info.count === 1 ? "" : "s"} & info (not counted)` : ""}`} href="/customer/alerts" link="Open alerts →" />
        </div>
        <Card title="Energy used" sub={live.energy?.sub ?? "No units in this selection"} action={<span className="text-xs text-muted">kWh</span>}>
          {live.energy ? <>
            <div className="text-[28px] font-bold">{live.energy.total ?? "—"} <span className="text-sm font-medium text-muted">{live.energy.total === null ? "no data" : "kWh"}</span></div>
            {live.energy.delta && <div className={live.energy.delta.tone === "ok" ? "mb-2 text-xs font-semibold text-ok" : "mb-2 text-xs font-semibold text-warn"}>{live.energy.delta.text}</div>}
            <BarChart labels={live.energy.labels} series={live.energy.series} />
            <div className="mt-1 flex gap-4 text-[11px] text-muted"><span><i className="mr-1 inline-block h-2 w-2 bg-primary" />Last 7 days</span><span><i className="mr-1 inline-block h-2 w-2 bg-[#c9d8ee]" />7 days before</span></div>
            {live.energy.gaps && <p className="mt-1 text-[11px] text-muted">Days without readings show no bar — not 0 kWh.</p>}
          </> : <p className="text-[13px] text-muted">Choose a property with units.</p>}
          <TextLink href="/customer/energy" className="mt-2 inline-block">Energy details & table view →</TextLink>
        </Card>
        <Card title="Estimated emissions" sub={live.emissions ? `${live.periodLabel} · ${live.emissions.scope}` : "No units in this selection"} action={<span className="text-xs text-muted">kgCO2e</span>}>
          {live.emissions && <>
            <div className="text-[28px] font-bold">{live.emissions.total ?? "—"} <span className="text-sm font-medium text-muted">{live.emissions.total === null ? "no data" : "kgCO2e"}</span></div>
            {live.emissions.factor && <p className="text-[11px] text-muted">Factor {live.emissions.factor}</p>}
            {live.emissions.baseline ? <><p className="mt-2 text-xs">Baseline <b>{live.emissions.baseline}</b></p><p className="text-xs">Estimated savings <b className="text-ok">{live.emissions.saved}</b></p></>
              : <p className="mt-2 text-xs text-muted">No comparable baseline for these units — no saving is estimated.</p>}
            <p className="mt-2 text-[11px] text-muted">Estimate only — savings are not a tradable balance.</p>
          </>}
          <TextLink href="/customer/energy" className="mt-2 inline-block">Carbon impact →</TextLink>
        </Card>
        <Card title={`Air quality${live.air ? ` · ${live.air.room}` : ""}`} action={live.air && <Badge tone={live.air.co2.feed.tone} icon={live.air.co2.feed.icon}>{live.air.co2.feed.text}</Badge>}>
          {live.air ? <>
            <div className="text-[28px] font-bold"><span className="text-xs font-normal text-muted">CO2 </span>{live.air.co2.value ?? "—"} <span className="text-sm font-medium text-muted">ppm</span></div>
            {live.air.co2.warn && <div className="my-1"><Badge tone="warn" icon="⚠">Ventilation recommended</Badge></div>}
            <p className="mb-2 text-[11px] text-muted">{live.air.co2.sub} · {live.air.unit} sensor · threshold 1000 ppm</p>
            {live.air.points.some((p) => p !== null) ? <LineChart points={live.air.points} min={400} max={Math.max(1200, ...live.air.points.filter((p): p is number => p !== null).map((p) => p + 100))} threshold={1000} height={90} />
              : <p className="text-[11px] text-muted">No CO2 readings in the last 24 h.</p>}
            {live.air.cut && <p className="text-[11px] text-muted">Chart from the latest 300 readings.</p>}
            <p className="mt-1 text-xs">PM2.5 <b>{live.air.pm25.value === null ? "—" : `${live.air.pm25.value} µg/m³`}</b> <span className="text-muted">· {live.air.pm25.advice}</span></p>
          </> : <p className="text-[13px] text-muted">No units in this selection.</p>}
          <TextLink href={live.air ? `/customer/air-quality?unitId=${live.air.id}` : "/customer/air-quality"} className="mt-1 inline-block">Air quality details →</TextLink>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,3fr)_minmax(0,1fr)]">
        <Card title={`Units${where || " in all properties"}`} sub="Latest value per unit — no averaged temperature across rooms (BR-C01)" action={<TextLink href="/customer/properties">View all →</TextLink>}>
          <div className="flex flex-col divide-y divide-line">
            <div className="hidden grid-cols-[minmax(0,1fr)_72px_72px_72px_120px_14px] gap-2 pb-2 text-[11px] text-muted lg:grid">
              <span>Unit · location · observed</span><span>Room temp</span><span>Humidity</span><span>Power now</span><span>Status</span><span />
            </div>
            {live.rows.map((u) => (
              <Link key={u.id} href={`/customer/units/${u.id}`} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-2 py-3 hover:bg-surface2/50 lg:grid-cols-[minmax(0,1fr)_72px_72px_72px_120px_14px]">
                <div className="flex min-w-0 items-center gap-2.5"><span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-primary-soft text-primary">❄</span><div className="min-w-0"><div className="truncate font-bold">{u.name}</div><div className="truncate text-[11px] text-muted">{u.place ? `${u.place} · ` : ""}{u.observed}</div></div></div>
                <div className="col-span-2 row-start-2 flex gap-4 text-xs lg:contents">
                  {([[u.temp, "°C"], [u.hum, "%"], [u.power, "W"]] as const).map(([v, unit], i) => (
                    <div key={i}><span className="text-[13px] font-bold">{v ?? "—"}</span> <span className="text-[11px] text-muted">{v === null ? "no data" : unit}</span></div>
                  ))}
                </div>
                <div className="col-start-2 row-start-1 flex flex-col items-end gap-1 lg:col-auto lg:row-auto lg:items-start"><PowerBadge s={u.state} /><ConnBadge s={u.conn} /></div>
                <span className="hidden text-muted lg:block">›</span>
              </Link>
            ))}
            {live.rows.length === 0 && <p className="py-4 text-center text-xs text-muted">No units in this selection.</p>}
          </div>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Needs attention" action={<span className={c.alertCount ? "font-bold text-warn" : "font-bold text-muted"}>{c.alertCount}</span>}>
            {a.items.length ? (
              <ul className="flex flex-col divide-y divide-line">
                {a.items.map((x) => (
                  <li key={x.id}><Link href="/customer/alerts" className="flex items-center justify-between gap-2 py-2.5"><div><div className="font-bold">{x.title}</div><div className="my-1"><SeverityBadge s={x.severity} /></div><div className="text-[11px] text-muted">{x.where}</div></div><span className="text-muted">›</span></Link></li>
                ))}
              </ul>
            ) : <p className="text-xs text-muted">Nothing needs your attention.</p>}
            {a.more > 0 && <p className="text-[11px] text-muted">+{a.more} more</p>}
            {a.info.count > 0 && <p className="mt-2 border-t border-line pt-2 text-[11px] text-muted">Reminders & info ({a.info.count}) — {a.info.text}</p>}
            <TextLink href="/customer/alerts" className="mt-1 inline-block">Open alerts →</TextLink>
          </Card>
          <Card title="Automations" action={<span className="text-xs text-muted">{live.automations.on} on · {live.automations.off} off</span>}>
            {live.automations.items.map((x) => (
              <div key={x.id} className="flex items-center justify-between gap-2 py-2"><div className="min-w-0"><div className="font-bold">{x.name}</div><div className="text-[11px] text-muted">{x.line}</div></div><Badge tone={x.status.tone} icon={x.status.tone === "ok" ? "●" : undefined}>{x.status.text}</Badge></div>
            ))}
            {live.automations.items.length === 0 && <p className="text-xs text-muted">No automations for these units.</p>}
            <TextLink href="/customer/automations" className="mt-1 inline-block">Manage automations →</TextLink>
          </Card>
        </div>
      </div>
    </Page>
  );
}
