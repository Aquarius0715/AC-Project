"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Page, SummaryList, Textarea, Timeline, useToast } from "@ac/web/components/ui";

type St = "Open" | "Acknowledged" | "Resolved";

/** Phase 1A demo view (fixture evidence). */
export function MockAlertEvidence({ id }: { id: string }) {
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
