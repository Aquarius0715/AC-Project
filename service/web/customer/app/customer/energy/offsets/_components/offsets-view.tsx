"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, Field, Input, Page, Steps, SummaryList } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { amount, klStamp } from "@ac/web/lib/energy";
import { kg3, type ApiOffsetRecord, type ApiQuote } from "@ac/web/lib/offsets";
import { showDate, showTime } from "@ac/web/lib/i18n";
import { useDisplay, useT } from "@ac/web/components/I18n";
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
  const t = useT();
  const display = useDisplay();
  // the quote's period is the energy selection's Kuala Lumpur days; its expiry and the records are instants in the user's zone
  const span = (q: Quote) => `${klStamp(q.period.from)} → ${klStamp(q.period.to)} (Asia/Kuala_Lumpur)`;
  const [pending, run] = useAction();
  const [step, setStep] = useState(0);
  const [amountKg, setAmountKg] = useState("1");
  const [purpose, setPurpose] = useState(() => t("Demo offset")); // a suggested purpose in the user's language; it is kept as typed
  const [quote, setQuote] = useState<Quote | null>(null);
  const [ack, setAck] = useState(false);
  const [done, setDone] = useState<ApiOffsetRecord | null>(null);
  const [explain, setExplain] = useState(false);
  const n = Number(amountKg);
  const amountError = !/^\d+(\.\d{1,3})?$/.test(amountKg.trim()) || !(n > 0 && n <= 100000) ? t("Above 0, at most 100000, up to 3 decimals") : undefined;
  const purposeError = purpose.trim().length < 1 || purpose.trim().length > 1000 ? t("1–1000 characters") : undefined;
  const ask = () => run(() => getQuote({ purpose: purpose.trim(), amountKg: n, period: { from: live.period.from, to: live.period.to }, unitIds: live.unitIds }), t("Demo quote ready — no record was created"), (q) => { setQuote(q); setStep(1); });
  const confirm = () => quote && run(() => requestOffset(quote.id, quote.version), t("Demo request recorded"), (r) => { setDone(r); setStep(3); },
    (f) => { if (f.code === "CONFLICT") { setQuote(null); setStep(0); } }); // an expired or used quote: get a new one
  const restart = () => { setDone(null); setQuote(null); setAck(false); setStep(0); };
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/customer/energy?${live.back}`} className="font-semibold text-primary">{t("← Energy & cost")}</Link> › {t("Carbon offsets (demo)")}</div>
      <Banner tone="warn" action={<DemoBadge />}>{t("Demo only — no real carbon credits are bought or retired. Energy savings do not become a tradable balance. Records are labelled “Demo”.")}</Banner>
      <Steps steps={[t("Amount"), t("Quote"), t("Confirm")]} current={step} />
      <Card title={t("Based on Energy & cost")} sub={`${live.subject} · ${live.period.label}${live.estimate ? ` · ${live.estimate.factor}` : ""}`}>
        {live.estimate ? <div className="flex flex-wrap gap-x-8 gap-y-1 text-[13px]"><span>{t("Estimated emissions")} <b>{live.estimate.emissions}</b></span><span>{t("Estimated savings")} <b className={live.estimate.comparable ? "text-ok" : "text-muted"}>{live.estimate.savedEmissions}</b></span></div>
          : <p className="text-[13px] text-muted">{t("No estimate for this selection — the emission factor or valid readings are missing.")}</p>}
        <button type="button" className="mt-2 text-xs font-semibold text-primary" onClick={() => setExplain((e) => !e)}>{t("ⓘ What is a carbon offset?")} {explain ? "▾" : "▸"}</button>
        {explain && <p className="mt-1 text-xs text-muted">{t("An offset retires a carbon credit to compensate emissions. Here it is simulated — nothing real is purchased, and no market, price or balance exists.")}</p>}
      </Card>
      {done ? (
        <Card title={t("Demo request recorded")} tone="ok">
          <Banner tone="ok">{t("{id} created · status {state} · proof {proof} · no external purchase was made.", { id: done.id.slice(0, 8), state: t(stateLabel[done.state]), proof: done.demoCertificateRef ?? t("Not issued") })}</Banner>
          <div className="mt-3"><Btn onClick={restart}>{t("Start another")}</Btn></div>
        </Card>
      ) : step === 0 ? (
        <Card title={t("How much would you like to offset?")}>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label={t("Amount (kgCO₂e, > 0)")} hint={t("Up to 3 decimals")} error={amountError}><Input inputMode="decimal" value={amountKg} onChange={(e) => setAmountKg(e.target.value)} /></Field>
            <Field label={t("Purpose")} error={purposeError}><Input value={purpose} onChange={(e) => setPurpose(e.target.value)} /></Field>
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><span className="text-xs text-muted">{t("Getting a quote creates no record.")}</span><Btn variant="primary" disabled={pending || !!amountError || !!purposeError} onClick={ask}>{t("Get demo quote")}</Btn></div>
        </Card>
      ) : step === 1 && quote ? (
        <Card title={<span className="flex items-center gap-2">{t("Demo quote")} <Badge tone="primary">{t("◷ Quoted")}</Badge></span>}>
          <SummaryList items={[
            [t("Amount"), `${kg3(quote.amountKg)} kgCO₂e`], [t("Period / units"), `${span(quote)} · ${live.subject}`],
            [t("Quote ID"), t("{id} (demo)", { id: quote.id.slice(0, 8) })], [t("Estimated amount"), quote.estimatedAmountMinor !== null && quote.currency ? t("{amount} (demo)", { amount: amount(quote.estimatedAmountMinor, quote.currency) }) : "—"],
            [t("Expires"), showTime(quote.expiresAt, display)], [t("Records created"), t("None yet")],
          ]} />
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(0)}>{t("← Back")}</Btn><Btn variant="primary" onClick={() => setStep(2)}>{t("Continue")}</Btn></div>
        </Card>
      ) : quote ? (
        <Card title={t("Confirm demo request")}>
          <SummaryList items={[[t("Amount"), `${kg3(quote.amountKg)} kgCO₂e`], [t("Quote"), t("{id} · expires {when}", { id: quote.id.slice(0, 8), when: showTime(quote.expiresAt, display) })], [t("Result"), t("A demo record “Demo requested” is created. Proof stays “Not issued”.")]]} />
          <div className="mt-3"><Check checked={ack} onChange={setAck} label={t("I understand this is a simulated request — not a real purchase.")} /></div>
          <div className="mt-3 flex gap-2"><Btn onClick={() => setStep(1)}>{t("← Back")}</Btn><Btn variant="primary" disabled={!ack || pending} onClick={confirm}>{t("Submit demo request")}</Btn></div>
        </Card>
      ) : null}
      <Card title={t("Demo offset records")} sub={t("Demo retirement ≠ certified credit")}>
        {live.records.length === 0 ? <p className="text-[13px] text-muted">{t("No demo record yet.")}</p> : (
          <ul className="flex flex-col divide-y divide-line text-[13px]">
            {live.records.map((r) => <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 py-2"><span><b>{r.id.slice(0, 8)}</b> <span className="text-xs text-muted">· {kg3(r.amountKg)} kgCO₂e · {showDate(r.createdAt, display)}</span></span><span className="flex items-center gap-2 text-muted">{t("Proof: {proof}", { proof: r.proof ?? t("Not issued") })}<Badge tone={stateTone[r.state]}>{t(stateLabel[r.state])}</Badge></span></li>)}
          </ul>
        )}
      </Card>
      <details className="rounded-2xl border border-line bg-surface p-4 text-[13px]">
        <summary className="cursor-pointer font-bold">{t("Future carbon market (concept)")} <span className="text-xs font-normal text-muted">{t("Not available · no prices, balances or trading")}</span></summary>
        <p className="mt-2 text-xs text-muted">{t("Concept only — provider {provider}, verification {verification}, ledger {ledger}. Integration partners and verification conditions are undecided; nothing here is tradable.", { provider: quote ? quote.marketConcept.providerLabel : t("not selected"), verification: quote ? quote.marketConcept.verificationStatus : t("unverified"), ledger: quote ? quote.marketConcept.ledgerStatus.replace("_", " ") : t("not connected") })}</p>
      </details>
    </Page>
  );
}
