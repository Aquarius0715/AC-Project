"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Choice, Field, Input, ListRow, Modal, Page, PageHead, Select, SummaryList, Tabs, Textarea, Timeline, cx } from "@ac/web/components/ui";
import { JobStatusBadge, OriginBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { klTime } from "@ac/web/lib/devices";
import type { JobStatus } from "@ac/web/lib/jobs";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { clientJobRefusal, contactError, followUpText, PROBLEMS, preferredError, RATING_TAGS, STATUS_TABS, type StatusTab } from "@ac/web/lib/customerMaintenance";
import { filterRefusal } from "@ac/web/lib/customerFilterCare";
import { addNote, cancelRequest, createRequest, rateJob, reportProblem, requestReschedule, respondProposal } from "../actions";
import type { MaintenanceLive } from "../_lib/load";
import { FilterCareTab, type CleaningPrefill } from "./filter-care";

type Detail = NonNullable<MaintenanceLive["detail"]>;
type Slot = { startAt: string; endAt: string };
type Row = { date: string; from: string; to: string };
type ModalKind = null | "new" | "decline" | "other" | "rate" | "problem" | "note" | "cancel";
const toSlot = (r: Row): Slot => ({ startAt: new Date(`${r.date}T${r.from}:00+08:00`).toISOString(), endAt: new Date(`${r.date}T${r.to}:00+08:00`).toISOString() });
const dayAfter = (now: number, n: number) => klTime(new Date(now + n * 86_400_000).toISOString()).slice(0, 10);
const threeRows = (now: number, start = 2): Row[] => [0, 1, 2].map((i) => ({ date: dayAfter(now, start + i), from: i === 1 ? "14:00" : "10:00", to: i === 1 ? "16:00" : "12:00" }));

/** Customer maintenance (FR-C09, FR-C17, FR-C18, Figma Client 07a–07p) from the Core API. The tab is in the URL (tab=
 * filter-care): the server reads only what the shown tab needs. */
export function MaintenanceView({ live }: { live: MaintenanceLive }) {
  const router = useRouter();
  const [going, startGo] = useTransition();
  const [target, setTarget] = useState<"requests" | "filters">(live.page);
  const page = going ? target : live.page;
  const [prefill, setPrefill] = useState<CleaningPrefill | null>(live.newFor ? { unitId: live.newFor, symptom: "", type: "reactive" } : null);
  const [tab, setTab] = useState<StatusTab>("all");
  const [origin, setOrigin] = useState<"all" | "request" | "plan">("all");
  const [modal, setModal] = useState<ModalKind>(live.newFor ? "new" : null);
  const [result, setResult] = useState<{ tone: "ok" | "warn" | "crit"; text: string } | null>(null);
  const d = live.detail;
  const count = (t: StatusTab) => live.rows.filter((r) => t === "all" || r.tab === t).length;
  const shown = live.rows.filter((r) => (tab === "all" || r.tab === tab) && (origin === "all" || r.origin === origin));
  const replies = count("reply");
  const close = (text?: string, tone: "ok" | "warn" = "ok") => { setModal(null); if (text) setResult({ tone, text }); };
  const fail = (f: ActionFailure) => { setModal(null); setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: clientJobRefusal(f) }); };
  const open = (jobId: string) => router.push(`/customer/maintenance?jobId=${jobId}`);
  const go = (v: "requests" | "filters") => {
    setTarget(v);
    setResult(null);
    startGo(() => router.replace(v === "filters" ? "/customer/maintenance?tab=filter-care" : "/customer/maintenance", { scroll: false }));
  };
  return (
    <Page>
      <Tabs value={page} onChange={go} tabs={[{ id: "requests", label: "My requests" }, { id: "filters", label: "Filter care" }]} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      {page === "filters" ? (
        live.filters && !going ? (
          <FilterCareTab f={live.filters} owner={live.owner} onRequest={(p) => { setPrefill(p); setModal("new"); }}
            onDone={(text) => setResult({ tone: "ok", text })} onFail={(f) => setResult({ tone: f.code === "FORBIDDEN" ? "warn" : "crit", text: filterRefusal(f) })} />
        ) : <Card title="Filters by AC"><p className="text-[13px] text-muted">Loading filter care…</p></Card>
      ) : going ? <Card title="My requests"><p className="text-[13px] text-muted">Loading your requests…</p></Card> : (
        <>
          {replies > 0 && <Banner tone="warn" icon="⇄" action={<Btn size="sm" variant="primary" onClick={() => { setTab("reply"); const r = live.rows.find((x) => x.tab === "reply"); if (r) open(r.id); }}>Review</Btn>}>A new visit time needs your reply ({replies}). Nothing is booked until you accept.</Banner>}
          {live.rateBanner && <Banner tone="ok" icon="✓" action={<Btn size="sm" variant="primary" onClick={() => open(live.rateBanner!.jobId)}>Confirm & rate</Btn>}><b>{live.rateBanner.text}</b> Is everything working? Your confirmation closes the job.</Banner>}
          <PageHead title="My requests" sub={`${live.rows.length} request${live.rows.length === 1 ? "" : "s"} · sorted by status (business order)`} action={<Btn variant="primary" onClick={() => setModal("new")}>+ New maintenance request</Btn>} />
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Tabs value={tab} onChange={setTab} tabs={STATUS_TABS.filter(([t]) => t !== "reply" || replies > 0).map(([id, label]) => ({ id, label, count: count(id) }))} />
            <Select aria-label="Origin" value={origin} onChange={(e) => setOrigin(e.target.value as typeof origin)} className="w-auto"><option value="all">Origin: All</option><option value="request">Origin: Client request</option><option value="plan">Origin: Periodic plan</option></Select>
          </div>
          <div className={cx("grid gap-4", d ? "grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" : "grid-cols-1")}>
            <div className="flex min-w-0 flex-col gap-2">
              {shown.map((r) => (
                <ListRow key={r.id} selected={d?.job.id === r.id} href={`/customer/maintenance?jobId=${r.id}`}>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2"><span className="text-xs font-semibold text-muted">{r.short}</span><b>{r.unit}</b><OriginBadge origin={r.origin} /></div>
                    <div className="text-xs text-muted">{r.place ? `${r.place} · ` : ""}{r.type}</div>
                    <div className={cx("text-xs", r.warn && "font-semibold text-warn")}>{r.line}</div>
                  </div>
                  <JobStatusBadge s={r.status as JobStatus} /><span className="text-muted">›</span>
                </ListRow>
              ))}
              {shown.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">{live.rows.length ? "No requests in this filter." : "No requests yet — “+ New maintenance request” asks HQ for a visit."}</p>}
              <p className="text-[11px] text-muted">“Scheduled” = a time is confirmed: one of your 3 preferred times, or a new time you accepted. Periodic plan visits are scheduled from your maintenance plan.</p>
            </div>
            {d ? <JobDetail key={`${d.job.id}:${d.job.version}`} d={d} open={setModal} onDone={close} onFail={fail} />
              : "missing" in live ? <Card title="Request"><p className="text-[13px] text-muted">That request no longer exists in your account.</p></Card> : null}
          </div>
        </>
      )}
      {modal === "new" && <NewRequest live={live} prefill={prefill} onClose={(text, tone) => { setPrefill(null); close(text, tone); }} onFail={(f) => { setPrefill(null); fail(f); }} />}
      {d && modal === "decline" && <DeclineModal d={d} now={Date.parse(live.now)} onClose={close} onFail={fail} />}
      {d && modal === "other" && <OtherTimeModal d={d} now={Date.parse(live.now)} onClose={close} onFail={fail} />}
      {d && modal === "rate" && <RateModal d={d} onClose={close} onFail={fail} />}
      {d && modal === "problem" && <ProblemModal d={d} now={Date.parse(live.now)} onClose={close} onFail={fail} />}
      {d && modal === "note" && <NoteModal d={d} onClose={close} onFail={fail} />}
      {d && modal === "cancel" && <CancelModal d={d} onClose={close} onFail={fail} />}
    </Page>
  );
}

type Done = (text?: string, tone?: "ok" | "warn") => void;

function JobDetail({ d, open, onDone, onFail }: { d: Detail; open: (m: ModalKind) => void; onDone: Done; onFail: (f: ActionFailure) => void }) {
  const j = d.job;
  const [pending, run] = useAction();
  const status = (j.slotProposal?.status === "pending" ? "time_proposed" : j.status) as JobStatus;
  const closed = j.status === "completed" || j.status === "cancelled";
  return (
    <Card title={j.id.slice(0, 8)} sub={`${d.unit.name} — ${j.origin === "periodic_plan" ? "periodic inspection" : j.symptom.slice(0, 48)}`} className="self-start"
      action={<div className="flex items-center gap-2"><JobStatusBadge s={status} /><Link href="/customer/maintenance" aria-label="Close" className="rounded px-2 text-muted hover:bg-surface2">✕</Link></div>}>
      <SummaryList items={d.facts} />
      {d.preferred.length > 0 && (
        <div className="mt-3 flex flex-col gap-1">
          <b className="text-[13px]">Your preferred times{j.preferenceRound > 1 ? ` (round ${j.preferenceRound})` : ""}</b>
          {d.preferred.map((p) => <div key={p.text} className="flex items-center justify-between rounded-lg bg-surface2 px-3 py-1.5 text-[13px]"><span>{p.text}</span>{p.note && <Badge tone="ok">{p.note}</Badge>}</div>)}
        </div>
      )}
      {d.declined && <div className="mt-3"><Banner tone="warn">{d.declined}</Banner></div>}
      {d.proposal && (
        <div className="mt-3 rounded-xl border border-[#f5c473] bg-[#fff8ec] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2"><b className="text-[14px]">{d.proposal.title}</b><Badge tone="warn">Reply by {d.proposal.replyBy}</Badge></div>
          <div className="my-2 text-xl font-bold">{d.proposal.when}</div>
          <SummaryList items={[["Who comes", d.proposal.who], ["Message", `“${d.proposal.message}”`]]} />
          <div className="mt-3 flex flex-wrap items-center justify-end gap-2">
            <span className="mr-auto text-[11px] text-warn">Accepting books this time. Declining keeps your request open.</span>
            <Btn variant="danger" disabled={pending} onClick={() => open("decline")}>Decline…</Btn>
            <Btn variant="primary" disabled={pending} onClick={() => run(() => respondProposal(j.id, j.version, d.proposal!.id, { decision: "accept" }), "Time accepted", () => onDone(`Time accepted — ${d.proposal!.when} is booked; the technician confirms it next.`), onFail)}>✓ Accept this time</Btn>
          </div>
        </div>
      )}
      {d.plan && (
        <div className="mt-3 rounded-xl border border-[#cdb8fa] bg-[#f7f2ff] p-4 text-[13px]">
          <b className="text-[#6d28d9]">↻ Booked from your maintenance plan</b>
          <p className="mt-1">You don’t need to request periodic visits — HQ schedules each one. If you can’t be home, ask for another time (3 options) up to 48 h before the visit.</p>
          <div className="mt-2 flex items-center justify-end gap-2">{d.plan.why && <span className="mr-auto text-[11px] text-muted">{d.plan.why}</span>}<Btn disabled={!d.plan.can || pending} onClick={() => open("other")}>Request another time…</Btn></div>
        </div>
      )}
      {j.status === "completed" && (
        <>
          <p className="mt-3 text-[13px] font-bold">Completed report {d.report ? <span className="text-xs font-normal text-muted">v{d.report.version}{d.report.acceptedAt ? ` · accepted ${klTime(d.report.acceptedAt).slice(5)}` : ""}</span> : <span className="text-xs font-normal text-muted">visible once HQ accepts it</span>}</p>
          {d.report && (
            <>
              <ul className="divide-y divide-line">{d.report.items.filter((i) => i.tone !== "ok").map((i) => <li key={i.id} className="flex items-center justify-between gap-2 py-1 text-[13px]"><span>{i.label}{i.reason && <span className="text-xs text-muted"> · {i.reason}</span>}</span><Badge tone={i.tone as "ok"}>{i.result}</Badge></li>)}</ul>
              <p className="mt-1 text-xs text-muted">{d.report.items.filter((i) => i.tone === "ok").length} checks OK{d.report.parts.length ? ` · parts: ${d.report.parts.join(", ")}` : " · no parts replaced"}</p>
              {d.report.readings.length > 0 && <div className="mt-2"><SummaryList items={d.report.readings} /></div>}
              {d.report.workText && <p className="mt-2 text-xs">{d.report.workText}</p>}
              {d.report.photos.length > 0 && <div className="mt-2 flex flex-wrap gap-2">{d.report.photos.map((p) => (
                // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
                <img key={p.id} src={p.url} alt={p.name} className="h-20 w-28 rounded-lg bg-surface2 object-cover" />
              ))}</div>}
            </>
          )}
          {d.feedback.kind === "rate" && <div className="mt-3 flex flex-wrap items-center justify-end gap-2"><span className="mr-auto text-[11px] text-muted">{d.feedback.text}</span><Btn onClick={() => open("problem")}>Report a problem</Btn><Btn variant="primary" onClick={() => open("rate")}>Confirm & rate</Btn></div>}
          {(d.feedback.kind === "edit" || d.feedback.kind === "rated") && <div className="mt-3 flex flex-wrap items-center justify-end gap-2"><span className="mr-auto text-[13px]">{d.feedback.text}</span>{d.feedback.kind === "edit" && <Btn size="sm" onClick={() => open("rate")}>Edit rating</Btn>}<Btn size="sm" onClick={() => open("problem")}>Report a problem</Btn></div>}
        </>
      )}
      {j.followUpOfJobId && <p className="mt-3 text-xs text-muted">Follow-up of <Link className="font-semibold text-primary hover:underline" href={`/customer/maintenance?jobId=${j.followUpOfJobId}`}>{j.followUpOfJobId.slice(0, 8)}</Link> · {followUpText(j.followUpClass)}</p>}
      <p className="mt-4 text-[13px] font-bold">Notes to coordinator</p>
      {d.notes.length === 0 ? <p className="text-xs text-muted">No notes yet.</p> : <ul className="mt-1 flex flex-col gap-1 text-[13px]">{d.notes.map((n) => <li key={n.id}><span className="text-muted">{n.who} · {n.at} — </span>“{n.text}”</li>)}</ul>}
      <p className="mt-4 text-[13px] font-bold">History</p>
      <Timeline items={d.history.map((h) => ({ time: h.time, title: h.title }))} />
      <div className="mt-3 flex flex-wrap justify-between gap-2">
        {(j.status === "requested") && j.origin === "client_request" ? <Btn size="sm" variant="danger" onClick={() => open("cancel")}>Cancel request</Btn> : <span />}
        {!closed && <Btn size="sm" onClick={() => open("note")}>+ Add note</Btn>}
      </div>
      <p className="mt-2 text-[11px] text-muted">Cancel is possible until a time is booked. Any time outside your preferred times is booked only after you accept it. Notes can’t change the time or technician.</p>
    </Card>
  );
}

type ModalProps = { onClose: Done; onFail: (f: ActionFailure) => void };

/** Three preferred times: date, from, to (each 1–4 h, from tomorrow, all different — IR113). */
function TimesInput({ rows, onChange, error, label = "Your 3 preferred times" }: { rows: Row[]; onChange: (r: Row[]) => void; error?: string | null; label?: string }) {
  const set = (i: number, k: keyof Row, v: string) => onChange(rows.map((r, n) => (n === i ? { ...r, [k]: v } : r)));
  return (
    <div className="flex flex-col gap-2">
      <span className="text-[13px] font-semibold">{label} <span className="font-normal text-muted">· each 1–4 h, from tomorrow, all different</span></span>
      {rows.map((r, i) => (
        <div key={i} className="flex flex-wrap items-end gap-2">
          <span className="mb-2 w-8 text-xs font-semibold text-primary">{["1st", "2nd", "3rd"][i]}</span>
          <Field label={`Date ${i + 1}`}><Input type="date" value={r.date} onChange={(e) => set(i, "date", e.target.value)} /></Field>
          <Field label={`From ${i + 1}`}><Input type="time" value={r.from} onChange={(e) => set(i, "from", e.target.value)} /></Field>
          <Field label={`To ${i + 1}`}><Input type="time" value={r.to} onChange={(e) => set(i, "to", e.target.value)} /></Field>
        </div>
      ))}
      {error && <p className="text-xs text-crit">✕ {error}</p>}
    </div>
  );
}

/** A new request; from Filter care it opens prefilled with the unit, type preventive and the cleaning line (DD-C18). */
function NewRequest({ live, prefill, onClose, onFail }: ModalProps & { live: MaintenanceLive; prefill: CleaningPrefill | null }) {
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(prefill?.unitId ?? live.units[0]?.id ?? "");
  const [type, setType] = useState<"reactive" | "preventive">(prefill?.type ?? (prefill ? "preventive" : "reactive"));
  const [sym, setSym] = useState(prefill?.symptom ?? "");
  const [rows, setRows] = useState<Row[]>(threeRows(now));
  const [contact, setContact] = useState("");
  const [tried, setTried] = useState(false);
  const slots = rows.map(toSlot);
  const symErr = sym.trim().length < 10 || sym.length > 2000 ? "Symptoms must be 10–2000 characters" : null;
  const slotErr = preferredError(slots, now);
  const cErr = contactError(contact);
  const submit = () => {
    setTried(true);
    if (!unitId || symErr || slotErr || cErr) return;
    run(() => createRequest({ unitId, type, symptom: sym, slots, contactWindow: contact }), "Request sent", (v) => { onClose(`Request ${v.id.slice(0, 8)} sent with 3 preferred times — nothing is booked until a time is confirmed.`); router.push(`/customer/maintenance?jobId=${v.id}`); }, onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title="New maintenance request" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={submit}>Submit request</Btn></>}>
      <Field label="Unit"><Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{live.units.map((u) => <option key={u.id} value={u.id}>{u.name}{u.place ? ` · ${u.place}` : ""}</option>)}</Select></Field>
      <Field label="Type"><Choice value={type} onChange={setType} options={[{ id: "reactive", label: "Repair" }, { id: "preventive", label: "Preventive" }]} /></Field>
      <Field label="Symptoms (10–2000 characters)" error={tried ? symErr ?? undefined : undefined} hint={`${sym.length} / 2000`}><Textarea value={sym} onChange={(e) => setSym(e.target.value)} placeholder="Cold air is weak and there is water dripping from the indoor unit." /></Field>
      <TimesInput rows={rows} onChange={setRows} error={tried ? slotErr : null} />
      <Field label="Contact window (optional)" hint="Use HH:mm for times (example: Weekdays 09:00-18:00) — no phone numbers or e-mail addresses" error={tried ? cErr ?? undefined : undefined}><Input value={contact} maxLength={200} onChange={(e) => setContact(e.target.value)} /></Field>
      <Banner>Nothing is booked yet — you will see “Scheduled” once a time is confirmed. Any time outside your 3 options needs your approval.</Banner>
    </Modal>
  );
}

function DeclineModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [reason, setReason] = useState<"not_home" | "too_late" | "other">("not_home");
  const [comment, setComment] = useState("");
  const [give, setGive] = useState(true);
  const [rows, setRows] = useState<Row[]>(threeRows(now, 3));
  const [tried, setTried] = useState(false);
  const slots = rows.map(toSlot);
  const err = give ? preferredError(slots, now) : null;
  const send = () => {
    setTried(true);
    if (err || comment.length > 1000) return;
    run(() => respondProposal(j.id, j.version, d.proposal!.id, { decision: "decline", declineReason: reason, comment, slots: give ? slots : [] }), "Declined", () => onClose(give ? "Declined — HQ looks at your 3 new times." : "Declined — HQ will look again.", "warn"), onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title="Decline the proposed time" footer={<><Btn onClick={() => onClose()}>Back</Btn><Btn variant="primary" disabled={pending} onClick={send}>{give ? "Decline & send new times" : "Decline"}</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · {d.unit.name} · proposed {d.proposal?.when}</p>
      <Field label="Reason"><Choice value={reason} onChange={setReason} options={[{ id: "not_home", label: "I’m not at home then" }, { id: "too_late", label: "Too late for me" }, { id: "other", label: "Other" }]} /></Field>
      <Field label="Comment to HQ (optional, up to 1000)"><Input value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} placeholder="Away that day. Any weekday afternoon next week works." /></Field>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="h-4 w-4 accent-[#005bea]" checked={give} onChange={(e) => setGive(e.target.checked)} />Give 3 new preferred times (recommended)</label>
      {give && <TimesInput rows={rows} onChange={setRows} error={tried ? err : null} label="New preferred times" />}
      <Banner>HQ looks again and either books one of your times or sends another proposal. Declining never cancels the request.</Banner>
    </Modal>
  );
}

function OtherTimeModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [rows, setRows] = useState<Row[]>(threeRows(now, 3));
  const [comment, setComment] = useState("");
  const [tried, setTried] = useState(false);
  const slots = rows.map(toSlot);
  const err = preferredError(slots, now);
  return (
    <Modal open wide onClose={() => onClose()} title="Request another time" footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (err) return; run(() => requestReschedule(j.id, j.version, slots, comment), "Sent to HQ", () => onClose("Asked HQ for another time — the visit goes back to booking with your 3 times."), onFail); }}>Send to HQ</Btn></>}>
      <p className="text-xs text-muted">{j.id.slice(0, 8)} · periodic visit {j.scheduledSlot ? klTime(j.scheduledSlot.startAt).slice(5) : ""} · up to 48 h before the visit</p>
      <TimesInput rows={rows} onChange={setRows} error={tried ? err : null} />
      <Field label="Comment (optional, up to 1000)"><Input value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} /></Field>
    </Modal>
  );
}

function RateModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [stars, setStars] = useState(j.rating?.stars ?? 0);
  const [tags, setTags] = useState<string[]>(j.rating?.tags ?? []);
  const [comment, setComment] = useState(j.rating?.comment ?? "");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title={`${j.rating ? "Edit rating" : "Confirm & rate"} ${j.id.slice(0, 8)}`} footer={<><Btn onClick={() => onClose()}>Not now</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!stars) return; run(() => rateJob(j.id, j.version, stars, tags, comment), "Thanks — rating saved", () => onClose(`Thanks — ${j.rating ? "rating updated" : "job confirmed and rated"} (${stars}★).`), onFail); }}>{j.rating ? "Save rating" : "Confirm & submit"}</Btn></>}>
      <Field label="How was the service? (required)" error={tried && !stars ? "Choose 1–5 stars" : undefined}><div className="flex gap-1 text-2xl">{[1, 2, 3, 4, 5].map((n) => <button key={n} type="button" aria-label={`${n} stars`} aria-pressed={n === stars} onClick={() => setStars(n)} className={n <= stars ? "text-[#f59e0b]" : "text-line"}>★</button>)}</div></Field>
      <Field label="What went well? (optional)"><div className="flex flex-wrap gap-1.5">{RATING_TAGS.map((t) => <button key={t} type="button" aria-pressed={tags.includes(t)} onClick={() => setTags((s) => (s.includes(t) ? s.filter((x) => x !== t) : [...s, t]))} className={cx("rounded-full border px-3 py-1 text-xs font-semibold", tags.includes(t) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{t}</button>)}</div></Field>
      {stars > 0 && stars <= 2 && <Banner tone="warn">What went wrong? You can also “Report a problem” — HQ then looks at it as a follow-up.</Banner>}
      <Field label="Comment (optional, up to 1000)"><Textarea value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} placeholder="Technician was quick and tidy." /></Field>
      <p className="text-[11px] text-muted">You can edit the rating for 7 days. Unconfirmed jobs are confirmed automatically after 7 days.</p>
    </Modal>
  );
}

function ProblemModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const router = useRouter();
  const j = d.job;
  const [pending, run] = useAction();
  const [reason, setReason] = useState<(typeof PROBLEMS)[number]["id"]>("same_problem");
  const [details, setDetails] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [visit, setVisit] = useState<Row | null>(null);
  const [tried, setTried] = useState(false);
  const total = files.reduce((a, f) => a + f.size, 0);
  const fileErr = files.length > 5 ? "Up to 5 photos." : files.some((f) => !["image/jpeg", "image/png"].includes(f.type) || f.size > 5 * 1024 * 1024) ? "Photos must be JPEG or PNG up to 5 MiB each." : total > 5.5 * 1024 * 1024 ? "The photos together may be 5.5 MB at most." : null;
  const detErr = details.trim().length < 10 || details.length > 2000 ? "Describe the problem (10–2000 characters)" : null;
  const visitErr = visit ? preferredError([toSlot(visit)], now, 1) : null;
  const send = () => {
    setTried(true);
    if (detErr || fileErr || visitErr) return;
    const form = new FormData();
    form.set("jobId", j.id); form.set("version", String(j.version)); form.set("reasonCode", reason); form.set("details", details);
    files.forEach((f) => form.append("photos", f));
    if (visit) { const s = toSlot(visit); form.set("visitStart", s.startAt); form.set("visitEnd", s.endAt); }
    run(() => reportProblem(form), "Problem reported", (v) => { onClose(`Problem reported — follow-up ${v.id.slice(0, 8)} is under HQ review.`, "warn"); router.push(`/customer/maintenance?jobId=${v.id}`); }, onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={`Report a problem — ${j.id.slice(0, 8)}`} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={send}>Send report</Btn></>}>
      <Field label="What happened?"><Choice value={reason} onChange={setReason} options={PROBLEMS} /></Field>
      <Field label="Details (10–2000 characters)" error={tried ? detErr ?? undefined : undefined}><Textarea value={details} maxLength={2000} onChange={(e) => setDetails(e.target.value)} /></Field>
      <Field label="Photos (optional, up to 5 JPEG/PNG)" error={fileErr ?? undefined}><input type="file" accept="image/jpeg,image/png" multiple onChange={(e) => setFiles(Array.from(e.target.files ?? []))} className="text-[13px]" /></Field>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="h-4 w-4 accent-[#005bea]" checked={!!visit} onChange={(e) => setVisit(e.target.checked ? threeRows(now)[0] : null)} />Suggest a visit time</label>
      {visit && <TimesInput rows={[visit]} onChange={(r) => setVisit(r[0])} error={tried ? visitErr : null} label="Preferred visit" />}
      <Banner>Creates a follow-up request linked to {j.id.slice(0, 8)} (“Under HQ review”). HQ decides within 1 business day: rework (free) or a new request.</Banner>
    </Modal>
  );
}

function NoteModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [t, setT] = useState("");
  const [tried, setTried] = useState(false);
  const bad = !t.trim() || t.length > 2000;
  return (
    <Modal open onClose={() => onClose()} title={`Add a note for ${j.id.slice(0, 8)}`} footer={<><Btn onClick={() => onClose()}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (bad) return; run(() => addNote(j.id, j.version, t), "Note sent", () => onClose("Note sent — HQ, the service partner and the technician can read it."), onFail); }}>Send note</Btn></>}>
      <Field label="Note to the HQ coordinator (1–2000 characters)" error={tried && bad ? "Write 1–2000 characters" : undefined} hint={`${t.length} / 2000`}><Textarea value={t} maxLength={2000} onChange={(e) => setT(e.target.value)} placeholder="Please call 30 minutes before arriving — the gate code changed to 4821." /></Field>
      <Banner>A note can’t change the visit time or the technician. To move the visit, write it here — HQ sends you a new time to accept. Notes are kept in the job history.</Banner>
    </Modal>
  );
}

function CancelModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const j = d.job;
  const [pending, run] = useAction();
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title={`Cancel ${j.id.slice(0, 8)}?`} footer={<><Btn onClick={() => onClose()}>Keep request</Btn><Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (!r.trim()) return; run(() => cancelRequest(j.id, j.version, r), "Request cancelled", () => onClose("Request cancelled.", "warn"), onFail); }}>Cancel request</Btn></>}>
      <Field label="Reason (required, up to 1000)" error={tried && !r.trim() ? "A reason is required" : undefined}><Input value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder="Resolved itself" /></Field>
    </Modal>
  );
}
