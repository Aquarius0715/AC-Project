"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, LinkBtn, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { alertTitle } from "@ac/web/lib/alerts";
import { until } from "@ac/web/lib/partnerOverview";
import { slotText } from "@ac/web/lib/partnerJobs";
import {
  decisionRefusal, delegationLeft, detailBanner, eventRows, ordinal, originLabel, qualificationLabel, range, timeline, typeLabel, when,
  type ApiDetail, type ApiOffer, type FitRow,
} from "@ac/web/lib/partnerJobDetail";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { acceptOffer, declineOffer, proposeSlot, withdrawProposal } from "../actions";
import type { JobDetailLive } from "../_lib/load";

const scopeLabel: Record<string, string> = { indoor: "Indoor unit", outdoor: "Outdoor unit", electrical: "Electrical / control" };
const statusTone: Record<string, "primary" | "ok" | "warn" | "crit" | "muted"> = { accepted: "primary", assigned: "primary", in_progress: "primary", submitted: "primary", completed: "ok", rework_requested: "warn", on_hold: "warn", cancelled: "muted" };
const statusText = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, " ");

/** The contractor's offer or job (FR-P02, FR-P08, Figma Contractor 02-7…02-14) from the Core API, by projection. */
export function JobDetailView({ live }: { live: JobDetailLive }) {
  if (live.kind === "offer") return <OfferView live={live} />;
  if (live.kind === "detail") return <DetailView live={live} />;
  const h = live.job;
  return (
    <Page narrow>
      <Crumb text={`${h.jobId.slice(0, 8)} · history`} />
      <Banner>Delegation ended — only the minimal history of your company’s decisions stays visible. Unit data and entry instructions are no longer shown (FR-P08).</Banner>
      <Card title={`${h.jobId.slice(0, 8)} · ${typeLabel(h.type)}`} action={<Badge tone={h.status === "completed" ? "ok" : "muted"}>{statusText(h.status)}</Badge>}>
        <SummaryList items={[["Job type", typeLabel(h.type)], ["Status", statusText(h.status)], ["Completed", h.completedAt ? when(h.completedAt) : "—"], ["Snapshot", when(h.asOf)]]} />
        <h3 className="mb-1 mt-4 text-[13px] font-semibold">Your company’s decisions</h3>
        {h.ownDecisionEvents.length ? <ul className="flex flex-col gap-1 text-xs">{eventRows(h.ownDecisionEvents).map((e) => <li key={e.id}><span className="text-muted">{e.at}</span> · {e.text}</li>)}</ul> : <p className="text-xs text-muted">No decision recorded.</p>}
      </Card>
    </Page>
  );
}

function Crumb({ text }: { text: string }) {
  return <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › <b className="text-ink">{text}</b></div>;
}

function FitCard({ title, rows, note, footer }: { title: string; rows: FitRow[]; note: string; footer?: React.ReactNode }) {
  return (
    <Card title={title} action={<Link className="text-xs font-semibold text-primary" href="/partner/team">Team & capacity →</Link>}>
      <p className="mb-2 text-[11px] text-muted">{note}</p>
      {rows.length === 0 && <p className="text-[13px] text-muted">No technicians in your company yet.</p>}
      {rows.map((r) => (
        <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 first:border-0">
          <span className="min-w-0"><b className="text-[13px]">{r.name}</b><span className="block text-[11px] text-muted">{r.sub}</span></span>
          <span className="flex items-center gap-1.5">
            {r.days.length > 1 && r.days.map((d, i) => <span key={i} className={`rounded-md px-1.5 py-0.5 text-center text-[10px] leading-tight ${d.free ? "bg-ok/15 text-ok" : "bg-surface2 text-muted"}`}>{d.label}<br />{d.text}</span>)}
            <Badge tone={r.badge.tone}>{r.badge.text}</Badge>
          </span>
        </div>
      ))}
      {footer}
    </Card>
  );
}

function OfferView({ live }: { live: Extract<JobDetailLive, { kind: "offer" }> }) {
  const j: ApiOffer = live.job;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [modal, setModal] = useState<null | "decline" | "propose">(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [declined, setDeclined] = useState(false);
  const failed = (f: ActionFailure) => setRefused(decisionRefusal(f));
  const p = j.partnerSlotProposal;
  const open = !!p && (p.status === "pending" || p.status === "sent_to_client");
  const short = j.jobId.slice(0, 8);
  const offers = Math.max(1, live.events.filter((e) => e.action === "job.offered").length);
  if (declined) {
    return <Page narrow><Crumb text={`${short} · declined`} /><Banner tone="warn" action={<LinkBtn size="sm" href="/partner/jobs">Back to jobs</LinkBtn>}>Declined with your reason — HQ was notified and the job is no longer offered to your company.</Banner></Page>;
  }
  return (
    <Page narrow>
      <Crumb text={`${short} · ${j.siteAddress ?? typeLabel(j.type)} · ${j.status === "offered" ? "offer" : "accepted"}`} />
      {refused && <Banner tone="crit">{refused}</Banner>}
      {open && <Banner tone="warn">{p!.status === "sent_to_client" ? "HQ asked the client about your proposed time" : "Your proposed time is with HQ"} ({slotText(p!.slot)} · {live.names[p!.technicianMembershipId] ?? "technician"}) — the offer stays reserved for you; Accept is disabled until it is answered.</Banner>}
      {p?.status === "approved" && <Banner tone="ok">The client approved your proposed time — the visit is now {slotText(j.visitSlot)}. Accept the offer to continue.</Banner>}
      {(p?.status === "declined" || p?.status === "kept") && <Banner tone="warn">Your time change was not taken — the agreed time stays {slotText(j.visitSlot)}. Accept or decline.</Banner>}
      {j.status === "accepted" && <Banner tone="ok" action={<LinkBtn size="sm" variant="primary" href={`/partner/schedule?jobId=${j.jobId}`}>Assign technician →</LinkBtn>}>Accepted — the delegation period starts {when(j.accessValidFrom)}; unit details open then. Assign your technician for exactly {slotText(j.visitSlot)}.</Banner>}
      <Card title={`${short} · Offer from HQ`} action={j.status === "offered" ? <Badge tone="warn">Expires {when(j.offerExpiresAt)}</Badge> : <Badge tone="primary">Accepted</Badge>}>
        <Banner>Before acceptance: job type, registered installation address and required qualifications only. No live telemetry, entry instructions or billing.</Banner>
        <div className="mt-3"><SummaryList items={[
          ["Origin", <OriginBadge key="o" origin={j.origin === "periodic_plan" ? "plan" : "request"} />],
          ["Job type", typeLabel(j.type)],
          ["Installation address", `${j.siteAddress ?? "—"} (registered)`],
          ["Required qualification", j.requiredQualifications.map(qualificationLabel).join(" · ") || "none"],
          ["Visit time (fixed)", <b key="v">{range(j.visitSlot.startAt, j.visitSlot.endAt)} · {originLabel(j.origin)}</b>],
          ["Access period if accepted", range(j.accessValidFrom, j.accessValidUntil)],
          ["Offer / delegation terms", `${j.offerId.slice(0, 8)} · terms ${j.termsVersion}`],
        ]} /></div>
        {j.status === "offered" && (
          <div className="mt-4 flex flex-col gap-2 border-t border-line pt-4">
            <Field label="Decline reason (required if declining, 1–1000 characters)" error={tried && !reason.trim() ? "A reason is required to decline (1–1000 characters)" : undefined}>
              <Input value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder="No qualified technician available in this window." />
            </Field>
            <Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (reason.trim()) setModal("decline"); }}>Decline</Btn>
            {open ? <Btn disabled={pending} onClick={() => run(() => withdrawProposal(j.jobId, p!.id, j.jobVersion), "Proposal withdrawn", () => setRefused(null), failed)}>Withdraw my proposal</Btn>
              : <Btn disabled={pending || !live.technicians.length} onClick={() => setModal("propose")}>Propose another time…</Btn>}
            <Btn variant="primary" disabled={pending || open || Date.parse(j.offerExpiresAt) <= now} onClick={() => run(() => acceptOffer(j.jobId, j.offerId, j.termsVersion, j.jobVersion), "Offer accepted — assign a technician next", () => setRefused(null), failed)}>Accept job</Btn>
          </div>
        )}
      </Card>
      <FitCard title={`Can your team take it? — visit ${slotText(j.visitSlot)}`} rows={live.fits}
        note={`Own-company technicians holding ${j.requiredQualifications.map(qualificationLabel).join(", ") || "the required qualifications"}. Free hours per day — no customer data is shown before acceptance.`}
        footer={<div className="mt-2 grid grid-cols-3 gap-2 rounded-xl bg-surface2 p-3 text-[11px]"><span><span className="block uppercase text-muted">Offered</span>{when(j.offeredAt)} by HQ</span><span><span className="block uppercase text-muted">Offer ID</span>{j.offerId.slice(0, 8)} ({ordinal(offers)} offer)</span><span><span className="block uppercase text-muted">Open offers</span>1 of {offers} · answer within {until(j.offerExpiresAt, now)}</span></div>} />
      {modal === "decline" && (
        <Modal open onClose={() => setModal(null)} title={`Decline offer ${short}?`} footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="danger" disabled={pending} onClick={() => run(() => declineOffer(j.jobId, j.offerId, j.jobVersion, reason.trim()), "Offer declined", () => { setModal(null); setDeclined(true); }, (f) => { setModal(null); failed(f); })}>Confirm decline</Btn></>}>
          <p className="text-[13px]">The job returns to HQ with your reason: “{reason.trim()}”. A new offer would get a new offer ID.</p>
        </Modal>
      )}
      {modal === "propose" && <ProposeModal j={j} technicians={live.technicians} onClose={() => setModal(null)} onSend={(slot, tech, why) => run(() => proposeSlot(j.jobId, j.offerId, j.jobVersion, slot, tech, why), "Sent to HQ — waiting for the client", () => { setModal(null); setRefused(null); }, (f) => { setModal(null); failed(f); })} pending={pending} />}
    </Page>
  );
}

function ProposeModal({ j, technicians, onClose, onSend, pending }: { j: ApiOffer; technicians: { id: string; name: string }[]; onClose: () => void; onSend: (slot: { startAt: string; endAt: string }, tech: string, reason: string) => void; pending: boolean }) {
  const kl = (iso: string) => new Date(Date.parse(iso) + 8 * 3600_000).toISOString();
  const start = kl(j.visitSlot.startAt), end = kl(j.visitSlot.endAt);
  const [date, setDate] = useState(new Date(Date.parse(start.slice(0, 10)) + 86_400_000).toISOString().slice(0, 10));
  const [from, setFrom] = useState(start.slice(11, 16));
  const [to, setTo] = useState(end.slice(11, 16));
  const [tech, setTech] = useState(technicians[0]?.id ?? "");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const iso = (d: string, t: string) => new Date(`${d}T${t}:00+08:00`).toISOString();
  const order = from < to;
  return (
    <Modal open onClose={onClose} title="Propose another time" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!reason.trim() || !order || !tech) return; onSend({ startAt: iso(date, from), endAt: iso(date, to) }, tech, reason.trim()); }}>Send to HQ</Btn></>}>
      <p className="text-xs text-muted">{j.jobId.slice(0, 8)} · agreed with the client: {slotText(j.visitSlot)}</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label="From"><Input type="time" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label="To" error={tried && !order ? "The end must be after the start" : undefined}><Input type="time" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
      </div>
      <Field label="Technician (own company, qualified)"><Select value={tech} onChange={(e) => setTech(e.target.value)}>{technicians.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</Select></Field>
      <Field label="Reason (sent to HQ and the client) · required, 1–1000 characters" error={tried && !reason.trim() ? "A reason is required" : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder="Our qualified technician is on a certificate course that morning." /></Field>
      <Banner>HQ forwards this to the client for approval. The offer stays reserved for you until the client answers; if they decline, HQ may keep the original time with another partner.</Banner>
    </Modal>
  );
}

function DetailView({ live }: { live: Extract<JobDetailLive, { kind: "detail" }> }) {
  const j: ApiDetail = live.job;
  const u = live.unit;
  const now = Date.parse(live.now);
  const short = j.id.slice(0, 8);
  const tech = j.assignment ? live.names[j.assignment.technicianMembershipId] ?? "Technician" : null;
  const banner = detailBanner(j, tech, now);
  const steps = timeline(j.status, live.events, !!j.assignment, j.origin);
  const settled = j.status === "submitted" || j.status === "completed" || j.status === "cancelled"; // the work is done: no technician to find any more
  const reading = u?.latestMeasurements?.find((m) => m.value !== null);
  const needsTech = j.status === "accepted" || (j.assignment?.acknowledgement === "cant_make") || (!!j.assignment && Date.parse(j.assignment.scheduledEnd) <= now && (j.status === "assigned" || j.status === "in_progress"));
  return (
    <Page>
      <Crumb text={`${short} · ${u?.displayName ?? typeLabel(j.type)}`} />
      {banner && <Banner tone={banner.tone}>{banner.text}</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`${short} · ${u?.displayName ?? "Unit"}`} action={<Badge tone={statusTone[j.status] ?? "muted"}>{statusText(j.status)}</Badge>}>
            <SummaryList items={[
              ["Job type", typeLabel(j.type)],
              ...(j.symptom ? [["Request", `“${j.symptom}”`] as [string, React.ReactNode]] : []),
              ["Site address", u?.location.address ?? "—"],
              ["Entry instructions", u?.location.accessInstructions ?? "—"],
              ["Required qualification", live.required.map(qualificationLabel).join(" · ") || "—"],
              ["Visit time (fixed)", `${j.offer ? range(j.offer.visitSlot.startAt, j.offer.visitSlot.endAt) : range(j.requestedSlot.startAt, j.requestedSlot.endAt)} · ${originLabel(j.origin)}`],
              ["Delegation (access) period", j.offer ? range(j.offer.accessValidFrom, j.offer.accessValidUntil) : "—"],
              ["Scheduled slot", j.assignment ? `${range(j.assignment.scheduledStart, j.assignment.scheduledEnd)} · ${tech}` : <b key="n" className="text-crit">Not assigned yet</b>],
            ]} />
            <div className="mt-4 flex flex-wrap gap-2">
              {needsTech && <LinkBtn variant="primary" href={`/partner/schedule?jobId=${j.id}`}>{j.assignment ? "Reassign technician →" : "Assign technician →"}</LinkBtn>}
              {j.status === "submitted" && <LinkBtn variant="primary" href={`/partner/jobs/${j.id}/review`}>Review report →</LinkBtn>}
              <LinkBtn href={`/partner/units/${j.unitId}?jobId=${j.id}`}>Open unit →</LinkBtn>
              <LinkBtn href={`/partner/history?jobId=${j.id}`}>Job history →</LinkBtn>
            </div>
          </Card>
          <Card title={`Target unit — ${u?.displayName ?? "not readable now"}`} action={<Link className="text-xs font-semibold text-primary" href={`/partner/units/${j.unitId}?jobId=${j.id}`}>Open unit →</Link>}>
            {u ? (
              <>
                <div className="grid-fluid" style={{ ["--min" as string]: "300px" }}>
                  <SummaryList items={[
                    ["Location", u.location.pathLabels.join(" › ") || "—"],
                    ["Model", `${u.capabilities.manufacturer} ${u.capabilities.model}`],
                    ["Connection", `${u.connection}${u.lastSeenAt ? ` · seen ${when(u.lastSeenAt).slice(11)}` : ""}`],
                  ]} />
                  <SummaryList items={[
                    ["Open alerts", live.alerts.length ? <span key="a" className="text-crit">{live.alerts.length} · {alertTitle(live.alerts[0])}</span> : "none"],
                    ["Last reading", reading ? `${reading.metric.replace(/_/g, " ")} ${reading.value} ${reading.unit}` : "—"],
                    ["Maintenance scope", u.serviceScope.map((s) => scopeLabel[s] ?? s).join(" · ")],
                  ]} />
                </div>
                {live.alerts.map((a) => <div key={a.id} className="mt-2"><Banner tone={a.severity === "critical" ? "crit" : "warn"}>{alertTitle(a)} — raised {when(a.detectedAt)} · linked to this job. Completing the job does not resolve the alert.</Banner></div>)}
              </>
            ) : <p className="text-[13px] text-muted">The unit is readable only inside the delegation period.</p>}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Status">
            <ol className="flex flex-col gap-2">
              {steps.map((s, i) => (
                <li key={s.label} className="flex items-start gap-2 text-xs">
                  <span className={`mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full text-[10px] font-bold ${s.state === "done" ? "bg-primary text-white" : s.state === "current" ? "border-2 border-primary text-primary" : "border border-line text-muted"}`}>{s.state === "done" ? "✓" : i + 1}</span>
                  <span><b className={s.state === "todo" ? "text-muted" : s.state === "current" ? "text-primary" : ""}>{s.label}</b><span className="block text-[11px] text-muted">{s.at ? when(s.at).slice(5) : s.state === "todo" ? "next" : ""} · {s.sub}</span></span>
                </li>
              ))}
            </ol>
            <p className="mt-2 text-[11px] text-muted">Live unit values are visible only within the delegation period. After it ends this page shows a minimal history snapshot.</p>
            {settled && j.offer && <p className="mt-2 text-[11px] text-muted">{delegationLeft(j.offer.accessValidUntil, now)}</p>}
          </Card>
          {!settled && <FitCard title="Who can take it" rows={live.fits} note="Own-company technicians: qualification for the unit’s scope and free hours from the visit day (members.capacity)."
            footer={j.offer && <p className={`mt-2 text-[11px] ${!j.assignment || Date.parse(j.offer.accessValidUntil) - now < 2 * 86_400_000 ? "text-crit" : "text-muted"}`}>{delegationLeft(j.offer.accessValidUntil, now, !j.assignment)}</p>} />}
        </div>
      </div>
    </Page>
  );
}
