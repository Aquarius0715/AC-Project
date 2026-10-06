"use client";

import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, DataTable, Field, Input, Kpi, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, Timeline, cx, useToast } from "@/components/ui";
import { JobStatusBadge, OriginBadge, PreferredSlotsInput, Rank, preferredError } from "@/components/JobBits";
import { Job, JobStatus, Slot, fitFor, fmt, jobActions, longDate, statusLabel, useJobs } from "@/lib/jobs";

const PIPE: JobStatus[] = ["requested", "time_proposed", "offered", "accepted", "assigned", "in_progress", "submitted", "rework_requested", "completed", "on_hold"];
const ORDER: JobStatus[] = ["requested", "time_proposed", "offered", "accepted", "assigned", "in_progress", "on_hold", "submitted", "rework_requested", "completed", "cancelled"];
const plans = [
  { id: "plan-living-a", name: "Living room AC — periodic inspection & filter cleaning", rule: "Every 3 months · customer-a · next visit 2026-12-08", unit: "Living room AC", delivery: "Internal · tech-internal-b" },
  { id: "plan-lobby-b", name: "Lobby AC — quarterly inspection", rule: "Every 3 months · customer-b · next 2026-12-16", unit: "Lobby AC", delivery: "Offer to contractor-a" },
];
type Tab = "jobs" | "plans" | "contractors" | "sla";

export default function AdminJobs({ searchParams }: { searchParams: Promise<{ jobId?: string; tab?: string }> }) {
  const sp = use(searchParams);
  const toast = useToast();
  const jobs = useJobs();
  const [tab, setTab] = useState<Tab>((["plans", "contractors", "sla"].includes(sp.tab ?? "") ? sp.tab : "jobs") as Tab);
  const [selId, setSelId] = useState(sp.jobId ?? "job-c07");
  const [origin, setOrigin] = useState<"all" | "request" | "plan">("all");
  const [delivery, setDelivery] = useState<"all" | "internal" | "contractor">("all");
  const [stage, setStage] = useState<JobStatus | null>(null);
  const [modal, setModal] = useState<null | { kind: "book"; slot: Slot } | { kind: "propose" } | { kind: "new" } | { kind: "reason"; what: "rework" | "hold" | "reassign" }>(null);
  const [plan, setPlan] = useState(plans[0]);
  const list = [...jobs].filter((j) => j.status !== "cancelled").sort((a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status))
    .filter((j) => (origin === "all" || j.origin === origin) && (delivery === "all" || j.delivery === delivery) && (!stage || j.status === stage));
  const sel = jobs.find((j) => j.id === selId) ?? list[0];
  return (
    <Page className="max-w-[1440px]">
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "jobs", label: "Jobs", count: jobs.filter((j) => j.status !== "cancelled").length }, { id: "plans", label: "Plans", count: 2 }, { id: "contractors", label: "Contractors", count: 3 }, { id: "sla", label: "SLA by customer" }]} />
      {tab === "jobs" && (
        <>
          <div className="scroll-x"><div className="grid min-w-[880px] grid-cols-10 overflow-hidden rounded-2xl border border-line bg-surface">
            {PIPE.map((s) => <button key={s} onClick={() => setStage(stage === s ? null : s)} className={cx("border-r border-line px-3 py-2 text-left last:border-r-0 hover:bg-surface2", stage === s && "bg-primary-soft")}><div className="text-[11px] text-muted">{statusLabel[s]}</div><div className={cx("text-lg font-bold", s === "time_proposed" && jobs.some((j) => j.status === s) && "text-warn")}>{jobs.filter((j) => j.status === s).length}</div></button>)}
          </div></div>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap gap-2">
              <Select className="w-auto" aria-label="Origin" value={origin} onChange={(e) => setOrigin(e.target.value as typeof origin)}><option value="all">Origin: All</option><option value="request">Origin: Client request</option><option value="plan">Origin: Periodic plan</option></Select>
              <Select className="w-auto" aria-label="Delivery" value={delivery} onChange={(e) => setDelivery(e.target.value as typeof delivery)}><option value="all">Delivery: All</option><option value="internal">Delivery: Internal</option><option value="contractor">Delivery: Contractor</option></Select>
              {stage && <Btn size="sm" variant="ghost" onClick={() => setStage(null)}>✕ {statusLabel[stage]}</Btn>}
            </div>
            <Btn variant="primary" onClick={() => setModal({ kind: "new" })}>+ New job</Btn>
          </div>
          <div className="split-rev">
            <Card title="Jobs · all customers" sub="status ↑" className="self-start">
              <div className="flex flex-col gap-1.5">{list.map((j) => (
                <ListRow key={j.id} selected={sel?.id === j.id} onClick={() => setSelId(j.id)}>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{j.id} · {j.unit}</b><JobStatusBadge s={j.status} /></div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] text-muted"><OriginBadge origin={j.origin} />{j.customer} · {j.type}</div>
                    <div className="text-[11px] text-muted">{rowLine(j)}</div>
                  </div>
                </ListRow>
              ))}{list.length === 0 && <p className="text-xs text-muted">No jobs in this filter.</p>}</div>
            </Card>
            {sel && <JobDetail j={sel} open={setModal} />}
          </div>
        </>
      )}
      {tab === "plans" && (
        <div className="split-rev">
          <Card title="Maintenance plans" className="self-start"><div className="flex flex-col gap-2">{plans.map((p) => <ListRow key={p.id} selected={plan.id === p.id} onClick={() => setPlan(p)}><div><b className="text-[13px]">{p.id}</b><div className="text-[11px] text-muted">{p.name}</div></div></ListRow>)}</div></Card>
          <Card title={plan.name} sub={plan.rule} action={<Btn size="sm" variant="primary" onClick={() => toast("CONFLICT — an occurrence already exists for this period", "warn")}>Generate next visit</Btn>}>
            <SummaryList items={[["Plan", plan.id], ["Unit", plan.unit], ["Rule", plan.rule], ["Delivery", plan.delivery], ["Client notice", "1 month before each visit · origin “Periodic plan”"]]} />
            <div className="mt-3"><Banner>Generated visits get a fixed time and go straight to assignment / offer. The client can “Request another time” with 3 options — that sends the visit back to triage like a client request (IR113).</Banner></div>
          </Card>
        </div>
      )}
      {tab === "contractors" && <Contractors />}
      {tab === "sla" && <Sla />}
      {modal?.kind === "book" && sel && <BookModal j={sel} slot={modal.slot} onClose={() => setModal(null)} />}
      {modal?.kind === "propose" && sel && <ProposeModal j={sel} onClose={() => setModal(null)} />}
      {modal?.kind === "new" && <NewJobModal onClose={() => setModal(null)} onDone={(id) => { setModal(null); setSelId(id); }} />}
      {modal?.kind === "reason" && sel && <ReasonModal what={modal.what} j={sel} onClose={() => setModal(null)} />}
    </Page>
  );
}

function rowLine(j: Job) {
  switch (j.status) {
    case "requested": return j.declined ? `Client declined · round ${j.round} · ${j.preferred.length} new times` : `${j.preferred.length} preferred times · book one or propose`;
    case "time_proposed": return `Proposed ${fmt(j.proposal?.slot)} · reply by ${j.proposal?.replyBy}`;
    case "offered": return j.partnerProposal?.status === "pending" ? `${j.contractor} proposes ${fmt(j.partnerProposal.slot)} · needs client OK` : `Offer to ${j.contractor} · ${fmt(j.scheduled)}`;
    case "accepted": return `${j.contractor} accepted · assign technician`;
    case "assigned": return `${j.delivery === "contractor" ? j.contractor + " · " : "Internal · "}${j.technician} · ${j.techAck?.status === "accepted" ? "accepted ✓" : j.techAck?.status === "cant_make" ? "can’t make it ⚠" : "awaiting acceptance"}`;
    default: return `${j.delivery === "contractor" ? j.contractor : "Internal"} · ${j.technician ?? "—"} · ${fmt(j.scheduled)}`;
  }
}

function JobDetail({ j, open }: { j: Job; open: (m: { kind: "book"; slot: Slot } | { kind: "propose" } | { kind: "reason"; what: "rework" | "hold" | "reassign" }) => void }) {
  const toast = useToast();
  const fits = j.preferred.map(fitFor);
  const anyFit = fits.some((f) => f.fits !== "none");
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <Card title={<span className="flex flex-wrap items-center gap-2">{j.id} · {j.unit} <JobStatusBadge s={j.status} /><OriginBadge origin={j.origin} /></span>} sub={`${j.type} · ${j.customer} · ${j.loc}${j.planId ? ` · ${j.planId} (${j.planVisit})` : ""}`} action={<div className="flex gap-2"><Btn size="sm" onClick={() => open({ kind: "reason", what: "hold" })}>Hold…</Btn><Btn size="sm" variant="danger" onClick={() => { jobActions.cancel(j.id, "cancelled by HQ"); toast("Job cancelled", "warn"); }}>Cancel…</Btn></div>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          {[["Symptom", j.symptom], ["Needs", j.type === "Periodic" ? "General maintenance" : "Refrigerant handling · 2 h"], ["Scheduled", j.scheduled ? longDate(j.scheduled) : "not booked"], ["Contact", "9:00–18:00 · call before arriving"]].map(([k, v]) => <div key={k} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{k}</div><div className="text-[13px] font-semibold">{v}</div></div>)}
        </div>
      </Card>

      {j.status === "requested" && (
        <>
          {j.declined && <Banner tone="crit" icon="✕"><b>Client declined the proposed time · {j.declined.at}</b><br />Reason: {j.declined.reason}{j.declined.comment ? ` — “${j.declined.comment}”` : ""}. Held capacity was released.</Banner>}
          <Card title={`Client’s preferred times${j.round > 1 ? ` (round ${j.round})` : ""} — book one of these`} sub="Availability: qualification + overlap + travel">
            <DataTable rowKey={(r) => r.i + ""} rows={j.preferred.map((s, i) => ({ s, i, f: fits[i] }))} cols={[
              { key: "r", label: "Rank", render: (r) => <Rank i={r.i} /> },
              { key: "t", label: "Time", render: (r) => <b>{longDate(r.s)}</b> },
              { key: "h", label: "HQ technicians", render: (r) => <span className="text-xs text-muted">{r.f.hq}</span>, hideBelow: "md" },
              { key: "c", label: "Contractors", render: (r) => <span className="text-xs text-muted">{r.f.partner}</span>, hideBelow: "md" },
              { key: "f", label: "Fit", render: (r) => r.f.fits === "none" ? <Badge tone="crit">No fit</Badge> : <Badge tone="ok">Fits</Badge> },
              { key: "a", label: "Action", render: (r) => <Btn size="sm" variant={r.f.fits === "none" ? "secondary" : "primary"} disabled={r.f.fits === "none"} onClick={() => open({ kind: "book", slot: r.s })}>Use this time</Btn> },
            ]} />
            <p className="mt-2 text-[11px] text-muted">“Use this time” books with the time locked. HQ cannot book a time outside the client’s preferred times without the client’s approval.</p>
          </Card>
          <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-[#f5c473] bg-[#fff8ec] p-4">
            <div className="min-w-0 flex-1"><b className="text-warn">{anyFit ? "Prefer another time?" : "None of the times works — propose another time"}</b><p className="text-xs">Earliest fits: Thu 10-01 10:00–12:00 (contractor-a · tech-external-a) · Thu 10-01 14:00–16:00 (tech-internal-a). Nothing is booked until the client accepts.</p></div>
            <Btn variant={anyFit ? "secondary" : "primary"} onClick={() => open({ kind: "propose" })}>Propose another time…</Btn>
          </div>
        </>
      )}

      {j.status === "time_proposed" && j.proposal && (
        <Card tone="warn" title={j.proposal.by === "contractor" ? "Partner’s time sent to the client — waiting" : "Proposal sent — waiting for the client"} action={<Badge tone="warn">Reply by {j.proposal.replyBy}</Badge>}>
          <SummaryList items={[["Proposed time", longDate(j.proposal.slot)], ["Held capacity", `${j.proposal.who} — booked automatically when the client accepts`], ["Message", `“${j.proposal.message}”`], ["Client", "Notified in the Client app · reminder 24 h before the deadline"]]} />
          <div className="mt-3 flex flex-wrap justify-end gap-2"><Btn variant="danger" onClick={() => { jobActions.withdraw(j.id); toast("Proposal withdrawn", "warn"); }}>Withdraw proposal</Btn><Btn onClick={() => open({ kind: "propose" })}>Edit proposal…</Btn><Btn onClick={() => toast("Reminder sent to the client")}>Remind client</Btn></div>
          <p className="mt-2 text-[11px] text-muted">Accept → booked with the held partner (no extra HQ step). Decline → back to Requested with the client’s reason and new times. No reply → stays Requested and a “Call the client” task is created.</p>
        </Card>
      )}

      {(j.status === "offered" || j.status === "accepted" || j.status === "assigned" || j.status === "in_progress") && (
        <Card title={j.delivery === "contractor" ? "Delivery · offer to contractor" : "Delivery · internal"}>
          <SummaryList items={[
            ["Agreed time", j.scheduled ? `${longDate(j.scheduled)} (${j.origin === "plan" ? "from plan" : "client-approved"})` : "—"],
            ...(j.delivery === "contractor" ? [["Contractor", `${j.contractor} · ${j.status === "offered" ? "offer open · expires 09-25 12:00" : "accepted"}`] as [string, string]] : []),
            ["Technician", j.technician ? `${j.technician} · ${j.techAck?.status === "accepted" ? `accepted ✓ ${j.techAck.at ?? ""}` : j.techAck?.status === "cant_make" ? "can’t make this time" : "awaiting acceptance"}` : "not assigned yet"],
          ]} />
          {j.techAck?.status === "cant_make" && <div className="mt-3"><Banner tone="warn" action={<span className="flex gap-2">{j.delivery === "internal" && <Btn size="sm" onClick={() => open({ kind: "reason", what: "reassign" })}>Reassign…</Btn>}<Btn size="sm" variant="primary" onClick={() => open({ kind: "propose" })}>Propose another time…</Btn></span>}>{j.technician}: “{j.techAck.reason}”{j.techAck.alt ? ` · could do ${j.techAck.alt}` : ""}. Any new time needs the client’s approval.</Banner></div>}
        </Card>
      )}

      {j.status === "offered" && j.partnerProposal?.status === "pending" && (
        <Card tone="warn" title={`⇄ ${j.contractor} proposes a different time`} action={<Badge tone="warn">Needs client approval</Badge>}>
          <SummaryList items={[["Agreed with client", <s key="s" className="text-muted">{fmt(j.scheduled)}</s>], ["Proposed by contractor", <b key="b">{longDate(j.partnerProposal.slot)} · {j.partnerProposal.tech}</b>], ["Reason", `“${j.partnerProposal.reason}” — ${j.contractor}, ${j.partnerProposal.sentAt}`]]} />
          <div className="mt-3 flex flex-wrap justify-end gap-2"><Btn onClick={() => toast("Use “+ New job › Offer” to choose another contractor (demo)")}>Offer to another contractor…</Btn><Btn onClick={() => { jobActions.keepTime(j.id); toast("Agreed time kept"); }}>Keep {fmt(j.scheduled)} · ask again</Btn><Btn variant="primary" onClick={() => { jobActions.forwardPartner(j.id, "09-25 12:00"); toast("Sent to the client for approval"); }}>Send to client for approval</Btn></div>
          <p className="mt-2 text-[11px] text-muted">An agreed time never changes without the client’s OK. The offer stays reserved for {j.contractor} while the client decides.</p>
        </Card>
      )}

      {j.status === "submitted" && (
        <Card title="Work report · version 1" sub={`Submitted by ${j.technician}`}>
          <SummaryList items={[["Filter cleaned", "Normal"], ["Drain line", "Normal"], ["Refrigerant pressure", "Normal · 412 kPa"], ["Airflow", "Needs attention · reduced on Low"]]} />
          {j.delivery === "contractor" ? <p className="mt-2 text-xs text-muted">Outsourced job — the contractor reviews first; HQ reviews only on escalation.</p> : <>
            <p className="mt-2 text-xs text-ok">You did not contribute to this report, so you can review it (self-approval is rejected).</p>
            <div className="mt-3 flex flex-wrap gap-2"><Btn variant="primary" onClick={() => toast("Report accepted — job completed")}>Accept report</Btn><Btn onClick={() => open({ kind: "reason", what: "rework" })}>Return for rework…</Btn></div>
          </>}
        </Card>
      )}

      <Card title="History"><Timeline items={j.history.slice(-6).map((h) => ({ time: h.at, title: h.text }))} /></Card>
      <Card title="Costs" action={<Btn size="sm" onClick={() => toast("Cost line added")}>+ Add cost line</Btn>}>
        <SummaryList items={[["Estimate", "Labour 80.00 MYR · Parts 25.00 MYR"], ["Actual", "80.00 MYR + 6.20 USD"]]} />
        <p className="mt-1 text-[11px] text-muted">Totals are shown per currency — never converted.</p>
      </Card>
    </div>
  );
}

function BookModal({ j, slot, onClose }: { j: Job; slot: Slot; onClose: () => void }) {
  const toast = useToast();
  const f = fitFor(slot);
  const [d, setD] = useState<"internal" | "contractor">(f.fits === "both" || f.fits === "internal" ? "internal" : "contractor");
  return (
    <Modal open onClose={onClose} title={`Book ${j.id}`} footer={<><Btn onClick={onClose}>← Back</Btn><Btn variant="primary" onClick={() => { jobActions.book(j.id, slot, d, d === "internal" ? "tech-internal-a" : "contractor-a"); toast(d === "internal" ? "Assigned — technician notified" : "Offer sent to contractor-a"); onClose(); }}>{d === "internal" ? "Assign" : "Send offer"}</Btn></>}>
      <p className="text-xs text-muted">{j.type} · {j.unit} · {j.customer} · {j.origin === "plan" ? "Periodic plan" : "Client request"}</p>
      <Field label="Visit time · from the client" hint="Cannot be edited here. Need another time? Close and use “Propose another time…” — the client must accept it."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {longDate(slot)}</div></Field>
      <Field label="Who does the work" hint={f.fits === "contractor" ? "No qualified HQ technician is free at this time — offer to a contractor." : undefined}><Choice value={d} onChange={setD} options={[...(f.fits === "contractor" ? [] : [{ id: "internal" as const, label: "Assign internally" }]), { id: "contractor" as const, label: "Offer to contractor" }]} /></Field>
      {d === "internal" ? (
        <Field label="Technician" hint="Only members with valid qualifications for the whole slot are listed"><Select><option>tech-internal-a · Available</option></Select></Field>
      ) : (
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <Field label="Contractor organization"><Select><option>contractor-a</option><option>contractor-b</option></Select></Field>
          <Field label="Offer expires at"><Input defaultValue="2026-09-25 12:00" /></Field>
          <Field label="Access valid from"><Input defaultValue={`${slot.date} 00:00`} /></Field>
        </div>
      )}
      <Banner>{d === "internal" ? "The technician must accept the assignment (受領)." : "contractor-a can Accept, Decline or Propose another time — a different time always goes back to the client."}</Banner>
    </Modal>
  );
}

function ProposeModal({ j, onClose }: { j: Job; onClose: () => void }) {
  const toast = useToast();
  const [date, setDate] = useState("2026-10-01");
  const [win, setWin] = useState("10:00–12:00");
  const [who, setWho] = useState("contractor-a · tech-external-a");
  const [msg, setMsg] = useState("All qualified technicians are booked Sep 28–30. The earliest free slot is Thursday morning. Sorry for the wait.");
  const [tried, setTried] = useState(false);
  const pre = j.preferred.some((s) => s.date === date && s.win === win);
  return (
    <Modal open onClose={onClose} title="Propose another time to the client" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!msg.trim() || pre) return; jobActions.propose(j.id, { date, win }, who, msg.trim(), "09-24 18:00"); toast("Proposal sent — waiting for the client"); onClose(); }}>Send proposal</Btn></>}>
      <p className="text-xs text-muted">{j.id} · {j.customer} · {j.unit}</p>
      <div className="rounded-xl border-2 border-primary bg-primary-soft/50 p-3">
        <div className="mb-2 flex items-center justify-between"><b className="text-[13px] text-primary">Option A</b><Badge tone="ok">Fits · capacity held</Badge></div>
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
          <Field label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
          <Field label="Window"><Select value={win} onChange={(e) => setWin(e.target.value)}>{["09:00–11:00", "10:00–12:00", "14:00–16:00"].map((w) => <option key={w}>{w}</option>)}</Select></Field>
        </div>
        <Field label="Who does it" className="mt-2"><Select value={who} onChange={(e) => setWho(e.target.value)}><option>contractor-a · tech-external-a</option><option>tech-internal-a</option></Select></Field>
        {pre && <p className="mt-1 text-xs text-crit">✕ This is one of the client’s own times — book it directly instead.</p>}
      </div>
      <Field label="Message to the client · required" error={tried && !msg.trim() ? "A message is required" : undefined}><Textarea value={msg} onChange={(e) => setMsg(e.target.value)} /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Reply by"><Select><option>09-24 18:00 (48 h)</option><option>09-23 18:00 (24 h)</option></Select></Field><Field label="If no reply"><Select><option>Keep “Requested” and call the client</option></Select></Field></div>
      <Banner>The client sees “Time proposed” with Accept / Decline. Accept → booked automatically with the held partner. Decline → the client sends new times. Held capacity is released on decline or timeout.</Banner>
    </Modal>
  );
}

function NewJobModal({ onClose, onDone }: { onClose: () => void; onDone: (id: string) => void }) {
  const toast = useToast();
  const [sym, setSym] = useState("");
  const [slots, setSlots] = useState<Slot[]>([{ date: "2026-10-05", win: "10:00–12:00" }, { date: "2026-10-06", win: "14:00–16:00" }, { date: "2026-10-07", win: "09:00–11:00" }]);
  const [tried, setTried] = useState(false);
  const err = tried ? preferredError(slots) : undefined;
  return (
    <Modal open onClose={onClose} title="New maintenance job — on behalf of a customer" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (sym.trim().length < 10 || preferredError(slots)) return; const id = jobActions.create({ unitId: "unit-online-rto", unit: "Bedroom AC", loc: "Home A › 1F › Bedroom", type: "Reactive", symptom: sym.trim(), preferred: slots }); toast(`${id} created as Requested`); onDone(id); }}>Create as requested</Btn></>}>
      <Field label="Customer · unit"><Select><option>customer-a · Bedroom AC (unit-online-rto)</option></Select></Field>
      <Field label="Symptom / scope (10+ characters)" error={tried && sym.trim().length < 10 ? "Enter at least 10 characters" : undefined}><Textarea value={sym} onChange={(e) => setSym(e.target.value)} placeholder="Customer called: unit leaks water at night." /></Field>
      <PreferredSlotsInput value={slots} onChange={setSlots} error={err} label="Customer’s preferred times (asked on the phone)" hint="Same rule as client requests: book one of these, or propose another time for the customer to accept." />
      <p className="text-[11px] text-muted">Periodic visits are created from Plans, not here.</p>
    </Modal>
  );
}

function ReasonModal({ what, j, onClose }: { what: "rework" | "hold" | "reassign"; j: Job; onClose: () => void }) {
  const toast = useToast();
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  const title = what === "rework" ? "Return for rework" : what === "hold" ? "Put on hold (IR56)" : `Reassign ${j.id}`;
  return (
    <Modal open onClose={onClose} title={title} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!r.trim()) return; if (what === "reassign") jobActions.book(j.id, j.scheduled!, "internal", "tech-internal-b"); toast(what === "rework" ? "Returned for rework" : what === "hold" ? "Job put on hold" : "Reassigned — tech-internal-b must accept", "warn"); onClose(); }}>Confirm</Btn></>}>
      {what === "reassign" && <Field label="New technician (same agreed time)"><Select><option>tech-internal-b · Available</option></Select></Field>}
      <Field label="Reason (required)" error={tried && !r.trim() ? "A reason is required" : undefined}><Textarea value={r} onChange={(e) => setR(e.target.value)} /></Field>
    </Modal>
  );
}

const contractors = [
  { id: "contractor-a", area: "KL, Selangor · 2 technicians", st: "Active" },
  { id: "contractor-b", area: "Penang · 3 technicians", st: "Active" },
  { id: "contractor-c", area: "Johor · offers suspended 09-20", st: "Suspended" },
];
function Contractors() {
  const toast = useToast();
  const [sel, setSel] = useState(contractors[0]);
  const [list, setList] = useState(contractors);
  return (
    <div className="split-rev">
      <Card title="Contractors" action={<Btn size="sm" onClick={() => toast("Add contractor (demo)")}>+ Add contractor</Btn>} className="self-start">
        <div className="flex flex-col gap-2">{list.map((c) => <ListRow key={c.id} selected={sel.id === c.id} onClick={() => setSel(c)}><div className="min-w-0 flex-1"><b className="text-[13px]">{c.id}</b><div className="text-[11px] text-muted">{c.area}</div></div><Badge tone={c.st === "Active" ? "ok" : "crit"}>{c.st}</Badge></ListRow>)}</div>
        <p className="mt-2 text-[11px] text-muted">Contractor companies are partners, not users — their staff accounts are managed in Access & roles.</p>
      </Card>
      <div className="flex min-w-0 flex-col gap-4">
        <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
          <Kpi label="Offer acceptance" value="92 %" sub="23 / 25 offers · 90 d" /><Kpi label="Arrival in window" value="96 %" sub="target ≥ 95 %" /><Kpi label="First-time accept" value="84 %" tone="warn" sub="below 90 % target" /><Kpi label="Customer rating" value="4.6 ★" sub="12 ratings (Client)" /><Kpi label="Time changes asked" value="2" sub="both approved by clients" />
        </div>
        <div className="split">
          <Card title={sel.id} action={<span className="flex gap-2"><Btn size="sm">Edit</Btn><Btn size="sm" variant="danger" onClick={() => { setList((l) => l.map((c) => (c.id === sel.id ? { ...c, st: c.st === "Active" ? "Suspended" : "Active" } : c))); toast(sel.st === "Active" ? "Offers suspended — existing jobs continue" : "Offers resumed", "warn"); }}>{list.find((c) => c.id === sel.id)?.st === "Active" ? "Suspend offers" : "Resume offers"}</Btn></span>}>
            <SummaryList items={[["Registration", "SSM 202301012345 (fictional)"], ["Service areas", "Kuala Lumpur, Selangor"], ["Delegation period", "2026-04-01 – 2027-03-31"], ["Insurance", "valid to 2027-01-31"]]} />
            <p className="mt-3 text-[13px] font-bold">Rate card rc-{sel.id} v3 (from 2026-07-01)</p>
            <SummaryList items={[["Periodic inspection (per unit)", "380.00 MYR"], ["Repair — base visit", "450.00 MYR + parts"], ["Emergency call-out (< 4 h)", "650.00 MYR"], ["Rework deduction (2nd return)", "− 120.00 MYR"]]} />
          </Card>
          <Card title="Technicians & certificates">
            <SummaryList items={[["tech-external-a", <Badge key="1" tone="warn">1 expiring · 10-15</Badge>], ["tech-external-a2", <Badge key="2" tone="ok">Valid</Badge>], ["tech-external-b", <Badge key="3" tone="muted">Expired</Badge>]]} />
            <div className="mt-3"><Banner>Certificates uploaded by the contractor need HQ verification before they count.</Banner></div>
            <div className="mt-3"><Btn size="sm" disabled>Verify uploads (0)</Btn></div>
          </Card>
        </div>
      </div>
    </div>
  );
}

function Sla() {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        <Kpi label="Response within SLA" value="94 %" sub="last 90 days" /><Kpi label="Booked on a preferred time" value="71 %" sub="29 % needed a proposal" /><Kpi label="Proposal accepted first time" value="80 %" sub="4 / 5" /><Kpi label="Breaches" value="2" tone="crit" sub="this month" />
      </div>
      <Card title="SLA scorecard by customer" action={<Btn size="sm">Edit SLA targets</Btn>}>
        <DataTable rowKey={(r) => r.c} rows={[{ c: "customer-a", plan: "RTO Standard", resp: "8 h", met: "96 %", pref: "75 %", br: 1 }, { c: "customer-b", plan: "Business Plus", resp: "4 h", met: "91 %", pref: "66 %", br: 1 }]} cols={[
          { key: "c", label: "Customer", render: (r) => <b>{r.c}</b> }, { key: "p", label: "Contract plan", render: (r) => r.plan }, { key: "r", label: "Response target", render: (r) => r.resp }, { key: "m", label: "Met", render: (r) => r.met }, { key: "pr", label: "On preferred time", render: (r) => r.pref, hideBelow: "sm" }, { key: "b", label: "Breaches", render: (r) => (r.br ? <Badge tone="crit">{r.br}</Badge> : "0") },
        ]} />
        <p className="mt-2 text-[11px] text-muted">Targets apply to jobs created after the target’s effective date. Waiting for a client reply on a proposed time does not count against response SLA.</p>
      </Card>
    </div>
  );
}
