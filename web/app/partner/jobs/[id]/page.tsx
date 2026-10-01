"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Field, Modal, Page, SummaryList, Textarea, LinkBtn, useToast } from "@/components/ui";

const cap = [["tech-external-a", "Refrigerant handling · valid to 2027-03-31", ["4 h", "4 h", "8 h", "—", "—"]], ["tech-external-a2", "No refrigerant qualification", ["—", "—", "—", "—", "—"]]] as const;
type S = "offer" | "accepted" | "declined" | "expired" | "snapshot";

export default function JobDetail({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const toast = useToast();
  const [s, setS] = useState<S>(id === "job-b-offer" ? "expired" : "offer");
  const [dec, setDec] = useState(false);
  const [why, setWhy] = useState("");
  const [tried, setTried] = useState(false);
  if (id === "job-b-offer") return <Page className="max-w-xl"><Card title="This page isn’t available" sub="The page doesn’t exist, or your account can’t open it." /></Page>;
  const err = tried && !why.trim() ? "A reason is required (1–1000 characters)" : undefined;
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/partner/jobs" className="font-semibold text-primary">← Jobs</Link> › {id} · Lobby AC · {s === "offer" ? "offer" : s}</div>
      {s === "expired" && <Banner tone="crit">This offer expired or was cancelled by HQ. No action is possible.</Banner>}
      {s === "accepted" && <Banner tone="ok">Accepted — receipt: job-p09 · access window 2026-09-14 01:00 – 2026-09-22 00:00. Assign a technician in Schedule.</Banner>}
      {s === "declined" && <Banner tone="warn">Declined with reason. HQ was notified.</Banner>}
      {s === "snapshot" && <Banner>Delegation period ended — showing a read-only history snapshot.</Banner>}
      <div className="split">
        <Card title="job-p09 · Offer from HQ" sub="Before acceptance: job type, registered installation address, and required qualifications only. No live telemetry, entry instructions, or billing.">
          <SummaryList items={[["Job type", "Reactive maintenance"], ["Installation address", "12 Jalan Ampang, Kuala Lumpur (registered)"], ["Required qualification", "Split-unit refrigerant handling"], ["Candidate dates", "2026-09-16 – 2026-09-20"], ["Access period if accepted", "2026-09-14 01:00 – 2026-09-22 00:00"], ["Offer / delegation terms", "offer-p09-1 · terms v3"]]} />
          {s === "offer" && <div className="mt-4 flex flex-wrap gap-2"><Btn variant="primary" onClick={() => { setS("accepted"); toast("Offer accepted"); }}>Accept offer</Btn><Btn variant="danger" onClick={() => setDec(true)}>Decline…</Btn></div>}
          {s === "accepted" && <div className="mt-4"><LinkBtn variant="primary" href="/partner/schedule?jobId=job-p02">Assign technician →</LinkBtn></div>}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Can your team take it?" sub="Candidate dates 09-16 – 09-20" action={<Link className="text-xs font-semibold text-primary" href="/partner/team">Team & capacity →</Link>}>
            <p className="mb-2 text-[11px] text-muted">Own-company technicians holding “Split-unit refrigerant handling”. Free hours per day — no customer data is shown before acceptance.</p>
            {cap.map(([n, q, h]) => <div key={n} className="mb-3"><b className="text-[13px]">{n}</b><div className="text-[11px] text-muted">{q}</div><div className="mt-1 grid grid-cols-5 gap-1 text-center text-[11px]">{["Wed", "Thu", "Fri", "Sat", "Sun"].map((d, i) => <div key={d} className={`rounded-lg py-1 ${h[i] === "—" ? "bg-surface2 text-muted" : "bg-ok-soft text-ok"}`}>{d}<div className="font-bold">{h[i]}</div></div>)}</div></div>)}
          </Card>
          <Card><SummaryList items={[["OFFERED", "09-13 18:20 by hq-operator"], ["OFFER ID", "offer-p09-1 (1st offer)"], ["OPEN OFFERS", "1 of 1"]]} /></Card>
          <button className="text-left text-[11px] text-muted underline" onClick={() => setS("expired")}>Preview: expired / cancelled state</button>
        </div>
      </div>
      <Modal open={dec} onClose={() => setDec(false)} title="Decline offer job-p09" footer={<><Btn onClick={() => setDec(false)}>Cancel</Btn><Btn variant="danger" onClick={() => { setTried(true); if (!why.trim()) return; setDec(false); setS("declined"); toast("Offer declined", "warn"); }}>Confirm decline</Btn></>}>
        <Field label="Decline reason (required, 1–1000 characters)" error={err}><Textarea value={why} onChange={(e) => setWhy(e.target.value)} placeholder="No qualified technician available in this window." /></Field>
      </Modal>
    </Page>
  );
}
