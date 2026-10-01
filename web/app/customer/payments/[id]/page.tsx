"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, DemoBadge, Modal, Page, SummaryList, Timeline, useToast } from "@/components/ui";

type Phase = "choose" | "processing" | "confirmed" | "failed";

export default function Invoice({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ view?: string }> }) {
  const { id } = use(params);
  const { view } = use(searchParams);
  const toast = useToast();
  const [phase, setPhase] = useState<Phase>("choose");
  const [method, setMethod] = useState<"credit" | "debit" | "bank">("credit");
  const [preview, setPreview] = useState<string | null>(null);
  const [reminders, setReminders] = useState(false);
  const paid = id !== "invoice-overdue-a" && id !== "invoice-sep-a";

  if (view === "restriction") {
    return (
      <Page>
        <div className="text-[13px] text-muted"><Link href={`/customer/payments/${id}`} className="font-semibold text-primary">← Invoice {id}</Link> › Cooling restriction</div>
        <Card title="Cooling restriction · restriction-limited-a" sub="RTO Plan · contract-rto-a">
          <SummaryList items={[["Reason", "Unpaid invoice invoice-overdue-a (120.00 MYR, due Sep 10)"], ["Notice", "Sent Sep 12 · in-app (demo)"], ["Start time", "Sep 13, 08:00 — policy: minimum cooling setpoint 24 °C"], ["Target units", "1 — Lobby AC (Office A)"], ["Release condition", "All cause invoices paid"], ["Release requested", "Sep 24, 09:41 — automatically after payment confirmation"]]} />
          <h3 className="mt-4 mb-2 text-[13px] font-bold">Per-unit state</h3>
          <div className="flex flex-col gap-2">
            <div className="rounded-xl border border-line p-3 text-[13px]"><b>Lobby AC</b> <Badge tone="warn">Applied</Badge> <Badge tone="primary">Release</Badge><div className="text-xs text-muted">Applied Sep 13 08:00 · offline since Sep 24 09:40 — release sent, waiting for device (fails after 30 s)</div></div>
            <div className="rounded-xl border border-line p-3 text-[13px]"><b>Study AC</b> <Badge tone="warn">Applied</Badge> <Badge tone="unknown">? Unknown (offline)</Badge><div className="text-xs text-muted">Offline since Sep 22 — apply result unknown; release sent, waiting for device response (fails after 30 s without response)</div></div>
          </div>
          <h3 className="mt-4 mb-2 text-[13px] font-bold">Timeline</h3>
          <Timeline items={[{ time: "Sep 12", title: "Scheduled" }, { time: "Sep 13 08:00", title: "Applied", tone: "warn" }, { time: "Sep 24 09:41", title: "Release requested" }, { time: "—", title: "Released (only when the unit confirms)", tone: "ok" }]} />
          <div className="mt-3"><Banner>Status stays “Release requested” until Lobby AC confirms. It is never shown as Released while the device result is unknown.</Banner></div>
          <p className="mt-3 text-xs text-muted">Inquiries are received in-app only — nothing is sent externally.</p>
        </Card>
      </Page>
    );
  }

  const pay = () => {
    setPhase("processing");
    setTimeout(() => setPhase(method === "debit" ? "failed" : "confirmed"), 1800);
  };
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href="/customer/payments" className="font-semibold text-primary">← Contracts & payments</Link> › Invoice {id}</div>
      <Banner tone="warn" action={<DemoBadge />}>Demo payment — no real money moves. No card number, CVV or expiry is collected.</Banner>
      <div className="split">
        <Card title="Amount due" action={paid ? <Badge tone="ok" icon="✓">Paid</Badge> : phase === "confirmed" ? <Badge tone="ok" icon="✓">Confirmed</Badge> : phase === "processing" ? <Badge tone="primary">Processing</Badge> : <Badge tone="crit">Unpaid</Badge>}>
          <div className="text-[32px] font-bold">120.00 <span className="text-base text-muted">MYR</span></div>
          <p className="text-xs text-muted">Due Sep 10, 2026 · issued Aug 27</p>
          <SummaryList items={[["Contract", "RTO Plan · contract-rto-a"], ["Contract scope", "3 units (Bedroom AC, Meeting room AC, Lobby AC)"]]} />
          {!paid && phase === "choose" && (
            <>
              <h3 className="mt-4 mb-2 text-[13px] font-bold">Payment method</h3>
              <div className="flex flex-col gap-2">
                {([["credit", "Credit card (demo)", "Simulated card payment — no card details"], ["debit", "Debit card (demo)", "Simulated card payment — no card details (this one fails in the demo)"], ["bank", "Payment instructions", "Bank transfer details — viewing them does not pay the invoice"]] as const).map(([k, t, d]) => (
                  <button key={k} onClick={() => setMethod(k)} aria-pressed={method === k} className={`rounded-xl border p-3 text-left ${method === k ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2"}`}><b className="text-[13px]">{t}</b><div className="text-xs text-muted">{d}</div></button>
                ))}
              </div>
              <div className="mt-3 flex justify-end">{method === "bank" ? <Btn onClick={() => toast("Bank instructions shown — invoice stays unpaid", "warn")}>View instructions</Btn> : <Btn variant="primary" onClick={pay}>Pay 120.00 MYR (demo)</Btn>}</div>
            </>
          )}
          {phase === "processing" && <div className="mt-4"><Banner>Processing… “Processing” is not paid. HQ confirms the payment.</Banner></div>}
          {phase === "confirmed" && <div className="mt-4"><Banner tone="ok">Payment confirmed (demo). Restriction release pending — cooling is restored only after the units confirm.</Banner></div>}
          {phase === "failed" && <div className="mt-4"><Banner tone="crit" action={<Btn size="sm" onClick={() => { setMethod("credit"); setPhase("choose"); }}>Retry</Btn>}>Payment failed — invoice stays unpaid. Retry or choose another method.</Banner></div>}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Payment history">{phase === "confirmed" || paid ? <p className="text-[13px]">Demo card payment · {paid ? "Aug 8" : "just now"} · 120.00 MYR</p> : <p className="text-xs text-muted">No payments yet</p>}</Card>
          <Card title="Payment reminders" action={<Btn size="sm" variant="ghost" onClick={() => setReminders((r) => !r)}>{reminders ? "▴" : "▾"}</Btn>} sub="Notifications only — not payment methods">
            {reminders && [["Email", "Demo record Sep 10"], ["WhatsApp", "Demo record Sep 12"]].map(([c, d]) => <div key={c} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px]"><span>{c} · {d}<span className="block text-xs text-muted">Sent {d.replace("Demo record ", "")} (demo record)</span></span><Btn size="sm" variant="ghost" onClick={() => setPreview(c)}>View preview</Btn></div>)}
          </Card>
        </div>
      </div>
      <Banner tone="warn" action={<Link className="text-xs font-semibold text-primary" href={`/customer/payments/${id}?view=restriction`}>View restriction →</Link>}><b>Cooling restriction active</b> — 1 unit in scope · Lobby AC (Office A) — minimum cooling setpoint 24 °C. Paying does not restore cooling immediately.</Banner>
      <Modal open={!!preview} onClose={() => setPreview(null)} title={`${preview} preview`} footer={<Btn onClick={() => setPreview(null)}>Close</Btn>}><p className="rounded-xl bg-surface2 p-3 text-[13px]">Your invoice invoice-overdue-a (120.00 MYR) was due on 10 Sep. Pay by card or view payment instructions in the app.</p><p className="text-xs text-muted">Preview only — nothing was sent.</p></Modal>
    </Page>
  );
}
