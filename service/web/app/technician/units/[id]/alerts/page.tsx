"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Field, Page, SeverityBadge, SummaryList, Textarea, Timeline, useToast } from "@/components/ui";
import { useBffSession, useOp, invalidate } from "@/lib/useOp";
import { callOp, OpError } from "@/lib/ops";
import { klTime } from "@/lib/units";

// DATA_SOURCE=api: alerts of this unit (alerts.list unitId), acknowledge / resolve with the alert version (IR87, IR94)
type ApiAlert = { id: string; version: number; severity: "critical" | "warning" | "normal"; status: "open" | "acknowledged" | "resolved"; type: string; causeCode: string; evidenceKind: string; evidenceText: string; observedAt: string; detectedAt: string; acknowledgedAt: string | null; resolvedAt: string | null; resolutionReason: string | null; previousAlertId: string | null; policyId: string | null };
const errText: Record<string, string> = {
  "errors.assignment_required": "You need an active assignment on this unit to act on its alerts.",
  "errors.assignment_not_started": "Your assignment has not started yet.",
  "errors.assignment_ended": "Your assignment on this unit has ended.",
  "error.versionConflict": "The alert changed — the latest state is shown.",
  "error.forbidden": "You don’t have alert.resolve.",
};
const label = (a: ApiAlert) => (a.causeCode !== "unknown" ? a.causeCode.replace(/_/g, " ") : `${a.type} alert`);

function ApiAlerts({ unitId }: { unitId: string }) {
  const toast = useToast();
  const list = useOp<{ items: ApiAlert[] }, ApiAlert[] | null>("alerts.list", { limit: 50, filters: { unitId } }, null, (p) => p.items);
  const [sel, setSel] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const alerts = list.data ?? [];
  const a = alerts.find((x) => x.id === sel) ?? alerts.find((x) => x.status !== "resolved") ?? alerts[0];
  const act = async (op: "alerts.acknowledge" | "alerts.resolve") => {
    if (!a) return;
    if (op === "alerts.resolve" && !reason.trim()) return setMsg("A resolution reason is required (1–1000 characters).");
    setBusy(true);
    setMsg(null);
    try {
      await callOp(op, op === "alerts.resolve" ? { alertId: a.id, resolutionReason: reason.trim(), resolutionEvidenceIds: [] } : { alertId: a.id }, { write: true, expectedVersion: a.version });
      toast(op === "alerts.resolve" ? "Alert resolved" : "Alert acknowledged");
      setReason("");
    } catch (e) {
      setMsg(e instanceof OpError ? (errText[e.error.messageKey] ?? `${e.error.code}: ${e.error.messageKey}`) : "The request failed.");
    } finally {
      setBusy(false);
      invalidate();
    }
  };
  if (list.error) return <Page className="max-w-xl"><Card title="Alerts aren’t available" sub={errText[list.error.error.messageKey] ?? list.error.error.code} /></Page>;
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/technician/units/${unitId}`} className="font-semibold text-primary">← Unit</Link> › Alert evidence</div>
      {!a ? <Card title={list.loading ? "Loading…" : "No alerts on this unit"} /> : (
        <>
          <div className="flex flex-wrap items-center gap-2"><h1 className="text-lg font-bold">{label(a)}</h1><SeverityBadge s={a.severity} /><Badge tone={a.status === "resolved" ? "ok" : a.status === "acknowledged" ? "primary" : "warn"}>{a.status}</Badge></div>
          <div className="split">
            <div className="flex min-w-0 flex-col gap-4">
              <Card title="Evidence" sub={`${a.evidenceKind.replace(/_/g, " ")} · observed ${klTime(a.observedAt, true)}`}><p className="text-[13px]">{a.evidenceText}</p></Card>
              <Card title="Resolution" sub="Automatic resolution needs a recovery remeasurement — or a reason below with alert.resolve and an active assignment. Completing the job does not resolve the alert.">
                <Field label="Resolution reason (required, 1–1000 characters)"><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} disabled={a.status === "resolved"} /></Field>
                {msg && <div className="mt-2"><Banner tone="crit">{msg}</Banner></div>}
                <div className="mt-3 flex flex-wrap gap-2"><Btn disabled={busy || a.status !== "open"} onClick={() => act("alerts.acknowledge")}>Acknowledge</Btn><Btn variant="primary" disabled={busy || a.status === "resolved"} onClick={() => act("alerts.resolve")}>Resolve alert</Btn></div>
                {a.resolutionReason && <p className="mt-2 text-xs text-muted">Resolved: {a.resolutionReason}</p>}
              </Card>
              <Card title="Alert history"><Timeline items={[a.resolvedAt && { time: klTime(a.resolvedAt), title: "Resolved", detail: a.resolutionReason ?? "", tone: "ok" as const }, a.acknowledgedAt && { time: klTime(a.acknowledgedAt), title: "Acknowledged", detail: "" }, { time: klTime(a.detectedAt), title: "Raised", detail: a.evidenceText, tone: "warn" as const }].filter(Boolean) as { time: string; title: string; detail: string }[]} /></Card>
            </div>
            <Card title={`Alerts on this unit (${alerts.length})`} className="self-start">{alerts.map((x) => <button key={x.id} onClick={() => { setSel(x.id); setMsg(null); }} className={`block w-full border-t border-line py-2 text-left text-[13px] first:border-0 ${x.id === a.id ? "font-bold" : ""}`}>{label(x)} · {x.status}<span className="block text-xs text-muted">{klTime(x.detectedAt, true)}</span></button>)}</Card>
          </div>
        </>
      )}
    </Page>
  );
}

type St = "Open" | "Acknowledged" | "Resolved";
export default function AlertEvidence({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const session = useBffSession();
  const toast = useToast();
  const [st, setSt] = useState<St>("Open");
  const [reason, setReason] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [canResolve, setCanResolve] = useState(false);
  const resolve = () => {
    if (!canResolve) return setMsg("FORBIDDEN — you don’t have alert.resolve. Resolution needs a remeasurement below 28°C sustained for 60 seconds.");
    if (!reason.trim()) return setMsg("A resolution reason is required (1–1000 characters).");
    setSt("Resolved"); setMsg(null); toast("Alert resolved after remeasurement");
  };
  if (session?.dataSource === "api") return <ApiAlerts unitId={id} />;
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/technician/units/${id}`} className="font-semibold text-primary">← Unit</Link> › Alert evidence · {id}</div>
      <div className="flex flex-wrap items-center gap-2"><h1 className="text-lg font-bold">alert-temp-a · Bedroom too hot</h1><Badge tone={st === "Resolved" ? "ok" : st === "Acknowledged" ? "primary" : "warn"}>{st}</Badge></div>
      <p className="text-xs text-muted">Policy: temperature ≥ 30°C, recovery ≤ 28°C, duration 60s</p>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <div className="grid-fluid" style={{ ["--min"as string]: "190px" }}>
            {[["MEASURED", "Sensor reading", "30.4°C", "sensor-online-temp · observed 09:12 · origin=measured", "ok"], ["ESTIMATED", "Modeled load", "≈ 29–31°C", "confidence unknown — no numeric probability shown", "unknown"], ["INSPECTION", "On-site check", "Confirmed elevated", "tech-internal-a · 09:20 · origin=inspection", "primary"]].map(([k, t, v, d, tone]) => <div key={k} className="rounded-2xl border border-line bg-surface p-4"><Badge tone={tone as "ok"}>{k}</Badge><div className="mt-2 text-xs text-muted">{t}</div><div className="text-lg font-bold">{v}</div><div className="text-[11px] text-muted">{d}</div></div>)}
          </div>
          <Card title="Resolution" sub="Automatic resolution requires a remeasurement below 28°C sustained for 60 seconds — or a reason below with alert.resolve permission.">
            <Field label="Resolution reason (required, 1–1000 characters)"><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="e.g. Confirmed on-site that airflow is restored after filter cleaning." /></Field>
            {msg && <div className="mt-2"><Banner tone="crit">{msg}</Banner></div>}
            <div className="mt-3 flex flex-wrap items-center gap-2"><Btn disabled={st !== "Open"} onClick={() => { setSt("Acknowledged"); toast("Alert acknowledged"); }}>Acknowledge</Btn><Btn variant="primary" disabled={st === "Resolved"} onClick={resolve}>Resolve alert</Btn><label className="ml-auto flex items-center gap-1.5 text-[11px] text-muted"><input type="checkbox" checked={canResolve} onChange={(e) => setCanResolve(e.target.checked)} /> demo: I have alert.resolve</label></div>
            <p className="mt-2 text-[11px] text-muted">Recurrence after resolution creates a new alertId with previousAlertId linking back to this event.</p>
          </Card>
          <Card title="Alert history"><Timeline items={[{ time: "09:20", title: "On-site check — confirmed elevated", detail: "tech-internal-a · origin=inspection · photo attached" }, { time: "09:14", title: "Notified", detail: "in-app → tech-internal-a, hq-operator, customer contact (simulated)" }, { time: "09:12", title: "Raised", detail: "sensor-online-temp 30.4 °C · ≥ 30 °C for 60 s", tone: "warn" }, { time: "09:05", title: "Modeled load rising", detail: "estimate 29–31 °C · confidence unknown" }]} /></Card>
        </div>
        <Card title="Related" className="self-start"><SummaryList items={[["Job", "job-t07 · Preventive"], ["Unit", "unit-online-rto · customer-a"], ["Policy", "Bedroom too hot · customer-a"], ["Recovery rule", "< 28 °C for 60 s"], ["Previous alert", "2026-09-02 · resolved"], ["You can resolve", "with remeasurement only"]]} /></Card>
      </div>
    </Page>
  );
}
