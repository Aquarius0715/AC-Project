"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, Modal, Page, Select, SummaryList, Textarea, LinkBtn, useToast } from "@/components/ui";
import { JobStatusBadge, OriginBadge, WINDOWS } from "@/components/JobBits";
import { Job, fmt, jobActions, longDate, useJobs } from "@/lib/jobs";

const cap = [["tech-external-a", "Refrigerant handling · valid to 2027-03-31", ["4 h", "4 h", "8 h", "—", "—"]], ["tech-external-a2", "No refrigerant qualification", ["—", "—", "—", "—", "—"]]] as const;
type S = "offer" | "accepted" | "declined" | "expired" | "proposed";

export default function JobDetail({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const live = useJobs().find((j) => j.id === id && j.contractor === "contractor-a" && j.id !== "job-p09");
  if (id === "job-b-offer") return <Page className="max-w-xl"><Card title="This page isn’t available" sub="The page doesn’t exist, or your account can’t open it." /></Page>;
  return live ? <LiveOffer j={live} /> : <StaticOffer id={id} />;
}

/** Offer backed by the shared job store (e.g. job-c07 after the client accepted HQ’s proposal). */
function LiveOffer({ j }: { j: Job }) {
  const toast = useToast();
  const [modal, setModal] = useState<null | "decline" | "propose">(null);
  const pending = j.partnerProposal && (j.partnerProposal.status === "pending" || j.partnerProposal.status === "sent_to_client");
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › {j.id} · {j.unit} · <JobStatusBadge s={j.status} /></div>
      {j.status === "accepted" && <Banner tone="ok" action={<LinkBtn size="sm" variant="primary" href={`/partner/schedule?jobId=${j.id}`}>Assign technician →</LinkBtn>}>Accepted — assign your technician for exactly {fmt(j.scheduled)}.</Banner>}
      {(j.status === "assigned" || j.status === "in_progress") && <Banner tone="ok">Assigned to {j.technician} · {j.techAck?.status === "accepted" ? "accepted by the technician ✓" : j.techAck?.status === "cant_make" ? "technician can’t make it — reassign in Schedule" : "waiting for the technician to accept"}.</Banner>}
      {j.status === "time_proposed" && <Banner tone="warn">HQ has asked the client about your proposed time. You will be notified of the answer.</Banner>}
      {j.partnerProposal?.status === "approved" && j.status === "offered" && <Banner tone="ok">The client approved your time ({fmt(j.scheduled)}). Accept the offer to continue.</Banner>}
      {j.partnerProposal?.status === "rejected" && j.status === "offered" && <Banner tone="warn">Your time change was not approved — the agreed time stays {fmt(j.scheduled)}. Accept or decline.</Banner>}
      <div className="split">
        <Card title={`${j.id} · Offer from HQ`} action={<Badge tone={pending ? "warn" : "warn"}>{pending ? "Time change proposed · waiting for client" : "Expires 2026-09-25 12:00"}</Badge>}>
          <Banner>{pending ? "The offer stays reserved for you until the client answers." : "The visit time was agreed with the client. Accept only if your team can come then; otherwise “Propose another time” — HQ asks the client first."}</Banner>
          <div className="mt-3"><SummaryList items={[
            ["Origin", <OriginBadge key="o" origin={j.origin} />],
            ["Job type", j.origin === "plan" ? "Periodic inspection" : `${j.type} maintenance`],
            ["Installation address", "Home A, Kuala Lumpur (registered)"],
            ["Required qualification", "Split-unit refrigerant handling"],
            ["Visit time (fixed)", pending ? <s key="s" className="text-muted">{fmt(j.scheduled)}</s> : <b key="b">{j.scheduled ? longDate(j.scheduled) : "—"} · {j.origin === "plan" ? "from the periodic plan" : "agreed with the client"}</b>],
            ...(pending ? [["Your proposal", <b key="p" className="text-warn">{longDate(j.partnerProposal!.slot)} · sent {j.partnerProposal!.sentAt}</b>] as [string, React.ReactNode]] : []),
            ["Offer / delegation terms", `offer-${j.id.slice(4)}-1 · terms v3`],
          ]} /></div>
          {j.status === "offered" && (
            <div className="mt-4 flex flex-col gap-2">
              {pending ? <Btn onClick={() => { jobActions.partnerWithdraw(j.id); toast("Proposal withdrawn"); }}>Withdraw my proposal</Btn> : <Btn onClick={() => setModal("propose")}>Propose another time…</Btn>}
              <div className="flex gap-2"><Btn variant="danger" className="flex-1" onClick={() => setModal("decline")}>Decline…</Btn><Btn variant="primary" className="flex-1" disabled={!!pending} onClick={() => { jobActions.partnerAccept(j.id); toast("Offer accepted"); }}>Accept job</Btn></div>
            </div>
          )}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Can your team take it? — ${fmt(j.scheduled)}`} action={<Link className="text-xs font-semibold text-primary" href="/partner/team">Team & capacity →</Link>}>
            <SummaryList items={[["tech-external-a", <Badge key="a" tone="ok">Fits · refrigerant ✓</Badge>], ["tech-external-a2", <Badge key="b" tone="muted">Not qualified</Badge>]]} />
            <p className="mt-2 text-[11px] text-muted">Own-company technicians only. No customer data before acceptance.</p>
          </Card>
          <Card title="Job history"><ul className="flex flex-col gap-1.5 text-xs">{j.history.slice(-5).map((h, i) => <li key={i}><span className="text-muted">{h.at}</span> · {h.text}</li>)}</ul></Card>
        </div>
      </div>
      {modal === "decline" && <DeclineModal id={j.id} onClose={() => setModal(null)} onDone={(r) => { jobActions.partnerDecline(j.id, r); toast("Offer declined", "warn"); setModal(null); }} />}
      {modal === "propose" && <ProposeTimeModal j={j} onClose={() => setModal(null)} onDone={(slot, tech, reason) => { jobActions.partnerPropose(j.id, slot, tech, reason); toast("Sent to HQ — waiting for the client"); setModal(null); }} />}
    </Page>
  );
}

function ProposeTimeModal({ j, onClose, onDone }: { j: Pick<Job, "id" | "scheduled">; onClose: () => void; onDone: (slot: { date: string; win: string }, tech: string, reason: string) => void }) {
  const [date, setDate] = useState("2026-10-02");
  const [win, setWin] = useState("14:00–16:00");
  const [tech, setTech] = useState("tech-external-a");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={onClose} title="Propose another time" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!reason.trim()) return; onDone({ date, win }, tech, reason.trim()); }}>Send to HQ</Btn></>}>
      <p className="text-xs text-muted">{j.id} · agreed with the client: {fmt(j.scheduled)}</p>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Field label="Date"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field><Field label="Window"><Select value={win} onChange={(e) => setWin(e.target.value)}>{WINDOWS.map((w) => <option key={w}>{w}</option>)}</Select></Field></div>
      <Field label="Technician"><Select value={tech} onChange={(e) => setTech(e.target.value)}><option value="tech-external-a">tech-external-a · free 14:00–18:00 ✓ refrigerant ✓</option><option value="tech-external-a2">tech-external-a2 · not qualified</option></Select></Field>
      <Field label="Reason (sent to HQ and the client) · required" error={tried && !reason.trim() ? "A reason is required" : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder="tech-external-a has a certificate renewal course on 10-01 morning." /></Field>
      <Banner>HQ forwards this to the client for approval. The offer stays reserved for you until the client answers. If the client declines, HQ may keep the original time with another partner.</Banner>
    </Modal>
  );
}

function DeclineModal({ id, onClose, onDone }: { id: string; onClose: () => void; onDone: (r: string) => void }) {
  const [why, setWhy] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Modal open onClose={onClose} title={`Decline offer ${id}`} footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="danger" onClick={() => { setTried(true); if (!why.trim()) return; onDone(why.trim()); }}>Confirm decline</Btn></>}>
      <Field label="Decline reason (required, 1–1000 characters)" error={tried && !why.trim() ? "A reason is required (1–1000 characters)" : undefined}><Textarea value={why} onChange={(e) => setWhy(e.target.value)} placeholder="No qualified technician available at this time." /></Field>
    </Modal>
  );
}

/** Seeded offer job-p09 (periodic plan visit) kept as in the Figma states 02-7…02-10. */
function StaticOffer({ id }: { id: string }) {
  const toast = useToast();
  const [s, setS] = useState<S>("offer");
  const [modal, setModal] = useState<null | "decline" | "propose">(null);
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › {id} · Lobby AC · {s === "offer" ? "offer" : s}</div>
      {s === "expired" && <Banner tone="crit">This offer expired or was cancelled by HQ. No action is possible.</Banner>}
      {s === "accepted" && <Banner tone="ok">Accepted — receipt: {id} · access window 2026-09-14 01:00 – 2026-09-22 00:00. Assign a technician in Schedule.</Banner>}
      {s === "declined" && <Banner tone="warn">Declined with reason. HQ was notified.</Banner>}
      {s === "proposed" && <Banner tone="warn">Time change sent to HQ — the client must approve it. The offer stays reserved for you.</Banner>}
      <div className="split">
        <Card title={`${id} · Offer from HQ`} sub="Before acceptance: job type, registered installation address, and required qualifications only. No live telemetry, entry instructions, or billing.">
          <SummaryList items={[["Origin", <OriginBadge key="o" origin="plan" />], ["Job type", "Periodic inspection"], ["Installation address", "12 Jalan Ampang, Kuala Lumpur (registered)"], ["Required qualification", "Split-unit refrigerant handling"], ["Visit time (fixed)", "2026-09-16 10:00–12:00 · from the periodic plan"], ["Access period if accepted", "2026-09-14 01:00 – 2026-09-22 00:00"], ["Offer / delegation terms", "offer-p09-1 · terms v3"]]} />
          {s === "offer" && <div className="mt-4 flex flex-col gap-2"><Btn onClick={() => setModal("propose")}>Propose another time…</Btn><div className="flex gap-2"><Btn variant="danger" className="flex-1" onClick={() => setModal("decline")}>Decline…</Btn><Btn variant="primary" className="flex-1" onClick={() => { setS("accepted"); toast("Offer accepted"); }}>Accept job</Btn></div></div>}
          {s === "accepted" && <div className="mt-4"><LinkBtn variant="primary" href="/partner/schedule?jobId=job-p02">Assign technician →</LinkBtn></div>}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Can your team take it? — visit 09-16 10:00–12:00" action={<Link className="text-xs font-semibold text-primary" href="/partner/team">Team & capacity →</Link>}>
            <p className="mb-2 text-[11px] text-muted">Own-company technicians holding “Split-unit refrigerant handling”. Free hours per day — no customer data is shown before acceptance.</p>
            {cap.map(([n, q, h]) => <div key={n} className="mb-3"><b className="text-[13px]">{n}</b><div className="text-[11px] text-muted">{q}</div><div className="mt-1 grid grid-cols-5 gap-1 text-center text-[11px]">{["Wed", "Thu", "Fri", "Sat", "Sun"].map((d, i) => <div key={d} className={`rounded-lg py-1 ${h[i] === "—" ? "bg-surface2 text-muted" : "bg-ok-soft text-ok"}`}>{d}<div className="font-bold">{h[i]}</div></div>)}</div></div>)}
          </Card>
          <Card><SummaryList items={[["OFFERED", "09-13 18:20 by hq-operator"], ["OFFER ID", "offer-p09-1 (1st offer)"], ["OPEN OFFERS", "1 of 1"]]} /></Card>
          <button className="text-left text-[11px] text-muted underline" onClick={() => setS("expired")}>Preview: expired / cancelled state</button>
        </div>
      </div>
      {modal === "decline" && <DeclineModal id={id} onClose={() => setModal(null)} onDone={() => { setModal(null); setS("declined"); toast("Offer declined", "warn"); }} />}
      {modal === "propose" && <ProposeTimeModal j={{ id, scheduled: { date: "2026-09-16", win: "10:00–12:00" } }} onClose={() => setModal(null)} onDone={() => { setModal(null); setS("proposed"); toast("Sent to HQ — waiting for the client"); }} />}
    </Page>
  );
}
