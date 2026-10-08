"use client";

import Link from "next/link";
import { use, useState } from "react";
import { BarChart, Badge, Banner, Card, ConnBadge, Page, SeverityBadge, SummaryList } from "@/components/ui";
import { useOp } from "@/lib/useOp";
import { klTime, type ApiUnitDetail } from "@/lib/units";

type ApiAlert = { id: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; type: string; evidenceKind: string; evidenceText: string; detectedAt: string };
type ApiJob = { projection: string; id?: string; jobId?: string; type: string; status: string; scheduledSlot?: { startAt: string; endAt: string } | null };
const evidence: Record<string, string> = { inferred: "Suspected", inspection: "Inspection record", demo_observation: "Observed (demo)" };

export default function PartnerUnit({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [ended, setEnded] = useState(false);
  // DATA_SOURCE=api: diagnosis-scoped reads for the company's accepted job (contractor:accepted-valid-offer, IR169)
  const unit = useOp<ApiUnitDetail, ApiUnitDetail | null>("units.get", { id }, null, (d) => d);
  const alerts = useOp<{ items: ApiAlert[] }, ApiAlert[]>("alerts.list", { limit: 20, filters: { unitId: id } }, [], (p) => p.items);
  const jobs = useOp<{ items: ApiJob[] }, ApiJob[]>("jobs.list", { limit: 50, filters: { unitId: id } }, [], (p) => p.items);
  if (unit.source === "api") {
    const d = unit.data;
    if (unit.error) return <Page className="max-w-xl"><Card title="This unit isn’t available" sub={unit.error.error.code === "NOT_FOUND" ? "Your company has no accepted job with an open access window on this unit. Only a history snapshot of past work remains." : unit.error.error.code} /></Page>;
    if (!d) return <Page><Card title="Loading…" /></Page>;
    const current = jobs.data.filter((j) => j.projection === "summary");
    const past = jobs.data.filter((j) => j.projection === "history");
    return (
      <Page>
        <div><h1 className="text-lg font-bold">{d.displayName}</h1><p className="text-xs text-muted">Diagnosis-scoped view for your accepted job — no billing, payments, or other contracts.</p></div>
        <div className="split">
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Unit register"><SummaryList items={[["Location", d.location.pathLabels.join(" › ")], ["Model", `${d.capabilities.manufacturer} ${d.capabilities.model}`], ["Maintenance scope", d.serviceScope.join(" · ")], ["Connection", <ConnBadge key="c" s={d.connection === "online" ? "online" : "offline"} />], ["Last seen", klTime(d.lastSeenAt, true)]]} /><p className="mt-2 text-[11px] text-muted">Read-only — no remote-control actions available to contractors.</p></Card>
            <Card title={`Alert evidence (${alerts.data.length})`}>{alerts.data.length === 0 ? <p className="text-[13px] text-muted">No alerts on this unit.</p> : alerts.data.map((a) => <div key={a.id} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{a.causeCode !== "unknown" ? a.causeCode.replace(/_/g, " ") : a.type}</b> <SeverityBadge s={a.severity} /> <Badge tone="muted">{a.status}</Badge><div className="text-xs text-muted">{evidence[a.evidenceKind] ?? a.evidenceKind} — {a.evidenceText} · {klTime(a.detectedAt, true)}</div></div>)}</Card>
            <Card title="Readings (diagnosis only)" sub={`observed ${klTime(d.observedState.observedAt, true)}`}><SummaryList items={d.latestMeasurements.length ? d.latestMeasurements.map((m) => [m.metric.replace(/_/g, " "), `${m.value ?? "—"} ${m.unit} · ${klTime(m.observedAt)}`] as [string, string]) : [["Measurements", "No recent valid measurements"]]} /></Card>
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Job context">{current.length === 0 ? <p className="text-[13px] text-muted">No current job.</p> : current.map((j) => <Link key={j.id} href={`/partner/jobs/${j.id}`} className="block border-t border-line py-2 text-[13px] first:border-0 hover:bg-surface2/50"><b>{j.type} · {j.status}</b><div className="text-xs text-muted">{j.scheduledSlot ? `${klTime(j.scheduledSlot.startAt, true)} – ${klTime(j.scheduledSlot.endAt)}` : "not scheduled"}</div></Link>)}<p className="mt-2 text-xs text-muted">Access ends with the accepted offer’s access window; after that only a history snapshot remains.</p></Card>
            <Card title="Our past work on this unit">{past.length === 0 ? <p className="text-[13px] text-muted">No past jobs.</p> : past.map((j) => <div key={j.jobId} className="border-t border-line py-2 text-[13px] first:border-0"><b>{j.type} · {j.status}</b><div className="text-xs text-muted">{(j.jobId ?? "").slice(0, 8)}</div></div>)}<p className="mt-1 text-[11px] text-muted">Only your company’s own jobs are listed — other contractors’ work and customer billing are hidden.</p></Card>
          </div>
        </div>
      </Page>
    );
  }
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2"><div><h1 className="text-lg font-bold">Rooftop unit · {id}</h1><p className="text-xs text-muted">Diagnosis-scoped view for job-p02 — no billing, payments, or other contracts.</p></div><button className="text-[11px] text-muted underline" onClick={() => setEnded((e) => !e)}>{ended ? "show live" : "preview: delegation ended"}</button></div>
      {ended && <Banner tone="crit">Delegation ended while open (FORBIDDEN) — live values cleared; only a history snapshot remains.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Unit register"><SummaryList items={[["Location", "customer-b · Tower A rooftop"], ["Model", "cap-split-std v2"], ["Maintenance scope", "General + refrigerant"], ["Connection", <ConnBadge key="c" s="online" />], ["Last seen", "2026-09-14 09:35"]]} /><p className="mt-2 text-[11px] text-muted">Read-only — no remote-control actions available to contractors.</p></Card>
          <Card title="Alert evidence">{[["Vibration anomaly detected", "Suspected — evidence: amplitude 2.1x baseline · 2026-09-13 22:40"], ["Refrigerant pressure low", "Inspection record — logged by tech-external-a · 2026-09-14 08:10"]].map(([a, b]) => <div key={a} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{a}</b><div className="text-xs text-muted">{b}</div></div>)}</Card>
          {!ended && <Card title="Readings (diagnosis only)" sub="observed 2026-09-14 09:35 · live"><SummaryList items={[["Supply air temperature", "14.2 °C · good"], ["Refrigerant pressure (low side)", "3.1 bar · good"], ["Vibration amplitude", "2.1× baseline · suspect"], ["Power state", "On · cooling"]]} /></Card>}
          {!ended && <Card title="Readings — last 24 h" sub="09-13 09:35 → 09-14 09:35"><div className="grid-fluid" style={{ ["--min"as string]: "260px" }}><div><div className="mb-1 text-xs font-semibold">Vibration amplitude (× baseline) · now 2.1</div><BarChart labels={["a", "b", "c", "d", "e", "f"]} series={[[1, 1.1, 1, 1.4, 2.1, 2.1]]} colors={["#b45309"]} height={110} /></div><div><div className="mb-1 text-xs font-semibold">Refrigerant pressure, low side (bar) · now 3.1</div><BarChart labels={["a", "b", "c", "d", "e", "f"]} series={[[3.4, 3.3, 3.3, 3.2, 3.1, 3.1]]} height={110} /></div></div><p className="mt-2 text-[11px] text-muted">Diagnosis only — read-only values, no control. Highlighted bars cross the alert threshold.</p></Card>}
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Job context" action={<Link className="text-xs font-semibold text-primary" href="/partner/jobs/job-p09">Job →</Link>}><SummaryList items={[["Job", "job-p02 · Reactive"], ["Technician", "Not assigned yet"], ["Access window", "09-14 01:00 → 09-22 00:00"]]} /><p className="mt-2 text-xs text-muted">Access ends in 14 h. After that, live values are cleared and only a history snapshot remains.</p></Card>
          <Card title="Our past work on this unit">{[["job-c01 · Preventive maintenance", "2026-06-12 · tech-external-a"], ["job-b88 · Filter replacement", "2026-03-04 · tech-external-a2"]].map(([a, b]) => <div key={a} className="border-t border-line py-2 text-[13px] first:border-0"><b>{a}</b><div className="text-xs text-muted">{b}</div></div>)}<p className="mt-1 text-[11px] text-muted">Only contractor-a’s own jobs are listed — other contractors’ work and customer billing are hidden.</p></Card>
        </div>
      </div>
    </Page>
  );
}
