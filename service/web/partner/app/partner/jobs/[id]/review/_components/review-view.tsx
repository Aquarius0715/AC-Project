"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, cx, EmptyState, Field, Modal, Page, SummaryList, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { resultText, type EvidenceRow, type InspectionRow, type VersionRow } from "@ac/web/lib/partnerReview";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { reviewReport } from "../actions";

export type ReviewLive = {
  job: { id: string; version: number; status: string; type: string; unit: string | null };
  reviewer: string;
  report: null | {
    id: string; version: number; author: string; submittedAt: string | null; items: InspectionRow[]; readings: [string, string][];
    parts: { name: string; quantity: number; source: string; replacesComponentKey: string | null }[]; refrigerant: { refrigerant: string; cylinderId: string; recoveredKg: number; chargedKg: number; leakCheck: string }[];
    signOff: { signerName: string; signedAt: string; absentReason: string | null } | null; workText: string; nextAction: { kind: "none" } | { kind: "follow_up"; date: string; note: string } | null;
    photos: { id: string; name: string; url: string }[]; timeOnSite: string; evidence: EvidenceRow[]; acceptable: boolean; missing: string[]; allowed: boolean; blocked: string | null;
    versions: VersionRow[]; last: { decision: "accept" | "return"; version: number; reason: string | null } | null;
  };
};

const refusal = (f: ActionFailure): string | null =>
  f.code === "CONFLICT" ? "CONFLICT — this report version was already accepted or returned, or the job changed in the meantime; the page shows the latest state."
    : f.code === "FORBIDDEN" ? "FORBIDDEN — a contributor to this report version cannot approve it (IR31), or your company does not hold this delegation."
    : f.code === "NOT_FOUND" ? "NOT_FOUND — the job is outside your company's delegation." : null;
const dot: Record<EvidenceRow["tone"], string> = { ok: "bg-ok", warn: "bg-warn", muted: "bg-line" };

/** Quality review of a submitted report (FR-P05): the technician's evidence, the DD-P05 evidence check, accept or
 * return with a reason; the reviewer never edits the original records and cannot approve their own contribution. */
export function ReviewView({ live }: { live: ReviewLive }) {
  const [pending, run] = useAction();
  const [ret, setRet] = useState(false);
  const [why, setWhy] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const r = live.report;
  const head = <div className="text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › {live.job.id.slice(0, 8)} · Quality review{live.job.unit ? ` · ${live.job.unit}` : ""}</div>;
  if (!r) return <Page>{head}<Card title="No report to review"><EmptyState title={live.job.status === "submitted" ? "Report not readable" : "Nothing submitted yet"}>{live.job.status === "submitted" ? "The submitted report is outside your company's delegation window." : `This job is ${live.job.status.replace(/_/g, " ")}; a review opens once the technician submits the report.`}</EmptyState></Card></Page>;
  const failed = (f: ActionFailure) => { setRefused(refusal(f)); if (f.fieldErrors.reason) setError("A reason is required (1–1000 characters)"); };
  const decide = (decision: "accept" | "return") => {
    if (decision === "return" && (why.trim().length < 1 || why.trim().length > 1000)) return setError("A reason is required (1–1000 characters)");
    setError(null);
    run(() => reviewReport(live.job.id, live.job.version, r.version, decision, decision === "return" ? why.trim() : null),
      (v) => (v.status === "completed" ? "Report accepted — job completed" : "Returned for rework"), () => { setRefused(null); setRet(false); setWhy(""); }, failed);
  };
  const open = live.job.status === "submitted" && r.allowed;
  return (
    <Page>
      {head}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {live.job.status === "completed" && <Banner tone="ok">Accepted → completed (receipt). Report v{r.last?.version ?? r.version} is now visible to the customer. The linked alert stays open until it is resolved by hand.</Banner>}
      {live.job.status === "rework_requested" && <Banner tone="warn">Returned for rework{r.last?.reason ? ` — “${r.last.reason}”` : ""}. {r.author} can resume as v{r.version + 1}.</Banner>}
      {live.job.status === "submitted" && r.blocked && <Banner tone="warn">{r.blocked}</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Submitted report — version ${r.version}`} sub={`${r.author}${r.submittedAt ? ` · submitted ${r.submittedAt.slice(0, 16).replace("T", " ")} UTC` : ""}`}>
            <ul className="divide-y divide-line">{r.items.map((it) => {
              const t = it.result ? resultText[it.result] : { label: "No result", tone: "crit" as const };
              return <li key={it.id} className="py-2 text-[13px]"><div className="flex flex-wrap items-center justify-between gap-2"><span>{it.label}</span><Badge tone={t.tone}>{t.label}</Badge></div>{it.reason && <div className="text-xs text-muted">Reason: {it.reason}</div>}</li>;
            })}</ul>
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Photos</h3>
            {r.photos.length === 0 ? <p className="text-xs text-muted">No photos attached.</p> : (
              <div className="grid-fluid" style={{ ["--min" as string]: "110px" }}>{r.photos.map((p) => (
                // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
                <img key={p.id} src={p.url} alt={p.name} className="aspect-[4/3] w-full rounded-xl bg-surface2 object-cover" />
              ))}</div>
            )}
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Work performed & next action</h3>
            <p className="text-[13px]">{r.workText}</p>
            <p className="mt-1 text-xs text-muted">Next: {r.nextAction?.kind === "follow_up" ? `follow-up ${r.nextAction.date.slice(0, 10)} — ${r.nextAction.note}` : "no follow-up needed"}</p>
            {(r.parts.length > 0 || r.refrigerant.length > 0) && <><h3 className="mt-4 mb-1 text-[13px] font-bold">Parts & refrigerant</h3>
              <SummaryList items={[...r.parts.map((p): [string, string] => [`${p.name} × ${p.quantity}`, `${p.source.replace(/_/g, " ")}${p.replacesComponentKey ? ` · replaces ${p.replacesComponentKey.replace(/_/g, " ")}` : ""}`]),
                ...r.refrigerant.map((x): [string, string] => [`${x.refrigerant} · cylinder ${x.cylinderId}`, `recovered ${x.recoveredKg} kg · charged ${x.chargedKg} kg · leak check ${x.leakCheck.replace(/_/g, " ")}`])]} /></>}
            <h3 className="mt-4 mb-1 text-[13px] font-bold">Readings recorded</h3>
            <SummaryList items={[...r.readings.map(([k, v]): [string, string] => [k, v]), ["Time on site", r.timeOnSite], ["Customer sign-off", r.signOff ? `${r.signOff.signerName} · ${r.signOff.signedAt.slice(0, 16).replace("T", " ")}` : "not signed"]]} />
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Quality review" sub={`Author: ${r.author} · Reviewer: you (must differ)`}>
            <p className="mb-2 rounded-xl bg-primary-soft/40 px-3 py-2 text-xs text-muted">ⓘ A contributor to this report version cannot approve it (IR31). The reviewer never edits the technician’s original records.</p>
            {open ? (
              <div className="flex flex-col gap-2">
                <Btn disabled={pending} onClick={() => { setError(null); setRet(true); }}>Return for rework…</Btn>
                <Btn variant="primary" disabled={pending || !r.acceptable} title={r.acceptable ? undefined : r.missing.join("; ")} onClick={() => decide("accept")}>Accept report</Btn>
              </div>
            ) : <p className="text-[13px] text-muted">{live.job.status === "submitted" ? "You cannot decide on this version." : `Decided — the job is ${live.job.status.replace(/_/g, " ")}.`}</p>}
            <p className="mt-2 text-[11px] text-muted">Completion does not automatically resolve the linked alert.</p>
          </Card>
          <Card title="Evidence check">
            <ul className="flex flex-col gap-1.5 text-[13px]">{r.evidence.map((e) => (
              <li key={e.label} className="flex items-center justify-between gap-2"><span className="flex items-center gap-2 text-muted"><span className={cx("h-2 w-2 rounded-full", dot[e.tone])} />{e.label}</span><b>{e.value}</b></li>
            ))}</ul>
            <p className={cx("mt-2 rounded-xl px-3 py-1.5 text-xs font-semibold", r.acceptable ? "bg-ok-soft text-ok" : "bg-warn-soft text-warn")}>{r.acceptable ? "All required evidence present — acceptable." : `Not acceptable yet: ${r.missing.join("; ")}.`}</p>
          </Card>
          <Card title="Versions & linked alert">
            <ul className="divide-y divide-line">{r.versions.map((v, i) => (
              <li key={i} className="flex items-start justify-between gap-2 py-2 text-[13px]"><div><b className="text-[13px]">{v.title}</b><div className="text-[11px] text-muted">{v.sub}</div></div>{v.badge && <Badge tone={v.badge.tone}>{v.badge.label}</Badge>}</li>
            ))}</ul>
          </Card>
        </div>
      </div>
      <Modal open={ret} onClose={() => setRet(false)} title="Return for rework" footer={<><Btn onClick={() => setRet(false)}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => decide("return")}>Return report</Btn></>}>
        <Field label="Reason (required, 1–1000 characters)" error={error ?? undefined}><Textarea value={why} maxLength={1000} onChange={(e) => setWhy(e.target.value)} placeholder="Missing evidence photo for the indoor unit filter" /></Field>
        <p className="text-[11px] text-muted">The technician sees the reason and resumes the report as a new version; the returned version is kept.</p>
      </Modal>
    </Page>
  );
}
