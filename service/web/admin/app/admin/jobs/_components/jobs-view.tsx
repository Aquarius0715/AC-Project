"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Textarea, cx } from "@ac/web/components/ui";
import { JobStatusBadge, OriginBadge } from "@ac/web/components/JobBits";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { showTime, zonedInstant } from "@ac/web/lib/i18n";
import { costLineOf, extendDefault, fromZonedInput, hqRefusal, longSlot, money, newJobErrors, offerDefaults, returnReason, slotText, tomorrowIn, zonedInput, type CostLine, type PreferredRow } from "@ac/web/lib/adminJobs";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { JobStatus } from "@ac/web/lib/jobs";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { assignInternal, cancelJob, classifyFollowUp, createJob, extendAccess, holdJob, offerToContractor, proposeSlot, resolvePartnerSlot, resumeJob, reviewReport, saveCosts, withdrawProposal } from "../actions";
import type { JobsLive } from "../_lib/load";
import { JobsTabs, ScopeBar } from "./jobs-header";

type Detail = NonNullable<JobsLive["detail"]>;
const TYPES = [{ id: "", label: "All" }, { id: "reactive", label: "Repair" }, { id: "periodic", label: "Periodic" }, { id: "preventive", label: "Preventive" }];
const RANK = ["1st", "2nd", "3rd"];
/** The calendar day after `date` (YYYY-MM-DD), whatever the time zone. */
const nextDay = (date: string) => new Date(Date.parse(`${date}T00:00:00Z`) + 86_400_000).toISOString().slice(0, 10);

/** HQ maintenance jobs (FR-A06, Figma Admin 06-1, 06-11…06-18): the Jobs tab from the Core API (Plans, Contractors and
 * SLA are PlansView, ContractorsView and SlaView). Texts in the user's display language; the times of the first render
 * come formatted from the loader, the dialogs format and read times in the display time zone (IR290, NFR-08). */
export function JobsView({ live }: { live: JobsLive }) {
  const t = useT();
  const patch = useUrlPatch();
  const [creating, setCreating] = useState(!!live.newJob); // opened by ?new=<unitId> (an alert's Request maintenance)
  // The job New job just created: its detail opens step 2 (book) or says it was saved as requested.
  const [fresh, setFresh] = useState<Fresh | null>(() => pendingFresh);
  useEffect(() => { pendingFresh = null; }, []);
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="jobs" counts={live.counts} q={live.q} action={<Btn variant="primary" size="sm" onClick={() => setCreating(true)}>{t("+ New job")}</Btn>} />
      {creating && <NewJobModal live={live} initial={live.newJob ?? undefined} onClose={() => { setCreating(false); if (live.newJob) patch({ new: null, alertId: null }); }} onCreated={(f) => { setCreating(false); setFresh(f); }} />}
      <JobsTab live={live} patch={patch} fresh={fresh} onFresh={() => setFresh(null)} />
    </Page>
  );
}

type Fresh = { id: string; book: boolean };
// New job opened from another tab hands the created job over here (browser module state; never set during rendering).
let pendingFresh: Fresh | null = null;

function JobsTab({ live, patch, fresh, onFresh }: { live: JobsLive; patch: (p: Record<string, string | null>) => void; fresh: Fresh | null; onFresh: () => void }) {
  const t = useT();
  const q = live.q;
  const sel = live.detail;
  const inScope = live.stages.reduce((a, s) => a + s.count, 0);
  return (
    <>
      <ScopeBar scope={live.scope} q={q} text={`${t(inScope === 1 ? "1 job in scope" : "{n} jobs in scope", { n: inScope })}${live.period ? ` · ${t("in the period")}` : ""}`} clear="jobId" />
      <div className="scroll-x"><div className="grid min-w-[980px] grid-cols-10 overflow-hidden rounded-2xl border border-line bg-surface">
        {live.stages.map((s) => <button key={s.id} type="button" aria-pressed={q.stage === s.id} onClick={() => patch({ stage: q.stage === s.id ? null : s.id, jobId: null })} className={cx("border-r border-line px-3 py-2 text-left last:border-r-0 hover:bg-surface2", q.stage === s.id && "bg-primary-soft")}><div className="text-[11px] text-muted">{s.label}</div><div className={cx("text-lg font-bold", s.id === "time_proposed" && s.count > 0 && "text-warn", s.count === 0 && "text-muted")}>{s.count}</div></button>)}
      </div></div>
      <div className="flex flex-wrap items-center gap-2">
        <Select aria-label={t("Type")} className="w-auto" value={q.type ?? ""} onChange={(e) => patch({ type: e.target.value || null })}>{TYPES.map((x) => <option key={x.id} value={x.id}>{t("Type: {value}", { value: t(x.label) })}</option>)}</Select>
        <Select aria-label={t("Origin")} className="w-auto" value={q.origin ?? ""} onChange={(e) => patch({ origin: e.target.value || null })}><option value="">{t("Origin: All")}</option><option value="client_request">{t("Origin: Client request")}</option><option value="periodic_plan">{t("Origin: Periodic plan")}</option></Select>
        <Select aria-label={t("Delivery")} className="w-auto" value={q.delivery ?? ""} onChange={(e) => patch({ delivery: e.target.value || null })}><option value="">{t("Delivery: All")}</option>{live.deliveries.map((d) => <option key={d.id} value={d.id}>{t("Delivery: {name}", { name: d.name })}</option>)}</Select>
        <Select aria-label={t("Assignee")} className="w-auto" value={q.assignee ?? ""} onChange={(e) => patch({ assignee: e.target.value || null })}><option value="">{t("Assignee: All")}</option>{live.assignees.map((a) => <option key={a.id} value={a.id}>{t("Assignee: {name}", { name: a.name })}</option>)}</Select>
        <Select aria-label={t("Sort")} className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "status" ? null : e.target.value })}>{live.sorts.map((s) => <option key={s.id} value={s.id}>{t("Sort: {sort}", { sort: s.text })}</option>)}</Select>
        <label className="flex items-center gap-1.5 rounded-control border border-line bg-surface px-3 py-1.5 text-[13px]"><input type="checkbox" checked={q.overdue === "1"} onChange={(e) => patch({ overdue: e.target.checked ? "1" : null })} /> {t("Overdue only")}</label>
        {live.periodChip && <button type="button" onClick={() => patch({ from: null, to: null, jobId: null })} className="rounded-full bg-primary-soft px-3 py-1 text-xs font-semibold text-primary" aria-label={t("Clear the requested-time period")}>{live.periodChip} ✕</button>}
      </div>
      <div className="split-rev">
        <Card title={t("Jobs · all customers")} sub={live.sorts.find((s) => s.id === live.sort)!.text} className="self-start">
          {live.rows.length === 0 ? <EmptyState title={t("No jobs in this view")}>{t("Change the scope or the filters.")}</EmptyState> : (
            <div className="flex flex-col gap-1.5">{live.rows.map((r) => (
              <ListRow key={r.id} selected={sel && !("missing" in live) ? sel.job.id === r.id : false} href={`/admin/jobs?${new URLSearchParams({ ...Object.fromEntries(Object.entries(q).filter(([, v]) => v)), ...(live.sort !== "status" ? { sort: live.sort } : {}), jobId: r.id })}`}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{r.short} · {r.title}</b><JobStatusBadge s={r.badge.status as JobStatus} /></div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] text-muted"><OriginBadge origin={r.origin === "periodic_plan" ? "plan" : "request"} />{r.customer} · {r.type}{r.badge.overdue && <Badge tone="crit">{t("!! Overdue")}</Badge>}</div>
                  <div className={cx("text-[11px]", r.lineTone === "warn" ? "text-warn" : "text-muted")}>{r.line}</div>
                </div>
              </ListRow>
            ))}</div>
          )}
          {live.total > live.rows.length && <p className="mt-2 text-[11px] text-muted">{t("Showing {n} of {total}.", { n: live.rows.length, total: live.total })}</p>}
        </Card>
        {sel ? <JobDetail key={sel.job.id} d={sel} live={live} fresh={fresh?.id === sel.job.id ? fresh : null} onFresh={onFresh} /> : <Card title={t("Job")}>{"missing" in live ? <p className="text-[13px] text-muted">{t("That job no longer exists in your scope.")}</p> : <p className="text-[13px] text-muted">{t("Pick a job.")}</p>}</Card>}
      </div>
    </>
  );
}

type ModalState = null | { kind: "book"; row: PreferredRow; fresh?: boolean } | { kind: "propose" } | { kind: "reason"; what: "hold" | "resume" | "cancel" | "reassign" } | { kind: "classify" }
  | { kind: "review"; decision: "accept" | "return" } | { kind: "cost" } | { kind: "extend" } | { kind: "report" };

function JobDetail({ d, live, fresh, onFresh }: { d: Detail; live: JobsLive; fresh: Fresh | null; onFresh: () => void }) {
  const t = useT();
  const j = d.job;
  const now = Date.parse(live.now);
  // Right after New job: “Next: choose delivery” opens step 2 on the 1st preferred time still ahead; “Save as requested” says so.
  const [modal, setModal] = useState<ModalState>(() => {
    const row = fresh?.book ? d.preferred.find((r) => Date.parse(r.slot.startAt) > now) : undefined;
    return row ? { kind: "book", row, fresh: true } : null;
  });
  const [pending, run] = useAction();
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(() => (fresh && !fresh.book ? { tone: "ok", text: t("Job created as requested — book one of the preferred times or propose another.") } : null));
  useEffect(() => {
    if (fresh) onFresh();
  }, [fresh, onFresh]);
  const failed = (f: ActionFailure) => setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: hqRefusal(f, t) });
  const close = (text?: string) => { setModal(null); if (text) setResult({ tone: "ok", text }); };
  const sp = j.slotProposal, pp = j.partnerSlotProposal;
  const c = d.controls;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      <Card title={<span className="flex flex-wrap items-center gap-2">{j.id.slice(0, 8)} · {d.title} <JobStatusBadge s={(sp?.status === "pending" ? "time_proposed" : j.status) as JobStatus} /><OriginBadge origin={j.origin === "periodic_plan" ? "plan" : "request"} /></span>}
        sub={`${typeLabel(j.type, t)} · ${d.customer}${d.location ? ` · ${d.location}` : ""}${j.planId ? ` · ${t("plan occurrence {date}", { date: d.texts.occurrence ?? "" })}` : ""}`}
        action={<div className="flex gap-2">
          {c.hold && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "hold" })}>{t("Hold…")}</Btn>}
          {c.resume && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "resume" })}>{t("Resume…")}</Btn>}
          <Btn size="sm" variant="danger" disabled={pending || !c.cancel} title={c.cancelWhy ?? undefined} onClick={() => setModal({ kind: "reason", what: "cancel" })}>{t("Cancel…")}</Btn>
        </div>}>
        <ol className="flex flex-wrap gap-4">{d.steps.map((s, i) => <li key={s.label} className="flex items-center gap-1.5 text-xs"><span className={cx("grid h-5 w-5 place-items-center rounded-full text-[10px] font-bold", s.state === "done" ? "bg-primary text-white" : s.state === "current" ? "border-2 border-primary text-primary" : "border border-line text-muted")}>{s.state === "done" ? "✓" : i + 1}</span><span className={s.state === "todo" ? "text-muted" : s.state === "current" ? "font-semibold text-primary" : ""}>{s.label}</span></li>)}</ol>
        {!c.cancel && c.cancelWhy && <p className="mt-2 text-[11px] text-muted">{j.contractorOrgId ? "" : `${t("Internal delivery skips Offered / Accepted.")} `}{t("Cancel: {why}", { why: c.cancelWhy })}</p>}
        <div className="mt-3 grid-fluid" style={{ ["--min" as string]: "180px" }}>
          {d.facts.map((f) => <div key={f.label} className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">{f.label}</div><div className="text-[13px] font-semibold">{f.value}</div><div className="text-[11px] text-muted">{f.sub}</div></div>)}
        </div>
      </Card>

      {j.followUpOfJobId && (
        <Card tone="warn" title={j.followUpClass === "pending" ? t("↩ Follow-up request — classify within 1 business day") : t("↩ Follow-up of {id}", { id: j.followUpOfJobId.slice(0, 8) })} action={j.followUpClass === "pending" ? <Badge tone="warn">{t("Classify by {time}", { time: d.texts.classifyBy })}</Badge> : <Badge tone={j.followUpClass === "rework" ? "ok" : "primary"}>{t(j.followUpClass === "rework" ? "Rework (free)" : "New request")}</Badge>}>
          <SummaryList items={[[t("Client’s report"), j.symptom || "—"], [t("Original job"), <Link key="o" className="text-primary" href={`/admin/jobs?jobId=${j.followUpOfJobId}`}>{j.followUpOfJobId.slice(0, 8)} →</Link>]]} />
          {j.followUpClass === "pending" && <div className="mt-3 flex items-center justify-between gap-2"><span className="text-[11px] text-muted">{t("Set once with a reason. Until then the client sees “Under HQ review”.")}</span><Btn variant="primary" disabled={pending} onClick={() => setModal({ kind: "classify" })}>{t("Classify…")}</Btn></div>}
        </Card>
      )}

      {j.status === "requested" && sp?.status !== "pending" && j.followUpClass !== "pending" && (
        <>
          {sp?.status === "declined" && <Banner tone="crit" icon="✕"><b>{t("The client declined the proposed time")}{d.texts.declinedAt ? ` · ${d.texts.declinedAt}` : ""}</b> — {sp.declineReason?.replace(/_/g, " ")}{sp.declineComment ? `: “${sp.declineComment}”` : ""}. {t("Held capacity was released; the client sent new preferred times (round {round}).", { round: j.preferenceRound })}</Banner>}
          {d.preferred.length > 0 ? (
            <Card title={d.preferred[0].plan ? t("Plan occurrence — book this time") : j.preferenceRound > 1 ? t("Client’s preferred times (round {round}) — book one of these", { round: j.preferenceRound }) : t("Client’s preferred times — book one of these")} sub={t("HQ technicians free and qualified for each time (members.eligible)")}>
              <div className="scroll-x"><table className="w-full min-w-[640px] text-[13px]">
                <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">{t("Rank")}</th><th>{t("Time")}</th><th>{t("HQ technicians")}</th><th>{t("Contractors")}</th><th>{t("Action")}</th></tr></thead>
                <tbody>{d.preferred.map((r) => <tr key={r.rank} className="border-t border-line"><td className="py-2"><span className="rounded-lg bg-primary-soft px-2 py-0.5 text-xs font-semibold text-primary">{r.plan ? t("Plan") : t(RANK[r.rank - 1] ?? "{n}th", { n: r.rank })}</span></td><td className="font-semibold">{r.text}</td><td className={cx("text-xs", r.fits ? "text-ok" : "text-muted")}>{r.hq}</td><td className="text-xs text-muted">{t("{names} — confirm in the offer", { names: live.contractors.filter((x) => x.status === "active").map((x) => x.name).join(", ") || "—" })}</td><td><Btn size="sm" variant="primary" disabled={pending || Date.parse(r.slot.startAt) <= now} onClick={() => setModal({ kind: "book", row: r })}>{t("Use this time")}</Btn></td></tr>)}</tbody>
              </table></div>
              <p className="mt-2 text-[11px] text-muted">{t(d.preferred[0].plan ? "“Use this time” opens Assign internally / Offer to contractor with that time locked. HQ cannot book a time outside the plan occurrence without the client’s approval (IR113)." : "“Use this time” opens Assign internally / Offer to contractor with that time locked. HQ cannot book a time outside these without the client’s approval (IR113).")}</p>
            </Card>
          ) : <Banner>{t("No agreed time on this job (HQ request without alternatives) — propose a time to the client.")}</Banner>}
          <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-[#f5c473] bg-[#fff8ec] p-4">
            <div className="min-w-0 flex-1"><b className="text-warn">{t(d.preferred.some((r) => r.fits) ? "Prefer another time?" : "None of the times works — propose another time")}</b><p className="text-xs">{t("One option with the capacity held; nothing is booked until the client accepts.")}</p></div>
            <Btn variant={d.preferred.some((r) => r.fits) ? "secondary" : "primary"} disabled={pending} onClick={() => setModal({ kind: "propose" })}>{t("Propose another time…")}</Btn>
          </div>
        </>
      )}

      {sp?.status === "pending" && (
        <Card tone="warn" title={t(sp.source === "contractor" ? "Partner’s time sent to the client — waiting" : "Proposal sent — waiting for the client")} action={<Badge tone="warn">{t("Reply by {time}", { time: d.texts.replyBy ?? "—" })}</Badge>}>
          <SummaryList items={[[t("Proposed time"), d.texts.proposed ?? "—"], [t("Held capacity"), t("{who} — booked automatically when the client accepts", { who: d.holdWho ?? "—" })], [t("Message"), `“${sp.message}”`]]} />
          {sp.source === "hq" && <div className="mt-3 flex justify-end"><Btn variant="danger" disabled={pending} onClick={() => run(() => withdrawProposal(j.id, j.version, sp.id), t("Proposal withdrawn"), () => close(t("Proposal withdrawn — the job is back to its preferred times.")), failed)}>{t("Withdraw proposal")}</Btn></div>}
          <p className="mt-2 text-[11px] text-muted">{t("Accept → booked with the held capacity. Decline → back to Requested with the client’s reason and new times. No reply by the deadline → the proposal expires.")}</p>
        </Card>
      )}

      {d.delivery && (
        <Card title={d.delivery.title} action={d.extend ? <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "extend" })}>{t("Extend access…")}</Btn> : undefined}>
          <SummaryList items={d.delivery.lines} />
          {d.delivery.ack && <div className="mt-3"><Banner tone={d.delivery.ack.tone === "ok" ? "ok" : d.delivery.ack.tone === "warn" ? "warn" : undefined} action={d.delivery.cantMake ? <span className="flex gap-2">{d.delivery.kind === "internal" && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "reassign" })}>{t("Reassign…")}</Btn>}</span> : undefined}>{d.delivery.ack.text}</Banner></div>}
        </Card>
      )}

      {pp?.status === "pending" && j.offer && (
        <Card tone="warn" title={t("⇄ {name} proposes a different time", { name: live.contractors.find((x) => x.id === j.offer!.contractorOrgId)?.name ?? t("The contractor") })} action={<Badge tone="warn">{t("Needs the client’s approval")}</Badge>}>
          <SummaryList items={[[t("Agreed with the client"), <s key="s" className="text-muted">{d.texts.agreed}</s>], [t("Proposed by the contractor"), <b key="b">{d.texts.partnerSlot} · {d.partnerTech}</b>], [t("Reason"), `“${pp.reason}” — ${d.texts.partnerSent}`]]} />
          <div className="mt-3 flex flex-wrap justify-end gap-2">
            <Btn disabled={pending} onClick={() => run(() => resolvePartnerSlot(j.id, j.version, pp.id, "keep"), t("Agreed time kept"), () => close(t("Kept the agreed time — the contractor can accept or decline it.")), failed)}>{t("Keep the agreed time")}</Btn>
            <Btn variant="primary" disabled={pending} onClick={() => run(() => resolvePartnerSlot(j.id, j.version, pp.id, "send_to_client", new Date(Math.min(now + 48 * 3600_000, Date.parse(pp.slot.startAt))).toISOString()), t("Sent to the client"), () => close(t("Sent to the client for approval (reply within 48 h).")), failed)}>{t("Send to client for approval")}</Btn>
          </div>
          <p className="mt-2 text-[11px] text-muted">{t("An agreed time never changes without the client’s OK; the offer stays reserved for the contractor while the client decides.")}</p>
        </Card>
      )}

      {d.report ? (
        <Card title={d.report.title} sub={d.report.sub} tone={d.report.tone === "warn" ? "warn" : d.report.tone === "ok" ? "ok" : undefined}
          action={<button type="button" className="text-xs font-semibold text-primary hover:underline" onClick={() => setModal({ kind: "report" })}>{t("Open report →")}</button>}>
          <ReportRows rows={d.report.rows} />
          {d.report.readings.length > 0 && <div className="mt-2"><SummaryList items={d.report.readings} /></div>}
          {d.report.photos.length > 0 && <div className="mt-2 flex flex-wrap gap-2">{d.report.photos.map((p) => (
            // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
            <img key={p.id} src={p.url} alt={p.name} className="h-20 w-28 rounded-lg bg-surface2 object-cover" />
          ))}</div>}
          {d.report.workText && <p className="mt-2 text-xs text-muted">{t("Work: {text}", { text: d.report.workText })}</p>}
          {d.report.reviews.length > 0 && <ul className="mt-2 flex flex-col gap-0.5 text-xs">{d.report.reviews.map((r) => <li key={r.text} className={r.tone === "ok" ? "text-ok" : "text-warn"}>{r.text}</li>)}</ul>}
          {d.draft && <p className="mt-2 text-xs text-muted">{t("A new version is being drafted by the technician.")}</p>}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <span className="min-w-0 flex-1 text-[11px] text-muted">{d.report.note}</span>
            {d.report.decide && <span className="flex gap-2">
              <Btn disabled={pending} onClick={() => setModal({ kind: "review", decision: "return" })}>{t("Return for rework…")}</Btn>
              <Btn variant="primary" disabled={pending} onClick={() => d.report!.mode === "normal" ? run(() => reviewReport(j.id, j.version, d.report!.version, "accept", "normal", ""), t("Report accepted"), () => close(t("Report accepted — the job is completed. Linked alerts stay open until they are resolved.")), failed) : setModal({ kind: "review", decision: "accept" })}>{t(d.report.mode === "normal" ? "Accept report" : "Accept report…")}</Btn>
            </span>}
          </div>
        </Card>
      ) : d.draft ? <Card title={t("Work report")}><p className="text-[13px] text-muted">{t("The technician is drafting the report — nothing submitted yet.")}</p></Card> : null}

      <Card title={t("Costs")} action={<Btn size="sm" disabled={pending || j.status === "cancelled"} title={j.status === "cancelled" ? t("A cancelled job’s costs cannot change.") : undefined} onClick={() => setModal({ kind: "cost" })}>{t("+ Add cost line")}</Btn>}>
        {d.costs.length === 0 ? <p className="text-[13px] text-muted">{t("No cost lines yet — add estimates before the work and actuals after it.")}</p> : (
          <div className="scroll-x"><table className="w-full min-w-[560px] text-[13px]">
            <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">{t("Kind")}</th><th>{t("Description")}</th><th>{t("Visibility")}</th><th className="text-right">{t("Amount")}</th><th className="w-8"><span className="sr-only">{t("Remove")}</span></th></tr></thead>
            <tbody>{d.costs.map((l, i) => <tr key={i} className="border-t border-line"><td className="py-1.5">{t(l.kind === "estimate" ? "Estimate" : "Actual")}</td><td>{l.description}</td><td className="text-xs text-muted">{t(l.visibility === "customer" ? "Customer" : "Internal")}</td><td className="text-right font-semibold">{money(l.amountMinor, l.currency)}</td>
              <td className="text-right"><button type="button" aria-label={t("Remove {name}", { name: l.description })} disabled={pending || j.status === "cancelled"} className="rounded px-1.5 text-muted hover:bg-surface2 hover:text-crit disabled:opacity-40" onClick={() => run(() => saveCosts(j.id, j.version, d.costs.filter((_, k) => k !== i)), t("Cost line removed"), () => close(t("Removed “{name}”.", { name: l.description })), failed)}>×</button></td></tr>)}</tbody>
          </table></div>
        )}
        <div className="mt-3 grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <div className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">{t("Estimate total")}</div><div className="text-[13px] font-semibold">{d.totals.estimate}</div></div>
          <div className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">{t("Actual total")}</div><div className="text-[13px] font-semibold">{d.totals.actual}</div></div>
        </div>
        <p className="mt-2 text-[11px] text-muted">{t("Totals are shown per currency — never converted. Customer lines are visible to the client; internal lines are not.")}</p>
      </Card>

      <Card title={t("History")}>
        {d.history.length === 0 ? <p className="text-[13px] text-muted">{t("No events.")}</p> : <ol className="flex flex-col gap-1 text-xs">{d.history.map((h) => <li key={h.id}><span className="text-muted">{h.at}</span> · {h.text}</li>)}</ol>}
      </Card>

      {modal?.kind === "book" && <BookModal d={d} live={live} row={modal.row} fresh={modal.fresh} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "review" && d.report && <ReviewModal d={d} report={d.report} decision={modal.decision} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "report" && d.report && <ReportModal report={d.report} draft={d.draft} onClose={() => setModal(null)} />}
      {modal?.kind === "cost" && <CostModal d={d} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "extend" && d.extend && <ExtendModal d={d} live={live} until={d.extend.until} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "propose" && <ProposeModal d={d} live={live} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "reason" && <ReasonModal d={d} live={live} what={modal.what} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "classify" && <ClassifyModal d={d} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
    </div>
  );
}

type ModalProps = { d: Detail; live: JobsLive; onClose: (text?: string) => void; onFail: (f: ActionFailure) => void };

function BookModal({ d, live, row: first, fresh, onClose, onFail }: ModalProps & { row: PreferredRow; fresh?: boolean }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const ahead = d.preferred.filter((r) => Date.parse(r.slot.startAt) > now);
  const [rank, setRank] = useState(first.rank);
  const row = ahead.find((r) => r.rank === rank) ?? first;
  const [who, setWho] = useState<"internal" | "contractor">(row.people.length ? "internal" : "contractor");
  const [tech, setTech] = useState(row.people[0]?.id ?? "");
  const active = live.contractors.filter((c) => c.status === "active");
  const [org, setOrg] = useState(active[0]?.id ?? "");
  const def = offerDefaults(row.slot, now);
  const [expires, setExpires] = useState(zonedInput(def.offerExpiresAt, zone));
  const [from, setFrom] = useState(zonedInput(def.accessValidFrom, zone));
  const [until, setUntil] = useState(zonedInput(def.accessValidUntil, zone));
  const pick = (r: number) => {
    const next = ahead.find((x) => x.rank === r)!;
    const nd = offerDefaults(next.slot, now);
    setRank(r);
    setTech(next.people[0]?.id ?? "");
    if (!next.people.length) setWho("contractor");
    setExpires(zonedInput(nd.offerExpiresAt, zone));
    setFrom(zonedInput(nd.accessValidFrom, zone));
    setUntil(zonedInput(nd.accessValidUntil, zone));
  };
  const [terms, setTerms] = useState("terms-demo-v1");
  const book = () => who === "internal"
    ? run(() => assignInternal(j.id, j.version, tech, row.slot), t("Assigned — the technician must accept"), () => onClose(t("Assigned {name} for {slot}. The technician must accept (受領).", { name: row.people.find((p) => p.id === tech)?.displayName ?? "", slot: slotText(row.slot, i) })), onFail)
    : run(() => offerToContractor(j.id, j.version, { contractorOrgId: org, visitSlot: row.slot, offerExpiresAt: fromZonedInput(expires, zone), accessValidFrom: fromZonedInput(from, zone), accessValidUntil: fromZonedInput(until, zone), termsVersion: terms.trim() }), t("Offer sent"), () => onClose(t("Offered {slot} to {name}. They accept, decline or propose another time — a different time goes back to the client.", { slot: slotText(row.slot, i), name: active.find((c) => c.id === org)?.name ?? "" })), onFail);
  return (
    <Modal open onClose={() => onClose()} title={fresh ? t("New maintenance job · step 2 of 2") : t("Book {id}", { id: j.id.slice(0, 8) })} footer={<><Btn onClick={() => onClose()}>{t(fresh ? "Leave it requested" : "← Back")}</Btn><Btn variant="primary" disabled={pending || (who === "internal" ? !tech : !org || !terms.trim())} onClick={book}>{t(who === "internal" ? "Assign" : "Send offer")}</Btn></>}>
      <p className="text-xs text-muted">{fresh ? `${t("Created {id} (requested) · choose who does the work —", { id: j.id.slice(0, 8) })} ` : ""}{d.title} · {d.customer} · {t(j.origin === "periodic_plan" ? "Periodic plan" : "Client request")}</p>
      {fresh && ahead.length > 1 ? (
        <Field label={t("Visit time · one of the customer’s preferred times")} hint={t("Only these times are agreed (IR113); another time needs a proposal the customer accepts.")}><Select value={String(rank)} onChange={(e) => pick(Number(e.target.value))}>{ahead.map((r) => <option key={r.rank} value={r.rank}>{t(RANK[r.rank - 1] ?? "{n}th", { n: r.rank })} · {r.text}{r.fits ? "" : ` · ${t("no HQ technician free")}`}</option>)}</Select></Field>
      ) : <Field label={t(row.plan ? "Visit time · the plan occurrence" : "Visit time · from the client")} hint={t("Fixed here. Need another time? Use “Propose another time…” — the client must accept it.")}><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {longSlot(row.slot, i)}</div></Field>}
      <Field label={t("Who does the work")} hint={row.people.length ? undefined : t("No qualified HQ technician is free at this time — offer it to a contractor.")}><Choice value={who} onChange={setWho} options={[...(row.people.length ? [{ id: "internal" as const, label: t("Assign internally") }] : []), { id: "contractor" as const, label: t("Offer to contractor") }]} /></Field>
      {who === "internal" ? (
        <Field label={t("Technician")} hint={t("Free and qualified for the whole slot (members.eligible)")}><Select value={tech} onChange={(e) => setTech(e.target.value)}>{row.people.map((p) => <option key={p.id} value={p.id}>{p.displayName}</option>)}</Select></Field>
      ) : (
        <>
          <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
            <Field label={t("Contractor organization")}><Select value={org} onChange={(e) => setOrg(e.target.value)}>{active.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
            <Field label={t("Terms version")}><Input value={terms} onChange={(e) => setTerms(e.target.value)} /></Field>
          </div>
          <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
            <Field label={t("Offer expires at")}><Input type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} /></Field>
            <Field label={t("Access valid from")}><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
            <Field label={t("Access valid until")}><Input type="datetime-local" value={until} onChange={(e) => setUntil(e.target.value)} /></Field>
          </div>
          <p className="text-[11px] text-muted">{t("Times in {zone}", { zone })}</p>
        </>
      )}
      <Banner>{t(who === "internal" ? "The technician must accept the assignment (受領)." : "The access window must cover the visit; the offer may not expire after the visit starts.")}</Banner>
    </Modal>
  );
}

function ProposeModal({ d, live, onClose, onFail }: ModalProps) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const tomorrow = tomorrowIn(now, zone);
  const [date, setDate] = useState(tomorrow);
  const [from, setFrom] = useState("10:00");
  const [to, setTo] = useState("12:00");
  const options = [...live.internalTechs.map((x) => ({ id: `i:${x.id}`, label: t("{name} (HQ)", { name: x.name }) })), ...live.contractors.filter((c) => c.status === "active").map((c) => ({ id: `c:${c.id}`, label: t("{name} (offer)", { name: c.name }) }))];
  const [who, setWho] = useState(options[0]?.id ?? "");
  const [msg, setMsg] = useState("");
  const [reply, setReply] = useState("48");
  const [tried, setTried] = useState(false);
  const slot = { startAt: zonedInstant(date, from, zone), endAt: zonedInstant(date, to, zone) }; // typed in the display time zone (NFR-08)
  const preferred = j.preferredSlots.some((s) => s.startAt === slot.startAt && s.endAt === slot.endAt);
  const send = () => {
    setTried(true);
    if (!msg.trim() || preferred || from >= to || !who) return;
    const hold = who.startsWith("i:") ? { kind: "internal" as const, membershipId: who.slice(2) } : { kind: "contractor" as const, contractorOrgId: who.slice(2), technicianMembershipId: null };
    const replyBy = new Date(Math.min(now + Number(reply) * 3600_000, Date.parse(slot.startAt))).toISOString();
    run(() => proposeSlot(j.id, j.version, slot, hold, msg, replyBy), t("Proposal sent — waiting for the client"), () => onClose(t("Proposed {slot} to the client (reply by {time}).", { slot: slotText(slot, i), time: showTime(replyBy, i.display) })), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={t("Propose another time to the client")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t("Send proposal")}</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.customer} · {d.title} · {t("times in {zone}", { zone })}</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label={t("Date")}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label={t("From")}><Input type="time" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label={t("To")} error={tried && from >= to ? t("The end must be after the start") : undefined}><Input type="time" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
      </div>
      {preferred && <p className="text-xs text-crit">{t("✕ This is one of the client’s own times — book it directly instead.")}</p>}
      <Field label={t("Capacity held for the time")}><Select value={who} onChange={(e) => setWho(e.target.value)}>{options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</Select></Field>
      <Field label={t("Message to the client · required (1–1000)")} error={tried && !msg.trim() ? t("A message is required") : undefined}><Textarea value={msg} maxLength={1000} onChange={(e) => setMsg(e.target.value)} placeholder={t("All qualified technicians are booked on your times. The earliest free slot is Thursday morning.")} /></Field>
      <Field label={t("Reply by")}><Select value={reply} onChange={(e) => setReply(e.target.value)}>{["24", "48", "72"].map((h) => <option key={h} value={h}>{t("{n} h", { n: h })}</option>)}</Select></Field>
      <Banner>{t("The client sees “Time proposed” with Accept / Decline. Accept → booked with the held capacity. Decline → the client sends new times.")}</Banner>
    </Modal>
  );
}

function ReasonModal({ d, live, what, onClose, onFail }: ModalProps & { what: "hold" | "resume" | "cancel" | "reassign" }) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const others = live.internalTechs.filter((x) => x.id !== j.assignment?.technicianMembershipId);
  const [tech, setTech] = useState(others[0]?.id ?? "");
  const title = t({ hold: "Put on hold (IR56)", resume: "Resume the job", cancel: "Cancel {id}", reassign: "Reassign {id}" }[what], { id: j.id.slice(0, 8) });
  const confirm = () => {
    setTried(true);
    if (!r.trim() || r.length > 1000 || (what === "reassign" && !tech)) return;
    const slot = j.assignment ? { startAt: j.assignment.scheduledStart, endAt: j.assignment.scheduledEnd } : j.scheduledSlot!;
    const done = t({ hold: "Job put on hold — scheduling and reports are blocked.", resume: "Job resumed.", cancel: "Job cancelled.", reassign: "Reassigned — the new technician must accept." }[what]);
    run(() => what === "hold" ? holdJob(j.id, j.version, r) : what === "resume" ? resumeJob(j.id, j.version, r) : what === "cancel" ? cancelJob(j.id, j.version, r) : assignInternal(j.id, j.version, tech, slot, r), done, () => onClose(done), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={title} footer={<><Btn onClick={() => onClose()}>{t("Back")}</Btn><Btn variant={what === "cancel" ? "danger" : "primary"} disabled={pending} onClick={confirm}>{t("Confirm")}</Btn></>}>
      {what === "reassign" && <Field label={t("New technician (same agreed time)")}><Select value={tech} onChange={(e) => setTech(e.target.value)}>{others.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}</Select></Field>}
      <Field label={t("Reason · required (1–1000)")} error={tried && !r.trim() ? t("A reason is required") : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} /></Field>
      {what === "cancel" && <Banner tone="warn">{t("Open offers are withdrawn and active assignments revoked. The client is notified.")}</Banner>}
    </Modal>
  );
}

function ClassifyModal({ d, onClose, onFail }: Omit<ModalProps, "live">) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [cls, setCls] = useState<"rework" | "new_request">("rework");
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title={t("Classify follow-up request")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!r.trim()) return; run(() => classifyFollowUp(j.id, j.version, cls, r), t("Classified"), () => onClose(t(cls === "rework" ? "Classified as rework (free) — book the client’s preferred visit." : "Classified as a new request — book it like any client request.")), onFail); }}>{t("Classify")}</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.customer} · {d.title} — {t("follow-up of {id}", { id: j.followUpOfJobId?.slice(0, 8) ?? "" })}</p>
      <Field label={t("Classification · required")}><Choice value={cls} onChange={setCls} options={[{ id: "rework", label: t("Rework (free)") }, { id: "new_request", label: t("New request") }]} /></Field>
      <Field label={t("Reason · required (1–1000)")} error={tried && !r.trim() ? t("A reason is required") : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} /></Field>
      <Banner>{t("Set once — cannot be changed later. The job stays “Requested” and is booked like any client request (IR113).")}</Banner>
    </Modal>
  );
}

/** The inspection results: the items that are not OK first, the OK ones folded into one line. */
function ReportRows({ rows }: { rows: NonNullable<Detail["report"]>["rows"] }) {
  const t = useT();
  const row = (r: (typeof rows)[number]) => <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 py-1.5 text-[13px]"><span>{r.label}{r.reason && <span className="text-xs text-muted"> · {r.reason}</span>}</span><Badge tone={r.tone}>{r.result}</Badge></li>;
  const ok = rows.filter((r) => r.tone === "ok");
  return (
    <>
      <ul className="divide-y divide-line">{rows.filter((r) => r.tone !== "ok").map(row)}</ul>
      {ok.length > 0 && <details className="border-t border-line pt-1.5 text-[13px]"><summary className="cursor-pointer text-muted">{t(ok.length === 1 ? "1 check OK" : "{n} checks OK", { n: ok.length })}</summary><ul className="divide-y divide-line">{ok.map(row)}</ul></details>}
    </>
  );
}

/** “Open report →”: the whole report — every inspection result, readings, photos, parts and refrigerant, the work
 * performed, the next action, the sign-off and the review history. */
function ReportModal({ report: r, draft, onClose }: { report: NonNullable<Detail["report"]>; draft: boolean; onClose: () => void }) {
  const t = useT();
  return (
    <Modal open wide onClose={onClose} title={r.title} footer={<Btn onClick={onClose}>{t("Close")}</Btn>}>
      <p className="text-xs text-muted">{r.sub}</p>
      <ul className="divide-y divide-line">{r.rows.map((x) => <li key={x.id} className="flex flex-wrap items-center justify-between gap-2 py-1.5 text-[13px]"><span>{x.label}{x.reason && <span className="text-xs text-muted"> · {x.reason}</span>}</span><Badge tone={x.tone}>{x.result}</Badge></li>)}</ul>
      <SummaryList items={[...r.readings, ...r.parts, [t("Work performed"), r.workText || "—"], [t("Next action"), r.next], [t("Customer sign-off"), r.signOff]]} />
      {r.photos.length > 0 && <div className="flex flex-wrap gap-2">{r.photos.map((p) => (
        // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
        <img key={p.id} src={p.url} alt={p.name} className="h-32 w-44 rounded-lg bg-surface2 object-cover" />
      ))}</div>}
      {r.reviews.length > 0 && <ul className="flex flex-col gap-0.5 text-xs">{r.reviews.map((x) => <li key={x.text} className={x.tone === "ok" ? "text-ok" : "text-warn"}>{x.text}</li>)}</ul>}
      {draft && <p className="text-xs text-muted">{t("A new version is being drafted by the technician.")}</p>}
    </Modal>
  );
}

function ReviewModal({ d, report, decision, onClose, onFail }: Omit<ModalProps, "live"> & { report: NonNullable<Detail["report"]>; decision: "accept" | "return" }) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [items, setItems] = useState(() => report.rework.filter((x) => x.checked).map((x) => x.label));
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const reason = decision === "return" ? returnReason(items, r, t) : r.trim();
  const error = !r.trim() ? t("A reason is required") : reason.length > 1000 ? t("{n} / 1000 characters with the ticked items", { n: reason.length }) : undefined;
  const confirm = () => {
    setTried(true);
    if (error) return;
    const done = t(decision === "return" ? "Returned v{v} for rework — the technician continues on the same assignment." : "Accepted v{v} — the job is completed.", { v: report.version });
    run(() => reviewReport(j.id, j.version, report.version, decision, report.mode, reason), t(decision === "return" ? "Returned for rework" : "Report accepted"), () => onClose(done), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={t(decision === "return" ? "Return for rework" : "Accept by escalation")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant={decision === "return" ? "danger" : "primary"} disabled={pending} onClick={confirm}>{t(decision === "return" ? "Return for rework" : "Accept report")}</Btn></>}>
      <p className="text-xs text-muted">jobs.review · decision={decision}{report.mode === "hq_escalation" ? ` · ${t("HQ escalation")}` : ""} · {t("report v{v} by {name}", { v: report.version, name: report.author })}</p>
      {decision === "return" && (
        <fieldset className="flex flex-col gap-1.5"><legend className="mb-1 text-[13px] font-semibold">{t("Items that need rework")}</legend>
          {report.rework.map((x) => <label key={x.label} className="flex items-start gap-2 text-[13px]"><input type="checkbox" className="mt-0.5" checked={items.includes(x.label)} onChange={(e) => setItems(e.target.checked ? [...items, x.label] : items.filter((v) => v !== x.label))} />{x.label}</label>)}
        </fieldset>
      )}
      <Field label={t("Reason · required")} hint={t("1–1000 characters · shown to the technician and kept in the review history.")} error={tried ? error : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder={t(decision === "return" ? "Airflow still low after cleaning; re-check the blower and attach an after photo." : "The contractor has not reviewed within the agreed time; HQ accepts on the evidence.")} /></Field>
      <Banner tone={decision === "return" ? "warn" : undefined}>{t(decision === "return" ? "Status submitted → rework_requested. The same assignment continues; the technician submits a new report version. Completion does not resolve linked alerts." : "Status submitted → completed. The customer sees the accepted report; linked alerts stay open until they are resolved.")}</Banner>
    </Modal>
  );
}

function CostModal({ d, onClose, onFail }: Omit<ModalProps, "live">) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [f, setF] = useState({ kind: j.status === "completed" || j.status === "submitted" ? "actual" : "estimate", description: "", visibility: "customer", amount: "", currency: "MYR" });
  const [tried, setTried] = useState(false);
  const { line, error } = costLineOf(f, t);
  const save = () => {
    setTried(true);
    if (!line) return;
    run(() => saveCosts(j.id, j.version, [...d.costs, line]), t("Cost line added"), () => onClose(t(line.kind === "estimate" ? "Added estimate “{name}” · {amount}." : "Added actual “{name}” · {amount}.", { name: line.description, amount: money(line.amountMinor, line.currency) })), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={t("Add cost line")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t("Save")}</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.title} · {d.customer} — {t(d.costs.length === 1 ? "saved with the job’s other 1 line (jobs.saveCost)" : "saved with the job’s other {n} lines (jobs.saveCost)", { n: d.costs.length })}</p>
      <Field label={t("Kind")}><Choice value={f.kind as CostLine["kind"]} onChange={(v) => setF({ ...f, kind: v })} options={[{ id: "estimate", label: t("Estimate") }, { id: "actual", label: t("Actual") }]} /></Field>
      <Field label={t("Description · required (1–200)")}><Input value={f.description} maxLength={200} onChange={(e) => setF({ ...f, description: e.target.value })} placeholder={t("Filter cleaning labour")} /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Amount")} error={tried && error ? error : undefined}><Input inputMode="decimal" value={f.amount} onChange={(e) => setF({ ...f, amount: e.target.value })} placeholder="80.00" /></Field>
        <Field label={t("Currency")}><Select value={f.currency} onChange={(e) => setF({ ...f, currency: e.target.value })}><option value="MYR">MYR</option><option value="USD">USD</option></Select></Field>
      </div>
      <Field label={t("Visibility")} hint={t("Customer lines appear on the client’s job; internal lines stay with HQ.")}><Choice value={f.visibility as CostLine["visibility"]} onChange={(v) => setF({ ...f, visibility: v })} options={[{ id: "customer", label: t("Customer") }, { id: "internal", label: t("Internal") }]} /></Field>
      <Banner>{t("Amounts are kept per currency and never converted. No payment is made (DD-A06).")}</Banner>
    </Modal>
  );
}

function ExtendModal({ d, live, until, onClose, onFail }: ModalProps & { until: string }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [end, setEnd] = useState(zonedInput(extendDefault(new Date(Math.max(Date.parse(until), now)).toISOString()), zone));
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const at = end ? fromZonedInput(end, zone) : "";
  const endError = !at || Date.parse(at) <= Date.parse(until) || Date.parse(at) <= now ? t("The new end must be later than the current end and in the future.") : undefined;
  const save = () => {
    setTried(true);
    if (endError || !r.trim()) return;
    run(() => extendAccess(j.id, j.version, at, r), t("Access extended"), () => onClose(t("Access extended to {time}. The contractor keeps the site details until then.", { time: showTime(at, i.display) })), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={t("Extend contractor access")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t("Extend")}</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.title} · {t(Date.parse(until) <= now ? "access ended {time}" : "access now ends {time}", { time: d.texts.accessUntil ?? showTime(until, i.display) })}</p>
      <Field label={t("New end · required")} hint={t("Times in {zone}", { zone })} error={tried ? endError : undefined}><Input type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} /></Field>
      <Field label={t("Reason · required (1–1000)")} error={tried && !r.trim() ? t("A reason is required") : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder={t("The rework visit runs into the next day.")} /></Field>
      <Banner>{t("Only the end moves; the contractor sees the access instructions until then (IR25).")}</Banner>
    </Modal>
  );
}

const JOB_TYPES = [{ id: "reactive" as const, label: "Repair" }, { id: "preventive" as const, label: "Preventive" }, { id: "periodic" as const, label: "Periodic" }];

/** New job (DD-A06 item 8, Figma 06-1 New job): step 1 creates the job (requested) with the customer's preferred times
 * asked by phone; “Save as requested” stops there, “Next: choose delivery” opens step 2 (the booking dialog) on it. */
export function NewJobModal({ live, initial, onClose, onCreated }: { live: { now: string; units: JobsLive["units"]; q: { unitId?: string } }; initial?: { unitId: string; symptom: string }; onClose: () => void; onCreated?: (f: Fresh) => void }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(initial?.unitId ?? live.q.unitId ?? live.units[0]?.id ?? "");
  const [type, setType] = useState<"reactive" | "preventive" | "periodic">("reactive");
  const [symptom, setSymptom] = useState(initial?.symptom ?? "");
  const tomorrow = tomorrowIn(now, zone);
  const [times, setTimes] = useState([{ date: tomorrow, from: "10:00", to: "12:00" }]);
  const [due, setDue] = useState("");
  const [contact, setContact] = useState("9:00–18:00");
  const [tried, setTried] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  // typed in the display time zone and sent as instants (NFR-08); "from tomorrow" stays the Kuala Lumpur day (IR113)
  const slots = times.map((x) => ({ startAt: zonedInstant(x.date, x.from, zone), endAt: zonedInstant(x.date, x.to, zone) }));
  const errors = newJobErrors({ unitId, symptom, slots }, now, t);
  const dueAt = due ? fromZonedInput(due, zone) : null;
  const dueError = dueAt && Date.parse(dueAt) < Date.parse(slots[0].endAt) ? t("The due time cannot be before the end of the 1st time.") : undefined;
  const set = (n: number, k: "date" | "from" | "to", v: string) => setTimes(times.map((x, m) => (m === n ? { ...x, [k]: v } : x)));
  const save = (book: boolean) => {
    setTried(true);
    setRefusal(null);
    if (Object.keys(errors).length || dueError) return;
    run(() => createJob({ unitId, type, symptom, slots, dueAt, contactWindow: contact }), t(book ? "Job created — choose who does the work" : "Job created (requested)"),
      (v) => {
        if (onCreated) onCreated({ id: v.id, book });
        else { pendingFresh = { id: v.id, book }; onClose(); }
        router.push(`/admin/jobs?jobId=${v.id}`);
      }, (f) => setRefusal(hqRefusal(f, t)));
  };
  const groups = [...new Set(live.units.map((u) => u.customer))];
  return (
    <Modal open wide onClose={onClose} title={t("New maintenance job · step 1 of 2")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn disabled={pending} onClick={() => save(false)}>{t("Save as requested")}</Btn><Btn variant="primary" disabled={pending} onClick={() => save(true)}>{t("Next: choose delivery →")}</Btn></>}>
      {refusal && <Banner tone="crit">{refusal}</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
        <Field label={t("Unit · required")} error={tried ? errors.unitId : undefined}><Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{groups.map((g) => <optgroup key={g} label={g}>{live.units.filter((u) => u.customer === g).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</optgroup>)}</Select></Field>
        <Field label={t("Type")}><Choice value={type} onChange={setType} options={JOB_TYPES.map((x) => ({ id: x.id, label: t(x.label) }))} /></Field>
      </div>
      <Field label={t("Symptom · required (10–2000)")} error={tried ? errors.symptom : undefined}><Textarea value={symptom} maxLength={2000} onChange={(e) => setSymptom(e.target.value)} placeholder={t("Unit shows offline since 08:12; the customer reports no cooling.")} /></Field>
      <div className="flex flex-col gap-2">
        <span className="text-[13px] font-semibold">{t("Customer’s preferred times")} <span className="font-normal text-muted">{t("· asked by phone · the 1st is the requested window, up to two more (each 1–4 h, from tomorrow) · times in {zone}", { zone })}</span></span>
        {times.map((x, n) => (
          <div key={n} className="flex flex-wrap items-end gap-2">
            <span className="mb-2 w-8 text-xs font-semibold text-primary">{t(RANK[n])}</span>
            <Field label={t("Date {n}", { n: n + 1 })} error={tried ? errors[`slot${n}`] : undefined}><Input type="date" value={x.date} onChange={(e) => set(n, "date", e.target.value)} /></Field>
            <Field label={t("From {n}", { n: n + 1 })}><Input type="time" value={x.from} onChange={(e) => set(n, "from", e.target.value)} /></Field>
            <Field label={t("To {n}", { n: n + 1 })}><Input type="time" value={x.to} onChange={(e) => set(n, "to", e.target.value)} /></Field>
            {n > 0 && <Btn size="sm" className="mb-1" onClick={() => setTimes(times.filter((_, m) => m !== n))}>{t("Remove")}</Btn>}
          </div>
        ))}
        {times.length < 3 && <div><Btn size="sm" onClick={() => setTimes([...times, { ...times[times.length - 1], date: nextDay(times[times.length - 1].date) }])}>{t("+ Add another time")}</Btn></div>}
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
        <Field label={t("Due · optional")} hint={t("Defaults to the end of the 1st time (IR38).")} error={tried ? dueError : undefined}><Input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} /></Field>
        <Field label={t("Contact window · optional")} hint={t("When to call — no phone numbers or e-mail addresses (IR64).")}><Input value={contact} maxLength={200} onChange={(e) => setContact(e.target.value)} /></Field>
      </div>
      <Banner>{t("Saving creates the job as requested. Step 2 books one of these times — assign an HQ technician or offer it to a contractor (IR113).")}</Banner>
    </Modal>
  );
}
