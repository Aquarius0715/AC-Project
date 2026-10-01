"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, useToast, cx } from "@/components/ui";

type Inv = { id: string; amt: string; meta: string; method: string; st: "Unpaid" | "Overdue" | "Processing" | "Paid"; ccy: string };
const seed: Inv[] = [
  { id: "invoice-general-b", amt: "50.00 MYR", meta: "customer-b · contract-general-b · Aug 2026", method: "Debit card · failed", st: "Unpaid", ccy: "MYR" },
  { id: "invoice-jul-a", amt: "120.00 MYR", meta: "customer-a · contract-rto-a v1 · Jul 2026", method: "Manual · ref manual-2026-0188", st: "Paid", ccy: "MYR" },
  { id: "invoice-overdue-a", amt: "120.00 MYR", meta: "customer-a · contract-rto-a v1 · Aug 2026", method: "Method not selected", st: "Overdue", ccy: "MYR" },
  { id: "invoice-overdue-b", amt: "85.00 MYR", meta: "customer-b · contract-rto-b v1 · Aug 2026", method: "Credit card · processing", st: "Processing", ccy: "MYR" },
  { id: "invoice-sep-a", amt: "50.00 MYR", meta: "customer-a · contract-general-a v1 · Sep 2026", method: "Method not selected", st: "Unpaid", ccy: "MYR" },
];
const inquiries = [{ id: "inquiry-a-01", cust: "customer-a", kind: "payment", at: "09-21", text: "“I paid by bank transfer on 20 Sep — why is cooling still limited?”" }, { id: "inquiry-b-02", cust: "customer-b", kind: "billing", at: "09-19", text: "“Please resend the August invoice.”" }];
type Tab = "invoices" | "inquiries";
const tone = (s: Inv["st"]) => (s === "Paid" ? "ok" : s === "Overdue" ? "crit" : s === "Processing" ? "primary" : "warn");

export default function Billing() {
  const toast = useToast();
  const [tab, setTab] = useState<Tab>("invoices");
  const [list, setList] = useState(seed);
  const [f, setF] = useState<"All" | Inv["st"]>("All");
  const [sel, setSel] = useState(seed[2]);
  const [iq, setIq] = useState(inquiries[0]);
  const [modal, setModal] = useState<null | "create" | "manual">(null);
  const [m, setM] = useState({ amount: "120.00", ref: "", paidAt: "2026-09-14" });
  const [tried, setTried] = useState(false);
  const [reminded, setReminded] = useState<string | null>(null);
  const [rReason, setRReason] = useState("");
  const [conflict, setConflict] = useState(false);
  const shown = list.filter((x) => f === "All" || x.st === f);
  const sum = (st: Inv["st"][]) => list.filter((x) => st.includes(x.st)).reduce((a, x) => a + parseFloat(x.amt), 0).toFixed(2);
  const close = () => { setModal(null); setTried(false); };
  const record = () => {
    setTried(true);
    if (!m.ref.trim() || +m.amount !== parseFloat(sel.amt)) return;
    if (reminded) setConflict(true);
    setList((l) => l.map((x) => (x.id === sel.id ? { ...x, st: "Paid", method: `Manual · ref ${m.ref}` } : x))); setSel({ ...sel, st: "Paid", method: `Manual · ref ${m.ref}` });
    toast("Manual payment recorded"); close();
  };
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "invoices", label: "Invoices", count: list.length }, { id: "inquiries", label: "Inquiries", count: inquiries.length }]} />
      {tab === "invoices" ? (
        <>
          <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}>{[["Outstanding", `${sum(["Unpaid", "Overdue", "Processing"])} MYR`, "unpaid · processing"], ["Overdue", `${sum(["Overdue"])} MYR`, "reminders allowed"], ["Processing", `${sum(["Processing"])} MYR`, "awaiting HQ confirmation"], ["Paid in period", `${sum(["Paid"])} MYR`, "Jul"]].map(([a, b, c]) => <div key={a} className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">{a}</div><div className="text-xl font-bold">{b}</div><div className="text-[11px] text-muted">{c}</div></div>)}</div>
          <p className="text-[11px] text-muted">Totals per currency — never converted</p>
          <div className="flex flex-wrap items-center justify-between gap-3"><Tabs value={f} onChange={setF} tabs={(["All", "Unpaid", "Overdue", "Processing", "Paid"] as const).map((k) => ({ id: k, label: k, count: k === "All" ? list.length : list.filter((x) => x.st === k).length }))} /><Btn size="sm" variant="primary" onClick={() => setModal("create")}>+ Create invoice</Btn></div>
          <div className="split-rev">
            <Card title="Invoices" className="self-start"><div className="flex flex-col gap-2">{shown.map((x) => <ListRow key={x.id} selected={sel.id === x.id} onClick={() => { setSel(x); setConflict(false); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{x.id}</b><b className="text-[13px]">{x.amt}</b></div><div className="text-[11px] text-muted">{x.meta}</div><div className="mt-0.5 flex items-center gap-2 text-[11px]"><Badge tone={tone(x.st)}>{x.st}</Badge><span className="text-muted">{x.method}</span></div></div></ListRow>)}</div></Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={sel.id} sub={`${sel.meta.replace(" · ", " · ")} · Invoice v1`} action={<span className="text-lg font-bold">{sel.amt}</span>}>
                <SummaryList cols={2} items={[["Billing period", "2026-08-01 → 09-01"], ["Due", "2026-09-10"], ["Payment method", sel.method.split(" · ")[0]], ["Payment status", sel.st === "Paid" ? "Paid" : sel.st === "Processing" ? "Processing — confirm to settle" : "— no payment yet"]]} />
                {sel.st === "Processing" && <div className="mt-3"><Btn variant="primary" onClick={() => { setList((l) => l.map((x) => (x.id === sel.id ? { ...x, st: "Paid" } : x))); setSel({ ...sel, st: "Paid" }); toast("Card payment confirmed"); }}>Confirm card payment</Btn></div>}
                {(sel.st === "Unpaid" || sel.st === "Overdue") && <><p className="mt-3 text-xs text-muted">The customer has not started a payment. Record a manual payment if it was received outside the app (e.g. bank transfer).</p><div className="mt-2"><Btn onClick={() => setModal("manual")}>Record manual payment…</Btn></div></>}
                {conflict && <div className="mt-3"><Banner tone="warn">Reminder CONFLICT — payment arrived first, the pending reminder was stopped.</Banner></div>}
              </Card>
              {sel.st === "Overdue" && <Card title="Payment reminder" sub="Only overdue unpaid invoices can be reminded. If payment arrives first, the reminder is stopped. Delivery is simulated.">
                <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Field label="Recipient"><Select><option>customer-a · Client</option></Select></Field><Field label="Channel"><Select><option>WhatsApp</option><option>Email</option></Select></Field></div>
                <div className="my-3 rounded-xl bg-surface2 p-3 text-xs"><b>Preview · WhatsApp</b> <span className="text-muted">nothing is sent yet</span><br />Your invoice invoice-overdue-a (120.00 MYR, Aug 2026) was due on 10 Sep. Pay by card or view payment instructions in the app →</div>
                <Field label="Reason · e.g. “2nd reminder after due date”"><Input value={rReason} onChange={(e) => setRReason(e.target.value)} /></Field>
                <div className="mt-3 flex justify-end"><Btn variant="primary" disabled={!rReason.trim()} onClick={() => { setReminded(sel.id); toast("Reminder queued (simulated)"); }}>Send reminder</Btn></div>
              </Card>}
              <Card title="Related restriction" action={<Link className="text-xs font-semibold text-primary" href="/admin/restrictions">Open restriction →</Link>}><p className="text-[13px]"><b>restriction-limited-a</b></p><p className="text-xs text-muted">This invoice is a cause of this restriction. Once all of its cause invoices are paid, it moves to release_requested. State and units are shown on the Restrictions screen (restriction.manage).</p></Card>
              <Card title="Customer inquiries" action={<button className="text-xs font-semibold text-primary" onClick={() => setTab("inquiries")}>Open in Inquiries →</button>}><p className="text-[13px]">{inquiries[0].text}</p><p className="text-[11px] text-muted">inquiry-a-01 · payment · received 09-21 · customer-a</p></Card>
            </div>
          </div>
        </>
      ) : (
        <div className="split-rev">
          <Card title="Customer inquiries" className="self-start"><div className="flex flex-col gap-2">{inquiries.map((x) => <ListRow key={x.id} selected={iq.id === x.id} onClick={() => setIq(x)}><div><b className="text-[13px]">{x.id}</b><div className="text-[11px] text-muted">{x.kind} · {x.cust} · {x.at}</div></div></ListRow>)}</div></Card>
          <Card title={iq.id} sub={`${iq.kind} · ${iq.cust} · received ${iq.at}`}><p className="rounded-xl bg-surface2 p-3 text-[13px]">{iq.text}</p><div className="mt-3"><Field label="Reply (in-app only)"><Textarea /></Field></div><div className="mt-2 flex justify-end"><Btn variant="primary" onClick={() => toast("Reply recorded in-app")}>Send reply</Btn></div></Card>
        </div>
      )}
      <Modal open={modal === "manual"} onClose={close} title="Record manual payment" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={record}>Record payment</Btn></>}>
        <Field label="Amount (must equal invoice — full payment only)" error={tried && +m.amount !== parseFloat(sel.amt) ? `Must equal ${sel.amt}` : undefined}><Input type="number" value={m.amount} onChange={(e) => setM({ ...m, amount: e.target.value })} /></Field>
        <Field label="Reference" error={tried && !m.ref.trim() ? "Reference is required" : undefined}><Input value={m.ref} onChange={(e) => setM({ ...m, ref: e.target.value })} placeholder="manual-2026-0189" /></Field>
        <Field label="Paid on"><Input type="date" value={m.paidAt} onChange={(e) => setM({ ...m, paidAt: e.target.value })} /></Field>
      </Modal>
      <Modal open={modal === "create"} onClose={close} title="Create invoice" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { toast("Invoice created"); close(); }}>Create</Btn></>}><Field label="Contract"><Select><option>contract-rto-a</option><option>contract-general-a</option></Select></Field><div className="grid-fluid" style={{ ["--min"as string]: "150px" }}><Field label="Billing period start"><Input type="date" /></Field><Field label="Due date"><Input type="date" /></Field></div></Modal>
    </Page>
  );
}
