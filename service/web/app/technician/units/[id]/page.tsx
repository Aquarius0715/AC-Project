"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Card, ConnBadge, LineChart, Page, SeverityBadge, SummaryList, Tabs } from "@/components/ui";

const comps = { Indoor: ["Filter: attention (2026-06-12)"], Outdoor: ["All normal (2026-06-12)"], Electrical: ["Capacitor not inspected — reason given"] };
const counts = { Indoor: 8, Outdoor: 5, Electrical: 5 };

export default function TechUnit({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [tab, setTab] = useState<"register" | "monitoring">("register");
  const [win, setWin] = useState<"1h" | "24h" | "7d">("24h");
  const [lost, setLost] = useState(false);
  if (id === "unit-other-customer") return <Page className="max-w-xl"><Card title="This page isn’t available" sub="Not in your assignments (NOT_FOUND)." /></Page>;
  const missing = id === "unit-non-rto";
  const temp = [26, 26, 25, 25, null, null, 26, 27, 28, 29, 30, 30.4];
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2"><div><h1 className="text-lg font-bold">Bedroom AC · {id}</h1><div className="mt-1 flex gap-2"><ConnBadge s={lost ? "offline" : "online"} /></div></div><Tabs value={tab} onChange={setTab} tabs={[{ id: "register", label: "Register" }, { id: "monitoring", label: "Monitoring" }]} /></div>
      {lost && <Banner tone="warn">Communication lost — updates stopped. Showing last known values.</Banner>}
      {tab === "register" ? (
        <div className="split">
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Unit register"><SummaryList cols={2} items={[["Location", "customer-a · Home A › 1F › Bedroom"], ["Manufacturer / model", missing ? "Not registered" : "AC-Co / ventilation-demo v3"], ["Configuration", missing ? "Not registered" : "Split · 2.5 kW · R32"], ["Installed", missing ? "Not registered" : "2025-03-01"], ["Capability version", "3"], ["Maintenance scope", "Indoor · Outdoor · Electrical"]]} /><p className="mt-2 text-xs text-muted">18 components across 3 groups (8 indoor, 5 outdoor, 5 electrical) — see Job Workspace for inspection.</p></Card>
            <Card title="Components (18) · last inspection"><div className="grid-fluid" style={{ ["--min"as string]: "200px" }}>{(Object.keys(comps) as (keyof typeof comps)[]).map((g) => <div key={g} className="rounded-xl bg-surface2 p-3"><b className="text-[13px]">{g} · {counts[g]} components</b>{comps[g].map((c) => <div key={c} className="text-xs text-muted">{c}</div>)}</div>)}</div></Card>
            <Card title="Maintenance history">{[["job-contractor-a · Reactive", "today 10:00–12:00 · you"], ["job-c01 · Preventive", "2026-06-12 · filter replaced"], ["job-a17 · Installation check", "2025-03-01 · commissioning"]].map(([a, b]) => <div key={a} className="border-t border-line py-2 text-[13px] first:border-0"><b>{a}</b><div className="text-xs text-muted">{b}</div></div>)}</Card>
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Open alerts (1)" action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${id}/alerts`}>Evidence →</Link>}><b className="text-[13px]">Bedroom too hot</b> <SeverityBadge s="warning" /><p className="text-xs text-muted">30.4 °C since 09:12 · policy ≥ 30 °C</p><p className="mt-1 text-xs text-muted">Acknowledged by nobody yet. Completing the job does not resolve it.</p></Card>
            <Card title="Live"><SummaryList items={[["Temperature", "24.5°C · updated 2s ago"], ["Power", "1200 W · updated 2s ago"], ["Connection", lost ? "Offline" : "Online · last seen 09:41"]]} /><button className="mt-2 text-[11px] text-muted underline" onClick={() => setLost((l) => !l)}>preview: communication lost</button></Card>
            <Card title="Diagnostics"><Link href={`/technician/units/${id}/control`} className="text-xs font-semibold text-primary">Open diagnostic control →</Link></Card>
          </div>
        </div>
      ) : (
        <div className="split">
          <Card title="Temperature · last 24 h (rolling)" action={<Tabs value={win} onChange={setWin} tabs={[{ id: "1h", label: "1h" }, { id: "24h", label: "24h" }, { id: "7d", label: "7d" }]} />}>
            <div className="mb-3 grid-fluid" style={{ ["--min"as string]: "130px" }}>{[["Temperature", "24.5°C", "observed 09:41:02"], ["Power", "1200 W", "updated 2s ago"], ["Operation", "Cooling", "observed 09:41:02"], ["Connection", lost ? "Offline" : "Online", "last seen 09:41"]].map(([a, b, c]) => <div key={a} className="rounded-xl bg-surface2 p-2.5"><div className="text-[11px] text-muted">{a}</div><b>{b}</b><div className="text-[10px] text-muted">{c}</div></div>)}</div>
            <LineChart points={temp} min={22} max={32} threshold={30} height={170} labels={["24 h ago", "", "", "", "now"]} />
            <p className="mt-1 text-[11px] text-muted">no data 03:00–05:10 (gap not connected) · alert ≥ 30 °C. Reordered/duplicate demo events never roll values backward.</p>
          </Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Other metrics">{[["Humidity", "60 %RH"], ["Power", "1.20 kW"], ["Supply air", "14.2 °C"]].map(([a, b]) => <div key={a} className="flex justify-between border-t border-line py-2 text-[13px] first:border-0"><span>{a}</span><span><b>{b}</b><span className="block text-[10px] text-muted">observed 09:41:02 · gap 03:00–05:10</span></span></div>)}</Card>
            <Card title="Events in this window"><div className="text-[13px]"><b>09:12 · Alert raised — Bedroom too hot</b><div className="text-xs text-muted">temperature ≥ 30 °C for 60 s · alert-temp-a</div></div><div className="mt-2 text-[13px]"><b>05:10 · Data resumed</b><div className="text-xs text-muted">gateway reconnected · 2 h 10 m gap left unconnected</div></div></Card>
          </div>
        </div>
      )}
    </Page>
  );
}
