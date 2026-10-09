"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, Kpi, Modal, Page, Select } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { slaCsv, slaRefusal, targetErrors } from "@ac/web/lib/adminSla";
import { fromKlInput, klInput } from "@ac/web/lib/adminPlans";
import { saveSlaTargets } from "../actions";
import type { SlaLive } from "../_lib/load";
import { JobsTabs } from "./jobs-header";
import { NewJobModal } from "./jobs-view";

/** HQ SLA by customer (FR-A22, DD-A22, Figma Admin 06-10) from the Core API. */
export function SlaView({ live }: { live: SlaLive }) {
  const patch = useUrlPatch();
  const [modal, setModal] = useState<null | "targets" | "job">(null);
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
  const exportCsv = () => {
    const url = URL.createObjectURL(new Blob([slaCsv(live.rows, live.scorecardPeriod)], { type: "text/csv;charset=utf-8" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: `sla-by-customer-${live.period.from.slice(0, 10)}-${live.period.to.slice(0, 10)}.csv` });
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="sla" counts={live.counts} q={{}} action={<Btn variant="primary" size="sm" onClick={() => setModal("job")}>+ New job</Btn>} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <Select aria-label="Period" className="w-auto" value={live.period.id} onChange={(e) => patch({ period: e.target.value === "90" ? null : e.target.value })}>{live.periods.map((p) => <option key={p.id} value={p.id}>Period: {p.label}</option>)}</Select>
        <Select aria-label="Contractor" className="w-auto" value={live.contractorId} onChange={(e) => patch({ contractorId: e.target.value || null })}><option value="">Contractor: All</option>{live.contractors.map((c) => <option key={c.id} value={c.id}>Contractor: {c.name}</option>)}</Select>
        <Btn size="sm" className="ml-auto" onClick={() => setModal("targets")}>Edit SLA targets</Btn>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>{live.tiles.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.sub} tone={k.tone} />)}</div>
      <div className="split">
        <Card title="Customers" action={<Btn size="sm" disabled={!live.rows.length} onClick={exportCsv}>Export CSV</Btn>}>
          {live.rows.length === 0 ? <EmptyState title="No jobs in this period">The scorecard counts jobs created in the period{live.contractorId ? " and delivered by the contractor" : ""}.</EmptyState> : (
            <div className="scroll-x"><table className="w-full min-w-[760px] text-[13px]">
              <thead><tr className="bg-surface2 text-left text-[11px] uppercase text-muted"><th className="px-2 py-1.5">Customer</th><th>Plan</th><th>Jobs</th><th>Response</th><th>Arrival</th><th>First-time fix</th><th>Rating</th><th>Overdue</th><th>Status</th></tr></thead>
              <tbody>{live.rows.map((r) => (
                <tr key={r.id} className="border-t border-line">
                  <td className="px-2 py-2"><b>{r.name}</b><div className="text-[11px] text-muted">{r.sub}</div></td><td className="text-xs">{r.plan}</td><td>{r.jobs}</td><td>{r.response}</td><td>{r.arrival}</td><td>{r.ftf}</td><td>{r.rating}</td>
                  <td>{r.overdue ? <Badge tone="crit">{r.overdue}</Badge> : "0"}</td><td><Badge tone={r.status.tone}>{r.status.label}</Badge></td>
                </tr>
              ))}</tbody>
            </table></div>
          )}
          <p className="mt-2 text-[11px] text-muted">Response = request → offer accepted (contractor) or first assignment (internal). Arrival = technician check-in inside the scheduled slot. First-time fix = report accepted without rework and no repeat job on the unit within 30 days. Status compares each customer with its plan’s targets; metrics without data show “—”.</p>
        </Card>
        <Card title="Recent breaches" sub={`${live.breaches.length} in the period · newest first`}>
          {live.breaches.length === 0 ? <p className="text-[13px] text-muted">No breaches in this period.</p> : (
            <ul className="divide-y divide-line">{live.breaches.map((b) => (
              <li key={b.key} className="flex items-start justify-between gap-2 py-2">
                <span className="min-w-0"><Link className="text-[13px] font-semibold text-primary hover:underline" href={`/admin/jobs?jobId=${b.jobId}`}>{b.short} →</Link><span className="block text-[11px] text-muted">{b.text}</span></span>
                <Badge tone={b.tone}>{b.kind}</Badge>
              </li>
            ))}</ul>
          )}
        </Card>
      </div>
      {modal === "targets" && <TargetsModal live={live} onClose={(text) => { setModal(null); if (text) setResult({ tone: "ok", text }); }} onFail={(text) => { setModal(null); setResult({ tone: "crit", text }); }} />}
      {modal === "job" && <NewJobModal live={{ now: live.now, units: live.units, q: {} }} onClose={() => setModal(null)} />}
    </Page>
  );
}

function TargetsModal({ live, onClose, onFail }: { live: SlaLive; onClose: (text?: string) => void; onFail: (text: string) => void }) {
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [plan, setPlan] = useState(live.targets[0].plan);
  const cur = live.targets.find((t) => t.plan === plan)!;
  const [f, setF] = useState({ responseHours: String(cur.now.responseHours), arrival: String(cur.now.arrivalInWindowPercent), ftf: String(cur.now.firstTimeFixPercent), from: klInput(new Date(now + 5 * 60_000).toISOString()) });
  const [tried, setTried] = useState(false);
  const effectiveFrom = f.from ? fromKlInput(f.from) : null;
  const errors = targetErrors({ responseHours: f.responseHours, arrival: f.arrival, ftf: f.ftf, effectiveFrom }, now);
  const pick = (p: string) => {
    const t = live.targets.find((x) => x.plan === p)!;
    setPlan(t.plan);
    setF({ ...f, responseHours: String(t.now.responseHours), arrival: String(t.now.arrivalInWindowPercent), ftf: String(t.now.firstTimeFixPercent) });
  };
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length) return;
    run(() => saveSlaTargets({ planType: plan, responseHours: Number(f.responseHours), arrivalInWindowPercent: Number(f.arrival), firstTimeFixPercent: Number(f.ftf), effectiveFrom: effectiveFrom! }), "SLA targets saved",
      (v) => onClose(`${cur.label} targets v${v.version} saved — jobs created from ${f.from.replace("T", " ")} use them.`), (r) => onFail(slaRefusal(r)));
  };
  return (
    <Modal open wide onClose={() => onClose()} title="Edit SLA targets" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Save targets</Btn></>}>
      <ul className="flex flex-col gap-1 text-xs">{live.targets.map((t) => <li key={t.plan}><b>{t.label}</b> — {t.line}{t.scheduled.map((s) => <span key={s} className="block pl-3 text-muted">next: {s}</span>)}</li>)}</ul>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Field label="Plan type"><Select value={plan} onChange={(e) => pick(e.target.value)}>{live.targets.map((t) => <option key={t.plan} value={t.plan}>{t.label}</option>)}</Select></Field>
        <Field label="Effective from" hint="Jobs created from then on" error={tried ? errors.effectiveFrom : undefined}><Input type="datetime-local" value={f.from} onChange={(e) => setF({ ...f, from: e.target.value })} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label="Response within (hours)" hint="1–168; every job should be answered within it" error={tried ? errors.responseHours : undefined}><Input inputMode="numeric" value={f.responseHours} onChange={(e) => setF({ ...f, responseHours: e.target.value })} /></Field>
        <Field label="Arrival in window (%)" hint="0–100" error={tried ? errors.arrival : undefined}><Input inputMode="decimal" value={f.arrival} onChange={(e) => setF({ ...f, arrival: e.target.value })} /></Field>
        <Field label="First-time fix (%)" hint="0–100" error={tried ? errors.ftf : undefined}><Input inputMode="decimal" value={f.ftf} onChange={(e) => setF({ ...f, ftf: e.target.value })} /></Field>
      </div>
      <Banner>A saved target never changes — each save is a new version for the plan type. A job keeps the target in effect when it was created; without one the default is 4 h / 90 % / 85 %.</Banner>
    </Modal>
  );
}
