"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, DemoBadge, EmptyState, Field, Modal, Page, SummaryList, Tabs, Textarea, Timeline } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { InvoiceStatus } from "@ac/web/lib/billing";
import { stateTone } from "@ac/web/lib/restrictions";
import { methodLabel, previewText, type InquiryLine, type NoticeUnit, type PaymentLine, type PreviewParams, type RestrictionNotice } from "@ac/web/lib/clientBilling";
import { demoOutcome, paymentInstructions, previewMessage, sendInquiry, startPayment } from "../actions";

export type InvoiceLive = {
  tab: "payment" | "restriction" | "inquiry";
  /** The display time zone is not Kuala Lumpur, so the due line says the date is a Kuala Lumpur business day. */
  otherZone: boolean;
  invoice: { id: string; version: number; number: string; amount: string; due: string; dueDate: string; status: InvoiceStatus; payable: boolean };
  contract: { name: string; id: string; scope: string } | null;
  payments: PaymentLine[]; open: { id: string; version: number; status: string; method: string | null } | null;
  last: { id: string; status: string; reference: string | null; method: string | null } | null;
  restriction: {
    notice: RestrictionNotice; units: NoticeUnit[]; timeline: { time: string; title: string; tone?: "warn" | "ok" }[]; inquiries: InquiryLine[];
    reason: string; noticeAt: string; start: string; released: string;
  } | null;
  inquiries: InquiryLine[]; inquiryId: string | null; reminders: { id: string; channel: string; at: string }[];
};
type Choice3 = "demo_credit_card" | "demo_debit_card" | "instructions";
const statusTone = { Overdue: "crit", Unpaid: "warn", Processing: "primary", Paid: "ok" } as const;
const paymentTone = { initiated: "primary", processing: "primary", confirmed: "ok", failed: "crit" } as const;
const paymentLabel = { initiated: "Initiated", processing: "Processing", confirmed: "Paid", failed: "Failed" } as const;
const channelName: Record<string, string> = { email: "Email", whatsapp: "WhatsApp", inApp: "In-app" };

/** Invoice / demo payment (FR-C11) and the cooling-restriction notice (FR-C12) in API mode. No card data is ever asked
 * for; an invoice is paid only after the confirmation event, and a restriction is never shown released before the
 * units confirm. Texts in the user's display language (IR267). */
export function InvoiceView({ live }: { live: InvoiceLive }) {
  const t = useT();
  const nav = useUrlPatch();
  const i = live.invoice;
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href="/customer/payments" className="font-semibold text-primary">{t("← Contracts & payments")}</Link> › {t("Invoice {number}", { number: i.number })}</div>
      <Tabs value={live.tab} onChange={(x) => nav({ tab: x === "payment" ? null : x })} tabs={[
        { id: "payment" as const, label: t("Payment") },
        ...(live.restriction ? [{ id: "restriction" as const, label: t("Cooling restriction") }] : []),
        { id: "inquiry" as const, label: t("Inquiries"), count: live.inquiries.length },
      ]} />
      {live.tab === "restriction" && live.restriction ? <RestrictionTab live={live} /> : live.tab === "inquiry" ? <InquiryTab live={live} /> : <PaymentTab live={live} />}
    </Page>
  );
}

function PaymentTab({ live }: { live: InvoiceLive }) {
  const t = useT();
  const [pending, run] = useAction();
  const [method, setMethod] = useState<Choice3>(live.last?.method === "demo_debit_card" ? "demo_debit_card" : "demo_credit_card");
  const [instructions, setInstructions] = useState<PreviewParams | null>(null);
  const [preview, setPreview] = useState<{ channel: "email" | "whatsapp"; text: { subject: string | null; body: string } } | null>(null);
  const i = live.invoice;
  const open = live.open;
  const failed = !open && i.payable && live.last?.status === "failed";
  const confirmed = i.status === "Paid" && live.last?.status === "confirmed";
  const methodText = (m: string | null, none: string) => (m ? (methodLabel[m] ? t(methodLabel[m]) : m) : t(none));
  const pay = () => method === "instructions"
    ? run(() => paymentInstructions(i.id, i.version), t("Payment instructions shown — the invoice stays unpaid"), setInstructions)
    : run(() => startPayment(i.id, i.version, method), t("Demo payment started — processing"));
  const show = (channel: "email" | "whatsapp") => run(() => previewMessage(i.id, channel), t("Preview only — nothing was sent"), () => setPreview({ channel, text: previewText(channel, i, t) }));
  const methods = [
    ["demo_credit_card", "Credit card (demo)", "Simulated card payment — no card details"], ["demo_debit_card", "Debit card (demo)", "Simulated card payment — no card details"],
    ["instructions", "Payment instructions", "Bank transfer details — viewing them does not pay the invoice"],
  ] as const;
  return (
    <>
      <Banner tone="warn" action={<DemoBadge />}>{t("Demo payment — no real money moves. No card number, CVV or expiry is collected.")}</Banner>
      <div className="split">
        <Card title={t(i.status === "Paid" ? "Amount paid" : "Amount due")} action={<Badge tone={statusTone[i.status]} icon={i.status === "Overdue" ? "‼" : i.status === "Paid" ? "✓" : i.status === "Processing" ? "⟳" : undefined}>{t(i.status)}</Badge>}>
          <div className="text-[32px] font-bold">{i.amount.split(" ")[0]} <span className="text-base text-muted">{i.amount.split(" ")[1]}</span></div>
          <p className="text-xs text-muted">{i.due}{live.otherZone && ` · ${t("Kuala Lumpur date (Asia/Kuala_Lumpur)")}`}</p>
          <SummaryList items={[[t("Contract"), live.contract ? `${live.contract.name} · ${live.contract.id.slice(0, 8)}` : "—"], [t("Contract scope"), live.contract?.scope ?? "—"]]} />
          {open && (
            <div className="mt-4 flex flex-col gap-3">
              <Banner icon="⟳"><b>{t("Processing payment")}</b><div className="text-xs">{t("{id} · {method}. Processing is not paid — please don’t start another payment. This page updates when the receipt is confirmed.", { id: open.id.slice(0, 8), method: methodText(open.method, "demo") })}</div></Banner>
              <div className="rounded-xl border border-dashed border-warn bg-warn-soft/30 p-3 text-[13px]">
                <div className="mb-2 flex items-center gap-2"><DemoBadge /><b>{t("Demo payment provider")}</b><span className="text-xs text-muted">{t("test events, separate from the payment form")}</span></div>
                <div className="flex flex-wrap gap-2">
                  {open.status === "initiated" && <Btn size="sm" disabled={pending} onClick={() => run(() => demoOutcome(open.id, open.version, "processing"), t("Payment is processing"))}>{t("Processing")}</Btn>}
                  {open.status === "processing" && <Btn size="sm" variant="primary" disabled={pending} onClick={() => run(() => demoOutcome(open.id, open.version, "confirm"), t("Payment confirmed — the invoice is paid"))}>{t("Confirm receipt")}</Btn>}
                  <Btn size="sm" variant="danger" disabled={pending} onClick={() => run(() => demoOutcome(open.id, open.version, "fail"), t("Payment declined — the invoice stays unpaid"))}>{t("Decline")}</Btn>
                </div>
              </div>
            </div>
          )}
          {confirmed && <div className="mt-4"><Banner tone="ok"><b>{t("Payment confirmed")}</b><div className="text-xs">{t("Reference {reference} · {method}. The invoice is now paid.", { reference: live.last!.reference ?? "—", method: methodText(live.last!.method, "recorded by HQ") })}{live.restriction?.notice.live ? ` ${t("A cooling-restriction release is requested — cooling returns only after the units confirm.")}` : ""}</div></Banner></div>}
          {failed && <div className="mt-4"><Banner tone="crit"><b>{t("Payment failed")}</b><div className="text-xs">{t("Demo payment {id} failed. Nothing was charged; the invoice is still unpaid. Your method is kept — you can try again.", { id: live.last!.id.slice(0, 8) })}</div></Banner></div>}
          {i.payable && !open && (
            <>
              <h3 className="mb-2 mt-4 text-[13px] font-bold">{t("Payment method")}</h3>
              <div className="flex flex-col gap-2">
                {methods.map(([k, label, d]) => (
                  <button key={k} type="button" onClick={() => setMethod(k)} aria-pressed={method === k} className={`rounded-xl border p-3 text-left ${method === k ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2"}`}><b className="text-[13px]">{t(label)}</b><div className="text-xs text-muted">{t(d)}</div></button>
                ))}
              </div>
              <div className="mt-3 flex justify-end"><Btn variant="primary" disabled={pending} onClick={pay}>{t(method === "instructions" ? "View instructions" : failed ? "Try again — Demo" : "Continue to payment — Demo")}</Btn></div>
            </>
          )}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Payment history")}>
            {live.payments.length === 0 ? <p className="text-xs text-muted">{t("No payments yet")}</p> : (
              <div className="flex flex-col">
                {live.payments.map((p) => <div key={p.id} className="flex flex-wrap items-center justify-between gap-2 border-t border-line py-2 text-[13px] first:border-0"><span>{p.label}<span className="block text-xs text-muted">{p.at}{p.reference ? ` · ${p.reference}` : ""}</span></span><Badge tone={paymentTone[p.status]}>{t(paymentLabel[p.status])}</Badge></div>)}
              </div>
            )}
          </Card>
          <Card title={t("Payment reminders")} sub={t("Notifications only — not payment methods")}>
            {live.reminders.length === 0 ? <p className="text-xs text-muted">{t("No reminder has been sent for this invoice.")}</p> : live.reminders.map((r) => (
              <div key={r.id} className="border-t border-line py-2 text-[13px] first:border-0">{t("{channel} · sent {date} (demo record)", { channel: channelName[r.channel] ? t(channelName[r.channel]) : r.channel, date: r.at })}</div>
            ))}
            <div className="mt-2 flex gap-2"><Btn size="sm" variant="ghost" disabled={pending} onClick={() => show("email")}>{t("Preview email")}</Btn><Btn size="sm" variant="ghost" disabled={pending} onClick={() => show("whatsapp")}>{t("Preview WhatsApp")}</Btn></div>
          </Card>
        </div>
      </div>
      {live.restriction && <RestrictionBanner live={live} />}
      <Modal open={!!instructions} onClose={() => setInstructions(null)} title={t("Payment instructions (demo)")} footer={<Btn onClick={() => setInstructions(null)}>{t("Close")}</Btn>}>
        {instructions && <><p className="rounded-xl bg-surface2 p-3 text-[13px]">{t("Transfer {amount} for {target} to the demo bank account “AC Project HQ (demo)” with the invoice number as the reference. HQ records the transfer when it arrives.", { amount: i.amount, target: instructions.targetName })}</p><p className="mt-2 text-xs text-muted">{t("Viewing the instructions does not pay the invoice — it stays unpaid until HQ confirms the transfer.")}</p></>}
      </Modal>
      <Modal open={!!preview} onClose={() => setPreview(null)} title={preview ? t("{channel} preview", { channel: t(channelName[preview.channel]) }) : ""} footer={<Btn onClick={() => setPreview(null)}>{t("Close")}</Btn>}>
        {preview && <>
          <div className="mb-2"><Choice value={preview.channel} onChange={(v: "email" | "whatsapp") => show(v)} options={[{ id: "email", label: t("Email") }, { id: "whatsapp", label: t("WhatsApp") }]} /></div>
          <div className="rounded-xl bg-surface2 p-3 text-[13px]">{preview.text.subject && <b className="mb-1 block">{t("Subject: {subject}", { subject: preview.text.subject })}</b>}{preview.text.body}</div>
          <p className="mt-2 text-xs text-muted">{t("Preview — not sent.")}</p>
        </>}
      </Modal>
    </>
  );
}

function RestrictionBanner({ live }: { live: InvoiceLive }) {
  const t = useT();
  const n = live.restriction!.notice;
  if (!n.live) return null;
  return (
    <Banner tone={n.state === "release_requested" ? "primary" : "warn"} action={<Link className="text-xs font-semibold text-primary" href={`/customer/payments/${live.invoice.id}?tab=restriction&restrictionId=${n.id}`}>{t("View restriction details →")}</Link>}>
      <span className="flex flex-wrap items-center gap-2"><b>{t(n.state === "release_requested" ? "Cooling restriction — release requested" : "Cooling restriction active")}</b><Badge tone={stateTone(n.state)}>{n.label}</Badge></span>
      <div className="text-xs">{t("{units} — {policy}. Related to payment, but separate from it: paying does not restore cooling immediately.", { units: n.units, policy: n.policy })}</div>
    </Banner>
  );
}

function RestrictionTab({ live }: { live: InvoiceLive }) {
  const t = useT();
  const r = live.restriction!;
  return (
    <>
      <Card title={t("Cooling restriction · {id}", { id: r.notice.id.slice(0, 8) })} sub={live.contract ? `${live.contract.name} · ${live.contract.id.slice(0, 8)}` : undefined} action={<Badge tone={stateTone(r.notice.state)}>{r.notice.label}</Badge>}>
        <SummaryList items={[[t("Reason"), r.reason], [t("Notice"), r.noticeAt], [t("Start time"), r.start], [t("Target units"), r.notice.units], [t("Release condition"), t("All cause invoices paid")], [t("Release requested"), r.released]]} />
        <h3 className="mb-2 mt-4 text-[13px] font-bold">{t("Per-unit state")}</h3>
        <div className="flex flex-col gap-2">
          {r.units.map((u) => (
            <div key={u.unitId} className="rounded-xl border border-line p-3 text-[13px]">
              <span className="flex flex-wrap items-center gap-2"><b>{u.name}</b><Badge tone={u.tone}>{u.apply}</Badge>{u.release && <Badge tone={u.releaseTone}>{u.release}</Badge>}</span>
              {u.note && <div className="text-xs text-muted">{u.note}</div>}
            </div>
          ))}
        </div>
        <h3 className="mb-2 mt-4 text-[13px] font-bold">{t("Timeline")}</h3>
        <Timeline items={r.timeline} />
        {r.notice.state === "release_requested" && <div className="mt-3"><Banner>{t("Status stays “Release requested” until every unit confirms. It is never shown as Released while a device result is unknown.")}</Banner></div>}
      </Card>
      <InquiryCard title={t("Send inquiry to HQ")} lines={r.inquiries} input={{ subjectType: "restriction", restrictionId: r.notice.id }} />
    </>
  );
}

function InquiryTab({ live }: { live: InvoiceLive }) {
  const t = useT();
  return <InquiryCard title={t("Inquiries about invoice {number}", { number: live.invoice.number })} lines={live.inquiries} input={{ subjectType: "payment", invoiceId: live.invoice.id }} selected={live.inquiryId} />;
}

function InquiryCard({ title, lines, input, selected }: { title: string; lines: InquiryLine[]; input: { subjectType: "payment"; invoiceId: string } | { subjectType: "restriction"; restrictionId: string }; selected?: string | null }) {
  const t = useT();
  const [pending, run] = useAction();
  const [message, setMessage] = useState("");
  const [tried, setTried] = useState(false);
  const e = message.trim().length < 1 || message.trim().length > 2000 ? t("1–2000 characters") : undefined;
  return (
    <Card title={title} sub={t("Inquiries are received in-app only — nothing is sent externally. HQ answers here and in your notifications.")}>
      {lines.length === 0 ? <EmptyState title={t("No inquiry yet")}>{t("Ask HQ about this payment or restriction below.")}</EmptyState> : (
        <div className="flex flex-col gap-2">
          {lines.map((q) => (
            <div key={q.id} className={`rounded-xl border p-3 text-[13px] ${q.id === selected ? "border-primary bg-primary-soft/40" : "border-line"}`}>
              <div className="flex flex-wrap items-center justify-between gap-2"><b>{q.subject}</b><span className="flex items-center gap-2 text-xs text-muted">{q.at}<Badge tone={q.state === "answered" ? "ok" : "primary"}>{t(q.state === "answered" ? "Answered" : "Received")}</Badge></span></div>
              <p className="mt-1 whitespace-pre-wrap">{q.message}</p>
              {q.reply && <p className="mt-2 rounded-lg bg-surface2 p-2 text-xs"><b>HQ:</b> {q.reply}</p>}
            </div>
          ))}
        </div>
      )}
      <div className="mt-3 flex flex-col gap-2">
        <Field label={t("Your message")} error={tried ? e : undefined}><Textarea rows={3} value={message} onChange={(x) => setMessage(x.target.value)} /></Field>
        <div className="flex justify-end"><Btn variant="primary" disabled={pending} onClick={() => { setTried(true); if (!e) run(() => sendInquiry({ ...input, message }), t("Inquiry sent to HQ"), () => { setMessage(""); setTried(false); }); }}>{t("Send inquiry")}</Btn></div>
      </div>
    </Card>
  );
}
