"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { Badge, Banner, Btn, Card, Choice, Field, Input, ListRow, Modal, Page, PageHead, Select, SummaryList, Tabs, Textarea, Timeline, cx } from "@ac/web/components/ui";
import { JobStatusBadge, OriginBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { showTime, zonedInstant, zonedParts } from "@ac/web/lib/i18n";
import { useDisplay, useT } from "@ac/web/components/I18n";
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
// The preferred times are typed in the user's display time zone and sent as instants (NFR-08); "" while incomplete.
const toSlot = (r: Row, tz: string): Slot => ({ startAt: zonedInstant(r.date, r.from, tz), endAt: zonedInstant(r.date, r.to, tz) });
const dayAfter = (now: number, n: number, tz: string) => zonedParts(new Date(now + n * 86_400_000).toISOString(), tz).date;
const threeRows = (now: number, tz: string, start = 2): Row[] => [0, 1, 2].map((i) => ({ date: dayAfter(now, start + i, tz), from: i === 1 ? "14:00" : "10:00", to: i === 1 ? "16:00" : "12:00" }));

/** Customer maintenance (FR-C09, FR-C17, FR-C18, Figma Client 07a–07p) from the Core API. The tab is in the URL (tab=
 * filter-care): the server reads only what the shown tab needs. */
export function MaintenanceView({ live }: { live: MaintenanceLive }) {
  const t = useT();
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
  const fail = (f: ActionFailure) => { setModal(null); setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: clientJobRefusal(f, t) }); };
  const open = (jobId: string) => router.push(`/customer/maintenance?jobId=${jobId}`);
  const go = (v: "requests" | "filters") => {
    setTarget(v);
    setResult(null);
    startGo(() => router.replace(v === "filters" ? "/customer/maintenance?tab=filter-care" : "/customer/maintenance", { scroll: false }));
  };
  return (
    <Page>
      <Tabs value={page} onChange={go} tabs={[{ id: "requests", label: t("My requests") }, { id: "filters", label: t("Filter care") }]} />
      {result && <Banner tone={result.tone}>{result.text}</Banner>}
      {page === "filters" ? (
        live.filters && !going ? (
          <FilterCareTab f={live.filters} owner={live.owner} onRequest={(p) => { setPrefill(p); setModal("new"); }}
            onDone={(text) => setResult({ tone: "ok", text })} onFail={(f) => setResult({ tone: f.code === "FORBIDDEN" ? "warn" : "crit", text: filterRefusal(f, t) })} />
        ) : <Card title={t("Filters by AC")}><p className="text-[13px] text-muted">{t("Loading filter care…")}</p></Card>
      ) : going ? <Card title={t("My requests")}><p className="text-[13px] text-muted">{t("Loading your requests…")}</p></Card> : (
        <>
          {replies > 0 && <Banner tone="warn" icon="⇄" action={<Btn size="sm" variant="primary" onClick={() => { setTab("reply"); const r = live.rows.find((x) => x.tab === "reply"); if (r) open(r.id); }}>{t("Review")}</Btn>}>{t("A new visit time needs your reply ({n}). Nothing is booked until you accept.", { n: replies })}</Banner>}
          {live.rateBanner && <Banner tone="ok" icon="✓" action={<Btn size="sm" variant="primary" onClick={() => open(live.rateBanner!.jobId)}>{t("Confirm & rate")}</Btn>}><b>{live.rateBanner.text}</b> {t("Is everything working? Your confirmation closes the job.")}</Banner>}
          <PageHead title={t("My requests")} sub={t(live.rows.length === 1 ? "{n} request · sorted by status (business order)" : "{n} requests · sorted by status (business order)", { n: live.rows.length })} action={<Btn variant="primary" onClick={() => setModal("new")}>{t("+ New maintenance request")}</Btn>} />
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Tabs value={tab} onChange={setTab} tabs={STATUS_TABS.filter(([s]) => s !== "reply" || replies > 0).map(([id, label]) => ({ id, label: t(label), count: count(id) }))} />
            <Select aria-label={t("Origin")} value={origin} onChange={(e) => setOrigin(e.target.value as typeof origin)} className="w-auto"><option value="all">{t("Origin: All")}</option><option value="request">{t("Origin: Client request")}</option><option value="plan">{t("Origin: Periodic plan")}</option></Select>
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
              {shown.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">{t(live.rows.length ? "No requests in this filter." : "No requests yet — “+ New maintenance request” asks HQ for a visit.")}</p>}
              <p className="text-[11px] text-muted">{t("“Scheduled” = a time is confirmed: one of your 3 preferred times, or a new time you accepted. Periodic plan visits are scheduled from your maintenance plan.")}</p>
            </div>
            {d ? <JobDetail key={`${d.job.id}:${d.job.version}`} d={d} open={setModal} onDone={close} onFail={fail} />
              : "missing" in live ? <Card title={t("Request")}><p className="text-[13px] text-muted">{t("That request no longer exists in your account.")}</p></Card> : null}
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
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const status = (j.slotProposal?.status === "pending" ? "time_proposed" : j.status) as JobStatus;
  const closed = j.status === "completed" || j.status === "cancelled";
  return (
    <Card title={j.id.slice(0, 8)} sub={`${d.unit.name} — ${j.origin === "periodic_plan" ? t("periodic inspection") : j.symptom.slice(0, 48)}`} className="self-start"
      action={<div className="flex items-center gap-2"><JobStatusBadge s={status} /><Link href="/customer/maintenance" aria-label={t("Close")} className="rounded px-2 text-muted hover:bg-surface2">✕</Link></div>}>
      <SummaryList items={d.facts} />
      {d.preferred.length > 0 && (
        <div className="mt-3 flex flex-col gap-1">
          <b className="text-[13px]">{j.preferenceRound > 1 ? t("Your preferred times (round {n})", { n: j.preferenceRound }) : t("Your preferred times")}</b>
          {d.preferred.map((p) => <div key={p.text} className="flex items-center justify-between rounded-lg bg-surface2 px-3 py-1.5 text-[13px]"><span>{p.text}</span>{p.note && <Badge tone="ok">{p.note}</Badge>}</div>)}
        </div>
      )}
      {d.declined && <div className="mt-3"><Banner tone="warn">{d.declined}</Banner></div>}
      {d.proposal && (
        <div className="mt-3 rounded-xl border border-[#f5c473] bg-[#fff8ec] p-4">
          <div className="flex flex-wrap items-center justify-between gap-2"><b className="text-[14px]">{d.proposal.title}</b><Badge tone="warn">{t("Reply by {when}", { when: d.proposal.replyBy })}</Badge></div>
          <div className="my-2 text-xl font-bold">{d.proposal.when}</div>
          <SummaryList items={[[t("Who comes"), d.proposal.who], [t("Message"), `“${d.proposal.message}”`]]} />
          <div className="mt-3 flex flex-wrap items-center justify-end gap-2">
            <span className="mr-auto text-[11px] text-warn">{t("Accepting books this time. Declining keeps your request open.")}</span>
            <Btn variant="danger" disabled={pending} onClick={() => open("decline")}>{t("Decline…")}</Btn>
            <Btn variant="primary" disabled={pending} onClick={() => run(() => respondProposal(j.id, j.version, d.proposal!.id, { decision: "accept" }), t("Time accepted"), () => onDone(t("Time accepted — {when} is booked; the technician confirms it next.", { when: d.proposal!.when })), onFail)}>{t("✓ Accept this time")}</Btn>
          </div>
        </div>
      )}
      {d.plan && (
        <div className="mt-3 rounded-xl border border-[#cdb8fa] bg-[#f7f2ff] p-4 text-[13px]">
          <b className="text-[#6d28d9]">{t("↻ Booked from your maintenance plan")}</b>
          <p className="mt-1">{t("You don’t need to request periodic visits — HQ schedules each one. If you can’t be home, ask for another time (3 options) up to 48 h before the visit.")}</p>
          <div className="mt-2 flex items-center justify-end gap-2">{d.plan.why && <span className="mr-auto text-[11px] text-muted">{d.plan.why}</span>}<Btn disabled={!d.plan.can || pending} onClick={() => open("other")}>{t("Request another time…")}</Btn></div>
        </div>
      )}
      {j.status === "completed" && (
        <>
          <p className="mt-3 text-[13px] font-bold">{t("Completed report")} {d.report ? <span className="text-xs font-normal text-muted">v{d.report.version}{d.report.accepted ? t(" · accepted {when}", { when: d.report.accepted }) : ""}</span> : <span className="text-xs font-normal text-muted">{t("visible once HQ accepts it")}</span>}</p>
          {d.report && (
            <>
              <ul className="divide-y divide-line">{d.report.items.filter((i) => i.tone !== "ok").map((i) => <li key={i.id} className="flex items-center justify-between gap-2 py-1 text-[13px]"><span>{i.label}{i.reason && <span className="text-xs text-muted"> · {i.reason}</span>}</span><Badge tone={i.tone as "ok"}>{i.result}</Badge></li>)}</ul>
              <p className="mt-1 text-xs text-muted">{t("{n} checks OK", { n: d.report.items.filter((i) => i.tone === "ok").length })}{d.report.parts.length ? t(" · parts: {parts}", { parts: d.report.parts.join(", ") }) : t(" · no parts replaced")}</p>
              {d.report.readings.length > 0 && <div className="mt-2"><SummaryList items={d.report.readings} /></div>}
              {d.report.workText && <p className="mt-2 text-xs">{d.report.workText}</p>}
              {d.report.photos.length > 0 && <div className="mt-2 flex flex-wrap gap-2">{d.report.photos.map((p) => (
                // eslint-disable-next-line @next/next/no-img-element -- data URL read through the DAL (attachments.getContent)
                <img key={p.id} src={p.url} alt={p.name} className="h-20 w-28 rounded-lg bg-surface2 object-cover" />
              ))}</div>}
            </>
          )}
          {d.feedback.kind === "rate" && <div className="mt-3 flex flex-wrap items-center justify-end gap-2"><span className="mr-auto text-[11px] text-muted">{d.feedback.text}</span><Btn onClick={() => open("problem")}>{t("Report a problem")}</Btn><Btn variant="primary" onClick={() => open("rate")}>{t("Confirm & rate")}</Btn></div>}
          {(d.feedback.kind === "edit" || d.feedback.kind === "rated") && <div className="mt-3 flex flex-wrap items-center justify-end gap-2"><span className="mr-auto text-[13px]">{d.feedback.text}</span>{d.feedback.kind === "edit" && <Btn size="sm" onClick={() => open("rate")}>{t("Edit rating")}</Btn>}<Btn size="sm" onClick={() => open("problem")}>{t("Report a problem")}</Btn></div>}
        </>
      )}
      {j.followUpOfJobId && <p className="mt-3 text-xs text-muted">{t("Follow-up of")} <Link className="font-semibold text-primary hover:underline" href={`/customer/maintenance?jobId=${j.followUpOfJobId}`}>{j.followUpOfJobId.slice(0, 8)}</Link> · {followUpText(j.followUpClass, t)}</p>}
      <p className="mt-4 text-[13px] font-bold">{t("Notes to coordinator")}</p>
      {d.notes.length === 0 ? <p className="text-xs text-muted">{t("No notes yet.")}</p> : <ul className="mt-1 flex flex-col gap-1 text-[13px]">{d.notes.map((n) => <li key={n.id}><span className="text-muted">{n.who} · {n.at} — </span>“{n.text}”</li>)}</ul>}
      <p className="mt-4 text-[13px] font-bold">{t("History")}</p>
      <Timeline items={d.history.map((h) => ({ time: h.time, title: h.title }))} />
      <div className="mt-3 flex flex-wrap justify-between gap-2">
        {(j.status === "requested") && j.origin === "client_request" ? <Btn size="sm" variant="danger" onClick={() => open("cancel")}>{t("Cancel request")}</Btn> : <span />}
        {!closed && <Btn size="sm" onClick={() => open("note")}>{t("+ Add note")}</Btn>}
      </div>
      <p className="mt-2 text-[11px] text-muted">{t("Cancel is possible until a time is booked. Any time outside your preferred times is booked only after you accept it. Notes can’t change the time or technician.")}</p>
    </Card>
  );
}

type ModalProps = { onClose: Done; onFail: (f: ActionFailure) => void };

/** Three preferred times: date, from, to (each 1–4 h, from tomorrow, all different — IR113). */
function TimesInput({ rows, onChange, error, label }: { rows: Row[]; onChange: (r: Row[]) => void; error?: string | null; label?: string }) {
  const t = useT();
  const { timeZone } = useDisplay();
  const set = (i: number, k: keyof Row, v: string) => onChange(rows.map((r, n) => (n === i ? { ...r, [k]: v } : r)));
  return (
    <div className="flex flex-col gap-2">
      <span className="text-[13px] font-semibold">{label ?? t("Your 3 preferred times")} <span className="font-normal text-muted">{t("· each 1–4 h, from tomorrow, all different · times in {zone}", { zone: timeZone })}</span></span>
      {rows.map((r, i) => (
        <div key={i} className="flex flex-wrap items-end gap-2">
          <span className="mb-2 w-8 text-xs font-semibold text-primary">{t(["1st", "2nd", "3rd"][i])}</span>
          <Field label={t("Date {n}", { n: i + 1 })}><Input type="date" value={r.date} onChange={(e) => set(i, "date", e.target.value)} /></Field>
          <Field label={t("From {n}", { n: i + 1 })}><Input type="time" value={r.from} onChange={(e) => set(i, "from", e.target.value)} /></Field>
          <Field label={t("To {n}", { n: i + 1 })}><Input type="time" value={r.to} onChange={(e) => set(i, "to", e.target.value)} /></Field>
        </div>
      ))}
      {error && <p className="text-xs text-crit">✕ {error}</p>}
    </div>
  );
}

/** A new request; from Filter care it opens prefilled with the unit, type preventive and the cleaning line (DD-C18). */
function NewRequest({ live, prefill, onClose, onFail }: ModalProps & { live: MaintenanceLive; prefill: CleaningPrefill | null }) {
  const t = useT();
  const { timeZone } = useDisplay();
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(prefill?.unitId ?? live.units[0]?.id ?? "");
  const [type, setType] = useState<"reactive" | "preventive">(prefill?.type ?? (prefill ? "preventive" : "reactive"));
  const [sym, setSym] = useState(prefill?.symptom ?? "");
  const [rows, setRows] = useState<Row[]>(threeRows(now, timeZone));
  const [contact, setContact] = useState("");
  const [tried, setTried] = useState(false);
  const slots = rows.map((r) => toSlot(r, timeZone));
  const symErr = sym.trim().length < 10 || sym.length > 2000 ? t("Symptoms must be 10–2000 characters") : null;
  const slotErr = preferredError(slots, now, 3, t);
  const cErr = contactError(contact, t);
  const submit = () => {
    setTried(true);
    if (!unitId || symErr || slotErr || cErr) return;
    run(() => createRequest({ unitId, type, symptom: sym, slots, contactWindow: contact }), t("Request sent"), (v) => { onClose(t("Request {job} sent with 3 preferred times — nothing is booked until a time is confirmed.", { job: v.id.slice(0, 8) })); router.push(`/customer/maintenance?jobId=${v.id}`); }, onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={t("New maintenance request")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={submit}>{t("Submit request")}</Btn></>}>
      <Field label={t("Unit")}><Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{live.units.map((u) => <option key={u.id} value={u.id}>{u.name}{u.place ? ` · ${u.place}` : ""}</option>)}</Select></Field>
      <Field label={t("Type")}><Choice value={type} onChange={setType} options={[{ id: "reactive", label: t("Repair") }, { id: "preventive", label: t("Preventive") }]} /></Field>
      <Field label={t("Symptoms (10–2000 characters)")} error={tried ? symErr ?? undefined : undefined} hint={`${sym.length} / 2000`}><Textarea value={sym} onChange={(e) => setSym(e.target.value)} placeholder={t("Cold air is weak and there is water dripping from the indoor unit.")} /></Field>
      <TimesInput rows={rows} onChange={setRows} error={tried ? slotErr : null} />
      <Field label={t("Contact window (optional)")} hint={t("Use HH:mm for times (example: Weekdays 09:00-18:00) — no phone numbers or e-mail addresses")} error={tried ? cErr ?? undefined : undefined}><Input value={contact} maxLength={200} onChange={(e) => setContact(e.target.value)} /></Field>
      <Banner>{t("Nothing is booked yet — you will see “Scheduled” once a time is confirmed. Any time outside your 3 options needs your approval.")}</Banner>
    </Modal>
  );
}

function DeclineModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const t = useT();
  const { timeZone } = useDisplay();
  const j = d.job;
  const [pending, run] = useAction();
  const [reason, setReason] = useState<"not_home" | "too_late" | "other">("not_home");
  const [comment, setComment] = useState("");
  const [give, setGive] = useState(true);
  const [rows, setRows] = useState<Row[]>(threeRows(now, timeZone, 3));
  const [tried, setTried] = useState(false);
  const slots = rows.map((r) => toSlot(r, timeZone));
  const err = give ? preferredError(slots, now, 3, t) : null;
  const send = () => {
    setTried(true);
    if (err || comment.length > 1000) return;
    run(() => respondProposal(j.id, j.version, d.proposal!.id, { decision: "decline", declineReason: reason, comment, slots: give ? slots : [] }), t("Declined"), () => onClose(t(give ? "Declined — HQ looks at your 3 new times." : "Declined — HQ will look again."), "warn"), onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={t("Decline the proposed time")} footer={<><Btn onClick={() => onClose()}>{t("Back")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t(give ? "Decline & send new times" : "Decline")}</Btn></>}>
      <p className="text-xs text-muted">{t("{job} · {unit} · proposed {when}", { job: j.id.slice(0, 8), unit: d.unit.name, when: d.proposal?.when ?? "" })}</p>
      <Field label={t("Reason")}><Choice value={reason} onChange={setReason} options={[{ id: "not_home", label: t("I’m not at home then") }, { id: "too_late", label: t("Too late for me") }, { id: "other", label: t("Other") }]} /></Field>
      <Field label={t("Comment to HQ (optional, up to 1000)")}><Input value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} placeholder={t("Away that day. Any weekday afternoon next week works.")} /></Field>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="h-4 w-4 accent-[#005bea]" checked={give} onChange={(e) => setGive(e.target.checked)} />{t("Give 3 new preferred times (recommended)")}</label>
      {give && <TimesInput rows={rows} onChange={setRows} error={tried ? err : null} label={t("New preferred times")} />}
      <Banner>{t("HQ looks again and either books one of your times or sends another proposal. Declining never cancels the request.")}</Banner>
    </Modal>
  );
}

function OtherTimeModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const t = useT();
  const display = useDisplay();
  const j = d.job;
  const [pending, run] = useAction();
  const [rows, setRows] = useState<Row[]>(threeRows(now, display.timeZone, 3));
  const [comment, setComment] = useState("");
  const [tried, setTried] = useState(false);
  const slots = rows.map((r) => toSlot(r, display.timeZone));
  const err = preferredError(slots, now, 3, t);
  return (
    <Modal open wide onClose={() => onClose()} title={t("Request another time")} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (err) return; run(() => requestReschedule(j.id, j.version, slots, comment), t("Sent to HQ"), () => onClose(t("Asked HQ for another time — the visit goes back to booking with your 3 times.")), onFail); }}>{t("Send to HQ")}</Btn></>}>
      <p className="text-xs text-muted">{t("{job} · periodic visit {when} · up to 48 h before the visit", { job: j.id.slice(0, 8), when: j.scheduledSlot ? showTime(j.scheduledSlot.startAt, display) : "" })}</p>
      <TimesInput rows={rows} onChange={setRows} error={tried ? err : null} />
      <Field label={t("Comment (optional, up to 1000)")}><Input value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} /></Field>
    </Modal>
  );
}

function RateModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [stars, setStars] = useState(j.rating?.stars ?? 0);
  const [tags, setTags] = useState<string[]>(j.rating?.tags ?? []);
  const [comment, setComment] = useState(j.rating?.comment ?? "");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title={t(j.rating ? "Edit rating {job}" : "Confirm & rate {job}", { job: j.id.slice(0, 8) })} footer={<><Btn onClick={() => onClose()}>{t("Not now")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!stars) return; run(() => rateJob(j.id, j.version, stars as 1 | 2 | 3 | 4 | 5, tags, comment), t("Thanks — rating saved"), () => onClose(t(j.rating ? "Thanks — rating updated ({stars}★)." : "Thanks — job confirmed and rated ({stars}★).", { stars })), onFail); }}>{t(j.rating ? "Save rating" : "Confirm & submit")}</Btn></>}>
      <Field label={t("How was the service? (required)")} error={tried && !stars ? t("Choose 1–5 stars") : undefined}><div className="flex gap-1 text-2xl">{[1, 2, 3, 4, 5].map((n) => <button key={n} type="button" aria-label={t("{n} stars", { n })} aria-pressed={n === stars} onClick={() => setStars(n)} className={n <= stars ? "text-[#f59e0b]" : "text-line"}>★</button>)}</div></Field>
      {/* the tags are stored in English (the API's values); only their labels follow the language */}
      <Field label={t("What went well? (optional)")}><div className="flex flex-wrap gap-1.5">{RATING_TAGS.map((tag) => <button key={tag} type="button" aria-pressed={tags.includes(tag)} onClick={() => setTags((s) => (s.includes(tag) ? s.filter((x) => x !== tag) : [...s, tag]))} className={cx("rounded-full border px-3 py-1 text-xs font-semibold", tags.includes(tag) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{t(tag)}</button>)}</div></Field>
      {stars > 0 && stars <= 2 && <Banner tone="warn">{t("What went wrong? You can also “Report a problem” — HQ then looks at it as a follow-up.")}</Banner>}
      <Field label={t("Comment (optional, up to 1000)")}><Textarea value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} placeholder={t("Technician was quick and tidy.")} /></Field>
      <p className="text-[11px] text-muted">{t("You can edit the rating for 7 days. Unconfirmed jobs are confirmed automatically after 7 days.")}</p>
    </Modal>
  );
}

function ProblemModal({ d, now, onClose, onFail }: ModalProps & { d: Detail; now: number }) {
  const t = useT();
  const { timeZone } = useDisplay();
  const router = useRouter();
  const j = d.job;
  const [pending, run] = useAction();
  const [reason, setReason] = useState<(typeof PROBLEMS)[number]["id"]>("same_problem");
  const [details, setDetails] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [visit, setVisit] = useState<Row | null>(null);
  const [tried, setTried] = useState(false);
  const total = files.reduce((a, f) => a + f.size, 0);
  const fileErr = files.length > 5 ? t("Up to 5 photos.") : files.some((f) => !["image/jpeg", "image/png"].includes(f.type) || f.size > 5 * 1024 * 1024) ? t("Photos must be JPEG or PNG up to 5 MiB each.") : total > 5.5 * 1024 * 1024 ? t("The photos together may be 5.5 MB at most.") : null;
  const detErr = details.trim().length < 10 || details.length > 2000 ? t("Describe the problem (10–2000 characters)") : null;
  const visitErr = visit ? preferredError([toSlot(visit, timeZone)], now, 1, t) : null;
  const send = () => {
    setTried(true);
    if (detErr || fileErr || visitErr) return;
    const form = new FormData();
    form.set("jobId", j.id); form.set("version", String(j.version)); form.set("reasonCode", reason); form.set("details", details);
    files.forEach((f) => form.append("photos", f));
    if (visit) { const s = toSlot(visit, timeZone); form.set("visitStart", s.startAt); form.set("visitEnd", s.endAt); }
    run(() => reportProblem(form), t("Problem reported"), (v) => { onClose(t("Problem reported — follow-up {job} is under HQ review.", { job: v.id.slice(0, 8) }), "warn"); router.push(`/customer/maintenance?jobId=${v.id}`); }, onFail);
  };
  return (
    <Modal open wide onClose={() => onClose()} title={t("Report a problem — {job}", { job: j.id.slice(0, 8) })} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t("Send report")}</Btn></>}>
      <Field label={t("What happened?")}><Choice value={reason} onChange={setReason} options={PROBLEMS.map((p) => ({ ...p, label: t(p.label) }))} /></Field>
      <Field label={t("Details (10–2000 characters)")} error={tried ? detErr ?? undefined : undefined}><Textarea value={details} maxLength={2000} onChange={(e) => setDetails(e.target.value)} /></Field>
      <Field label={t("Photos (optional, up to 5 JPEG/PNG)")} error={fileErr ?? undefined}><input type="file" accept="image/jpeg,image/png" multiple onChange={(e) => setFiles(Array.from(e.target.files ?? []))} className="text-[13px]" /></Field>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" className="h-4 w-4 accent-[#005bea]" checked={!!visit} onChange={(e) => setVisit(e.target.checked ? threeRows(now, timeZone)[0] : null)} />{t("Suggest a visit time")}</label>
      {visit && <TimesInput rows={[visit]} onChange={(r) => setVisit(r[0])} error={tried ? visitErr : null} label={t("Preferred visit")} />}
      <Banner>{t("Creates a follow-up request linked to {job} (“Under HQ review”). HQ decides within 1 business day: rework (free) or a new request.", { job: j.id.slice(0, 8) })}</Banner>
    </Modal>
  );
}

function NoteModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [text, setText] = useState("");
  const [tried, setTried] = useState(false);
  const bad = !text.trim() || text.length > 2000;
  return (
    <Modal open onClose={() => onClose()} title={t("Add a note for {job}", { job: j.id.slice(0, 8) })} footer={<><Btn onClick={() => onClose()}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (bad) return; run(() => addNote(j.id, j.version, text), t("Note sent"), () => onClose(t("Note sent — HQ, the service partner and the technician can read it.")), onFail); }}>{t("Send note")}</Btn></>}>
      <Field label={t("Note to the HQ coordinator (1–2000 characters)")} error={tried && bad ? t("Write 1–2000 characters") : undefined} hint={`${text.length} / 2000`}><Textarea value={text} maxLength={2000} onChange={(e) => setText(e.target.value)} placeholder={t("Please call 30 minutes before arriving — the gate code changed to 4821.")} /></Field>
      <Banner>{t("A note can’t change the visit time or the technician. To move the visit, write it here — HQ sends you a new time to accept. Notes are kept in the job history.")}</Banner>
    </Modal>
  );
}

function CancelModal({ d, onClose, onFail }: ModalProps & { d: Detail }) {
  const t = useT();
  const j = d.job;
  const [pending, run] = useAction();
  const [r, setR] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={() => onClose()} title={t("Cancel {job}?", { job: j.id.slice(0, 8) })} footer={<><Btn onClick={() => onClose()}>{t("Keep request")}</Btn><Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (!r.trim()) return; run(() => cancelRequest(j.id, j.version, r), t("Request cancelled"), () => onClose(t("Request cancelled."), "warn"), onFail); }}>{t("Cancel request")}</Btn></>}>
      <Field label={t("Reason (required, up to 1000)")} error={tried && !r.trim() ? t("A reason is required") : undefined}><Input value={r} maxLength={1000} onChange={(e) => setR(e.target.value)} placeholder={t("Resolved itself")} /></Field>
    </Modal>
  );
}
