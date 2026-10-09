"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Textarea, cx } from "@ac/web/components/ui";
import { JobStatusBadge, OriginBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klTime } from "@ac/web/lib/devices";
import { classifyBy, costLineOf, extendDefault, hqRefusal, longSlot, money, newJobErrors, offerDefaults, returnReason, slotText, type CostLine, type PreferredRow } from "@ac/web/lib/adminJobs";
import type { JobStatus } from "@ac/web/lib/jobs";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { assignInternal, cancelJob, classifyFollowUp, createJob, extendAccess, holdJob, offerToContractor, proposeSlot, resolvePartnerSlot, resumeJob, reviewReport, saveCosts, withdrawProposal } from "../actions";
import type { JobsLive } from "../_lib/load";
import { Contractors, PlansDemo, Sla } from "./jobs-demo";

type Detail = NonNullable<JobsLive["detail"]>;
const TYPES = [{ id: "", label: "Type: All" }, { id: "reactive", label: "Type: Repair" }, { id: "periodic", label: "Type: Periodic" }, { id: "preventive", label: "Type: Preventive" }];
const klLocal = (iso: string) => klTime(iso).replace(" ", "T");
const fromLocal = (v: string) => new Date(`${v}:00+08:00`).toISOString();

/** HQ maintenance jobs (FR-A06, Figma Admin 06-1, 06-11…06-18): the Jobs tab from the Core API. */
export function JobsView({ live, tab }: { live: JobsLive; tab: string }) {
  const patch = useUrlPatch();
  const [creating, setCreating] = useState(false);
  // The job New job just created: its detail opens step 2 (book) or says it was saved as requested.
  const [fresh, setFresh] = useState<Fresh | null>(null);
  const tabs = [{ id: "jobs", label: "Jobs", count: live.total }, { id: "plans", label: "Plans" }, { id: "contractors", label: "Contractors" }, { id: "sla", label: "SLA by customer" }];
  return (
    <Page className="max-w-[1440px]">
      <div className="flex flex-wrap items-end justify-between gap-2 border-b border-line">
        <div role="tablist" className="flex flex-wrap gap-4">
          {tabs.map((t) => <Link key={t.id} role="tab" aria-selected={tab === t.id} href={t.id === "jobs" ? "/admin/jobs" : `/admin/jobs?tab=${t.id}`} className={cx("-mb-px border-b-2 px-1 pb-2 text-[13px] font-semibold", tab === t.id ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink")}>{t.label}{t.count !== undefined && <span className="ml-1.5 rounded bg-surface2 px-1.5 text-[11px]">{t.count}</span>}</Link>)}
        </div>
        <Btn variant="primary" size="sm" className="mb-1.5" onClick={() => setCreating(true)}>+ New job</Btn>
      </div>
      {creating && <NewJobModal live={live} onClose={() => setCreating(false)} onCreated={(f) => { setCreating(false); setFresh(f); }} />}
      {tab !== "jobs" && <Banner tone="warn">{tabs.find((t) => t.id === tab)?.label} is not connected to the Core API yet — illustrative data (FR-A06 / A21 / A22, next rounds).</Banner>}
      {tab === "plans" && <PlansDemo />}
      {tab === "contractors" && <Contractors />}
      {tab === "sla" && <Sla />}
      {tab === "jobs" && <JobsTab live={live} patch={patch} fresh={fresh} onFresh={() => setFresh(null)} />}
    </Page>
  );
}

type Fresh = { id: string; book: boolean };

function JobsTab({ live, patch, fresh, onFresh }: { live: JobsLive; patch: (p: Record<string, string | null>) => void; fresh: Fresh | null; onFresh: () => void }) {
  const q = live.q;
  const sel = live.detail;
  return (
    <>
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <span className="text-muted">Scope</span>
        <Select aria-label="Customer" className="w-auto" value={q.customerId ?? ""} onChange={(e) => patch({ customerId: e.target.value || null, propertyId: null, unitId: null, jobId: null })}><option value="">Customer: All</option>{live.scope.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select>
        <Select aria-label="Property" className="w-auto" value={q.propertyId ?? ""} onChange={(e) => patch({ propertyId: e.target.value || null, unitId: null, jobId: null })}><option value="">Property: All</option>{live.scope.properties.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select>
        <Select aria-label="Unit" className="w-auto" value={q.unitId ?? ""} onChange={(e) => patch({ unitId: e.target.value || null, jobId: null })}><option value="">Unit: All</option>{live.scope.units.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select>
        <span className="ml-auto text-xs text-muted">{live.stages.reduce((a, s) => a + s.count, 0)} jobs in scope</span>
      </div>
      <div className="scroll-x"><div className="grid min-w-[980px] grid-cols-10 overflow-hidden rounded-2xl border border-line bg-surface">
        {live.stages.map((s) => <button key={s.id} type="button" aria-pressed={q.stage === s.id} onClick={() => patch({ stage: q.stage === s.id ? null : s.id, jobId: null })} className={cx("border-r border-line px-3 py-2 text-left last:border-r-0 hover:bg-surface2", q.stage === s.id && "bg-primary-soft")}><div className="text-[11px] text-muted">{s.label}</div><div className={cx("text-lg font-bold", s.id === "time_proposed" && s.count > 0 && "text-warn", s.count === 0 && "text-muted")}>{s.count}</div></button>)}
      </div></div>
      <div className="flex flex-wrap items-center gap-2">
        <Select aria-label="Type" className="w-auto" value={q.type ?? ""} onChange={(e) => patch({ type: e.target.value || null })}>{TYPES.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}</Select>
        <Select aria-label="Origin" className="w-auto" value={q.origin ?? ""} onChange={(e) => patch({ origin: e.target.value || null })}><option value="">Origin: All</option><option value="client_request">Origin: Client request</option><option value="periodic_plan">Origin: Periodic plan</option></Select>
        <Select aria-label="Delivery" className="w-auto" value={q.delivery ?? ""} onChange={(e) => patch({ delivery: e.target.value || null })}><option value="">Delivery: All</option>{live.deliveries.map((d) => <option key={d.id} value={d.id}>Delivery: {d.name}</option>)}</Select>
        <Select aria-label="Assignee" className="w-auto" value={q.assignee ?? ""} onChange={(e) => patch({ assignee: e.target.value || null })}><option value="">Assignee: All</option>{live.assignees.map((a) => <option key={a.id} value={a.id}>Assignee: {a.name}</option>)}</Select>
        <Select aria-label="Sort" className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "status" ? null : e.target.value })}>{live.sorts.map((s) => <option key={s.id} value={s.id}>Sort: {s.text}</option>)}</Select>
        <label className="flex items-center gap-1.5 rounded-control border border-line bg-surface px-3 py-1.5 text-[13px]"><input type="checkbox" checked={q.overdue === "1"} onChange={(e) => patch({ overdue: e.target.checked ? "1" : null })} /> Overdue only</label>
      </div>
      <div className="split-rev">
        <Card title="Jobs · all customers" sub={live.sorts.find((s) => s.id === live.sort)!.text.toLowerCase()} className="self-start">
          {live.rows.length === 0 ? <EmptyState title="No jobs in this view">Change the scope or the filters.</EmptyState> : (
            <div className="flex flex-col gap-1.5">{live.rows.map((r) => (
              <ListRow key={r.id} selected={sel && !("missing" in live) ? sel.job.id === r.id : false} href={`/admin/jobs?${new URLSearchParams({ ...Object.fromEntries(Object.entries(q).filter(([, v]) => v)), ...(live.sort !== "status" ? { sort: live.sort } : {}), jobId: r.id })}`}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{r.short} · {r.title}</b><JobStatusBadge s={r.badge.status as JobStatus} /></div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] text-muted"><OriginBadge origin={r.origin === "periodic_plan" ? "plan" : "request"} />{r.customer} · {r.type}{r.badge.overdue && <Badge tone="crit">!! Overdue</Badge>}</div>
                  <div className={cx("text-[11px]", r.lineTone === "warn" ? "text-warn" : "text-muted")}>{r.line}</div>
                </div>
              </ListRow>
            ))}</div>
          )}
          {live.total > live.rows.length && <p className="mt-2 text-[11px] text-muted">Showing {live.rows.length} of {live.total}.</p>}
        </Card>
        {sel ? <JobDetail key={sel.job.id} d={sel} live={live} fresh={fresh?.id === sel.job.id ? fresh : null} onFresh={onFresh} /> : <Card title="Job">{"missing" in live ? <p className="text-[13px] text-muted">That job no longer exists in your scope.</p> : <p className="text-[13px] text-muted">Pick a job.</p>}</Card>}
      </div>
    </>
  );
}

type ModalState = null | { kind: "book"; row: PreferredRow; fresh?: boolean } | { kind: "propose" } | { kind: "reason"; what: "hold" | "resume" | "cancel" | "reassign" } | { kind: "classify" }
  | { kind: "review"; decision: "accept" | "return" } | { kind: "cost" } | { kind: "extend" } | { kind: "report" };

function JobDetail({ d, live, fresh, onFresh }: { d: Detail; live: JobsLive; fresh: Fresh | null; onFresh: () => void }) {
  const j = d.job;
  const now = Date.parse(live.now);
  // Right after New job: “Next: choose delivery” opens step 2 on the 1st preferred time still ahead; “Save as requested” says so.
  const [modal, setModal] = useState<ModalState>(() => {
    const row = fresh?.book ? d.preferred.find((r) => Date.parse(r.slot.startAt) > now) : undefined;
    return row ? { kind: "book", row, fresh: true } : null;
  });
  const [pending, run] = useAction();
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(() => (fresh && !fresh.book ? { tone: "ok", text: "Job created as requested — book one of the preferred times or propose another." } : null));
  useEffect(() => {
    if (fresh) onFresh();
  }, [fresh, onFresh]);
  const failed = (f: ActionFailure) => setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: hqRefusal(f) });
  const close = (text?: string) => { setModal(null); if (text) setResult({ tone: "ok", text }); };
  const sp = j.slotProposal, pp = j.partnerSlotProposal;
  const c = d.controls;
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      <Card title={<span className="flex flex-wrap items-center gap-2">{j.id.slice(0, 8)} · {d.title} <JobStatusBadge s={(sp?.status === "pending" ? "time_proposed" : j.status) as JobStatus} /><OriginBadge origin={j.origin === "periodic_plan" ? "plan" : "request"} /></span>}
        sub={`${({ periodic: "Periodic", reactive: "Repair", preventive: "Preventive" } as Record<string, string>)[j.type] ?? j.type} · ${d.customer}${d.location ? ` · ${d.location}` : ""}${j.planId ? ` · plan occurrence ${j.occurrenceAt ? klTime(j.occurrenceAt).slice(0, 10) : ""}` : ""}`}
        action={<div className="flex gap-2">
          {c.hold && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "hold" })}>Hold…</Btn>}
          {c.resume && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "resume" })}>Resume…</Btn>}
          <Btn size="sm" variant="danger" disabled={pending || !c.cancel} title={c.cancelWhy ?? undefined} onClick={() => setModal({ kind: "reason", what: "cancel" })}>Cancel…</Btn>
        </div>}>
        <ol className="flex flex-wrap gap-4">{d.steps.map((s, i) => <li key={s.label} className="flex items-center gap-1.5 text-xs"><span className={cx("grid h-5 w-5 place-items-center rounded-full text-[10px] font-bold", s.state === "done" ? "bg-primary text-white" : s.state === "current" ? "border-2 border-primary text-primary" : "border border-line text-muted")}>{s.state === "done" ? "✓" : i + 1}</span><span className={s.state === "todo" ? "text-muted" : s.state === "current" ? "font-semibold text-primary" : ""}>{s.label}</span></li>)}</ol>
        {!c.cancel && c.cancelWhy && <p className="mt-2 text-[11px] text-muted">{j.contractorOrgId ? "" : "Internal delivery skips Offered / Accepted. "}Cancel: {c.cancelWhy}</p>}
        <div className="mt-3 grid-fluid" style={{ ["--min" as string]: "180px" }}>
          {d.facts.map((f) => <div key={f.label} className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">{f.label}</div><div className="text-[13px] font-semibold">{f.value}</div><div className="text-[11px] text-muted">{f.sub}</div></div>)}
        </div>
      </Card>

      {j.followUpOfJobId && (
        <Card tone="warn" title={j.followUpClass === "pending" ? "↩ Follow-up request — classify within 1 business day" : `↩ Follow-up of ${j.followUpOfJobId.slice(0, 8)}`} action={j.followUpClass === "pending" ? <Badge tone="warn">Classify by {classifyBy(j.createdAt)}</Badge> : <Badge tone={j.followUpClass === "rework" ? "ok" : "primary"}>{j.followUpClass === "rework" ? "Rework (free)" : "New request"}</Badge>}>
          <SummaryList items={[["Client’s report", j.symptom || "—"], ["Original job", <Link key="o" className="text-primary" href={`/admin/jobs?jobId=${j.followUpOfJobId}`}>{j.followUpOfJobId.slice(0, 8)} →</Link>]]} />
          {j.followUpClass === "pending" && <div className="mt-3 flex items-center justify-between gap-2"><span className="text-[11px] text-muted">Set once with a reason. Until then the client sees “Under HQ review”.</span><Btn variant="primary" disabled={pending} onClick={() => setModal({ kind: "classify" })}>Classify…</Btn></div>}
        </Card>
      )}

      {j.status === "requested" && sp?.status !== "pending" && j.followUpClass !== "pending" && (
        <>
          {sp?.status === "declined" && <Banner tone="crit" icon="✕"><b>The client declined the proposed time{sp.decidedAt ? ` · ${klTime(sp.decidedAt).slice(5)}` : ""}</b> — {sp.declineReason?.replace(/_/g, " ")}{sp.declineComment ? `: “${sp.declineComment}”` : ""}. Held capacity was released; the client sent new preferred times (round {j.preferenceRound}).</Banner>}
          {d.preferred.length > 0 ? (
            <Card title={`Client’s preferred times${j.preferenceRound > 1 ? ` (round ${j.preferenceRound})` : ""} — book one of these`} sub="HQ technicians free and qualified for each time (members.eligible)">
              <div className="scroll-x"><table className="w-full min-w-[640px] text-[13px]">
                <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">Rank</th><th>Time</th><th>HQ technicians</th><th>Contractors</th><th>Action</th></tr></thead>
                <tbody>{d.preferred.map((r) => <tr key={r.rank} className="border-t border-line"><td className="py-2"><span className="rounded-lg bg-primary-soft px-2 py-0.5 text-xs font-semibold text-primary">{["1st", "2nd", "3rd"][r.rank - 1] ?? `${r.rank}th`}</span></td><td className="font-semibold">{r.text}</td><td className={cx("text-xs", r.fits ? "text-ok" : "text-muted")}>{r.hq}</td><td className="text-xs text-muted">{live.contractors.filter((x) => x.status === "active").map((x) => x.name).join(", ") || "—"} — confirm in the offer</td><td><Btn size="sm" variant="primary" disabled={pending || Date.parse(r.slot.startAt) <= now} onClick={() => setModal({ kind: "book", row: r })}>Use this time</Btn></td></tr>)}</tbody>
              </table></div>
              <p className="mt-2 text-[11px] text-muted">“Use this time” opens Assign internally / Offer to contractor with that time locked. HQ cannot book a time outside these without the client’s approval (IR113).</p>
            </Card>
          ) : <Banner>No preferred times on this job (HQ request without alternatives) — propose a time to the client.</Banner>}
          <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-[#f5c473] bg-[#fff8ec] p-4">
            <div className="min-w-0 flex-1"><b className="text-warn">{d.preferred.some((r) => r.fits) ? "Prefer another time?" : "None of the times works — propose another time"}</b><p className="text-xs">One option with the capacity held; nothing is booked until the client accepts.</p></div>
            <Btn variant={d.preferred.some((r) => r.fits) ? "secondary" : "primary"} disabled={pending} onClick={() => setModal({ kind: "propose" })}>Propose another time…</Btn>
          </div>
        </>
      )}

      {sp?.status === "pending" && (
        <Card tone="warn" title={sp.source === "contractor" ? "Partner’s time sent to the client — waiting" : "Proposal sent — waiting for the client"} action={<Badge tone="warn">Reply by {klTime(sp.replyBy).slice(5)}</Badge>}>
          <SummaryList items={[["Proposed time", longSlot(sp.slot)], ["Held capacity", `${d.holdWho} — booked automatically when the client accepts`], ["Message", `“${sp.message}”`]]} />
          {sp.source === "hq" && <div className="mt-3 flex justify-end"><Btn variant="danger" disabled={pending} onClick={() => run(() => withdrawProposal(j.id, j.version, sp.id), "Proposal withdrawn", () => close("Proposal withdrawn — the job is back to its preferred times."), failed)}>Withdraw proposal</Btn></div>}
          <p className="mt-2 text-[11px] text-muted">Accept → booked with the held capacity. Decline → back to Requested with the client’s reason and new times. No reply by the deadline → the proposal expires.</p>
        </Card>
      )}

      {d.delivery && (
        <Card title={d.delivery.title} action={d.extend ? <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "extend" })}>Extend access…</Btn> : undefined}>
          <SummaryList items={d.delivery.lines} />
          {d.delivery.ack && <div className="mt-3"><Banner tone={d.delivery.ack.tone === "ok" ? "ok" : d.delivery.ack.tone === "warn" ? "warn" : undefined} action={d.delivery.cantMake ? <span className="flex gap-2">{d.delivery.kind === "internal" && <Btn size="sm" disabled={pending} onClick={() => setModal({ kind: "reason", what: "reassign" })}>Reassign…</Btn>}</span> : undefined}>{d.delivery.ack.text}</Banner></div>}
        </Card>
      )}

      {pp?.status === "pending" && j.offer && (
        <Card tone="warn" title={`⇄ ${live.contractors.find((x) => x.id === j.offer!.contractorOrgId)?.name ?? "The contractor"} proposes a different time`} action={<Badge tone="warn">Needs the client’s approval</Badge>}>
          <SummaryList items={[["Agreed with the client", <s key="s" className="text-muted">{slotText(j.offer.visitSlot)}</s>], ["Proposed by the contractor", <b key="b">{longSlot(pp.slot)} · {d.partnerTech}</b>], ["Reason", `“${pp.reason}” — ${klTime(pp.sentAt).slice(5)}`]]} />
          <div className="mt-3 flex flex-wrap justify-end gap-2">
            <Btn disabled={pending} onClick={() => run(() => resolvePartnerSlot(j.id, j.version, pp.id, "keep"), "Agreed time kept", () => close("Kept the agreed time — the contractor can accept or decline it."), failed)}>Keep the agreed time</Btn>
            <Btn variant="primary" disabled={pending} onClick={() => run(() => resolvePartnerSlot(j.id, j.version, pp.id, "send_to_client", new Date(Math.min(now + 48 * 3600_000, Date.parse(pp.slot.startAt))).toISOString()), "Sent to the client", () => close("Sent to the client for approval (reply within 48 h)."), failed)}>Send to client for approval</Btn>
          </div>
          <p className="mt-2 text-[11px] text-muted">An agreed time never changes without the client’s OK; the offer stays reserved for the contractor while the client decides.</p>
        </Card>
      )}

      {d.report ? (
        <Card title={d.report.title} sub={d.report.sub} tone={d.report.tone === "warn" ? "warn" : d.report.tone === "ok" ? "ok" : undefined}
          action={<button type="button" className="text-xs font-semibold text-primary hover:underline" onClick={() => setModal({ kind: "report" })}>Open report →</button>}>
          <ReportRows rows={d.report.rows} />
          {d.report.readings.length > 0 && <div className="mt-2"><SummaryList items={d.report.readings} /></div>}
          {d.report.photos.length > 0 && <div className="mt-2 flex flex-wrap gap-2">{d.report.photos.map((p) => (
            // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
            <img key={p.id} src={p.url} alt={p.name} className="h-20 w-28 rounded-lg bg-surface2 object-cover" />
          ))}</div>}
          {d.report.workText && <p className="mt-2 text-xs text-muted">Work: {d.report.workText}</p>}
          {d.report.reviews.length > 0 && <ul className="mt-2 flex flex-col gap-0.5 text-xs">{d.report.reviews.map((r) => <li key={r.text} className={r.tone === "ok" ? "text-ok" : "text-warn"}>{r.text}</li>)}</ul>}
          {d.draft && <p className="mt-2 text-xs text-muted">A new version is being drafted by the technician.</p>}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <span className="min-w-0 flex-1 text-[11px] text-muted">{d.report.note}</span>
            {d.report.decide && <span className="flex gap-2">
              <Btn disabled={pending} onClick={() => setModal({ kind: "review", decision: "return" })}>Return for rework…</Btn>
              <Btn variant="primary" disabled={pending} onClick={() => d.report!.mode === "normal" ? run(() => reviewReport(j.id, j.version, d.report!.version, "accept", "normal", ""), "Report accepted", () => close("Report accepted — the job is completed. Linked alerts stay open until they are resolved."), failed) : setModal({ kind: "review", decision: "accept" })}>{d.report.mode === "normal" ? "Accept report" : "Accept report…"}</Btn>
            </span>}
          </div>
        </Card>
      ) : d.draft ? <Card title="Work report"><p className="text-[13px] text-muted">The technician is drafting the report — nothing submitted yet.</p></Card> : null}

      <Card title="Costs" action={<Btn size="sm" disabled={pending || j.status === "cancelled"} title={j.status === "cancelled" ? "A cancelled job’s costs cannot change." : undefined} onClick={() => setModal({ kind: "cost" })}>+ Add cost line</Btn>}>
        {d.costs.length === 0 ? <p className="text-[13px] text-muted">No cost lines yet — add estimates before the work and actuals after it.</p> : (
          <div className="scroll-x"><table className="w-full min-w-[560px] text-[13px]">
            <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">Kind</th><th>Description</th><th>Visibility</th><th className="text-right">Amount</th><th className="w-8"><span className="sr-only">Remove</span></th></tr></thead>
            <tbody>{d.costs.map((l, i) => <tr key={i} className="border-t border-line"><td className="py-1.5">{l.kind === "estimate" ? "Estimate" : "Actual"}</td><td>{l.description}</td><td className="text-xs text-muted">{l.visibility}</td><td className="text-right font-semibold">{money(l.amountMinor, l.currency)}</td>
              <td className="text-right"><button type="button" aria-label={`Remove ${l.description}`} disabled={pending || j.status === "cancelled"} className="rounded px-1.5 text-muted hover:bg-surface2 hover:text-crit disabled:opacity-40" onClick={() => run(() => saveCosts(j.id, j.version, d.costs.filter((_, k) => k !== i)), "Cost line removed", () => close(`Removed “${l.description}”.`), failed)}>×</button></td></tr>)}</tbody>
          </table></div>
        )}
        <div className="mt-3 grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <div className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">Estimate total</div><div className="text-[13px] font-semibold">{d.totals.estimate}</div></div>
          <div className="rounded-xl border border-line p-3"><div className="text-[11px] text-muted">Actual total</div><div className="text-[13px] font-semibold">{d.totals.actual}</div></div>
        </div>
        <p className="mt-2 text-[11px] text-muted">Totals are shown per currency — never converted. Customer lines are visible to the client; internal lines are not.</p>
      </Card>

      <Card title="History">
        {d.history.length === 0 ? <p className="text-[13px] text-muted">No events.</p> : <ol className="flex flex-col gap-1 text-xs">{d.history.map((h) => <li key={h.id}><span className="text-muted">{h.at}</span> · {h.text}</li>)}</ol>}
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
  const [expires, setExpires] = useState(klLocal(def.offerExpiresAt));
  const [from, setFrom] = useState(klLocal(def.accessValidFrom));
  const [until, setUntil] = useState(klLocal(def.accessValidUntil));
  const pick = (r: number) => {
    const next = ahead.find((x) => x.rank === r)!;
    const nd = offerDefaults(next.slot, now);
    setRank(r);
    setTech(next.people[0]?.id ?? "");
    if (!next.people.length) setWho("contractor");
    setExpires(klLocal(nd.offerExpiresAt));
    setFrom(klLocal(nd.accessValidFrom));
    setUntil(klLocal(nd.accessValidUntil));
  };
  const [terms, setTerms] = useState("terms-demo-v1");
  const book = () => who === "internal"
    ? run(() => assignInternal(j.id, j.version, tech, row.slot), "Assigned — the technician must accept", () => onClose(`Assigned ${row.people.find((p) => p.id === tech)?.displayName} for ${slotText(row.slot)}. The technician must accept (受領).`), onFail)
    : run(() => offerToContractor(j.id, j.version, { contractorOrgId: org, visitSlot: row.slot, offerExpiresAt: fromLocal(expires), accessValidFrom: fromLocal(from), accessValidUntil: fromLocal(until), termsVersion: terms.trim() }), "Offer sent", () => onClose(`Offered ${slotText(row.slot)} to ${active.find((c) => c.id === org)?.name}. They accept, decline or propose another time — a different time goes back to the client.`), onFail);
  return (
    <Modal open onClose={() => onClose()} title={fresh ? "New maintenance job · step 2 of 2" : `Book ${j.id.slice(0, 8)}`} footer={<><Btn onClick={() => onClose()}>{fresh ? "Leave it requested" : "← Back"}</Btn><Btn variant="primary" disabled={pending || (who === "internal" ? !tech : !org || !terms.trim())} onClick={book}>{who === "internal" ? "Assign" : "Send offer"}</Btn></>}>
      <p className="text-xs text-muted">{fresh ? `Created ${j.id.slice(0, 8)} (requested) · choose who does the work — ` : ""}{d.title} · {d.customer} · {j.origin === "periodic_plan" ? "Periodic plan" : "Client request"}</p>
      {fresh && ahead.length > 1 ? (
        <Field label="Visit time · one of the customer’s preferred times" hint="Only these times are agreed (IR113); another time needs a proposal the customer accepts."><Select value={String(rank)} onChange={(e) => pick(Number(e.target.value))}>{ahead.map((r) => <option key={r.rank} value={r.rank}>{["1st", "2nd", "3rd"][r.rank - 1]} · {r.text}{r.fits ? "" : " · no HQ technician free"}</option>)}</Select></Field>
      ) : <Field label="Visit time · from the client" hint="Fixed here. Need another time? Use “Propose another time…” — the client must accept it."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {longSlot(row.slot)}</div></Field>}
      <Field label="Who does the work" hint={row.people.length ? undefined : "No qualified HQ technician is free at this time — offer it to a contractor."}><Choice value={who} onChange={setWho} options={[...(row.people.length ? [{ id: "internal" as const, label: "Assign internally" }] : []), { id: "contractor" as const, label: "Offer to contractor" }]} /></Field>
      {who === "internal" ? (
        <Field label="Technician" hint="Free and qualified for the whole slot (members.eligible)"><Select value={tech} onChange={(e) => setTech(e.target.value)}>{row.people.map((p) => <option key={p.id} value={p.id}>{p.displayName}</option>)}</Select></Field>
      ) : (
        <>
          <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
            <Field label="Contractor organization"><Select value={org} onChange={(e) => setOrg(e.target.value)}>{active.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
            <Field label="Terms version"><Input value={terms} onChange={(e) => setTerms(e.target.value)} /></Field>
          </div>
          <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
            <Field label="Offer expires at"><Input type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} /></Field>
            <Field label="Access valid from"><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
            <Field label="Access valid until"><Input type="datetime-local" value={until} onChange={(e) => setUntil(e.target.value)} /></Field>
          </div>
        </>
      )}
      <Banner>{who === "internal" ? "The technician must accept the assignment (受領)." : "The access window must cover the visit; the offer may not expire after the visit starts."}</Banner>
    </Modal>
  );
}

function ProposeModal({ d, live, onClose, onFail }: ModalProps) {
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const tomorrow = klTime(new Date(now + 86_400_000).toISOString()).slice(0, 10);
  const [date, setDate] = useState(tomorrow);
  const [from, setFrom] = useState("10:00");
  const [to, setTo] = useState("12:00");
  const options = [...live.internalTechs.map((t) => ({ id: `i:${t.id}`, label: `${t.name} (HQ)` })), ...live.contractors.filter((c) => c.status === "active").map((c) => ({ id: `c:${c.id}`, label: `${c.name} (offer)` }))];
  const [who, setWho] = useState(options[0]?.id ?? "");
  const [msg, setMsg] = useState("");
  const [reply, setReply] = useState("48");
  const [tried, setTried] = useState(false);
  const slot = { startAt: new Date(`${date}T${from}:00+08:00`).toISOString(), endAt: new Date(`${date}T${to}:00+08:00`).toISOString() };
  const preferred = j.preferredSlots.some((s) => s.startAt === slot.startAt && s.endAt === slot.endAt);
  const send = () => {
    setTried(true);
    if (!msg.trim() || preferred || from >= to || !who) return;
    const hold = who.startsWith("i:") ? { kind: "internal" as const, membershipId: who.slice(2) } : { kind: "contractor" as const, contractorOrgId: who.slice(2), technicianMembershipId: null };
    const replyBy = new Date(Math.min(now + Number(reply) * 3600_000, Date.parse(slot.startAt))).toISOString();
    run(() => proposeSlot(j.id, j.version, slot, hold, msg, replyBy), "Proposal sent — waiting for the client", () => onClose(`Proposed ${slotText(slot)} to the client (reply by ${klTime(replyBy).slice(5)}).`), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title="Propose another time to the client" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={send}>Send proposal</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.customer} · {d.title}</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label="From"><Input type="time" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label="To" error={tried && from >= to ? "The end must be after the start" : undefined}><Input type="time" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
      </div>
      {preferred && <p className="text-xs text-crit">✕ This is one of the client’s own times — book it directly instead.</p>}
      <Field label="Capacity held for the time"><Select value={who} onChange={(e) => setWho(e.target.value)}>{options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</Select></Field>
      <Field label="Message to the client · required (1–1000)" error={tried && !msg.trim() ? "A message is required" : undefined}><Textarea value={msg} maxLength={1000} onChange={(e) => setMsg(e.target.value)} placeholder="All qualified technicians are booked on your times. The earliest free slot is Thursday morning." /></Field>
      <Field label="Reply by"><Select value={reply} onChange={(e) => setReply(e.target.value)}><option value="24">24 h</option><option value="48">48 h</option><option value="72">72 h</option></Select></Field>
      <Banner>The client sees “Time proposed” with Accept / Decline. Accept → booked with the held capacity. Decline → the client sends new times.</Banner>
    </Modal>
  );
}

function ReasonModal({ d, live, what, onClose, onFail }: ModalProps & { what: "hold" | "resume" | "cancel" | "reassign" }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const others = live.internalTechs.filter((t) => t.id !== j.assignment?.technicianMembershipId);
  const [tech, setTech] = useState(others[0]?.id ?? "");
  const title = { hold: "Put on hold (IR56)", resume: "Resume the job", cancel: `Cancel ${j.id.slice(0, 8)}`, reassign: `Reassign ${j.id.slice(0, 8)}` }[what];
  const confirm = () => {
    setTried(true);
    if (!r.trim() || r.length > 1000 || (what === "reassign" && !tech)) return;
    const slot = j.assignment ? { startAt: j.assignment.scheduledStart, endAt: j.assignment.scheduledEnd } : j.scheduledSlot!;
    const done = { hold: "Job put on hold — scheduling and reports are blocked.", resume: "Job resumed.", cancel: "Job cancelled.", reassign: "Reassigned — the new technician must accept." }[what];
    run(() => what === "hold" ? holdJob(j.id, j.version, r) : what === "resume" ? resumeJob(j.id, j.version, r) : what === "cancel" ? cancelJob(j.id, j.version, r) : assignInternal(j.id, j.version, tech, slot, r), done, () => onClose(done), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={title} footer={<><Btn onClick={() => onClose()}>Back</Btn><Btn variant={what === "cancel" ? "danger" : "primary"} disabled={pending} onClick={confirm}>Confirm</Btn></>}>
      {what === "reassign" && <Field label="New technician (same agreed time)"><Select value={tech} onChange={(e) => setTech(e.target.value)}>{others.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</Select></Field>}
      <Field label="Reason · required (1–1000)" error={tried && !r.trim() ? "A reason is required" : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} /></Field>
      {what === "cancel" && <Banner tone="warn">Open offers are withdrawn and active assignments revoked. The client is notified.</Banner>}
    </Modal>
  );
}

function ClassifyModal({ d, onClose, onFail }: Omit<ModalProps, "live">) {
  const j = d.job;
  const [pending, run] = useAction();
  const [cls, setCls] = useState<"rework" | "new_request">("rework");
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title="Classify follow-up request" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!r.trim()) return; run(() => classifyFollowUp(j.id, j.version, cls, r), "Classified", () => onClose(cls === "rework" ? "Classified as rework (free) — book the client’s preferred visit." : "Classified as a new request — book it like any client request."), onFail); }}>Classify</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.customer} · {d.title} — follow-up of {j.followUpOfJobId?.slice(0, 8)}</p>
      <Field label="Classification · required"><Choice value={cls} onChange={setCls} options={[{ id: "rework", label: "Rework (free)" }, { id: "new_request", label: "New request" }]} /></Field>
      <Field label="Reason · required (1–1000)" error={tried && !r.trim() ? "A reason is required" : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} /></Field>
      <Banner>Set once — cannot be changed later. The job stays “Requested” and is booked like any client request (IR113).</Banner>
    </Modal>
  );
}

/** The inspection results: the items that are not OK first, the OK ones folded into one line. */
function ReportRows({ rows }: { rows: NonNullable<Detail["report"]>["rows"] }) {
  const row = (r: (typeof rows)[number]) => <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 py-1.5 text-[13px]"><span>{r.label}{r.reason && <span className="text-xs text-muted"> · {r.reason}</span>}</span><Badge tone={r.tone}>{r.result}</Badge></li>;
  const ok = rows.filter((r) => r.tone === "ok");
  return (
    <>
      <ul className="divide-y divide-line">{rows.filter((r) => r.tone !== "ok").map(row)}</ul>
      {ok.length > 0 && <details className="border-t border-line pt-1.5 text-[13px]"><summary className="cursor-pointer text-muted">{ok.length} check{ok.length === 1 ? "" : "s"} OK</summary><ul className="divide-y divide-line">{ok.map(row)}</ul></details>}
    </>
  );
}

/** “Open report →”: the whole report — every inspection result, readings, photos, parts and refrigerant, the work
 * performed, the next action, the sign-off and the review history. */
function ReportModal({ report: r, draft, onClose }: { report: NonNullable<Detail["report"]>; draft: boolean; onClose: () => void }) {
  return (
    <Modal open wide onClose={onClose} title={r.title} footer={<Btn onClick={onClose}>Close</Btn>}>
      <p className="text-xs text-muted">{r.sub}</p>
      <ul className="divide-y divide-line">{r.rows.map((x) => <li key={x.id} className="flex flex-wrap items-center justify-between gap-2 py-1.5 text-[13px]"><span>{x.label}{x.reason && <span className="text-xs text-muted"> · {x.reason}</span>}</span><Badge tone={x.tone}>{x.result}</Badge></li>)}</ul>
      <SummaryList items={[...r.readings, ...r.parts, ["Work performed", r.workText || "—"], ["Next action", r.next], ["Customer sign-off", r.signOff]]} />
      {r.photos.length > 0 && <div className="flex flex-wrap gap-2">{r.photos.map((p) => (
        // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
        <img key={p.id} src={p.url} alt={p.name} className="h-32 w-44 rounded-lg bg-surface2 object-cover" />
      ))}</div>}
      {r.reviews.length > 0 && <ul className="flex flex-col gap-0.5 text-xs">{r.reviews.map((x) => <li key={x.text} className={x.tone === "ok" ? "text-ok" : "text-warn"}>{x.text}</li>)}</ul>}
      {draft && <p className="text-xs text-muted">A new version is being drafted by the technician.</p>}
    </Modal>
  );
}

function ReviewModal({ d, report, decision, onClose, onFail }: Omit<ModalProps, "live"> & { report: NonNullable<Detail["report"]>; decision: "accept" | "return" }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [items, setItems] = useState(() => report.rework.filter((x) => x.checked).map((x) => x.label));
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const reason = decision === "return" ? returnReason(items, r) : r.trim();
  const error = !r.trim() ? "A reason is required" : reason.length > 1000 ? `${reason.length} / 1000 characters with the ticked items` : undefined;
  const confirm = () => {
    setTried(true);
    if (error) return;
    const done = decision === "return" ? `Returned v${report.version} for rework — the technician continues on the same assignment.` : `Accepted v${report.version} — the job is completed.`;
    run(() => reviewReport(j.id, j.version, report.version, decision, report.mode, reason), decision === "return" ? "Returned for rework" : "Report accepted", () => onClose(done), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title={decision === "return" ? "Return for rework" : "Accept by escalation"} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant={decision === "return" ? "danger" : "primary"} disabled={pending} onClick={confirm}>{decision === "return" ? "Return for rework" : "Accept report"}</Btn></>}>
      <p className="text-xs text-muted">jobs.review · decision={decision}{report.mode === "hq_escalation" ? " · HQ escalation" : ""} · report v{report.version} by {report.author}</p>
      {decision === "return" && (
        <fieldset className="flex flex-col gap-1.5"><legend className="mb-1 text-[13px] font-semibold">Items that need rework</legend>
          {report.rework.map((x) => <label key={x.label} className="flex items-start gap-2 text-[13px]"><input type="checkbox" className="mt-0.5" checked={items.includes(x.label)} onChange={(e) => setItems(e.target.checked ? [...items, x.label] : items.filter((v) => v !== x.label))} />{x.label}</label>)}
        </fieldset>
      )}
      <Field label="Reason · required" hint="1–1000 characters · shown to the technician and kept in the review history." error={tried ? error : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder={decision === "return" ? "Airflow still low after cleaning; re-check the blower and attach an after photo." : "The contractor has not reviewed within the agreed time; HQ accepts on the evidence."} /></Field>
      <Banner tone={decision === "return" ? "warn" : undefined}>{decision === "return" ? "Status submitted → rework_requested. The same assignment continues; the technician submits a new report version. Completion does not resolve linked alerts." : "Status submitted → completed. The customer sees the accepted report; linked alerts stay open until they are resolved."}</Banner>
    </Modal>
  );
}

function CostModal({ d, onClose, onFail }: Omit<ModalProps, "live">) {
  const j = d.job;
  const [pending, run] = useAction();
  const [f, setF] = useState({ kind: j.status === "completed" || j.status === "submitted" ? "actual" : "estimate", description: "", visibility: "customer", amount: "", currency: "MYR" });
  const [tried, setTried] = useState(false);
  const { line, error } = costLineOf(f);
  const save = () => {
    setTried(true);
    if (!line) return;
    run(() => saveCosts(j.id, j.version, [...d.costs, line]), "Cost line added", () => onClose(`Added ${line.kind} “${line.description}” · ${money(line.amountMinor, line.currency)}.`), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title="Add cost line" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Save</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.title} · {d.customer} — saved with the job’s other {d.costs.length} line{d.costs.length === 1 ? "" : "s"} (jobs.saveCost)</p>
      <Field label="Kind"><Choice value={f.kind as CostLine["kind"]} onChange={(v) => setF({ ...f, kind: v })} options={[{ id: "estimate", label: "Estimate" }, { id: "actual", label: "Actual" }]} /></Field>
      <Field label="Description · required (1–200)"><Input value={f.description} maxLength={200} onChange={(e) => setF({ ...f, description: e.target.value })} placeholder="Filter cleaning labour" /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label="Amount" error={tried && error ? error : undefined}><Input inputMode="decimal" value={f.amount} onChange={(e) => setF({ ...f, amount: e.target.value })} placeholder="80.00" /></Field>
        <Field label="Currency"><Select value={f.currency} onChange={(e) => setF({ ...f, currency: e.target.value })}><option value="MYR">MYR</option><option value="USD">USD</option></Select></Field>
      </div>
      <Field label="Visibility" hint="Customer lines appear on the client’s job; internal lines stay with HQ."><Choice value={f.visibility as CostLine["visibility"]} onChange={(v) => setF({ ...f, visibility: v })} options={[{ id: "customer", label: "Customer" }, { id: "internal", label: "Internal" }]} /></Field>
      <Banner>Amounts are kept per currency and never converted. No payment is made (DD-A06).</Banner>
    </Modal>
  );
}

function ExtendModal({ d, live, until, onClose, onFail }: ModalProps & { until: string }) {
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [end, setEnd] = useState(klLocal(extendDefault(new Date(Math.max(Date.parse(until), now)).toISOString())));
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const at = end ? fromLocal(end) : "";
  const endError = !at || Date.parse(at) <= Date.parse(until) || Date.parse(at) <= now ? "The new end must be later than the current end and in the future." : undefined;
  const save = () => {
    setTried(true);
    if (endError || !r.trim()) return;
    run(() => extendAccess(j.id, j.version, at, r), "Access extended", () => onClose(`Access extended to ${klTime(at).slice(5)}. The contractor keeps the site details until then.`), onFail);
  };
  return (
    <Modal open onClose={() => onClose()} title="Extend contractor access" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Extend</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.title} · access now ends {klTime(until).slice(5)}{Date.parse(until) <= now ? " (ended)" : ""}</p>
      <Field label="New end · required" error={tried ? endError : undefined}><Input type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} /></Field>
      <Field label="Reason · required (1–1000)" error={tried && !r.trim() ? "A reason is required" : undefined}><Textarea value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder="The rework visit runs into the next day." /></Field>
      <Banner>Only the end moves; the contractor sees the access instructions until then (IR25).</Banner>
    </Modal>
  );
}

const JOB_TYPES = [{ id: "reactive" as const, label: "Repair" }, { id: "preventive" as const, label: "Preventive" }, { id: "periodic" as const, label: "Periodic" }];

/** New job (DD-A06 item 8, Figma 06-1 New job): step 1 creates the job (requested) with the customer's preferred times
 * asked by phone; “Save as requested” stops there, “Next: choose delivery” opens step 2 (the booking dialog) on it. */
function NewJobModal({ live, onClose, onCreated }: { live: JobsLive; onClose: () => void; onCreated: (f: Fresh) => void }) {
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(live.q.unitId ?? live.units[0]?.id ?? "");
  const [type, setType] = useState<"reactive" | "preventive" | "periodic">("reactive");
  const [symptom, setSymptom] = useState("");
  const tomorrow = klTime(new Date(now + 86_400_000).toISOString()).slice(0, 10);
  const [times, setTimes] = useState([{ date: tomorrow, from: "10:00", to: "12:00" }]);
  const [due, setDue] = useState("");
  const [contact, setContact] = useState("9:00–18:00");
  const [tried, setTried] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  const slots = times.map((t) => ({ startAt: new Date(`${t.date}T${t.from}:00+08:00`).toISOString(), endAt: new Date(`${t.date}T${t.to}:00+08:00`).toISOString() }));
  const errors = newJobErrors({ unitId, symptom, slots }, now);
  const dueAt = due ? fromLocal(due) : null;
  const dueError = dueAt && Date.parse(dueAt) < Date.parse(slots[0].endAt) ? "The due time cannot be before the end of the 1st time." : undefined;
  const set = (i: number, k: "date" | "from" | "to", v: string) => setTimes(times.map((t, n) => (n === i ? { ...t, [k]: v } : t)));
  const save = (book: boolean) => {
    setTried(true);
    setRefusal(null);
    if (Object.keys(errors).length || dueError) return;
    run(() => createJob({ unitId, type, symptom, slots, dueAt, contactWindow: contact }), book ? "Job created — choose who does the work" : "Job created (requested)",
      (v) => { onCreated({ id: v.id, book }); router.push(`/admin/jobs?jobId=${v.id}`); }, (f) => setRefusal(hqRefusal(f)));
  };
  const groups = [...new Set(live.units.map((u) => u.customer))];
  return (
    <Modal open wide onClose={onClose} title="New maintenance job · step 1 of 2" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn disabled={pending} onClick={() => save(false)}>Save as requested</Btn><Btn variant="primary" disabled={pending} onClick={() => save(true)}>Next: choose delivery →</Btn></>}>
      {refusal && <Banner tone="crit">{refusal}</Banner>}
      <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
        <Field label="Unit · required" error={tried ? errors.unitId : undefined}><Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{groups.map((g) => <optgroup key={g} label={g}>{live.units.filter((u) => u.customer === g).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</optgroup>)}</Select></Field>
        <Field label="Type"><Choice value={type} onChange={setType} options={JOB_TYPES} /></Field>
      </div>
      <Field label="Symptom · required (10–2000)" error={tried ? errors.symptom : undefined}><Textarea value={symptom} maxLength={2000} onChange={(e) => setSymptom(e.target.value)} placeholder="Unit shows offline since 08:12; the customer reports no cooling." /></Field>
      <div className="flex flex-col gap-2">
        <span className="text-[13px] font-semibold">Customer’s preferred times <span className="font-normal text-muted">· asked by phone · the 1st is the requested window, up to two more (each 1–4 h, from tomorrow)</span></span>
        {times.map((t, i) => (
          <div key={i} className="flex flex-wrap items-end gap-2">
            <span className="mb-2 w-8 text-xs font-semibold text-primary">{["1st", "2nd", "3rd"][i]}</span>
            <Field label={`Date ${i + 1}`} error={tried ? errors[`slot${i}`] : undefined}><Input type="date" value={t.date} onChange={(e) => set(i, "date", e.target.value)} /></Field>
            <Field label={`From ${i + 1}`}><Input type="time" value={t.from} onChange={(e) => set(i, "from", e.target.value)} /></Field>
            <Field label={`To ${i + 1}`}><Input type="time" value={t.to} onChange={(e) => set(i, "to", e.target.value)} /></Field>
            {i > 0 && <Btn size="sm" className="mb-1" onClick={() => setTimes(times.filter((_, n) => n !== i))}>Remove</Btn>}
          </div>
        ))}
        {times.length < 3 && <div><Btn size="sm" onClick={() => setTimes([...times, { ...times[times.length - 1], date: klTime(new Date(Date.parse(`${times[times.length - 1].date}T00:00:00+08:00`) + 86_400_000).toISOString()).slice(0, 10) }])}>+ Add another time</Btn></div>}
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
        <Field label="Due · optional" hint="Defaults to the end of the 1st time (IR38)." error={tried ? dueError : undefined}><Input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} /></Field>
        <Field label="Contact window · optional" hint="When to call — no phone numbers or e-mail addresses (IR64)."><Input value={contact} maxLength={200} onChange={(e) => setContact(e.target.value)} /></Field>
      </div>
      <Banner>Saving creates the job as requested. Step 2 books one of these times — assign an HQ technician or offer it to a contractor (IR113).</Banner>
    </Modal>
  );
}
