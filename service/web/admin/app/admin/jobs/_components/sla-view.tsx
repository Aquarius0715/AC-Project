"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, Kpi, Modal, Page, Select } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { showTime } from "@ac/web/lib/i18n";
import { slaCsv, slaRefusal, targetErrors } from "@ac/web/lib/adminSla";
import { fromZonedInput, zonedInput } from "@ac/web/lib/adminJobs";
import { saveSlaTargets } from "../actions";
import type { SlaLive } from "../_lib/load";
import { JobsTabs } from "./jobs-header";
import { NewJobModal } from "./jobs-view";

/** HQ SLA by customer (FR-A22, DD-A22, Figma Admin 06-10) from the Core API. Texts in the display language; a target's
 * start is typed and shown in the display time zone (IR291). */
export function SlaView({ live }: { live: SlaLive }) {
  const i = useI18n(), { t } = i;
  const patch = useUrlPatch();
  const [modal, setModal] = useState<null | "targets" | "job">(null);
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
  const exportCsv = () => {
    const url = URL.createObjectURL(new Blob([slaCsv(live.rows, live.scorecardPeriod, i)], { type: "text/csv;charset=utf-8" }));
    const a = Object.assign(document.createElement("a"), { href: url, download: `sla-by-customer-${live.period.from.slice(0, 10)}-${live.period.to.slice(0, 10)}.csv` });
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="sla" counts={live.counts} q={{}} action={<Btn variant="primary" size="sm" onClick={() => setModal("job")}>{t("+ New job")}</Btn>} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <Select aria-label={t("Period")} className="w-auto" value={live.period.id} onChange={(e) => patch({ period: e.target.value === "90" ? null : e.target.value })}>{live.periods.map((p) => <option key={p.id} value={p.id}>{t("Period: {label}", { label: p.label })}</option>)}</Select>
        <Select aria-label={t("Contractor")} className="w-auto" value={live.contractorId} onChange={(e) => patch({ contractorId: e.target.value || null })}><option value="">{t("Contractor: All")}</option>{live.contractors.map((c) => <option key={c.id} value={c.id}>{t("Contractor: {name}", { name: c.name })}</option>)}</Select>
        <Btn size="sm" className="ml-auto" onClick={() => setModal("targets")}>{t("Edit SLA targets")}</Btn>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>{live.tiles.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.sub} tone={k.tone} />)}</div>
      <div className="split">
        <Card title={t("Customers")} action={<Btn size="sm" disabled={!live.rows.length} onClick={exportCsv}>{t("Export CSV")}</Btn>}>
          {live.rows.length === 0 ? <EmptyState title={t("No jobs in this period")}>{t(live.contractorId ? "The scorecard counts jobs created in the period and delivered by the contractor." : "The scorecard counts jobs created in the period.")}</EmptyState> : (
            <div className="scroll-x"><table className="w-full min-w-[760px] text-[13px]">
              <thead><tr className="bg-surface2 text-left text-[11px] uppercase text-muted"><th className="px-2 py-1.5">{t("Customer")}</th><th>{t("Plan")}</th><th>{t("Jobs")}</th><th>{t("Response")}</th><th>{t("Arrival")}</th><th>{t("First-time fix")}</th><th>{t("Rating")}</th><th>{t("Overdue")}</th><th>{t("Status")}</th></tr></thead>
              <tbody>{live.rows.map((r) => (
                <tr key={r.id} className="border-t border-line">
                  <td className="px-2 py-2"><b>{r.name}</b><div className="text-[11px] text-muted">{r.sub}</div></td><td className="text-xs">{r.plan}</td><td>{r.jobs}</td><td>{r.response}</td><td>{r.arrival}</td><td>{r.ftf}</td><td>{r.rating}</td>
                  <td>{r.overdue ? <Badge tone="crit">{r.overdue}</Badge> : "0"}</td><td><Badge tone={r.status.tone}>{r.status.label}</Badge></td>
                </tr>
              ))}</tbody>
            </table></div>
          )}
          <p className="mt-2 text-[11px] text-muted">{t("Response = request → offer accepted (contractor) or first assignment (internal). Arrival = technician check-in inside the scheduled slot. First-time fix = report accepted without rework and no repeat job on the unit within 30 days. Status compares each customer with its plan’s targets; metrics without data show “—”.")}</p>
        </Card>
        <Card title={t("Recent breaches")} sub={t("{n} in the period · newest first", { n: live.breaches.length })}>
          {live.breaches.length === 0 ? <p className="text-[13px] text-muted">{t("No breaches in this period.")}</p> : (
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
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [plan, setPlan] = useState(live.targets[0].plan);
  const cur = live.targets.find((t) => t.plan === plan)!;
  const [f, setF] = useState({ responseHours: String(cur.now.responseHours), arrival: String(cur.now.arrivalInWindowPercent), ftf: String(cur.now.firstTimeFixPercent), from: zonedInput(new Date(now + 5 * 60_000).toISOString(), zone) });
  const [tried, setTried] = useState(false);
  const effectiveFrom = f.from ? fromZonedInput(f.from, zone) || null : null; // typed in the display time zone (NFR-08)
  const errors = targetErrors({ responseHours: f.responseHours, arrival: f.arrival, ftf: f.ftf, effectiveFrom }, now, t);
  const pick = (p: string) => {
    const t = live.targets.find((x) => x.plan === p)!;
    setPlan(t.plan);
    setF({ ...f, responseHours: String(t.now.responseHours), arrival: String(t.now.arrivalInWindowPercent), ftf: String(t.now.firstTimeFixPercent) });
  };
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length) return;
    run(() => saveSlaTargets({ planType: plan, responseHours: Number(f.responseHours), arrivalInWindowPercent: Number(f.arrival), firstTimeFixPercent: Number(f.ftf), effectiveFrom: effectiveFrom! }), t("SLA targets saved"),
      (v) => onClose(t("{plan} targets v{v} saved — jobs created from {time} use them.", { plan: cur.label, v: v.version, time: showTime(effectiveFrom, i.display) })), (r) => onFail(slaRefusal(r, t)));
  };
  return (
    <Modal open wide onClose={() => onClose()} title={t("Edit SLA targets")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t("Save targets")}</Btn></>}>
      <ul className="flex flex-col gap-1 text-xs">{live.targets.map((x) => <li key={x.plan}><b>{x.label}</b> — {x.line}{x.scheduled.map((s) => <span key={s} className="block pl-3 text-muted">{t("next: {target}", { target: s })}</span>)}</li>)}</ul>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        <Field label={t("Plan type")}><Select value={plan} onChange={(e) => pick(e.target.value)}>{live.targets.map((x) => <option key={x.plan} value={x.plan}>{x.label}</option>)}</Select></Field>
        <Field label={t("Effective from")} hint={t("Jobs created from then on · times in {zone}", { zone })} error={tried ? errors.effectiveFrom : undefined}><Input type="datetime-local" value={f.from} onChange={(e) => setF({ ...f, from: e.target.value })} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Response within (hours)")} hint={t("1–168; every job should be answered within it")} error={tried ? errors.responseHours : undefined}><Input inputMode="numeric" value={f.responseHours} onChange={(e) => setF({ ...f, responseHours: e.target.value })} /></Field>
        <Field label={t("Arrival in window (%)")} hint="0–100" error={tried ? errors.arrival : undefined}><Input inputMode="decimal" value={f.arrival} onChange={(e) => setF({ ...f, arrival: e.target.value })} /></Field>
        <Field label={t("First-time fix (%)")} hint="0–100" error={tried ? errors.ftf : undefined}><Input inputMode="decimal" value={f.ftf} onChange={(e) => setF({ ...f, ftf: e.target.value })} /></Field>
      </div>
      <Banner>{t("A saved target never changes — each save is a new version for the plan type. A job keeps the target in effect when it was created; without one the default is 4 h / 90 % / 85 %.")}</Banner>
    </Modal>
  );
}
