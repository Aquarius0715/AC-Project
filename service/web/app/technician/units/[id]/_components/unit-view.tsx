"use client";

import Link from "next/link";
import { useState } from "react";
import { Banner, Card, ConnBadge, LineChart, Page, SeverityBadge, SummaryList, Tabs } from "@/components/ui";
import { bucket, componentGroups, klTime, latest, windowMs, type ApiUnitDetail } from "@/lib/units";

export type ApiAlert = { id: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; type: string; evidenceText: string; detectedAt: string; acknowledgedAt: string | null };
export type ApiJob = { projection: string; id?: string; type?: string; status?: string; scheduledSlot?: { startAt: string; endAt: string } | null; requestedSlot?: { startAt: string; endAt: string } | null };
export type ApiMeasurement = { value: number | null; unit: string; observedAt: string; quality: string };
const jobType: Record<string, string> = { reactive: "Reactive", preventive: "Preventive", installation: "Installation", inspection: "Inspection" };

const comps = { Indoor: ["Filter: attention (2026-06-12)"], Outdoor: ["All normal (2026-06-12)"], Electrical: ["Capacitor not inspected — reason given"] };
const counts = { Indoor: 8, Outdoor: 5, Electrical: 5 };

export type TechUnitData = { d: ApiUnitDetail; alerts: ApiAlert[]; jobs: ApiJob[]; series: ApiMeasurement[]; now: string };

/** Technician unit page. `data` comes from the Server Component in API mode (series = latest 7 days, up to 100
 * temperature measurements); the window tabs filter it on the client. The demo uses fixture values. */
export function TechUnitView({ id, data }: { id: string; data?: TechUnitData }) {
  const [tab, setTab] = useState<"register" | "monitoring">("register");
  const [win, setWin] = useState<"1h" | "24h" | "7d">("24h");
  const [lost, setLost] = useState(false);
  const d = data?.d ?? null;
  const alerts = { data: data?.alerts ?? [] };
  const jobs = { data: data?.jobs ?? [] };
  const to = new Date(data?.now ?? 0);
  const from = new Date(to.getTime() - windowMs[win]);
  const series = { data: (data?.series ?? []).filter((m) => new Date(m.observedAt) >= from), loading: false };
  if (d) {
    const groups = componentGroups(d.components);
    const temp = latest(d, "temperature");
    const pow = latest(d, "power");
    const hum = latest(d, "humidity");
    const offline = d.connection !== "online";
    const points = bucket(series.data, from, to);
    const sched = (j: ApiJob) => j.scheduledSlot ?? j.requestedSlot;
    return (
      <Page>
        <div className="flex flex-wrap items-center justify-between gap-2"><div><h1 className="text-lg font-bold">{d.displayName}</h1><div className="mt-1 flex gap-2"><ConnBadge s={offline ? "offline" : "online"} /></div></div><Tabs value={tab} onChange={setTab} tabs={[{ id: "register", label: "Register" }, { id: "monitoring", label: "Monitoring" }]} /></div>
        {offline && <Banner tone="warn">Communication lost — updates stopped. Showing last known values (last seen {klTime(d.lastSeenAt, true)}).</Banner>}
        {tab === "register" ? (
          <div className="split">
            <div className="flex min-w-0 flex-col gap-4">
              <Card title="Unit register"><SummaryList cols={2} items={[["Location", d.location.pathLabels.join(" › ")], ["Manufacturer / model", `${d.capabilities.manufacturer} / ${d.capabilities.model}`], ["Installed", d.installedAt ? klTime(d.installedAt, true).split(",")[0] : "Not registered"], ["Capability version", String(d.capabilityVersion)], ["Maintenance scope", d.serviceScope.map((x) => x[0].toUpperCase() + x.slice(1)).join(" · ")], ["Access", d.location.accessInstructions ?? "—"]]} /><p className="mt-2 text-xs text-muted">{d.components.length} components across 3 groups ({groups.Indoor.length} indoor, {groups.Outdoor.length} outdoor, {groups.Electrical.length} electrical) — inspect them in the Job Workspace.</p></Card>
              <Card title={`Components (${d.components.length})`}><div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>{(Object.keys(groups) as (keyof typeof groups)[]).map((g) => <div key={g} className="rounded-xl bg-surface2 p-3"><b className="text-[13px]">{g} · {groups[g].length} components</b><div className="text-xs text-muted">{groups[g].join(", ") || "—"}</div></div>)}</div></Card>
              <Card title="Maintenance history">{jobs.data.length === 0 ? <p className="text-[13px] text-muted">No jobs you can see on this unit.</p> : jobs.data.map((j) => <Link key={j.id} href={`/technician/jobs/${j.id}`} className="block border-t border-line py-2 text-[13px] first:border-0 hover:bg-surface2/50"><b>{jobType[j.type ?? ""] ?? j.type} · {j.status}</b><div className="text-xs text-muted">{sched(j) ? klTime(sched(j)!.startAt, true) : "not scheduled"}</div></Link>)}</Card>
            </div>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={`Open alerts (${alerts.data.length})`} action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${id}/alerts`}>Evidence →</Link>}>{alerts.data.length === 0 ? <p className="text-[13px] text-muted">No open alerts.</p> : alerts.data.map((a) => <div key={a.id} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{a.causeCode !== "unknown" ? a.causeCode.replace(/_/g, " ") : a.type}</b> <SeverityBadge s={a.severity} /><p className="text-xs text-muted">{a.evidenceText} · {klTime(a.detectedAt, true)}</p><p className="mt-1 text-xs text-muted">{a.acknowledgedAt ? "Acknowledged" : "Acknowledged by nobody yet"}. Completing the job does not resolve it.</p></div>)}</Card>
              <Card title="Live"><SummaryList items={[["Temperature", temp ? `${temp.text} · ${temp.at}` : "—"], ["Power", pow ? `${pow.text} · ${pow.at}` : "—"], ["Humidity", hum ? `${hum.text} · ${hum.at}` : "—"], ["Connection", `${d.connection} · last seen ${klTime(d.lastSeenAt)}`]]} /></Card>
              <Card title="Diagnostics"><Link href={`/technician/units/${id}/control`} className="text-xs font-semibold text-primary">Open diagnostic control →</Link></Card>
            </div>
          </div>
        ) : (
          <Card title={`Temperature · last ${win} (rolling)`} action={<Tabs value={win} onChange={setWin} tabs={[{ id: "1h", label: "1h" }, { id: "24h", label: "24h" }, { id: "7d", label: "7d" }]} />}>
            {points.every((p) => p === null) ? <p className="text-[13px] text-muted">{series.loading ? "Loading…" : "No valid measurements in this window."}</p> : <LineChart points={points} height={170} labels={[`${win} ago`, "", "", "", "now"]} />}
            <p className="mt-1 text-[11px] text-muted">latest {series.data.length} measurements · gaps stay unconnected; suspect or missing readings are not plotted.</p>
          </Card>
        )}
      </Page>
    );
  }
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
