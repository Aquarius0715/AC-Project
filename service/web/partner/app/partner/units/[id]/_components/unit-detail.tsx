// Contractor unit page in API mode (Server Component): diagnosis-scoped read-only view of a unit of the company's
// accepted job (contractor:accepted-valid-offer, IR169). Rendered from data the page read through the DAL.
import Link from "next/link";
import { Badge, Card, ConnBadge, Page, SeverityBadge, SummaryList } from "@ac/web/components/ui";
import { klTime, type ApiUnitDetail } from "@ac/web/lib/units";

export type ApiAlert = { id: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; type: string; evidenceKind: string; evidenceText: string; detectedAt: string };
export type ApiJob = { projection: string; id?: string; jobId?: string; type: string; status: string; scheduledSlot?: { startAt: string; endAt: string } | null };
const evidence: Record<string, string> = { inferred: "Suspected", inspection: "Inspection record", demo_observation: "Observed (demo)" };

export function PartnerUnitDetail({ d, alerts, jobs }: { d: ApiUnitDetail; alerts: ApiAlert[]; jobs: ApiJob[] }) {
  const current = jobs.filter((j) => j.projection === "summary");
  const past = jobs.filter((j) => j.projection === "history");
  return (
    <Page>
      <div><h1 className="text-lg font-bold">{d.displayName}</h1><p className="text-xs text-muted">Diagnosis-scoped view for your accepted job — no billing, payments, or other contracts.</p></div>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Unit register"><SummaryList items={[["Location", d.location.pathLabels.join(" › ")], ["Model", `${d.capabilities.manufacturer} ${d.capabilities.model}`], ["Maintenance scope", d.serviceScope.join(" · ")], ["Connection", <ConnBadge key="c" s={d.connection === "online" ? "online" : "offline"} />], ["Last seen", klTime(d.lastSeenAt, true)]]} /><p className="mt-2 text-[11px] text-muted">Read-only — no remote-control actions available to contractors.</p></Card>
          <Card title={`Alert evidence (${alerts.length})`}>{alerts.length === 0 ? <p className="text-[13px] text-muted">No alerts on this unit.</p> : alerts.map((a) => <div key={a.id} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{a.causeCode !== "unknown" ? a.causeCode.replace(/_/g, " ") : a.type}</b> <SeverityBadge s={a.severity} /> <Badge tone="muted">{a.status}</Badge><div className="text-xs text-muted">{evidence[a.evidenceKind] ?? a.evidenceKind} — {a.evidenceText} · {klTime(a.detectedAt, true)}</div></div>)}</Card>
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
