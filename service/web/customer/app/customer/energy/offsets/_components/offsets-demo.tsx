"use client";

import Link from "next/link";
import { useState } from "react";
import { Banner, Btn, Card, Check, DemoBadge, Field, Input, Page, Steps, SummaryList, useToast } from "@ac/web/components/ui";

/** The Phase 1A demo of /customer/energy/offsets (fixtures; DATA_SOURCE≠api). */
export function OffsetsDemo() {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [amount, setAmount] = useState("1");
  const [ack, setAck] = useState(false);
  const [done, setDone] = useState(false);
  const [explain, setExplain] = useState(false);
  const valid = +amount > 0;
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href="/customer/energy" className="font-semibold text-primary">← Energy & cost</Link> › Carbon offsets (demo)</div>
      <Banner tone="warn" action={<DemoBadge />}>Demo only — no real carbon credits are bought or retired. Energy savings do not become a tradable balance. Records are labelled “Demo”.</Banner>
      <Steps steps={["Amount", "Quote", "Confirm"]} current={done ? 3 : step} />
      <Card title="Based on Energy & cost" sub="Bedroom AC · Sep 14–20 · factor-demo-2026">
        <div className="flex flex-wrap gap-x-8 gap-y-1 text-[13px]"><span>Estimated emissions <b>40.0</b> kgCO2e</span><span>Estimated savings <b className="text-ok">10.0</b> kgCO2e</span></div>
        <button className="mt-2 text-xs font-semibold text-primary" onClick={() => setExplain((e) => !e)}>ⓘ What is a carbon offset? {explain ? "▾" : "▸"}</button>
        {explain && <p className="mt-1 text-xs text-muted">An offset retires a carbon credit to compensate emissions. Here it is simulated — nothing real is purchased.</p>}
      </Card>
      {done ? (
        <Card title="Demo request recorded" tone="ok"><Banner tone="ok">A demo record “Demo requested” (offset-0232) was created. Proof stays “Not issued”.</Banner><div className="mt-3"><Btn onClick={() => { setDone(false); setStep(0); setAck(false); }}>Start another</Btn></div></Card>
      ) : step === 0 ? (
        <Card title="How much would you like to offset?">
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label="Amount (kgCO2e, > 0)" error={valid ? undefined : "Positive numbers only"} hint="Positive numbers only"><Input type="number" value={amount} onChange={(e) => setAmount(e.target.value)} /></Field>
            <Field label="Purpose"><Input value="Demo offset" readOnly /></Field>
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><span className="text-xs text-muted">Getting a quote creates no record.</span><Btn variant="primary" disabled={!valid} onClick={() => setStep(1)}>Get demo quote</Btn></div>
        </Card>
      ) : step === 1 ? (
        <Card title="Demo quote">
          <SummaryList items={[["Amount", `${amount} kgCO2e`], ["Period / units", "Sep 14–20 · Bedroom AC"], ["Quote ID", "quote-0107 (demo)"], ["Expires", "09:55 (15 minutes from now)"], ["Records created", "None yet"]]} />
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(0)}>← Back</Btn><Btn variant="primary" onClick={() => setStep(2)}>Continue</Btn></div>
        </Card>
      ) : (
        <Card title="Confirm demo request">
          <SummaryList items={[["Amount", `${amount} kgCO2e`], ["Quote", "quote-0107 · expires 09:55"], ["Result", "A demo record “Demo requested” is created. Proof stays “Not issued”."]]} />
          <div className="mt-3"><Check checked={ack} onChange={setAck} label="I understand this is a simulated request — not a real purchase." /></div>
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(1)}>← Back</Btn><Btn variant="primary" disabled={!ack} onClick={() => { setDone(true); toast("Demo request recorded"); }}>Confirm demo request</Btn></div>
        </Card>
      )}
      <Card title="Demo offset records" sub="Demo retirement ≠ certified credit">
        <ul className="flex flex-col divide-y divide-line text-[13px]">{[["offset-0231", "Proof: DEMO-CERT-0231"], ["offset-0198", "Proof: Not issued"]].map(([a, b]) => <li key={a} className="flex flex-wrap justify-between gap-2 py-2"><b>{a}</b><span className="text-muted">{b}</span></li>)}</ul>
      </Card>
      <details className="rounded-2xl border border-line bg-surface p-4 text-[13px]"><summary className="cursor-pointer font-bold">Future carbon market (concept) <span className="text-xs font-normal text-muted">Not available · no prices, balances or trading</span></summary><p className="mt-2 text-xs text-muted">Concept only — nothing here is tradable.</p></details>
    </Page>
  );
}
