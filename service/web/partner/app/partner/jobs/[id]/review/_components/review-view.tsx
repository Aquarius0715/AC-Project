"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, cx, EmptyState, Field, Modal, Page, SummaryList, Textarea } from "@ac/web/components/ui";
import { Checkbox } from "@ac/web/components/ui/checkbox";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import type { T } from "@ac/web/lib/i18n";
import { statusWord } from "@ac/web/lib/partnerJobDetail";
import { resultText, returnReason, type EvidenceRow, type InspectionRow, type ReworkItem, type VersionRow } from "@ac/web/lib/partnerReview";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { focusField } from "@ac/web/lib/focusField";
import { reviewReport } from "../actions";
import { Icon } from "@ac/web/components/Icon";

export interface ReviewLive {
  job: { id: string; version: number; status: string; type: string; unit: string | null };
  reviewer: string;
  report: null | {
    id: string; version: number; author: string; submitted: string | null; items: InspectionRow[]; readings: [string, string][]; parts: [string, string][];
    signOff: string | null; workText: string; next: string;
    photos: { id: string; name: string; url: string }[]; timeOnSite: string; evidence: EvidenceRow[]; acceptable: boolean; missing: string[]; allowed: boolean; blocked: string | null;
    versions: VersionRow[]; last: { decision: "accept" | "return"; version: number; reason: string | null } | null; rework: ReworkItem[];
  };
}

const refusal = (f: ActionFailure, t: T): string | null => {
  return f.code === "CONFLICT" ? t("This report version was already accepted or returned, or the job changed in the meantime; the page shows the latest state.")
    : f.code === "FORBIDDEN" ? t("A contributor to this report version cannot approve it, or your company does not hold this delegation.")
    : f.code === "NOT_FOUND" ? t("This job is outside your company’s delegation.") : null;
};
const dot: Record<EvidenceRow["tone"], string> = { ok: "bg-ok", warn: "bg-warn", muted: "bg-line" };

/** Quality review of a submitted report (FR-P05): the technician's evidence, the DD-P05 evidence check, accept or
return with a reason; the reviewer never edits the original records and cannot approve their own contribution.
Texts in the user's display language; the times come formatted from the loader (IR274). */
export function ReviewView({ live }: Readonly<{ live: ReviewLive }>) {
  const t = useT();
  const [pending, run] = useAction();
  const [isReturned, setReturned] = useState(false);
  const [why, setWhy] = useState("");
  const [ticked, setTicked] = useState<string[] | null>(null); // the items to redo; null until the dialog opens
  const [error, setError] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const r = live.report;
  const status = statusWord(live.job.status, t);
  const head = <div className="text-[13px] text-muted-foreground"><Link href="/partner/jobs" className="font-semibold text-primary"><Icon name="arrowLeft" /> {t("Jobs")}</Link> › {t("{id} · Quality review", { id: live.job.id.slice(0, 8) })}{live.job.unit ? ` · ${live.job.unit}` : ""}</div>;
  if (!r) {
    return <Page>{head}<Card title={t("No report to review")}><EmptyState title={t(live.job.status === "submitted" ? "Report not readable" : "Nothing submitted yet")}>{live.job.status === "submitted" ? t("The submitted report is outside your company's delegation window.") : t("This job is {status}; a review opens once the technician submits the report.", { status })}</EmptyState></Card></Page>;
  }
  const items = ticked ?? r.rework.filter((x) => x.checked).map((x) => x.label);
  // the ticked items lead the reason the technician reads, as on HQ's dialog (IR609)
  const reason = returnReason(items, why, t);
  const reasonError = why.trim() ? (reason.length > 1000 ? t("{n} / 1000 characters with the ticked items", { n: reason.length }) : null) : t("A reason is required (1–1000 characters)");
  // a refused return takes focus to the reason, the one field to fix (IR597, as IR586)
  const badReason = (message = reasonError ?? t("A reason is required (1–1000 characters)")) => { setError(message); focusField("return-reason"); };
  const failed = (f: ActionFailure) => { setRefused(refusal(f, t)); if (f.fieldErrors.reason) badReason(); };
  const decide = (decision: "accept" | "return") => {
    if (decision === "return" && reasonError) { badReason(reasonError); return; }
    setError(null);
    run(() => reviewReport(live.job.id, live.job.version, r.version, decision, decision === "return" ? reason : null),
      (v) => t(v.status === "completed" ? "Report accepted — job completed" : "Returned for rework"), () => { setRefused(null); setReturned(false); setWhy(""); setTicked(null); }, failed);
  };
  const isOpen = live.job.status === "submitted" && r.allowed;
  return (
    <Page>
      {head}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {live.job.status === "completed" && <Banner tone="ok">{t("Accepted → completed (receipt). Report v{version} is now visible to the customer. The linked alert stays open until it is resolved by hand.", { version: r.last?.version ?? r.version })}</Banner>}
      {live.job.status === "rework_requested" && <Banner tone="warn">{r.last?.reason ? t("Returned for rework — “{reason}”. {name} can resume as v{version}.", { reason: r.last.reason, name: r.author, version: r.version + 1 }) : t("Returned for rework. {name} can resume as v{version}.", { name: r.author, version: r.version + 1 })}</Banner>}
      {live.job.status === "submitted" && r.blocked && <Banner tone="warn">{r.blocked}</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Submitted report — version {version}", { version: r.version })} sub={r.submitted ? t("{name} · submitted {time}", { name: r.author, time: r.submitted }) : r.author}>
            <ul className="divide-y divide-line">{r.items.map((it) => {
              const res = it.result ? resultText[it.result] : { label: "No result", tone: "crit" as const };
              return <li key={it.id} className="py-2 text-[13px]"><div className="flex flex-wrap items-center justify-between gap-2"><span>{it.label}</span><Badge tone={res.tone}>{t(res.label)}</Badge></div>{it.reason && <div className="text-xs text-muted-foreground">{t("Reason: {reason}", { reason: it.reason })}</div>}{it.photos.length > 0 && <div className="flex items-center gap-1 text-xs text-muted-foreground"><Icon name="camera" />{t("Linked photos: {names}", { names: it.photos.join(", ") })}</div>}</li>;
            })}</ul>
            <h3 className="mt-4 mb-1 text-[13px] font-bold">{t("Photos")}</h3>
            {r.photos.length === 0 ? <p className="text-xs text-muted-foreground">{t("No photos attached.")}</p> : (
              <div className="grid-fluid" style={{ ["--min" as string]: "110px" }}>{r.photos.map((p) => (
                // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
                <img key={p.id} src={p.url} alt={p.name} className="aspect-[4/3] w-full rounded-xl bg-surface2 object-cover" />
              ))}</div>
            )}
            <h3 className="mt-4 mb-1 text-[13px] font-bold">{t("Work performed & next action")}</h3>
            <p className="text-[13px]">{r.workText}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t("Next: {action}", { action: r.next })}</p>
            {r.parts.length > 0 && <><h3 className="mt-4 mb-1 text-[13px] font-bold">{t("Parts & refrigerant")}</h3><SummaryList items={r.parts} /></>}
            <h3 className="mt-4 mb-1 text-[13px] font-bold">{t("Readings recorded")}</h3>
            <SummaryList items={[...r.readings.map(([k, v]): [string, string] => [k, v]), [t("Time on site"), r.timeOnSite], [t("Customer sign-off"), r.signOff ?? t("not signed")]]} />
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Quality review")} sub={t("Author: {name} · Reviewer: you (must differ)", { name: r.author })}>
            <p className="mb-2 flex items-start gap-1.5 rounded-xl bg-primary-soft/40 px-3 py-2 text-xs text-muted-foreground"><Icon name="info" className="mt-px" />{t("A contributor to this report version cannot approve it. The reviewer never edits the technician’s original records.")}</p>
            {isOpen ? (
              <div className="flex flex-col gap-2">
                <Btn disabled={pending} onClick={() => { setError(null); setReturned(true); }}>{t("Return for rework…")}</Btn>
                <Btn variant="primary" disabled={pending || !r.acceptable} title={r.acceptable ? undefined : r.missing.join("; ")} onClick={() => { decide("accept"); }}>{t("Accept report")}</Btn>
              </div>
            ) : <p className="text-[13px] text-muted-foreground">{live.job.status === "submitted" ? t("You cannot decide on this version.") : t("Decided — the job is {status}.", { status })}</p>}
            <p className="mt-2 text-[11px] text-muted-foreground">{t("Completion does not automatically resolve the linked alert.")}</p>
          </Card>
          <Card title={t("Evidence check")}>
            <ul className="flex flex-col gap-1.5 text-[13px]">{r.evidence.map((e) => {
              return (
                <li key={e.label} className="flex items-center justify-between gap-2"><span className="flex items-center gap-2 text-muted-foreground"><span className={cx("h-2 w-2 rounded-full", dot[e.tone])} />{e.label}</span><b>{e.value}</b></li>
              );
            })}</ul>
            <p className={cx("mt-2 rounded-xl px-3 py-1.5 text-xs font-semibold", r.acceptable ? "bg-ok-soft text-ok" : "bg-warn-soft text-warn")}>{r.acceptable ? t("All required evidence present — acceptable.") : t("Not acceptable yet: {missing}.", { missing: r.missing.join("; ") })}</p>
          </Card>
          <Card title={t("Versions & linked alert")}>
            <ul className="divide-y divide-line">{r.versions.map((v, i) => {
              return (
                <li key={i} className="flex items-start justify-between gap-2 py-2 text-[13px]"><div><b className="text-[13px]">{v.title}</b><div className="text-[11px] text-muted-foreground">{v.sub}</div></div>{v.badge && <Badge tone={v.badge.tone}>{v.badge.label}</Badge>}</li>
              );
            })}</ul>
          </Card>
        </div>
      </div>
      <Modal open={isReturned} onClose={() => { setReturned(false); }} title={t("Return for rework")} footer={<><Btn onClick={() => { setReturned(false); }}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { decide("return"); }}>{t("Return report")}</Btn></>}>
        <fieldset className="flex flex-col gap-1.5"><legend className="mb-1 text-[13px] font-semibold">{t("Items that need rework")}</legend>
          {r.rework.map((x) => <label key={x.label} className="flex items-start gap-2 text-[13px]"><Checkbox className="mt-0.5" checked={items.includes(x.label)} onCheckedChange={(v) => { setTicked(v ? [...items, x.label] : items.filter((y) => y !== x.label)); }} />{x.label}</label>)}
        </fieldset>
        <Field label={t("Reason (required, 1–1000 characters)")} error={error ?? undefined}><Textarea id="return-reason" value={why} maxLength={1000} onChange={(e) => { setWhy(e.target.value); }} placeholder={t("Missing evidence photo for the indoor unit filter")} /></Field>
        <p className="text-[11px] text-muted-foreground">{t("The technician sees the reason and resumes the report as a new version; the returned version is kept.")}</p>
      </Modal>
    </Page>
  );
}
