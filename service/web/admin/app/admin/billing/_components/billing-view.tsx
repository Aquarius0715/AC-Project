"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { ContractorPayouts } from "@ac/web/components/Features";
import { Badge, Banner, Btn, Card, DataTable, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, useToast } from "@ac/web/components/ui";
import { useUrlTab } from "@ac/web/lib/useUrlTab";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { reminderText, totals, type ApiRecipient, type ContractOption, type InquiryRow, type InvoiceDetail, type InvoiceRow, type InvoiceStatus, type StatementDetail, type StatementRow } from "@ac/web/lib/billing";
import {
  answerInquiry, confirmCardPayment, createInvoice, generatePayouts, previewReminder, recordManualPayment, resolvePayoutQuery, sendReminder, transitionStatement, type ActionResult,
} from "../actions";

type Tab = "invoices" | "inquiries" | "payouts";
export type BillingLive = {
  tab: Tab; scope: { customerId?: string; propertyId?: string; contractId?: string; overdueOnly: boolean };
  rows: InvoiceRow[]; contracts: ContractOption[]; inquiries: InquiryRow[];
  customers: { id: string; name: string }[]; properties: { id: string; name: string; customerId: string }[]; contractsOfCustomer: { id: string; customerId: string }[];
  selectedInvoice?: InvoiceDetail; recipients?: ApiRecipient[]; selectedInquiryId?: string; statements?: StatementRow[]; statement?: StatementDetail;
};

const seed: InvoiceRow[] = [
  { id: "invoice-general-b", number: "invoice-general-b", version: 1, amt: "50.00 MYR", amountMinor: 5000, currency: "MYR", meta: "customer-b · contract-general-b · Aug 2026", method: "Debit card · failed", st: "Unpaid", customerId: "customer-b", contractId: "contract-general-b", period: "2026-08-01 → 2026-09-01", due: "2026-09-10" },
  { id: "invoice-jul-a", number: "invoice-jul-a", version: 1, amt: "120.00 MYR", amountMinor: 12000, currency: "MYR", meta: "customer-a · contract-rto-a v1 · Jul 2026", method: "Method not selected · confirmed", st: "Paid", customerId: "customer-a", contractId: "contract-rto-a", period: "2026-07-01 → 2026-08-01", due: "2026-08-10" },
  { id: "invoice-overdue-a", number: "invoice-overdue-a", version: 1, amt: "120.00 MYR", amountMinor: 12000, currency: "MYR", meta: "customer-a · contract-rto-a v1 · Aug 2026", method: "Method not selected", st: "Overdue", customerId: "customer-a", contractId: "contract-rto-a", period: "2026-08-01 → 2026-09-01", due: "2026-09-10" },
  { id: "invoice-overdue-b", number: "invoice-overdue-b", version: 1, amt: "85.00 MYR", amountMinor: 8500, currency: "MYR", meta: "customer-b · contract-rto-b v1 · Aug 2026", method: "Credit card · processing", st: "Processing", customerId: "customer-b", contractId: "contract-rto-b", period: "2026-08-01 → 2026-09-01", due: "2026-09-10" },
  { id: "invoice-sep-a", number: "invoice-sep-a", version: 1, amt: "50.00 MYR", amountMinor: 5000, currency: "MYR", meta: "customer-a · contract-general-a v1 · Sep 2026", method: "Method not selected", st: "Unpaid", customerId: "customer-a", contractId: "contract-general-a", period: "2026-09-01 → 2026-10-01", due: "2026-10-10" },
];
const seedInquiries: InquiryRow[] = [
  { id: "inquiry-a-01", version: 1, cust: "customer-a", kind: "payment", at: "09-21", text: "“I paid by bank transfer on 20 Sep — why is cooling still limited?”", state: "received", reply: null, invoiceId: "invoice-overdue-a", restrictionId: "restriction-limited-a" },
  { id: "inquiry-b-02", version: 1, cust: "customer-b", kind: "billing", at: "09-19", text: "“Please resend the August invoice.”", state: "received", reply: null, invoiceId: null, restrictionId: null },
];
const seedContracts: ContractOption[] = [{ id: "contract-rto-a", version: 1, label: "customer-a · rto · 120.00 MYR", priceMinor: 12000, currency: "MYR" }, { id: "contract-general-a", version: 1, label: "customer-a · general · 50.00 MYR", priceMinor: 5000, currency: "MYR" }];
const tone = (s: InvoiceStatus) => (s === "Paid" ? "ok" : s === "Overdue" ? "crit" : s === "Processing" ? "primary" : "warn");
const noReminder = { recipient: "", channel: "email", reason: "", preview: "" };

/** HQ billing: invoices, inquiries and contractor payouts. `live` comes from the Server Component in API mode (the
 * scope and selection live in the URL; writes are Server Actions that re-render the route); the demo keeps fixtures. */
export function BillingView({ live }: { live?: BillingLive }) {
  const toast = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const [pending, start] = useTransition();
  const [conflict, setConflict] = useState(false);
  const nav = (patch: Record<string, string | null>) => {
    const q = new URLSearchParams(window.location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v) q.set(k, v);
      else q.delete(k);
    }
    router.replace(q.size ? `${pathname}?${q}` : pathname, { scroll: false });
  };
  const act = <T,>(fn: () => Promise<ActionResult<T>>, ok: string | ((v: T) => string), after?: (v: T) => void) =>
    start(async () => {
      const r = await fn();
      if (r.ok) {
        toast(typeof ok === "string" ? ok : ok(r.value));
        after?.(r.value);
        return;
      }
      toast(actionMessage(r), "crit");
      if (r.messageKey === "error.versionConflict") setConflict(true);
    });

  const [urlTab, setUrlTab] = useUrlTab<Tab>({ invoices: "invoices", inquiries: "inquiries", payouts: "payouts" }, "invoices");
  const tab = live ? live.tab : urlTab;
  const setTab = (t: Tab) => (live ? nav({ tab: t === "invoices" ? null : t, invoiceId: null, inquiryId: null, statementId: null }) : setUrlTab(t));

  // invoices
  const [demoList, setDemoList] = useState(seed);
  const list = live ? live.rows : demoList;
  const [f, setF] = useState<"All" | InvoiceStatus>("All");
  const [demoSel, setDemoSel] = useState(seed[2].id);
  const sel = list.find((x) => x.id === (live ? live.selectedInvoice?.id : demoSel)) ?? list[0] ?? null;
  const detail = live?.selectedInvoice;
  const shown = list.filter((x) => f === "All" || x.st === f);
  const hasPayment = live ? (detail?.payments.length ?? 0) > 0 : (sel?.method.includes(" · ") ?? false); // a payment status follows the method
  const [modal, setModal] = useState<null | "create" | "manual" | "confirm">(null);
  const [m, setM] = useState({ amount: "", ref: "", reason: "", paidAt: "2026-09-14" });
  const [tried, setTried] = useState(false);
  const [reminder, setReminder] = useState(noReminder);
  const [demoReminded, setDemoReminded] = useState<string | null>(null);
  const contracts = live ? live.contracts : seedContracts;
  const [nw, setNw] = useState({ contractId: "", from: "", to: "", dueAt: "" });
  const contract = contracts.find((k) => k.id === nw.contractId) ?? contracts[0];
  const close = () => { setModal(null); setTried(false); };
  const select = (id: string) => {
    setConflict(false);
    setReminder(noReminder);
    if (live) nav({ invoiceId: id, inquiryId: null });
    else setDemoSel(id);
  };
  const recipients = live?.recipients ?? [];
  const recipient = recipients.find((r) => r.id === reminder.recipient) ?? recipients[0];
  const channels: string[] = live ? recipient?.allowedChannels ?? [] : ["whatsapp", "email"];
  const channel = channels.includes(reminder.channel) ? reminder.channel : channels[0] ?? "email";
  const openPayment = (kind: "manual" | "confirm") => {
    if (!sel) return;
    setM({ amount: (sel.amountMinor / 100).toFixed(2), ref: "", reason: "", paidAt: m.paidAt });
    setModal(kind);
  };
  const record = () => {
    setTried(true);
    if (!sel || !m.ref.trim() || Math.round(+m.amount * 100) !== sel.amountMinor || (live && !m.reason.trim())) return;
    if (live) return act(() => recordManualPayment(sel.id, sel.version, m.ref.trim(), sel.amountMinor, sel.currency, m.reason.trim()), "Manual payment recorded", close);
    if (demoReminded) setConflict(true);
    setDemoList((l) => l.map((x) => (x.id === sel.id ? { ...x, st: "Paid", method: "Method not selected · confirmed" } : x)));
    toast("Manual payment recorded");
    close();
  };
  const confirm = () => {
    setTried(true);
    const id = detail?.processingPaymentId;
    const version = detail?.processingPaymentVersion;
    if (!sel || !m.ref.trim() || !m.reason.trim() || !id || version == null) return;
    act(() => confirmCardPayment(id, version, m.ref.trim(), sel.amountMinor, sel.currency, m.reason.trim()), "Card payment confirmed", close);
  };
  const create = () => {
    setTried(true);
    if (!contract || !nw.from || !nw.to || !nw.dueAt || nw.from >= nw.to) return;
    if (!live) {
      toast("Invoice created");
      return close();
    }
    act(() => createInvoice({ contractId: contract.id, contractVersion: contract.version, from: nw.from, to: nw.to, dueAt: nw.dueAt, amountMinor: contract.priceMinor, currency: contract.currency }), "Invoice created", close);
  };
  const preview = () => {
    if (!sel || !recipient || !reminder.reason.trim()) return;
    act(() => previewReminder(sel.id, recipient.id, channel, reminder.reason.trim()), "Preview ready — nothing is sent yet", (name) => setReminder((r) => ({ ...r, preview: reminderText(name, sel) })));
  };
  const send = () => {
    if (!sel || !reminder.reason.trim()) return;
    if (!live) {
      setDemoReminded(sel.id);
      return toast("Reminder queued (simulated)");
    }
    if (!recipient) return;
    act(() => sendReminder(sel.id, sel.version, recipient.id, channel, reminder.reason.trim()), "Reminder recorded (delivery simulated)", () => setReminder(noReminder));
  };

  // inquiries
  const inquiries = live ? live.inquiries : seedInquiries;
  const [demoIq, setDemoIq] = useState(seedInquiries[0].id);
  const [iqState, setIqState] = useState<"all" | "received" | "answered">("all");
  const iqShown = inquiries.filter((x) => iqState === "all" || x.state === iqState);
  const iq = inquiries.find((x) => x.id === (live ? live.selectedInquiryId : demoIq)) ?? iqShown[0] ?? null;
  const [reply, setReply] = useState("");
  const linked = sel ? inquiries.filter((q) => q.invoiceId === sel.id) : [];
  const invoiceLabel = (id: string) => list.find((x) => x.id === id)?.number ?? id;
  const openInquiry = (id: string) => {
    setReply("");
    setTried(false);
    if (live) nav({ tab: "inquiries", inquiryId: id, invoiceId: null });
    else { setDemoIq(id); setTab("inquiries"); }
  };
  const openInvoice = (id: string) => {
    if (live) return nav({ tab: null, invoiceId: id, inquiryId: null, customerId: null, propertyId: null, contractId: null, overdueOnly: null });
    setDemoSel(id);
    setTab("invoices");
  };

  // payouts
  const [period, setPeriod] = useState("");
  const [answers, setAnswers] = useState<Record<string, { reply: string; adjustment: string }>>({});
  const answer = (id: string) => answers[id] ?? { reply: "", adjustment: "" };
  const setAnswer = (id: string, patch: Partial<{ reply: string; adjustment: string }>) => setAnswers((a) => ({ ...a, [id]: { ...answer(id), ...patch } }));

  const scopeProps = live ? live.properties.filter((p) => !live.scope.customerId || p.customerId === live.scope.customerId) : [];
  const scopeContracts = live ? live.contractsOfCustomer.filter((k) => !live.scope.customerId || k.customerId === live.scope.customerId) : [];
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "invoices", label: "Invoices", count: list.length }, { id: "inquiries", label: "Inquiries", count: inquiries.length }, { id: "payouts" as Tab, label: "Contractor payouts", count: live?.statements?.length ?? 1 }]} />
      {tab === "payouts" ? (live ? (
        <div className="split-rev">
          <Card title="Statements" className="self-start" action={<span className="flex items-center gap-2"><Input aria-label="Closed month" value={period} onChange={(e) => setPeriod(e.target.value)} placeholder="2026-09" className="w-28" /><Btn size="sm" disabled={pending || !/^\d{4}-\d{2}$/.test(period)} onClick={() => act(() => generatePayouts(period), (n) => `${n} draft statement(s) generated for ${period}`)}>Generate</Btn></span>}>
            {(live.statements ?? []).length === 0 ? <EmptyState title="No statements">Generate the drafts of a closed month.</EmptyState> : <div className="flex flex-col gap-2">{live.statements!.map((s) => <ListRow key={s.id} selected={live.statement?.id === s.id} onClick={() => nav({ statementId: s.id })}><div className="min-w-0 flex-1"><b className="text-[13px]">{s.contractor}</b><div className="text-[11px] text-muted">{s.period} · {s.jobs} jobs · {s.net}</div></div><Badge tone={s.status === "Paid" ? "ok" : s.status === "Draft" ? "muted" : "primary"}>{s.status}</Badge></ListRow>)}</div>}
          </Card>
          {live.statement && (() => {
            const st = live.statement;
            return (
              <Card title={`${st.contractor} · ${st.period}`} sub={`Priced by the rate card effective at acceptance · pays ${st.payDate}`} action={<span className="flex gap-2">{st.status === "Draft" && <Btn size="sm" variant="primary" disabled={pending} onClick={() => act(() => transitionStatement(st.id, st.version, "approve"), "Statement approved")}>Approve</Btn>}{st.status === "Approved" && <Btn size="sm" variant="primary" disabled={pending || !st.payable} title={st.payable ? undefined : `Payable on or after ${st.payDate}`} onClick={() => act(() => transitionStatement(st.id, st.version, "mark_paid"), "Marked paid")}>Mark paid</Btn>}</span>}>
                <SummaryList items={[["Jobs", String(st.jobs)], ["Gross", st.gross], ["Deductions", st.deductions], ["Total", st.net], ["Status", st.status === "Approved" && !st.payable ? `Approved · payable on or after ${st.payDate}` : st.status]]} />
                {st.lines.length > 0 && <div className="mt-3"><DataTable rows={st.lines} rowKey={(r) => r.id} cols={[{ key: "j", label: "Job", render: (r) => <span className="font-mono text-xs">{r.job}</span> }, { key: "w", label: "Work", render: (r) => r.work }, { key: "k", label: "Kind", render: (r) => r.kind }, { key: "a", label: "Amount", render: (r) => r.amount }, { key: "n", label: "Note", render: (r) => r.note }]} /></div>}
                {st.queries.map((q) => (
                  <div key={q.id} className="mt-3 rounded-xl bg-surface2 p-3 text-[13px]">
                    <b>{q.topic}</b> · {q.line} · <Badge tone={q.state === "open" ? "warn" : "ok"}>{q.state}</Badge>
                    <p className="mt-1">“{q.message}”</p>
                    {q.reply ? <p className="mt-1 text-xs text-muted">Reply: {q.reply}{q.adjustment && ` · adjustment ${q.adjustment}, added to the contractor's next draft`}</p> : (
                      <div className="mt-2 flex flex-col gap-2">
                        <Textarea aria-label="Reply to the contractor" value={answer(q.id).reply} onChange={(e) => setAnswer(q.id, { reply: e.target.value })} placeholder="Reply to the contractor" />
                        <div className="flex flex-wrap items-center gap-2"><Input aria-label="Adjustment" type="number" value={answer(q.id).adjustment} onChange={(e) => setAnswer(q.id, { adjustment: e.target.value })} placeholder="Adjustment (optional)" className="w-48" /><Btn size="sm" disabled={pending || !answer(q.id).reply.trim()} onClick={() => act(() => resolvePayoutQuery(st.id, st.version, q.id, answer(q.id).reply.trim(), answer(q.id).adjustment ? Math.round(+answer(q.id).adjustment * 100) : null), "Answered")}>Answer</Btn></div>
                      </div>
                    )}
                  </div>
                ))}
                <p className="mt-2 text-[11px] text-muted">Contractors see approved and paid statements only (Partner › Payouts).</p>
              </Card>
            );
          })()}
        </div>
      ) : <ContractorPayouts />) : tab === "invoices" ? (
        <>
          {live && (
            <div className="flex flex-wrap items-end gap-3">
              <Field label="Customer"><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, propertyId: null, contractId: null, invoiceId: null })}><option value="">All customers</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
              <Field label="Property"><Select value={live.scope.propertyId ?? ""} onChange={(e) => nav({ propertyId: e.target.value || null, invoiceId: null })}><option value="">All properties</option>{scopeProps.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
              <Field label="Contract"><Select value={live.scope.contractId ?? ""} onChange={(e) => nav({ contractId: e.target.value || null, invoiceId: null })}><option value="">All contracts</option>{scopeContracts.map((k) => <option key={k.id} value={k.id}>{contracts.find((c) => c.id === k.id)?.label ?? k.id}</option>)}</Select></Field>
              {live.scope.overdueOnly && <Btn size="sm" onClick={() => nav({ overdueOnly: null, invoiceId: null })} aria-label="Show all invoices, not only overdue">Overdue only ✕</Btn>}
            </div>
          )}
          <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>{[["Outstanding", totals(list, ["Unpaid", "Overdue", "Processing"]), "unpaid · processing"], ["Overdue", totals(list, ["Overdue"]), "reminders allowed"], ["Processing", totals(list, ["Processing"]), "awaiting HQ confirmation"], ["Paid", totals(list, ["Paid"]), "in this scope"]].map(([a, b, c]) => <div key={a} className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">{a}</div><div className="text-xl font-bold">{b}</div><div className="text-[11px] text-muted">{c}</div></div>)}</div>
          <p className="text-[11px] text-muted">Totals per currency — never converted</p>
          <div className="flex flex-wrap items-center justify-between gap-3"><Tabs value={f} onChange={setF} tabs={(["All", "Unpaid", "Overdue", "Processing", "Paid"] as const).map((k) => ({ id: k, label: k, count: k === "All" ? list.length : list.filter((x) => x.st === k).length }))} /><Btn size="sm" variant="primary" onClick={() => { setNw({ contractId: contracts[0]?.id ?? "", from: "", to: "", dueAt: "" }); setModal("create"); }}>+ Create invoice</Btn></div>
          {!sel ? <Card title="Invoices"><EmptyState title="No invoices">No invoice matches this scope.</EmptyState></Card> : (
            <div className="split-rev">
              <Card title="Invoices" className="self-start">{shown.length === 0 ? <EmptyState title={`No ${f.toLowerCase()} invoices`}>Choose another status.</EmptyState> : <div className="flex flex-col gap-2">{shown.map((x) => <ListRow key={x.id} selected={sel.id === x.id} onClick={() => select(x.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{x.number}</b><b className="text-[13px]">{x.amt}</b></div><div className="text-[11px] text-muted">{x.meta}</div><div className="mt-0.5 flex items-center gap-2 text-[11px]"><Badge tone={tone(x.st)}>{x.st}</Badge><span className="text-muted">{x.method}</span></div></div></ListRow>)}</div>}</Card>
              <div className="flex min-w-0 flex-col gap-4">
                <Card title={sel.number} sub={`${sel.meta} · Invoice v${sel.version}`} action={<span className="text-lg font-bold">{sel.amt}</span>}>
                  <SummaryList cols={2} items={[["Billing period", sel.period], ["Due", sel.due], ["Payment method", sel.method.split(" · ")[0]], ["Payment status", sel.st === "Paid" ? "Paid" : sel.st === "Processing" ? "Processing — confirm to settle" : hasPayment ? sel.method.split(" · ")[1] ?? "—" : "— no payment yet"]]} />
                  {detail && detail.payments.length > 0 && <p className="mt-2 text-xs text-muted">Payments: {detail.payments.join(" · ")}</p>}
                  {sel.st === "Processing" && <div className="mt-3"><Btn variant="primary" disabled={pending || (live && !detail?.processingPaymentId)} onClick={() => { if (live) return openPayment("confirm"); setDemoList((l) => l.map((x) => (x.id === sel.id ? { ...x, st: "Paid" } : x))); toast("Card payment confirmed"); }}>Confirm card payment</Btn></div>}
                  {(sel.st === "Unpaid" || sel.st === "Overdue") && (hasPayment
                    ? <p className="mt-3 text-xs text-muted">Manual recording is only for invoices without a payment — this invoice already has one ({sel.method}). The customer can retry the card payment.</p>
                    : <><p className="mt-3 text-xs text-muted">The customer has not started a payment. Record a manual payment if it was received outside the app (e.g. bank transfer).</p><div className="mt-2"><Btn onClick={() => openPayment("manual")}>Record manual payment…</Btn></div></>)}
                  {conflict && <div className="mt-3"><Banner tone="warn">CONFLICT — the invoice changed first (for example a payment arrived), so the action was stopped. The screen shows the latest version.</Banner></div>}
                </Card>
                {sel.st === "Overdue" && <Card title="Payment reminder" sub="Only overdue unpaid invoices can be reminded. If payment arrives first, the reminder is stopped. Delivery is simulated.">
                  {live && recipients.length === 0 ? <p className="text-xs text-muted">No client of this customer can receive the reminder.</p> : <>
                    <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}><Field label="Recipient">{live ? <Select value={recipient?.id ?? ""} onChange={(e) => setReminder({ ...reminder, recipient: e.target.value, preview: "" })}>{recipients.map((r) => <option key={r.id} value={r.id}>{r.displayLabel} · {r.role}</option>)}</Select> : <Select><option>customer-a · Client</option></Select>}</Field><Field label="Channel"><Select value={channel} onChange={(e) => setReminder({ ...reminder, channel: e.target.value, preview: "" })}>{channels.map((c) => <option key={c} value={c}>{c === "whatsapp" ? "WhatsApp" : c === "email" ? "Email" : "In-app"}</option>)}</Select></Field></div>
                    <Field label="Reason · e.g. “2nd reminder after due date”"><Input value={reminder.reason} onChange={(e) => setReminder({ ...reminder, reason: e.target.value, preview: "" })} /></Field>
                    {(reminder.preview || !live) && <div className="my-3 rounded-xl bg-surface2 p-3 text-xs"><b>Preview · {channel}</b> <span className="text-muted">nothing is sent yet</span><br />{reminder.preview || `Your invoice ${sel.number} (${sel.amt}) was due on ${sel.due}. Pay by card or view payment instructions in the app →`}</div>}
                    <div className="mt-3 flex justify-end gap-2">{live && <Btn disabled={pending || !reminder.reason.trim()} onClick={preview}>Preview</Btn>}<Btn variant="primary" disabled={pending || !reminder.reason.trim() || (live && !reminder.preview)} onClick={send}>Send reminder</Btn></div>
                  </>}
                </Card>}
                <Card title="Related restriction" action={<Link className="text-xs font-semibold text-primary" href="/admin/restrictions">Open restrictions →</Link>}>
                  {live ? (detail?.restrictionIds.length ? detail.restrictionIds.map((id) => <p key={id} className="text-[13px]"><Link className="font-mono font-semibold text-primary" href={`/admin/restrictions/${id}`}>{id}</Link></p>) : <p className="text-xs text-muted">No restriction cites this invoice.</p>) : <p className="text-[13px]"><b>restriction-limited-a</b></p>}
                  <p className="text-xs text-muted">Once all cause invoices of a restriction are paid, it moves to release_requested. State and units are shown on the Restrictions screen (restriction.read).</p>
                </Card>
                <Card title="Customer inquiries">
                  {linked.length === 0 ? <p className="text-xs text-muted">No inquiry about this invoice.</p> : <div className="flex flex-col gap-2">{linked.map((q) => (
                    <div key={q.id} className={`rounded-xl p-2 ${live?.selectedInquiryId === q.id ? "bg-primary-soft" : ""}`}><p className="text-[13px]">{q.text}</p><p className="flex flex-wrap items-center gap-2 text-[11px] text-muted">{q.kind} · received {q.at} · {q.cust} {q.state === "answered" && <Badge tone="ok">Answered</Badge>}<button className="font-semibold text-primary" onClick={() => openInquiry(q.id)}>Open in Inquiries →</button></p></div>
                  ))}</div>}
                </Card>
              </div>
            </div>
          )}
        </>
      ) : inquiries.length === 0 ? <Card title="Customer inquiries"><EmptyState title="No inquiries">Customers have not sent any inquiry.</EmptyState></Card> : (
        <>
          <Tabs value={iqState} onChange={setIqState} tabs={(["all", "received", "answered"] as const).map((k) => ({ id: k, label: k === "all" ? "All" : k === "received" ? "Received" : "Answered", count: k === "all" ? inquiries.length : inquiries.filter((x) => x.state === k).length }))} />
          <div className="split-rev">
            <Card title="Customer inquiries" className="self-start">{iqShown.length === 0 ? <EmptyState title={`No ${iqState} inquiries`}>Choose another state.</EmptyState> : <div className="flex flex-col gap-2">{iqShown.map((x) => <ListRow key={x.id} selected={iq?.id === x.id} onClick={() => openInquiry(x.id)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{x.text}</b>{x.state === "answered" ? <Badge tone="ok">Answered</Badge> : <Badge tone="warn">Received</Badge>}</div><div className="text-[11px] text-muted">{x.kind} · {x.cust} · {x.at}</div></div></ListRow>)}</div>}</Card>
            {iq && (
              <Card title={`${iq.kind} inquiry · ${iq.cust}`} sub={`received ${iq.at}`}>
                <p className="rounded-xl bg-surface2 p-3 text-[13px]">{iq.text}</p>
                <SummaryList items={[["Invoice", iq.invoiceId ? <button key="i" className="font-semibold text-primary" onClick={() => openInvoice(iq.invoiceId!)}>{invoiceLabel(iq.invoiceId)} →</button> : "—"], ["Restriction", iq.restrictionId ? (live ? <Link key="r" className="font-mono font-semibold text-primary" href={`/admin/restrictions/${iq.restrictionId}`}>{iq.restrictionId}</Link> : iq.restrictionId) : "—"]]} />
                {iq.reply ? <p className="mt-3 text-[13px]"><b>Reply:</b> {iq.reply}</p> : <>
                  <div className="mt-3"><Field label="Reply (in-app only)" error={tried && !reply.trim() ? "A reply is required" : undefined}><Textarea value={reply} onChange={(e) => setReply(e.target.value)} maxLength={2000} /></Field></div>
                  <p className="mt-1 text-[11px] text-muted">A reply never changes invoices or restrictions.</p>
                  <div className="mt-2 flex justify-end"><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!reply.trim()) return; if (!live) return toast("Reply recorded in-app"); act(() => answerInquiry(iq.id, iq.version, reply.trim()), "Reply sent in-app", () => { setReply(""); setTried(false); }); }}>Send reply</Btn></div>
                </>}
              </Card>
            )}
          </div>
        </>
      )}
      <Modal open={modal === "manual" || modal === "confirm"} onClose={close} title={modal === "confirm" ? "Confirm card payment" : "Record manual payment"} footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={modal === "confirm" ? confirm : record}>{modal === "confirm" ? "Confirm payment" : "Record payment"}</Btn></>}>
        {sel && <Field label="Amount (must equal invoice — full payment only)" error={tried && Math.round(+m.amount * 100) !== sel.amountMinor ? `Must equal ${sel.amt}` : undefined}><Input type="number" value={m.amount} onChange={(e) => setM({ ...m, amount: e.target.value })} disabled={modal === "confirm"} /></Field>}
        <Field label="Reference" error={tried && !m.ref.trim() ? "Reference is required" : undefined}><Input value={m.ref} onChange={(e) => setM({ ...m, ref: e.target.value })} placeholder="manual-2026-0189" maxLength={128} /></Field>
        {live ? <Field label="Reason" error={tried && !m.reason.trim() ? "A reason is required" : undefined}><Input value={m.reason} onChange={(e) => setM({ ...m, reason: e.target.value })} placeholder="Bank transfer received" maxLength={1000} /></Field> : <Field label="Paid on"><Input type="date" value={m.paidAt} onChange={(e) => setM({ ...m, paidAt: e.target.value })} /></Field>}
        {modal === "manual" && <p className="text-[11px] text-muted">Marks the invoice paid; the payment method stays “not selected”. Related restrictions move to release_requested when all their cause invoices are paid. Resubmitting the same reference returns the same result.</p>}
      </Modal>
      <Modal open={modal === "create"} onClose={close} title="Create invoice" footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={create}>Create</Btn></>}>
        <Field label="Contract"><Select value={contract?.id ?? ""} onChange={(e) => setNw({ ...nw, contractId: e.target.value })}>{contracts.map((k) => <option key={k.id} value={k.id}>{k.label}</option>)}</Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}><Field label="Billing period start" error={tried && !nw.from ? "Required" : undefined}><Input type="date" value={nw.from} onChange={(e) => setNw({ ...nw, from: e.target.value })} /></Field><Field label="Billing period end" error={tried && (!nw.to || nw.from >= nw.to) ? "After the start" : undefined}><Input type="date" value={nw.to} onChange={(e) => setNw({ ...nw, to: e.target.value })} /></Field><Field label="Due date" error={tried && !nw.dueAt ? "Required" : undefined}><Input type="date" value={nw.dueAt} onChange={(e) => setNw({ ...nw, dueAt: e.target.value })} /></Field></div>
        {contract && <p className="text-xs text-muted">Amount from the contract version: {(contract.priceMinor / 100).toFixed(2)} {contract.currency}</p>}
      </Modal>
    </Page>
  );
}
