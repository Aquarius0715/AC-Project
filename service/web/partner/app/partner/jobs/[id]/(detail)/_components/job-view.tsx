"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, LinkBtn, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { alertTitle } from "@ac/web/lib/alerts";
import { zonedInstant } from "@ac/web/lib/i18n";
import { decisionRefusal, originLabel, qualificationLabel, typeLabel, type ApiDetail, type ApiOffer, type FitRow } from "@ac/web/lib/partnerJobDetail";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { acceptOffer, declineOffer, proposeSlot, withdrawProposal } from "../actions";
import type { JobDetailLive } from "../_lib/load";

const scopeLabel: Record<string, string> = { indoor: "Indoor unit", outdoor: "Outdoor unit", electrical: "Electrical / control" };
const statusTone: Record<string, "primary" | "ok" | "warn" | "crit" | "muted"> = { accepted: "primary", assigned: "primary", in_progress: "primary", submitted: "primary", completed: "ok", rework_requested: "warn", on_hold: "warn", cancelled: "muted" };
const statusWord: Record<string, string> = {
  offered: "Offered", accepted: "Accepted", assigned: "Assigned", in_progress: "In progress", submitted: "Submitted", completed: "Completed",
  rework_requested: "Rework requested", on_hold: "On hold", cancelled: "Cancelled",
};
const connectionWord: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };

/** The contractor's offer or job (FR-P02, FR-P08, Figma Contractor 02-7…02-14) from the Core API, by projection.
 * Texts in the user's display language; the times come formatted from the loader (IR272). */
export function JobDetailView({ live }: { live: JobDetailLive }) {
  const t = useT();
  if (live.kind === "offer") return <OfferView live={live} />;
  if (live.kind === "detail") return <DetailView live={live} />;
  const h = live.job;
  const status = statusWord[h.status] ? t(statusWord[h.status]) : h.status.replace(/_/g, " ");
  return (
    <Page narrow>
      <Crumb text={t("{id} · history", { id: h.jobId.slice(0, 8) })} />
      <Banner>{t("Delegation ended — only the minimal history of your company’s decisions stays visible. Unit data and entry instructions are no longer shown (FR-P08).")}</Banner>
      <Card title={`${h.jobId.slice(0, 8)} · ${typeLabel(h.type, t)}`} action={<Badge tone={h.status === "completed" ? "ok" : "muted"}>{status}</Badge>}>
        <SummaryList items={[[t("Job type"), typeLabel(h.type, t)], [t("Status"), status], [t("Completed"), live.text.completed], [t("Snapshot"), live.text.snapshot]]} />
        <h3 className="mb-1 mt-4 text-[13px] font-semibold">{t("Your company’s decisions")}</h3>
        {live.text.events.length ? <ul className="flex flex-col gap-1 text-xs">{live.text.events.map((e) => <li key={e.id}><span className="text-muted">{e.at}</span> · {e.text}</li>)}</ul> : <p className="text-xs text-muted">{t("No decision recorded.")}</p>}
      </Card>
    </Page>
  );
}

function Crumb({ text }: { text: string }) {
  const t = useT();
  return <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">{t("← Jobs")}</Link> › <b className="text-ink">{text}</b></div>;
}

function FitCard({ title, rows, note, footer }: { title: string; rows: FitRow[]; note: string; footer?: React.ReactNode }) {
  const t = useT();
  return (
    <Card title={title} action={<Link className="text-xs font-semibold text-primary" href="/partner/team">{t("Team & capacity →")}</Link>}>
      <p className="mb-2 text-[11px] text-muted">{note}</p>
      {rows.length === 0 && <p className="text-[13px] text-muted">{t("No technicians in your company yet.")}</p>}
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
  const t = useT();
  const j: ApiOffer = live.job;
  const x = live.text;
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [modal, setModal] = useState<null | "decline" | "propose">(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [declined, setDeclined] = useState(false);
  const failed = (f: ActionFailure) => setRefused(decisionRefusal(f, t));
  const p = j.partnerSlotProposal;
  const open = !!p && (p.status === "pending" || p.status === "sent_to_client");
  const short = j.jobId.slice(0, 8);
  const quals = j.requiredQualifications.map((q) => qualificationLabel(q, t));
  if (declined) {
    return <Page narrow><Crumb text={t("{id} · declined", { id: short })} /><Banner tone="warn" action={<LinkBtn size="sm" href="/partner/jobs">{t("Back to jobs")}</LinkBtn>}>{t("Declined with your reason — HQ was notified and the job is no longer offered to your company.")}</Banner></Page>;
  }
  return (
    <Page narrow>
      <Crumb text={`${short} · ${j.siteAddress ?? typeLabel(j.type, t)} · ${t(j.status === "offered" ? "offer" : "accepted")}`} />
      {refused && <Banner tone="crit">{refused}</Banner>}
      {open && <Banner tone="warn">{t(p!.status === "sent_to_client" ? "HQ asked the client about your proposed time ({slot} · {name}) — the offer stays reserved for you; Accept is disabled until it is answered." : "Your proposed time is with HQ ({slot} · {name}) — the offer stays reserved for you; Accept is disabled until it is answered.", { slot: x.proposal ?? "", name: live.names[p!.technicianMembershipId] ?? t("technician") })}</Banner>}
      {p?.status === "approved" && <Banner tone="ok">{t("The client approved your proposed time — the visit is now {slot}. Accept the offer to continue.", { slot: x.visit })}</Banner>}
      {(p?.status === "declined" || p?.status === "kept") && <Banner tone="warn">{t("Your time change was not taken — the agreed time stays {slot}. Accept or decline.", { slot: x.visit })}</Banner>}
      {j.status === "accepted" && <Banner tone="ok" action={<LinkBtn size="sm" variant="primary" href={`/partner/schedule?jobId=${j.jobId}`}>{t("Assign technician →")}</LinkBtn>}>{t("Accepted — the delegation period starts {time}; unit details open then. Assign your technician for exactly {slot}.", { time: x.accessFrom, slot: x.visit })}</Banner>}
      <Card title={t("{id} · Offer from HQ", { id: short })} action={j.status === "offered" ? <Badge tone="warn">{t("Expires {time}", { time: x.expires })}</Badge> : <Badge tone="primary">{t("Accepted")}</Badge>}>
        <Banner>{t("Before acceptance: job type, registered installation address and required qualifications only. No live telemetry, entry instructions or billing.")}</Banner>
        <div className="mt-3"><SummaryList items={[
          [t("Origin"), <OriginBadge key="o" origin={j.origin === "periodic_plan" ? "plan" : "request"} />],
          [t("Job type"), typeLabel(j.type, t)],
          [t("Installation address"), t("{address} (registered)", { address: j.siteAddress ?? "—" })],
          [t("Required qualification"), quals.join(" · ") || t("none")],
          [t("Visit time (fixed)"), <b key="v">{x.visit} · {originLabel(j.origin, t)}</b>],
          [t("Access period if accepted"), x.access],
          [t("Offer / delegation terms"), t("{id} · terms {version}", { id: j.offerId.slice(0, 8), version: j.termsVersion })],
        ]} /></div>
        {j.status === "offered" && (
          <div className="mt-4 flex flex-col gap-2 border-t border-line pt-4">
            <Field label={t("Decline reason (required if declining, 1–1000 characters)")} error={tried && !reason.trim() ? t("A reason is required to decline (1–1000 characters)") : undefined}>
              <Input value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder={t("No qualified technician available in this window.")} />
            </Field>
            <Btn variant="danger" disabled={pending} onClick={() => { setTried(true); if (reason.trim()) setModal("decline"); }}>{t("Decline")}</Btn>
            {open ? <Btn disabled={pending} onClick={() => run(() => withdrawProposal(j.jobId, p!.id, j.jobVersion), t("Proposal withdrawn"), () => setRefused(null), failed)}>{t("Withdraw my proposal")}</Btn>
              : <Btn disabled={pending || !live.technicians.length} onClick={() => setModal("propose")}>{t("Propose another time…")}</Btn>}
            <Btn variant="primary" disabled={pending || open || Date.parse(j.offerExpiresAt) <= now} onClick={() => run(() => acceptOffer(j.jobId, j.offerId, j.termsVersion, j.jobVersion), t("Offer accepted — assign a technician next"), () => setRefused(null), failed)}>{t("Accept job")}</Btn>
          </div>
        )}
      </Card>
      <FitCard title={t("Can your team take it? — visit {slot}", { slot: x.visit })} rows={live.fits}
        note={t("Own-company technicians holding {quals}. Free hours per day — no customer data is shown before acceptance.", { quals: quals.join(", ") || t("the required qualifications") })}
        footer={<div className="mt-2 grid grid-cols-3 gap-2 rounded-xl bg-surface2 p-3 text-[11px]">
          <span><span className="block uppercase text-muted">{t("Offered")}</span>{t("{time} by HQ", { time: x.offered })}</span>
          <span><span className="block uppercase text-muted">{t("Offer ID")}</span>{t("{id} ({nth} offer)", { id: j.offerId.slice(0, 8), nth: x.nth })}</span>
          <span><span className="block uppercase text-muted">{t("Open offers")}</span>{t("1 of {n} · answer within {left}", { n: x.offers, left: x.left })}</span>
        </div>} />
      {modal === "decline" && (
        <Modal open onClose={() => setModal(null)} title={t("Decline offer {id}?", { id: short })} footer={<><Btn onClick={() => setModal(null)}>{t("Cancel")}</Btn><Btn variant="danger" disabled={pending} onClick={() => run(() => declineOffer(j.jobId, j.offerId, j.jobVersion, reason.trim()), t("Offer declined"), () => { setModal(null); setDeclined(true); }, (f) => { setModal(null); failed(f); })}>{t("Confirm decline")}</Btn></>}>
          <p className="text-[13px]">{t("The job returns to HQ with your reason: “{reason}”. A new offer would get a new offer ID.", { reason: reason.trim() })}</p>
        </Modal>
      )}
      {modal === "propose" && <ProposeModal j={j} x={x} technicians={live.technicians} onClose={() => setModal(null)} onSend={(slot, tech, why) => run(() => proposeSlot(j.jobId, j.offerId, j.jobVersion, slot, tech, why), t("Sent to HQ — waiting for the client"), () => { setModal(null); setRefused(null); }, (f) => { setModal(null); failed(f); })} pending={pending} />}
    </Page>
  );
}

/** Propose another time (Figma 02-8): date and times typed in the user's display time zone (NFR-08). */
function ProposeModal({ j, x, technicians, onClose, onSend, pending }: { j: ApiOffer; x: Extract<JobDetailLive, { kind: "offer" }>["text"]; technicians: { id: string; name: string }[]; onClose: () => void; onSend: (slot: { startAt: string; endAt: string }, tech: string, reason: string) => void; pending: boolean }) {
  const t = useT();
  const [date, setDate] = useState(x.propose.date);
  const [from, setFrom] = useState(x.propose.from);
  const [to, setTo] = useState(x.propose.to);
  const [tech, setTech] = useState(technicians[0]?.id ?? "");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const startAt = zonedInstant(date, from, x.propose.zone), endAt = zonedInstant(date, to, x.propose.zone);
  const order = !!startAt && !!endAt && startAt < endAt;
  return (
    <Modal open onClose={onClose} title={t("Propose another time")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!reason.trim() || !order || !tech) return; onSend({ startAt, endAt }, tech, reason.trim()); }}>{t("Send to HQ")}</Btn></>}>
      <p className="text-xs text-muted">{t("{id} · agreed with the client: {slot} · times in {zone}", { id: j.jobId.slice(0, 8), slot: x.visit, zone: x.propose.zone })}</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label={t("Date")}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label={t("From")}><Input type="time" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label={t("To")} error={tried && !order ? t("The end must be after the start") : undefined}><Input type="time" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
      </div>
      <Field label={t("Technician (own company, qualified)")}><Select value={tech} onChange={(e) => setTech(e.target.value)}>{technicians.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}</Select></Field>
      <Field label={t("Reason (sent to HQ and the client) · required, 1–1000 characters")} error={tried && !reason.trim() ? t("A reason is required") : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder={t("Our qualified technician is on a certificate course that morning.")} /></Field>
      <Banner>{t("HQ forwards this to the client for approval. The offer stays reserved for you until the client answers; if they decline, HQ may keep the original time with another partner.")}</Banner>
    </Modal>
  );
}

function DetailView({ live }: { live: Extract<JobDetailLive, { kind: "detail" }> }) {
  const t = useT();
  const j: ApiDetail = live.job;
  const x = live.text;
  const u = live.unit;
  const now = Date.parse(live.now);
  const short = j.id.slice(0, 8);
  const settled = j.status === "submitted" || j.status === "completed" || j.status === "cancelled"; // the work is done: no technician to find any more
  const reading = u?.latestMeasurements?.find((m) => m.value !== null);
  const needsTech = j.status === "accepted" || (j.assignment?.acknowledgement === "cant_make") || (!!j.assignment && Date.parse(j.assignment.scheduledEnd) <= now && (j.status === "assigned" || j.status === "in_progress"));
  const status = statusWord[j.status] ? t(statusWord[j.status]) : j.status.replace(/_/g, " ");
  const connection = u ? (connectionWord[u.connection] ? t(connectionWord[u.connection]) : u.connection) : "";
  return (
    <Page>
      <Crumb text={`${short} · ${u?.displayName ?? typeLabel(j.type, t)}`} />
      {x.banner && <Banner tone={x.banner.tone}>{x.banner.text}</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`${short} · ${u?.displayName ?? t("Unit")}`} action={<Badge tone={statusTone[j.status] ?? "muted"}>{status}</Badge>}>
            <SummaryList items={[
              [t("Job type"), typeLabel(j.type, t)],
              ...(j.symptom ? [[t("Request"), `“${j.symptom}”`] as [string, React.ReactNode]] : []),
              [t("Site address"), u?.location.address ?? "—"],
              [t("Entry instructions"), u?.location.accessInstructions ?? "—"],
              [t("Required qualification"), live.required.map((q) => qualificationLabel(q, t)).join(" · ") || "—"],
              [t("Visit time (fixed)"), `${x.visit} · ${originLabel(j.origin, t)}`],
              [t("Delegation (access) period"), x.access ?? "—"],
              [t("Scheduled slot"), x.scheduled ? `${x.scheduled} · ${x.tech}` : <b key="n" className="text-crit">{t("Not assigned yet")}</b>],
            ]} />
            <div className="mt-4 flex flex-wrap gap-2">
              {needsTech && <LinkBtn variant="primary" href={`/partner/schedule?jobId=${j.id}`}>{t(j.assignment ? "Reassign technician →" : "Assign technician →")}</LinkBtn>}
              {j.status === "submitted" && <LinkBtn variant="primary" href={`/partner/jobs/${j.id}/review`}>{t("Review report →")}</LinkBtn>}
              <LinkBtn href={`/partner/units/${j.unitId}?jobId=${j.id}`}>{t("Open unit →")}</LinkBtn>
              <LinkBtn href={`/partner/history?jobId=${j.id}`}>{t("Job history →")}</LinkBtn>
            </div>
          </Card>
          <Card title={t("Target unit — {name}", { name: u?.displayName ?? t("not readable now") })} action={<Link className="text-xs font-semibold text-primary" href={`/partner/units/${j.unitId}?jobId=${j.id}`}>{t("Open unit →")}</Link>}>
            {u ? (
              <>
                <div className="grid-fluid" style={{ ["--min" as string]: "300px" }}>
                  <SummaryList items={[
                    [t("Location"), u.location.pathLabels.join(" › ") || "—"],
                    [t("Model"), `${u.capabilities.manufacturer} ${u.capabilities.model}`],
                    [t("Connection"), x.seen ? t("{state} · seen {time}", { state: connection, time: x.seen }) : connection],
                  ]} />
                  <SummaryList items={[
                    [t("Open alerts"), live.alerts.length ? <span key="a" className="text-crit">{live.alerts.length} · {alertTitle(live.alerts[0], t)}</span> : t("none")],
                    [t("Last reading"), reading ? `${reading.metric.replace(/_/g, " ")} ${reading.value} ${reading.unit}` : "—"],
                    [t("Maintenance scope"), u.serviceScope.map((s) => (scopeLabel[s] ? t(scopeLabel[s]) : s)).join(" · ")],
                  ]} />
                </div>
                {live.alerts.map((a) => <div key={a.id} className="mt-2"><Banner tone={a.severity === "critical" ? "crit" : "warn"}>{t("{alert} — raised {time} · linked to this job. Completing the job does not resolve the alert.", { alert: alertTitle(a, t), time: x.raised[a.id] ?? "" })}</Banner></div>)}
              </>
            ) : <p className="text-[13px] text-muted">{t("The unit is readable only inside the delegation period.")}</p>}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Status")}>
            <ol className="flex flex-col gap-2">
              {x.steps.map((s, i) => (
                <li key={s.label} className="flex items-start gap-2 text-xs">
                  <span className={`mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full text-[10px] font-bold ${s.state === "done" ? "bg-primary text-white" : s.state === "current" ? "border-2 border-primary text-primary" : "border border-line text-muted"}`}>{s.state === "done" ? "✓" : i + 1}</span>
                  <span><b className={s.state === "todo" ? "text-muted" : s.state === "current" ? "text-primary" : ""}>{s.label}</b><span className="block text-[11px] text-muted">{s.atText ?? (s.state === "todo" ? t("next") : "")} · {s.sub}</span></span>
                </li>
              ))}
            </ol>
            <p className="mt-2 text-[11px] text-muted">{t("Live unit values are visible only within the delegation period. After it ends this page shows a minimal history snapshot.")}</p>
            {settled && x.delegation && <p className="mt-2 text-[11px] text-muted">{x.delegation.left}</p>}
          </Card>
          {!settled && <FitCard title={t("Who can take it")} rows={live.fits} note={t("Own-company technicians: qualification for the unit’s scope and free hours from the visit day (members.capacity).")}
            footer={j.offer && x.delegation && <p className={`mt-2 text-[11px] ${!j.assignment || Date.parse(j.offer.accessValidUntil) - now < 2 * 86_400_000 ? "text-crit" : "text-muted"}`}>{j.assignment ? x.delegation.left : x.delegation.toSchedule}</p>} />}
        </div>
      </div>
    </Page>
  );
}
