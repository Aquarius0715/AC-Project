"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, EmptyState, Field, Input, ListRow, Modal, Page, Select, Steps, SummaryList, Tabs, Textarea, Timeline } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klStamp } from "@ac/web/lib/energy";
import { eventItem, kg3, quoteErrors, quoteInput, stages, type ApiQuote, type QuoteDraft, type RecordRow } from "@ac/web/lib/offsets";
import { quoteOffset, requestOffset, retryOffset, simulateOffset } from "../actions";

type Live = {
  tab: "records" | "market"; canWrite: boolean; rows: RecordRow[]; selectedId: string | null; customers: { id: string; name: string }[];
  units: { id: string; label: string; customerId: string }[];
};
const tone = (s: RecordRow["state"]) => (s === "demo_retired" ? "ok" : s === "failed" ? "crit" : s === "demo_purchased" ? "primary" : "warn");
const blank: QuoteDraft = { customerId: "", unitIds: [], from: "", to: "", purpose: "", amountKg: "" };

/** The offset demo registry (FR-A15) in API mode: records come from the server; quotes, requests and the simulated
 * purchase / retirement / failure / retry are Server Actions. Nothing here is a real credit, certificate or market. */
export function OffsetsView({ live }: { live: Live }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const sel = live.rows.find((r) => r.id === live.selectedId) ?? null;
  const [modal, setModal] = useState(false);
  const [d, setD] = useState<QuoteDraft>(blank);
  const [tried, setTried] = useState(false);
  const [quote, setQuote] = useState<ApiQuote | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [ack, setAck] = useState(false);
  const errors = quoteErrors(d);
  const set = (patch: Partial<QuoteDraft>) => { setQuote(null); setConfirmed(false); setD((x) => ({ ...x, ...patch })); };
  const units = live.units.filter((u) => u.customerId === d.customerId);
  const close = () => { setModal(false); setTried(false); setQuote(null); setConfirmed(false); };
  const getQuote = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => quoteOffset(quoteInput(d)), "Demo quote ready — valid for 15 minutes", setQuote);
  };
  const request = () => quote && confirmed && run(() => requestOffset(quote.id, quote.version), "Demo request recorded", (id) => { close(); setD(blank); nav({ recordId: id }); });
  const step = (event: "purchase_confirm" | "retire" | "fail") => sel?.current && run(() => simulateOffset(sel.id, sel.version, sel.current!.id, event), event === "purchase_confirm" ? "Demo purchase confirmed" : event === "retire" ? "Demo retired" : "Failure recorded — the previous stage is kept", () => setAck(false));
  const retry = () => sel?.current && run(() => retryOffset(sel.id, sel.version, sel.current!.id), "Retry started — a new attempt", () => setAck(false));
  return (
    <Page>
      <Banner tone="warn" action={<DemoBadge />}>Demo only — quotes, purchases and retirements are simulated. No real credit, certificate, balance or market is involved.</Banner>
      <Tabs value={live.tab} onChange={(t) => nav({ tab: t === "records" ? null : t })} tabs={[{ id: "records", label: "Demo records", count: live.rows.length }, { id: "market", label: "Market concept" }]} />
      {live.tab === "market" ? (
        <Card title="Carbon market — future concept" sub="Read only · no orders, prices, balances or trading">
          <SummaryList items={[["Stage", "future_concept"], ["Provider", "Not selected"], ["Verification", "unverified"], ["Ledger", "not_connected"]]} />
          <p className="mt-3 text-[13px]">Future partners and verification conditions are undecided. Nothing here is tradable, priced or balance-carrying, and it is a different state from a simulated retirement.</p>
        </Card>
      ) : (
        <div className="split-rev">
          <Card title="Demo offset records" action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => { setD(blank); setModal(true); }}>+ New demo quote</Btn>} className="self-start">
            {live.rows.length === 0 ? <EmptyState title="No records">Get a demo quote and request it to create a record.</EmptyState> : <div className="flex flex-col gap-2">{live.rows.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setAck(false); nav({ recordId: x.id }); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)}</b><Badge tone={tone(x.state)}>{x.state}</Badge></div><div className="text-[11px] text-muted">{x.customer} · {x.amount}</div><div className="text-[11px] text-muted">{x.next}</div></div></ListRow>)}</div>}
            <p className="mt-3 text-[11px] text-muted">Energy savings (<Link className="text-primary" href="/admin/energy">Energy analysis</Link>) and MRV estimates (<Link className="text-primary" href="/admin/mrv">MRV</Link>) are shown separately and never converted into credits.</p>
          </Card>
          {!sel ? <Card title="Record"><EmptyState title="Nothing selected">Choose a record.</EmptyState></Card> : (
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={`Record ${sel.id.slice(0, 8)}`} sub={`${sel.customer} · quote ${sel.r.quoteId.slice(0, 8)} · version ${sel.version}`}>
                {sel.stage >= 0 ? <Steps steps={stages} current={sel.stage} /> : (
                  <Banner tone="crit" action={live.canWrite && sel.current && <Btn size="sm" disabled={pending} onClick={retry}>Retry</Btn>}>{sel.next}. A retry starts a new attempt of the failed stage.</Banner>
                )}
                <div className="mt-4"><SummaryList items={[
                  ["Simulated purchase", `${sel.amount} — retires as a whole in 1A`], ["Provider / scheme", "Not selected · demo"],
                  ["Current attempt", sel.current ? `${sel.current.stage} · ${sel.current.status} · ${klStamp(sel.current.startedAt)}` : "none"],
                  ["Purchase reference", sel.r.purchaseRef ?? "—"],
                  ["Demo certificate", sel.r.demoCertificateRef ? `${sel.r.demoCertificateRef} (demo — not a real certificate)` : "Shown only after the demo retirement (DEMO-…)"],
                ]} /></div>
              </Card>
              {live.canWrite && sel.state === "demo_requested" && sel.current && (
                <Card title="Next step · demo purchase"><div className="flex flex-wrap gap-2"><Btn variant="primary" disabled={pending} onClick={() => step("purchase_confirm")}>Confirm simulated purchase</Btn><Btn disabled={pending} onClick={() => step("fail")}>Simulate a failure</Btn></div></Card>
              )}
              {live.canWrite && sel.state === "demo_purchased" && sel.current && (
                <Card title="Next step · demo retirement">
                  <Check label="I understand this is a demo retirement. It does not retire a real credit." checked={ack} onChange={setAck} />
                  <p className="my-1 text-[11px] text-muted">Disabled until confirmed. Retiring before the purchase is not possible.</p>
                  <div className="flex flex-wrap gap-2"><Btn variant="primary" disabled={pending || !ack} onClick={() => step("retire")}>Retire (demo)</Btn><Btn disabled={pending} onClick={() => step("fail")}>Simulate a failure</Btn></div>
                </Card>
              )}
              <Card title="Event history" action={<Link className="text-xs font-semibold text-primary" href="/admin/audit">Open in audit →</Link>}>
                {sel.r.eventHistory.length === 0 ? <p className="text-xs text-muted">No events yet.</p> : <Timeline items={sel.r.eventHistory.map(eventItem)} />}
              </Card>
            </div>
          )}
        </div>
      )}
      <Modal open={modal} onClose={close} title="New demo quote" wide footer={<><Btn onClick={close}>Cancel</Btn>{!quote ? <Btn variant="primary" disabled={pending} onClick={getQuote}>Get demo quote</Btn> : <Btn variant="primary" disabled={pending || !confirmed} onClick={request}>Request (demo)</Btn>}</>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Field label="Customer" error={tried ? errors.customerId : undefined}><Select value={d.customerId} onChange={(e) => set({ customerId: e.target.value, unitIds: [] })}><option value="">Select…</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label="Amount (kgCO₂e)" error={tried ? errors.amountKg : undefined}><Input type="number" min="0" step="0.001" value={d.amountKg} onChange={(e) => set({ amountKg: e.target.value })} /></Field>
          <Field label="Period start (Kuala Lumpur)" error={tried ? errors.period : undefined}><Input type="datetime-local" value={d.from} onChange={(e) => set({ from: e.target.value })} /></Field>
          <Field label="Period end"><Input type="datetime-local" value={d.to} onChange={(e) => set({ to: e.target.value })} /></Field>
        </div>
        <Field label="Purpose" error={tried ? errors.purpose : undefined}><Textarea value={d.purpose} maxLength={1000} onChange={(e) => set({ purpose: e.target.value })} placeholder="Offset the demo office's Scope 2 for August (simulation)" /></Field>
        <div><p className="mb-1 text-[13px] font-semibold">Units</p>{!d.customerId ? <p className="text-xs text-muted">Choose the customer first.</p> : <div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{units.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} />)}</div>}{tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}</div>
        {quote && (
          <div className="flex flex-col gap-2 rounded-xl border border-line p-3">
            <div className="flex items-center gap-2"><b className="text-[13px]">Demo quote {quote.id.slice(0, 8)} · v{quote.version}</b><DemoBadge /></div>
            <SummaryList items={[["Amount", `${kg3(quote.amountKg)} kgCO₂e`], ["Expires", klStamp(quote.expiresAt)], ["Provider / scheme", `Not selected · ${quote.scheme}`], ["Price", "none — no market price"], ["Market concept", `${quote.marketConcept.stage} · verification ${quote.marketConcept.verificationStatus} · ledger ${quote.marketConcept.ledgerStatus}`]]} />
            <Check label="I confirm no real transaction takes place (demoConfirmed)." checked={confirmed} onChange={setConfirmed} />
          </div>
        )}
      </Modal>
    </Page>
  );
}
