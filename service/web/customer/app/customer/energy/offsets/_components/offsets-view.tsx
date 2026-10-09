"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, Field, Input, Page, Steps, SummaryList } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { amount, klStamp } from "@ac/web/lib/energy";
import { kg3, type ApiOffsetRecord, type ApiQuote } from "@ac/web/lib/offsets";
import { getQuote, requestOffset } from "../actions";

type Live = {
  unitIds: string[]; subject: string; period: { from: string; to: string; label: string }; estimate: { emissions: string; savedEmissions: string; factor: string; comparable: boolean } | null; back: string;
  records: { id: string; amountKg: number; state: ApiOffsetRecord["state"]; proof: string | null; createdAt: string }[];
};
type Quote = ApiQuote & { estimatedAmountMinor: number | null; currency: string | null };
const stateLabel: Record<ApiOffsetRecord["state"], string> = { demo_requested: "Demo requested", demo_purchased: "Demo purchased", demo_retired: "Demo retired", failed: "Failed" };
const stateTone = { demo_requested: "primary", demo_purchased: "primary", demo_retired: "ok", failed: "crit" } as const;

/** Carbon offsets (FR-C13) in API mode: a demo quote (offsets.preview, no record) and a demo request (offsets.simulate
 * request) for the energy selection. Nothing real is bought or retired; savings never become a tradable balance. */
export function OffsetsView({ live }: { live: Live }) {
  const [pending, run] = useAction();
  const [step, setStep] = useState(0);
  const [amountKg, setAmountKg] = useState("1");
  const [purpose, setPurpose] = useState("Demo offset");
  const [quote, setQuote] = useState<Quote | null>(null);
  const [ack, setAck] = useState(false);
  const [done, setDone] = useState<ApiOffsetRecord | null>(null);
  const [explain, setExplain] = useState(false);
  const n = Number(amountKg);
  const amountError = !/^\d+(\.\d{1,3})?$/.test(amountKg.trim()) || !(n > 0 && n <= 100000) ? "Above 0, at most 100000, up to 3 decimals" : undefined;
  const purposeError = purpose.trim().length < 1 || purpose.trim().length > 1000 ? "1–1000 characters" : undefined;
  const ask = () => run(() => getQuote({ purpose: purpose.trim(), amountKg: n, period: { from: live.period.from, to: live.period.to }, unitIds: live.unitIds }), "Demo quote ready — no record was created", (q) => { setQuote(q); setStep(1); });
  const confirm = () => quote && run(() => requestOffset(quote.id, quote.version), "Demo request recorded", (r) => { setDone(r); setStep(3); },
    (f) => { if (f.code === "CONFLICT") { setQuote(null); setStep(0); } }); // an expired or used quote: get a new one
  const restart = () => { setDone(null); setQuote(null); setAck(false); setStep(0); };
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/customer/energy?${live.back}`} className="font-semibold text-primary">← Energy & cost</Link> › Carbon offsets (demo)</div>
      <Banner tone="warn" action={<DemoBadge />}>Demo only — no real carbon credits are bought or retired. Energy savings do not become a tradable balance. Records are labelled “Demo”.</Banner>
      <Steps steps={["Amount", "Quote", "Confirm"]} current={step} />
      <Card title="Based on Energy & cost" sub={`${live.subject} · ${live.period.label}${live.estimate ? ` · ${live.estimate.factor}` : ""}`}>
        {live.estimate ? <div className="flex flex-wrap gap-x-8 gap-y-1 text-[13px]"><span>Estimated emissions <b>{live.estimate.emissions}</b></span><span>Estimated savings <b className={live.estimate.comparable ? "text-ok" : "text-muted"}>{live.estimate.savedEmissions}</b></span></div>
          : <p className="text-[13px] text-muted">No estimate for this selection — the emission factor or valid readings are missing.</p>}
        <button type="button" className="mt-2 text-xs font-semibold text-primary" onClick={() => setExplain((e) => !e)}>ⓘ What is a carbon offset? {explain ? "▾" : "▸"}</button>
        {explain && <p className="mt-1 text-xs text-muted">An offset retires a carbon credit to compensate emissions. Here it is simulated — nothing real is purchased, and no market, price or balance exists.</p>}
      </Card>
      {done ? (
        <Card title="Demo request recorded" tone="ok">
          <Banner tone="ok">{done.id.slice(0, 8)} created · status {stateLabel[done.state]} · proof {done.demoCertificateRef ?? "Not issued"} · no external purchase was made.</Banner>
          <div className="mt-3"><Btn onClick={restart}>Start another</Btn></div>
        </Card>
      ) : step === 0 ? (
        <Card title="How much would you like to offset?">
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label="Amount (kgCO₂e, > 0)" hint="Up to 3 decimals" error={amountError}><Input inputMode="decimal" value={amountKg} onChange={(e) => setAmountKg(e.target.value)} /></Field>
            <Field label="Purpose" error={purposeError}><Input value={purpose} onChange={(e) => setPurpose(e.target.value)} /></Field>
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><span className="text-xs text-muted">Getting a quote creates no record.</span><Btn variant="primary" disabled={pending || !!amountError || !!purposeError} onClick={ask}>Get demo quote</Btn></div>
        </Card>
      ) : step === 1 && quote ? (
        <Card title={<span className="flex items-center gap-2">Demo quote <Badge tone="primary">◷ Quoted</Badge></span>}>
          <SummaryList items={[
            ["Amount", `${kg3(quote.amountKg)} kgCO₂e`], ["Period / units", `${klStamp(quote.period.from)} → ${klStamp(quote.period.to)} · ${live.subject}`],
            ["Quote ID", `${quote.id.slice(0, 8)} (demo)`], ["Estimated amount", quote.estimatedAmountMinor !== null && quote.currency ? `${amount(quote.estimatedAmountMinor, quote.currency)} (demo)` : "—"],
            ["Expires", klStamp(quote.expiresAt)], ["Records created", "None yet"],
          ]} />
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(0)}>← Back</Btn><Btn variant="primary" onClick={() => setStep(2)}>Continue</Btn></div>
        </Card>
      ) : quote ? (
        <Card title="Confirm demo request">
          <SummaryList items={[["Amount", `${kg3(quote.amountKg)} kgCO₂e`], ["Quote", `${quote.id.slice(0, 8)} · expires ${klStamp(quote.expiresAt)}`], ["Result", "A demo record “Demo requested” is created. Proof stays “Not issued”."]]} />
          <div className="mt-3"><Check checked={ack} onChange={setAck} label="I understand this is a simulated request — not a real purchase." /></div>
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(1)}>← Back</Btn><Btn variant="primary" disabled={!ack || pending} onClick={confirm}>Submit demo request</Btn></div>
        </Card>
      ) : null}
      <Card title="Demo offset records" sub="Demo retirement ≠ certified credit">
        {live.records.length === 0 ? <p className="text-[13px] text-muted">No demo record yet.</p> : (
          <ul className="flex flex-col divide-y divide-line text-[13px]">
            {live.records.map((r) => <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 py-2"><span><b>{r.id.slice(0, 8)}</b> <span className="text-xs text-muted">· {kg3(r.amountKg)} kgCO₂e · {klStamp(r.createdAt).slice(0, 10)}</span></span><span className="flex items-center gap-2 text-muted">Proof: {r.proof ?? "Not issued"}<Badge tone={stateTone[r.state]}>{stateLabel[r.state]}</Badge></span></li>)}
          </ul>
        )}
      </Card>
      <details className="rounded-2xl border border-line bg-surface p-4 text-[13px]">
        <summary className="cursor-pointer font-bold">Future carbon market (concept) <span className="text-xs font-normal text-muted">Not available · no prices, balances or trading</span></summary>
        <p className="mt-2 text-xs text-muted">Concept only — provider {quote ? quote.marketConcept.providerLabel : "not selected"}, verification {quote ? quote.marketConcept.verificationStatus : "unverified"}, ledger {quote ? quote.marketConcept.ledgerStatus.replace("_", " ") : "not connected"}. Integration partners and verification conditions are undecided; nothing here is tradable.</p>
      </details>
    </Page>
  );
}
