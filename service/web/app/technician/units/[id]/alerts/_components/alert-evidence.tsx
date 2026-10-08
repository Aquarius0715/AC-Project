"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Page, SeverityBadge, Textarea, Timeline, useToast } from "@/components/ui";
import { klTime } from "@/lib/units";
import { acknowledgeAlert, resolveAlert } from "../actions";

// DATA_SOURCE=api: alerts of this unit (alerts.list unitId), acknowledge / resolve with the alert version (IR87, IR94)
export type ApiAlert = { id: string; version: number; severity: "critical" | "warning" | "normal"; status: "open" | "acknowledged" | "resolved"; type: string; causeCode: string; evidenceKind: string; evidenceText: string; observedAt: string; detectedAt: string; acknowledgedAt: string | null; resolvedAt: string | null; resolutionReason: string | null; previousAlertId: string | null; policyId: string | null };
const errText: Record<string, string> = {
  "errors.assignment_required": "You need an active assignment on this unit to act on its alerts.",
  "errors.assignment_not_started": "Your assignment has not started yet.",
  "errors.assignment_ended": "Your assignment on this unit has ended.",
  "error.versionConflict": "The alert changed — the latest state is shown.",
  "error.forbidden": "You don’t have alert.resolve.",
};
const label = (a: ApiAlert) => (a.causeCode !== "unknown" ? a.causeCode.replace(/_/g, " ") : `${a.type} alert`);

/** Alert evidence in API mode: rows from the Server Component, acknowledge / resolve as Server Actions. */
export function AlertEvidenceView({ unitId, alerts }: { unitId: string; alerts: ApiAlert[] }) {
  const toast = useToast();
  const [sel, setSel] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const a = alerts.find((x) => x.id === sel) ?? alerts.find((x) => x.status !== "resolved") ?? alerts[0];
  const act = async (op: "alerts.acknowledge" | "alerts.resolve") => {
    if (!a) return;
    if (op === "alerts.resolve" && !reason.trim()) return setMsg("A resolution reason is required (1–1000 characters).");
    setBusy(true);
    setMsg(null);
    const res = op === "alerts.resolve" ? await resolveAlert(a.id, a.version, reason.trim()) : await acknowledgeAlert(a.id, a.version);
    setBusy(false);
    if (!res.ok) return setMsg(errText[res.messageKey] ?? `${res.code}: ${res.messageKey}`);
    toast(op === "alerts.resolve" ? "Alert resolved" : "Alert acknowledged");
    setReason("");
  };
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/technician/units/${unitId}`} className="font-semibold text-primary">← Unit</Link> › Alert evidence</div>
      {!a ? <Card title="No alerts on this unit" /> : (
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

