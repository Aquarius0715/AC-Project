"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Textarea, cx } from "@ac/web/components/ui";
import { JobStatusBadge, OriginBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klTime } from "@ac/web/lib/devices";
import { classifyBy, hqRefusal, longSlot, offerDefaults, slotText, type PreferredRow } from "@ac/web/lib/adminJobs";
import type { JobStatus } from "@ac/web/lib/jobs";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { assignInternal, cancelJob, classifyFollowUp, holdJob, offerToContractor, proposeSlot, resolvePartnerSlot, resumeJob, withdrawProposal } from "../actions";
import type { JobsLive } from "../_lib/load";
import { Contractors, PlansDemo, Sla } from "./jobs-demo";

type Detail = NonNullable<JobsLive["detail"]>;
const TYPES = [{ id: "", label: "Type: All" }, { id: "reactive", label: "Type: Repair" }, { id: "periodic", label: "Type: Periodic" }, { id: "preventive", label: "Type: Preventive" }];
const klLocal = (iso: string) => klTime(iso).replace(" ", "T");
const fromLocal = (v: string) => new Date(`${v}:00+08:00`).toISOString();

/** HQ maintenance jobs (FR-A06, Figma Admin 06-1, 06-11…06-18): the Jobs tab from the Core API. */
export function JobsView({ live, tab }: { live: JobsLive; tab: string }) {
  const patch = useUrlPatch();
  const tabs = [{ id: "jobs", label: "Jobs", count: live.total }, { id: "plans", label: "Plans" }, { id: "contractors", label: "Contractors" }, { id: "sla", label: "SLA by customer" }];
  return (
    <Page className="max-w-[1440px]">
      <div role="tablist" className="flex flex-wrap gap-4 border-b border-line">
        {tabs.map((t) => <Link key={t.id} role="tab" aria-selected={tab === t.id} href={t.id === "jobs" ? "/admin/jobs" : `/admin/jobs?tab=${t.id}`} className={cx("-mb-px border-b-2 px-1 pb-2 text-[13px] font-semibold", tab === t.id ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink")}>{t.label}{t.count !== undefined && <span className="ml-1.5 rounded bg-surface2 px-1.5 text-[11px]">{t.count}</span>}</Link>)}
      </div>
      {tab !== "jobs" && <Banner tone="warn">{tabs.find((t) => t.id === tab)?.label} is not connected to the Core API yet — illustrative data (FR-A06 / A21 / A22, next rounds).</Banner>}
      {tab === "plans" && <PlansDemo />}
      {tab === "contractors" && <Contractors />}
      {tab === "sla" && <Sla />}
      {tab === "jobs" && <JobsTab live={live} patch={patch} />}
    </Page>
  );
}

function JobsTab({ live, patch }: { live: JobsLive; patch: (p: Record<string, string | null>) => void }) {
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
        {sel ? <JobDetail key={sel.job.id} d={sel} live={live} /> : <Card title="Job">{"missing" in live ? <p className="text-[13px] text-muted">That job no longer exists in your scope.</p> : <p className="text-[13px] text-muted">Pick a job.</p>}</Card>}
      </div>
    </>
  );
}

type ModalState = null | { kind: "book"; row: PreferredRow } | { kind: "propose" } | { kind: "reason"; what: "hold" | "resume" | "cancel" | "reassign" } | { kind: "classify" };

function JobDetail({ d, live }: { d: Detail; live: JobsLive }) {
  const j = d.job;
  const now = Date.parse(live.now);
  const [modal, setModal] = useState<ModalState>(null);
  const [pending, run] = useAction();
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
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
        <Card title={d.delivery.title}>
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

      <Card title="History">
        {d.history.length === 0 ? <p className="text-[13px] text-muted">No events.</p> : <ol className="flex flex-col gap-1 text-xs">{d.history.map((h) => <li key={h.id}><span className="text-muted">{h.at}</span> · {h.text}</li>)}</ol>}
      </Card>

      {modal?.kind === "book" && <BookModal d={d} live={live} row={modal.row} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "propose" && <ProposeModal d={d} live={live} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "reason" && <ReasonModal d={d} live={live} what={modal.what} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
      {modal?.kind === "classify" && <ClassifyModal d={d} onClose={close} onFail={(f) => { setModal(null); failed(f); }} />}
    </div>
  );
}

type ModalProps = { d: Detail; live: JobsLive; onClose: (text?: string) => void; onFail: (f: ActionFailure) => void };

function BookModal({ d, live, row, onClose, onFail }: ModalProps & { row: PreferredRow }) {
  const j = d.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [who, setWho] = useState<"internal" | "contractor">(row.people.length ? "internal" : "contractor");
  const [tech, setTech] = useState(row.people[0]?.id ?? "");
  const active = live.contractors.filter((c) => c.status === "active");
  const [org, setOrg] = useState(active[0]?.id ?? "");
  const def = offerDefaults(row.slot, now);
  const [expires, setExpires] = useState(klLocal(def.offerExpiresAt));
  const [from, setFrom] = useState(klLocal(def.accessValidFrom));
  const [until, setUntil] = useState(klLocal(def.accessValidUntil));
  const [terms, setTerms] = useState("terms-demo-v1");
  const book = () => who === "internal"
    ? run(() => assignInternal(j.id, j.version, tech, row.slot), "Assigned — the technician must accept", () => onClose(`Assigned ${row.people.find((p) => p.id === tech)?.displayName} for ${slotText(row.slot)}. The technician must accept (受領).`), onFail)
    : run(() => offerToContractor(j.id, j.version, { contractorOrgId: org, visitSlot: row.slot, offerExpiresAt: fromLocal(expires), accessValidFrom: fromLocal(from), accessValidUntil: fromLocal(until), termsVersion: terms.trim() }), "Offer sent", () => onClose(`Offered ${slotText(row.slot)} to ${active.find((c) => c.id === org)?.name}. They accept, decline or propose another time — a different time goes back to the client.`), onFail);
  return (
    <Modal open onClose={() => onClose()} title={`Book ${j.id.slice(0, 8)}`} footer={<><Btn onClick={() => onClose()}>← Back</Btn><Btn variant="primary" disabled={pending || (who === "internal" ? !tech : !org || !terms.trim())} onClick={book}>{who === "internal" ? "Assign" : "Send offer"}</Btn></>}>
      <p className="text-xs text-muted">{d.title} · {d.customer} · {j.origin === "periodic_plan" ? "Periodic plan" : "Client request"}</p>
      <Field label="Visit time · from the client" hint="Fixed here. Need another time? Use “Propose another time…” — the client must accept it."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {longSlot(row.slot)}</div></Field>
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
