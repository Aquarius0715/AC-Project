"use client";

import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, DataTable, Field, Input, ListRow, Modal, Page, PageHead, Select, SummaryList, Tabs, Textarea, Timeline, UtilBar, cx, useToast } from "@/components/ui";
import { JobStatusBadge, OriginBadge, PreferredSlotsInput, SlotLine, preferredError } from "@/components/JobBits";
import { Job, JobStatus, Slot, fitFor, fmt, jobActions, longDate, useJobs } from "@/lib/jobs";
import { units } from "@/lib/client";

type StatusTab = "all" | "reply" | "requested" | "scheduled" | "progress" | "completed" | "cancelled";
const tabOf = (s: JobStatus): StatusTab =>
  s === "time_proposed" ? "reply" : s === "requested" ? "requested" : s === "offered" || s === "accepted" || s === "assigned" ? "scheduled" : s === "completed" ? "completed" : s === "cancelled" ? "cancelled" : "progress";
const ORDER: JobStatus[] = ["time_proposed", "requested", "in_progress", "rework_requested", "submitted", "offered", "accepted", "assigned", "on_hold", "completed", "cancelled"];
const blank = (): Slot[] => [{ date: "2026-09-28", win: "14:00–16:00" }, { date: "2026-09-29", win: "10:00–12:00" }, { date: "2026-09-30", win: "09:00–11:00" }];

function scheduleLine(j: Job) {
  if (j.status === "time_proposed" && j.proposal) return { icon: "⇄", text: `${j.proposal.by === "contractor" ? "Service partner asks for" : "HQ proposed"} ${fmt(j.proposal.slot)} — reply by ${j.proposal.replyBy}`, warn: true };
  if (j.status === "requested") return { icon: "◷", text: `Preferred ${fmt(j.preferred[0])} · +${Math.max(0, j.preferred.length - 1)} more (not confirmed yet)` };
  if (j.status === "completed") return { icon: "◷", text: `Completed ${fmt(j.scheduled)} · report available` };
  if (j.status === "cancelled") return { icon: "◷", text: "Cancelled" };
  return { icon: "◷", text: `${j.origin === "plan" ? "Scheduled" : "Confirmed"} ${fmt(j.scheduled)}${j.technician && j.techAck?.status === "accepted" ? ` · ${j.technician}` : j.status === "offered" ? " · booking the technician" : ""}` };
}

export default function Maintenance({ searchParams }: { searchParams: Promise<{ jobId?: string; tab?: string }> }) {
  const sp = use(searchParams);
  const toast = useToast();
  const all = useJobs().filter((j) => j.customer === "customer-a" && /^job-c/.test(j.id));
  const [page, setPage] = useState<"requests" | "filters">(sp.tab === "filter-care" || sp.tab === "filters" ? "filters" : "requests");
  const [tab, setTab] = useState<StatusTab>("all");
  const [origin, setOrigin] = useState<"all" | "request" | "plan">("all");
  const [selId, setSelId] = useState<string | null>(sp.jobId ?? null);
  const [modal, setModal] = useState<null | "new" | "decline" | "other" | "rate" | "problem" | "cancel" | "note">(null);
  const jobs = [...all].sort((a, b) => ORDER.indexOf(a.status) - ORDER.indexOf(b.status));
  const shown = jobs.filter((j) => (tab === "all" || tabOf(j.status) === tab) && (origin === "all" || j.origin === origin));
  const count = (t: StatusTab) => jobs.filter((j) => t === "all" || tabOf(j.status) === t).length;
  const sel = all.find((j) => j.id === selId) ?? null;
  const replyCount = count("reply");

  return (
    <Page>
      <Tabs value={page} onChange={setPage} tabs={[{ id: "requests", label: "My requests" }, { id: "filters", label: "Filter care" }]} />
      {page === "filters" ? <FilterCare onRequest={() => { setPage("requests"); setModal("new"); }} /> : (
        <>
          {replyCount > 0 && <Banner tone="warn" icon="⇄" action={<Btn size="sm" variant="primary" onClick={() => { setTab("reply"); setSelId(jobs.find((j) => j.status === "time_proposed")?.id ?? null); }}>Review</Btn>}>A new visit time needs your reply ({replyCount}). Nothing is booked until you accept.</Banner>}
          {(() => { const done = all.find((j) => j.status === "completed" && j.id === "job-c02"); return done && !rated.has(done.id) ? <Banner tone="ok" icon="✓" action={<span className="flex gap-2"><Btn size="sm" onClick={() => { setSelId(done.id); setModal("problem"); }}>Report a problem</Btn><Btn size="sm" variant="primary" onClick={() => { setSelId(done.id); setModal("rate"); }}>Confirm & rate</Btn></span>}><b>Work finished — job-c02 · Living room AC · Sep 8.</b> Is everything working? Your confirmation closes the job.</Banner> : null; })()}
          <PageHead title="My requests" sub={`${jobs.length} requests · sorted by status (business order)`} action={<Btn variant="primary" onClick={() => setModal("new")}>+ New maintenance request</Btn>} />
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Tabs value={tab} onChange={setTab} tabs={([["all", "All"], ["reply", "Needs your reply"], ["requested", "Requested"], ["scheduled", "Scheduled"], ["progress", "In progress"], ["completed", "Completed"], ["cancelled", "Cancelled"]] as [StatusTab, string][]).filter(([t]) => t !== "reply" || replyCount > 0).map(([id, label]) => ({ id, label, count: count(id) }))} />
            <Select aria-label="Origin" value={origin} onChange={(e) => setOrigin(e.target.value as typeof origin)} className="w-auto"><option value="all">Origin: All</option><option value="request">Origin: Client request</option><option value="plan">Origin: Periodic plan</option></Select>
          </div>
          <div className={cx("grid gap-4", sel ? "grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" : "grid-cols-1")}>
            <div className="flex min-w-0 flex-col gap-2">
              {shown.map((j) => {
                const l = scheduleLine(j);
                return (
                  <ListRow key={j.id} selected={sel?.id === j.id} onClick={() => setSelId(j.id)}>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2"><span className="text-xs font-semibold text-muted">{j.id}</span><b>{j.unit}</b><OriginBadge origin={j.origin} />{j.followUpOf && <FollowUpBadge j={j} />}</div>
                      <div className="text-xs text-muted">{j.loc} · {j.origin === "plan" ? "Periodic inspection" : j.type}</div>
                      <div className={cx("text-xs", l.warn && "font-semibold text-warn")}>{l.icon} {l.text}</div>
                    </div>
                    <JobStatusBadge s={j.status} /><span className="text-muted">›</span>
                  </ListRow>
                );
              })}
              {shown.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">No requests in this filter.</p>}
              <p className="text-[11px] text-muted">“Scheduled” = a time is confirmed: one of your 3 preferred times, or a new time you accepted. Periodic plan visits are scheduled from your maintenance plan.</p>
            </div>
            {sel && <Detail j={sel} onClose={() => setSelId(null)} open={setModal} />}
          </div>
        </>
      )}
      <NewRequest open={modal === "new"} onClose={() => setModal(null)} onDone={(id) => { setModal(null); setSelId(id); toast(`Request ${id} sent with 3 preferred times`); }} />
      {sel && <DeclineModal open={modal === "decline"} j={sel} onClose={() => setModal(null)} onDone={() => { setModal(null); toast("Declined — HQ will look again", "warn"); }} />}
      {sel && <OtherTimeModal open={modal === "other"} j={sel} onClose={() => setModal(null)} onDone={() => { setModal(null); toast("Asked HQ for another time"); }} />}
      {sel && <RateModal open={modal === "rate"} onClose={() => setModal(null)} onDone={() => { rated.add(sel.id); setModal(null); toast("Thanks — job confirmed and rated"); }} />}
      {sel && <ProblemModal open={modal === "problem"} j={sel} onClose={() => setModal(null)} onDone={(id) => { rated.add(sel.id); setModal(null); setSelId(id); toast(`Problem reported — follow-up ${id} is under HQ review`, "warn"); }} />}
      {sel && <NoteModal open={modal === "note"} j={sel} onClose={() => setModal(null)} onDone={() => { setModal(null); toast("Note sent — HQ, the service partner and the technician can read it"); }} />}
      {sel && <CancelModal open={modal === "cancel"} j={sel} onClose={() => setModal(null)} onDone={() => { setModal(null); toast("Request cancelled", "warn"); }} />}
    </Page>
  );
}
const rated = new Set<string>();

function Detail({ j, onClose, open }: { j: Job; onClose: () => void; open: (m: "decline" | "other" | "rate" | "problem" | "cancel" | "note") => void }) {
  const toast = useToast();
  const p = j.proposal;
  return (
    <Card title={j.id} sub={`${j.unit} — ${j.origin === "plan" ? "periodic inspection" : j.symptom.slice(0, 40)}`} action={<div className="flex items-center gap-2"><JobStatusBadge s={j.status} /><Btn size="sm" variant="ghost" onClick={onClose}>✕</Btn></div>} className="self-start">
      <SummaryList items={[
        ["Unit", `${j.unit} · ${j.loc}`],
        ["Origin · type", j.origin === "plan" ? `Periodic plan · ${j.planId} · visit ${j.planVisit}` : `Client request · ${j.type}`],
        ...(j.followUpOf ? [["Follow-up of", <span key="f" className="flex flex-wrap items-center gap-2">{j.followUpOf}<FollowUpBadge j={j} /></span>] as [string, React.ReactNode]] : []),
        ...(j.origin === "request" ? [["Request", `“${j.symptom}”`] as [string, string]] : []),
        ...(j.scheduled && j.status !== "time_proposed" ? [[j.origin === "plan" ? "Scheduled" : "Confirmed time", `${longDate(j.scheduled)}${j.origin === "plan" ? " · set by HQ from your plan" : ""}`] as [string, string]] : []),
        ...(j.technician && j.techAck?.status === "accepted" ? [["Technician", `${j.technician}${j.delivery === "contractor" ? ` (${j.contractor})` : " · HQ"}`] as [string, string]] : []),
      ]} />
      {j.origin === "request" && j.preferred.length > 0 && j.status !== "completed" && j.status !== "cancelled" && (
        <div className="mt-3 flex flex-col gap-1.5">
          <b className="text-[13px]">Your preferred times{j.round > 1 ? ` (round ${j.round})` : ""}</b>
          {j.preferred.map((s, i) => {
            const chosen = j.scheduled && s.date === j.scheduled.date && s.win === j.scheduled.win;
            const none = j.status === "time_proposed" && fitFor(s).fits === "none";
            return <SlotLine key={i} i={i} slot={s} struck={none} note={chosen ? "✓ booked" : none ? "Not available — no qualified technician free" : undefined} />;
          })}
        </div>
      )}
      {j.declined && j.status === "requested" && <div className="mt-3"><Banner tone="warn">You declined the proposed time ({j.declined.reason}). HQ is looking at your new times.</Banner></div>}
      {j.status === "time_proposed" && p && (
        <div className="mt-3 rounded-xl border border-[#f5c473] bg-[#fff8ec] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2"><b className="text-[14px]">{p.by === "contractor" ? "Your service partner asks for another time (via HQ)" : "HQ proposes a new time"}</b><Badge tone="warn">Reply by {p.replyBy}</Badge></div>
          <div className="my-2 text-xl font-bold">{longDate(p.slot)}</div>
          <SummaryList items={[["Who comes", p.by === "contractor" || p.who.startsWith("contractor") ? `Partner technician from ${p.who.split(" ")[0]} (name shown once assigned)` : "HQ technician (name shown once confirmed)"], ["Message", `“${p.message}”`]]} />
          <div className="mt-3 flex flex-wrap items-center justify-end gap-2"><span className="mr-auto text-[11px] text-warn">Accepting books this time. Declining keeps your request open.</span><Btn variant="danger" onClick={() => open("decline")}>Decline…</Btn><Btn variant="primary" onClick={() => { jobActions.accept(j.id); toast("Time accepted — booking the technician"); }}>✓ Accept this time</Btn></div>
        </div>
      )}
      {j.origin === "plan" && (j.status === "assigned" || j.status === "offered") && (
        <div className="mt-3 rounded-xl border border-[#cdb8fa] bg-[#f7f2ff] p-4 text-[13px]">
          <b className="text-[#6d28d9]">↻ Booked from your maintenance plan</b>
          <p className="mt-1">You don’t need to request periodic visits — HQ schedules each one and tells you 1 month ahead. If you can’t be home, ask for another time (3 options) up to 48 h before the visit.</p>
          <div className="mt-2 flex justify-end"><Btn onClick={() => open("other")}>Request another time…</Btn></div>
        </div>
      )}
      {j.status === "completed" && (
        <>
          <p className="mt-3 text-[13px] font-bold">Completed report <span className="text-xs font-normal text-muted">Accepted by HQ (visible after review)</span></p>
          <SummaryList items={[["Filter", "Cleaned — normal"], ["Evaporator coil", "Normal"], ["Drain pipe", "Attention — partly blocked, cleared"], ["Parts", "None"], ["Supply air", "12.5 °C"], ["Return air", "25.0 °C"]]} />
          <p className="mt-2 text-xs text-muted">Photos (2) · ⎙ service-report.pdf</p>
          {!rated.has(j.id) && <div className="mt-3 flex justify-end gap-2"><Btn onClick={() => open("problem")}>Report a problem</Btn><Btn variant="primary" onClick={() => open("rate")}>Confirm & rate</Btn></div>}
        </>
      )}
      {j.status !== "completed" && j.status !== "cancelled" && (
        <>
          <p className="mt-4 text-[13px] font-bold">Notes to coordinator</p>
          {(j.notes ?? []).length === 0 ? <p className="text-xs text-muted">No notes yet.</p> : <ul className="mt-1 flex flex-col gap-1 text-[13px]">{(j.notes ?? []).map((n, i) => <li key={i}><span className="text-muted">You · {n.at} — </span>“{n.text}”</li>)}</ul>}
        </>
      )}
      <p className="mt-4 text-[13px] font-bold">History</p>
      <Timeline items={j.history.slice(-5).map((h) => ({ time: h.at, title: h.text }))} />
      <div className="mt-3 flex flex-wrap justify-between gap-2">
        {(j.status === "requested" || j.status === "time_proposed") && j.origin === "request" ? <Btn size="sm" variant="danger" onClick={() => open("cancel")}>Cancel request</Btn> : <span />}
        {j.status !== "completed" && j.status !== "cancelled" && <Btn size="sm" onClick={() => open("note")}>+ Add note</Btn>}
      </div>
      <p className="mt-2 text-[11px] text-muted">Cancel is possible until a time is booked. Any time outside your preferred times is booked only after you accept it. Notes can’t change the time or technician.</p>
    </Card>
  );
}

function NewRequest({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (id: string) => void }) {
  const [unitId, setUnitId] = useState("unit-bedroom-2");
  const [type, setType] = useState<"Reactive" | "Preventive">("Reactive");
  const [sym, setSym] = useState("");
  const [slots, setSlots] = useState<Slot[]>(blank());
  const [tried, setTried] = useState(false);
  const symErr = tried && (sym.trim().length < 10 || sym.length > 2000) ? "Symptoms must be 10–2000 characters" : undefined;
  const slotErr = tried ? preferredError(slots) : undefined;
  const submit = () => {
    setTried(true);
    if (sym.trim().length < 10 || sym.length > 2000 || preferredError(slots)) return;
    const u = units.find((x) => x.id === unitId)!;
    const id = jobActions.create({ unitId, unit: u.name, loc: u.loc, type, symptom: sym.trim(), preferred: slots });
    setTried(false); setSym(""); setSlots(blank()); onDone(id);
  };
  return (
    <Modal open={open} onClose={onClose} title="New maintenance request" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={submit}>Submit request</Btn></>}>
      <Field label="Unit"><Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{units.map((u) => <option key={u.id} value={u.id}>{u.name} · {u.loc}</option>)}</Select></Field>
      <Field label="Type"><Choice value={type} onChange={setType} options={[{ id: "Reactive", label: "Reactive" }, { id: "Preventive", label: "Preventive" }]} /></Field>
      <Field label="Symptoms (10–2000 characters)" error={symErr} hint={`${sym.length} / 2000`}><Textarea value={sym} onChange={(e) => setSym(e.target.value)} placeholder="Cold air is weak and there is water dripping from the indoor unit." /></Field>
      <PreferredSlotsInput value={slots} onChange={setSlots} error={slotErr} />
      <Banner>Nothing is booked yet — you will see “Scheduled” once a time is confirmed. Any time outside your 3 options needs your approval.</Banner>
    </Modal>
  );
}

function DeclineModal({ open, j, onClose, onDone }: { open: boolean; j: Job; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState<"not_home" | "too_late" | "other">("not_home");
  const [comment, setComment] = useState("");
  const [give, setGive] = useState(true);
  const [slots, setSlots] = useState<Slot[]>([{ date: "2026-10-02", win: "14:00–16:00" }, { date: "2026-10-05", win: "10:00–12:00" }, { date: "2026-10-06", win: "14:00–16:00" }]);
  const [tried, setTried] = useState(false);
  const err = tried && give ? preferredError(slots) : undefined;
  const label = { not_home: "I’m not at home then", too_late: "Too late for me", other: "Other" }[reason];
  return (
    <Modal open={open} onClose={onClose} title="Decline the proposed time" footer={<><Btn onClick={onClose}>Back</Btn><Btn variant="primary" onClick={() => { setTried(true); if (give && preferredError(slots)) return; jobActions.decline(j.id, label, comment.trim(), give ? slots : []); onDone(); }}>{give ? "Decline & send new times" : "Decline"}</Btn></>}>
      <p className="text-xs text-muted">{j.id} · {j.unit} · proposed {j.proposal ? longDate(j.proposal.slot) : ""}</p>
      <Field label="Reason"><Choice value={reason} onChange={setReason} options={[{ id: "not_home", label: "I’m not at home then" }, { id: "too_late", label: "Too late for me" }, { id: "other", label: "Other" }]} /></Field>
      <Field label="Comment to HQ (optional)"><Input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Away on Oct 1. Any weekday afternoon next week works." /></Field>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="h-4 w-4 accent-[#005bea]" checked={give} onChange={(e) => setGive(e.target.checked)} />Give 3 new preferred times (recommended)</label>
      {give && <PreferredSlotsInput value={slots} onChange={setSlots} error={err} label="New preferred times" hint="Declining never cancels the request — use “Cancel request” for that." />}
      <Banner>HQ looks again and either books one of your times or sends another proposal.</Banner>
    </Modal>
  );
}

function OtherTimeModal({ open, j, onClose, onDone }: { open: boolean; j: Job; onClose: () => void; onDone: () => void }) {
  const [slots, setSlots] = useState<Slot[]>([{ date: "2026-12-09", win: "09:00–11:00" }, { date: "2026-12-10", win: "14:00–16:00" }, { date: "2026-12-11", win: "09:00–11:00" }]);
  const [comment, setComment] = useState("");
  const [tried, setTried] = useState(false);
  const err = tried ? preferredError(slots) : undefined;
  return (
    <Modal open={open} onClose={onClose} title="Request another time" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (preferredError(slots)) return; jobActions.requestOther(j.id, slots, comment.trim()); onDone(); }}>Send to HQ</Btn></>}>
      <p className="text-xs text-muted">{j.id} · periodic visit {fmt(j.scheduled)} · up to 48 h before the visit</p>
      <PreferredSlotsInput value={slots} onChange={setSlots} error={err} />
      <Field label="Comment (optional)"><Input value={comment} onChange={(e) => setComment(e.target.value)} /></Field>
    </Modal>
  );
}

function RateModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const [stars, setStars] = useState(4);
  const [tags, setTags] = useState<string[]>(["On time"]);
  const TAGS = ["On time", "Clean work", "Explained clearly", "Polite"];
  return (
    <Modal open={open} onClose={onClose} title="Confirm & rate job-c02" footer={<><Btn onClick={onClose}>Not now</Btn><Btn variant="primary" onClick={onDone}>Confirm & submit</Btn></>}>
      <Field label="How was the service? (required)"><div className="flex gap-1 text-2xl">{[1, 2, 3, 4, 5].map((n) => <button key={n} aria-label={`${n} stars`} onClick={() => setStars(n)} className={n <= stars ? "text-[#f59e0b]" : "text-line"}>★</button>)}</div></Field>
      <Field label="What went well? (optional)"><div className="flex flex-wrap gap-1.5">{TAGS.map((t) => <button key={t} aria-pressed={tags.includes(t)} onClick={() => setTags((s) => (s.includes(t) ? s.filter((x) => x !== t) : [...s, t]))} className={cx("rounded-full border px-3 py-1 text-xs font-semibold", tags.includes(t) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{t}</button>)}</div></Field>
      {stars <= 2 && <Banner tone="warn">What went wrong? You can also “Report a problem” instead.</Banner>}
      <Field label="Comment (optional)"><Textarea placeholder="Technician was quick and tidy." /></Field>
      <p className="text-[11px] text-muted">You can edit the rating for 7 days. Unconfirmed jobs are confirmed automatically after 7 days.</p>
    </Modal>
  );
}

const PROBLEM: Record<"same" | "new" | "incomplete" | "other", string> = { same: "Same problem again", new: "New damage", incomplete: "Work not completed", other: "Other" };
function ProblemModal({ open, j, onClose, onDone }: { open: boolean; j: Job; onClose: () => void; onDone: (id: string) => void }) {
  const [reason, setReason] = useState<"same" | "new" | "incomplete" | "other">("same");
  const [d, setD] = useState("");
  const [visit, setVisit] = useState("2026-09-25");
  const [tried, setTried] = useState(false);
  const [err, setErr] = useState<string | undefined>();
  return (
    <Modal open={open} onClose={onClose} title={`Report a problem — ${j.id}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (d.trim().length < 10 || d.length > 2000) return; const r = jobActions.reportProblem(j.id, PROBLEM[reason], d.trim(), visit); if ("error" in r) { setErr(r.error); return; } setD(""); setTried(false); onDone(r.id!); }}>Send report</Btn></>}>
      <Field label="What happened?"><Choice value={reason} onChange={setReason} options={[{ id: "same", label: "Same problem again" }, { id: "new", label: "New damage" }, { id: "incomplete", label: "Work not completed" }, { id: "other", label: "Other" }]} /></Field>
      <Field label="Details (10–2000 characters)" error={tried && (d.trim().length < 10 || d.length > 2000) ? "Describe the problem (10–2000 characters)" : undefined}><Textarea value={d} onChange={(e) => setD(e.target.value)} /></Field>
      <Field label="Preferred visit (optional)"><Input type="date" value={visit} onChange={(e) => setVisit(e.target.value)} /></Field>
      {err && <Banner tone="crit">{err}</Banner>}
      <Banner>Creates a follow-up request linked to {j.id} (“Under HQ review”). HQ decides within 1 business day: rework (free) or a new request.</Banner>
    </Modal>
  );
}

function FollowUpBadge({ j }: { j: Job }) {
  return j.followUpClass === "rework" ? <Badge tone="ok">Rework (free)</Badge> : j.followUpClass === "new_request" ? <Badge tone="primary">New request</Badge> : <Badge tone="warn">Under HQ review</Badge>;
}

function NoteModal({ open, j, onClose, onDone }: { open: boolean; j: Job; onClose: () => void; onDone: () => void }) {
  const [t, setT] = useState("");
  const [tried, setTried] = useState(false);
  const [err, setErr] = useState<string | undefined>();
  const bad = !t.trim() || t.length > 2000;
  return (
    <Modal open={open} onClose={onClose} title={`Add a note for ${j.id}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (bad) return; const e = jobActions.addNote(j.id, t); if (e) { setErr(e); return; } setT(""); setTried(false); onDone(); }}>Send note</Btn></>}>
      <Field label="Note to the HQ coordinator (1–2000 characters)" error={tried && bad ? "Write 1–2000 characters" : undefined} hint={`${t.length} / 2000 · Visible to: HQ coordinator${j.delivery === "contractor" ? ` · ${j.contractor}` : ""}${j.technician ? ` · assigned technician (${j.technician})` : ""}`}>
        <Textarea value={t} onChange={(e) => setT(e.target.value)} placeholder="Please call 30 minutes before arriving — the gate code changed to 4821." />
      </Field>
      {err && <Banner tone="crit">{err}</Banner>}
      <Banner>A note can’t change the visit time or the technician. To move the visit, write it here — HQ sends you a new time to accept. Notes are kept in the job history.</Banner>
    </Modal>
  );
}

function CancelModal({ open, j, onClose, onDone }: { open: boolean; j: Job; onClose: () => void; onDone: () => void }) {
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open={open} onClose={onClose} title={`Cancel ${j.id}?`} footer={<><Btn onClick={onClose}>Keep request</Btn><Btn variant="danger" onClick={() => { setTried(true); if (!r.trim()) return; jobActions.cancel(j.id, r.trim()); onDone(); }}>Cancel request</Btn></>}>
      <Field label="Reason (required)" error={tried && !r.trim() ? "A reason is required" : undefined}><Input value={r} onChange={(e) => setR(e.target.value)} placeholder="Resolved itself" /></Field>
    </Modal>
  );
}

const filters = [
  { unit: "Meeting room AC", loc: "Office A · last cleaned Jul 21", h: 268, st: "Overdue" as const },
  { unit: "Bedroom AC", loc: "Home A › 1F · last cleaned Aug 30", h: 212, st: "Due soon" as const },
  { unit: "Lobby AC", loc: "Office A · last cleaned Aug 12", h: 120, st: "OK" as const },
  { unit: "Living room AC", loc: "Home A › 1F · cleaned by technician Sep 8 (job-c02)", h: 41, st: "OK" as const },
];
function FilterCare({ onRequest }: { onRequest: () => void }) {
  const toast = useToast();
  const [rows, setRows] = useState(filters);
  return (
    <div className="split">
      <Card title="Filters by AC" sub="Run time since the last cleaning, from telemetry · reminder at 250 h (model default)">
        <DataTable rowKey={(r) => r.unit} rows={rows} cols={[
          { key: "u", label: "AC", render: (r) => <div><b>{r.unit}</b><div className="text-[11px] text-muted">{r.loc}</div></div> },
          { key: "h", label: "Run time", render: (r) => `${r.h} h since cleaning` },
          { key: "p", label: "Progress", render: (r) => <div className="w-28"><UtilBar pct={Math.min(100, (r.h / 250) * 100)} tone={r.st === "Overdue" ? "crit" : r.st === "Due soon" ? "warn" : "ok"} /></div>, hideBelow: "sm" },
          { key: "s", label: "Status", render: (r) => <Badge tone={r.st === "Overdue" ? "crit" : r.st === "Due soon" ? "warn" : "ok"}>{r.st}</Badge> },
          { key: "a", label: "", render: (r) => r.st === "Overdue" ? <Btn size="sm" variant="primary" onClick={onRequest}>Request cleaning</Btn> : <Btn size="sm" onClick={() => { setRows((s) => s.map((x) => (x.unit === r.unit ? { ...x, h: 0, st: "OK", loc: x.loc.split(" · ")[0] + " · cleaned by you just now" } : x))); toast("Marked cleaned — counter reset"); }}>Mark cleaned</Btn> },
        ]} />
        <p className="mt-2 text-[11px] text-muted">Run time counts hours the AC was running. Unknown connection → “Unknown”.</p>
      </Card>
      <Card title="Reminders" action={<Btn size="sm" onClick={() => toast("Reminder settings saved")}>Edit reminders</Btn>}>
        <SummaryList items={[["Remind at", "250 h of running (or 90 days)"], ["Channel", "In-app notification"], ["Next due", "Bedroom AC in ~38 h"]]} />
        <div className="mt-3"><Banner>Notifications link here. A technician visit (e.g. job-c02) resets the counter automatically.</Banner></div>
      </Card>
    </div>
  );
}
