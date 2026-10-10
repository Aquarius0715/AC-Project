"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, EmptyState, Field, Input, ListRow, Modal, Page, Select, Steps, SummaryList, Tabs, Textarea, Timeline } from "@ac/web/components/ui";
import { useI18n } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { showTime } from "@ac/web/lib/i18n";
import { conceptText, kg3, quoteErrors, quoteInput, type ApiQuote, type QuoteDraft, type RecordRow } from "@ac/web/lib/offsets";
import { quoteOffset, requestOffset, retryOffset, simulateOffset } from "../actions";

type Live = {
  tab: "records" | "market"; canWrite: boolean; rows: RecordRow[]; selectedId: string | null; customers: { id: string; name: string }[];
  units: { id: string; label: string; customerId: string }[];
  /** the record filters of the URL (IR324) and whether the URL's recordId is outside them */
  scope: { customerId?: string; status?: RecordRow["state"]; created: "7d" | "30d" | "90d" | "all" }; recordMissing: boolean;
};
const tone = (s: RecordRow["state"]) => (s === "demo_retired" ? "ok" : s === "failed" ? "crit" : s === "demo_purchased" ? "primary" : "warn");
const blank: QuoteDraft = { customerId: "", unitIds: [], from: "", to: "", purpose: "", amountKg: "" };

/** The offset demo registry (FR-A15) in API mode: records come from the server; quotes, requests and the simulated
 * purchase / retirement / failure / retry are Server Actions. Nothing here is a real credit, certificate or market.
 * Texts in the display language; a quote's period is typed in Kuala Lumpur time, its expiry shown in the user's display
 * time zone (IR298). */
export function OffsetsView({ live }: { live: Live }) {
  const i = useI18n(), { t } = i;
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const sel = live.rows.find((r) => r.id === live.selectedId) ?? null;
  const [modal, setModal] = useState(false);
  const [d, setD] = useState<QuoteDraft>(blank);
  const [tried, setTried] = useState(false);
  const [quote, setQuote] = useState<ApiQuote | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [ack, setAck] = useState(false);
  const errors = quoteErrors(d, t);
  const set = (patch: Partial<QuoteDraft>) => { setQuote(null); setConfirmed(false); setD((x) => ({ ...x, ...patch })); };
  const units = live.units.filter((u) => u.customerId === d.customerId);
  const close = () => { setModal(false); setTried(false); setQuote(null); setConfirmed(false); };
  const getQuote = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => quoteOffset(quoteInput(d)), t("Demo quote ready — valid for 15 minutes"), setQuote);
  };
  const request = () => quote && confirmed && run(() => requestOffset(quote.id, quote.version), t("Demo request recorded"), (id) => { close(); setD(blank); nav({ recordId: id }); });
  const done = { purchase_confirm: t("Demo purchase confirmed"), retire: t("Demo retired"), fail: t("Failure recorded — the previous stage is kept") };
  const step = (event: "purchase_confirm" | "retire" | "fail") => sel?.current && run(() => simulateOffset(sel.id, sel.version, sel.current!.id, event), done[event], () => setAck(false));
  const retry = () => sel?.current && run(() => retryOffset(sel.id, sel.version, sel.current!.id), t("Retry started — a new attempt"), () => setAck(false));
  return (
    <Page>
      <Banner tone="warn" action={<DemoBadge />}>{t("Demo only — quotes, purchases and retirements are simulated. No real credit, certificate, balance or market is involved.")}</Banner>
      <Tabs value={live.tab} onChange={(id) => nav({ tab: id === "records" ? null : id })} tabs={[{ id: "records", label: t("Demo records"), count: live.rows.length }, { id: "market", label: t("Market concept") }]} />
      {live.tab === "market" ? (
        <Card title={t("Carbon market — future concept")} sub={t("Read only · no orders, prices, balances or trading")}>
          <SummaryList items={[[t("Stage"), t("future concept")], [t("Provider"), t("Not selected")], [t("Verification"), t("unverified")], [t("Ledger"), t("not connected")]]} />
          <p className="mt-3 text-[13px]">{t("Future partners and verification conditions are undecided. Nothing here is tradable, priced or balance-carrying, and it is a different state from a simulated retirement.")}</p>
        </Card>
      ) : (
        <>
        <div className="flex flex-wrap items-end gap-3">
          <span className="pb-2 text-xs font-semibold text-muted">{t("Filter")}</span>
          <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, recordId: null })}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label={t("Status")}><Select value={live.scope.status ?? ""} onChange={(e) => nav({ status: e.target.value || null, recordId: null })}><option value="">{t("All statuses")}</option>{(["demo_requested", "demo_purchased", "demo_retired", "failed"] as const).map((x) => <option key={x} value={x}>{t(x === "demo_requested" ? "Demo requested" : x === "demo_purchased" ? "Demo purchased" : x === "demo_retired" ? "Demo retired" : "Failed")}</option>)}</Select></Field>
          <Field label={t("Created")}><Select value={live.scope.created} onChange={(e) => nav({ created: e.target.value === "30d" ? null : e.target.value, recordId: null })}><option value="7d">{t("Last 7 days")}</option><option value="30d">{t("Last 30 days")}</option><option value="90d">{t("Last 90 days")}</option><option value="all">{t("All time")}</option></Select></Field>
        </div>
        <div className="split-rev">
          <Card title={t("Demo offset records")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => { setD(blank); setModal(true); }}>{t("+ New demo quote")}</Btn>} className="self-start">
            {live.rows.length === 0 ? <EmptyState title={t("No records")}>{t(live.scope.customerId || live.scope.status || live.scope.created !== "all" ? "No record matches these filters — widen them, or get a demo quote and request it to create a record." : "Get a demo quote and request it to create a record.")}</EmptyState> : <div className="flex flex-col gap-2">{live.rows.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setAck(false); nav({ recordId: x.id }); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)}</b><Badge tone={tone(x.state)}>{x.stateText}</Badge></div><div className="text-[11px] text-muted">{x.customer} · {x.amount}</div><div className="text-[11px] text-muted">{x.next}</div></div></ListRow>)}</div>}
            <p className="mt-3 text-[11px] text-muted">{t("Energy savings and MRV estimates are shown separately and never converted into credits.")} <Link className="text-primary" href="/admin/energy">{t("Energy analysis")}</Link> · <Link className="text-primary" href="/admin/mrv">MRV</Link></p>
          </Card>
          {!sel ? <Card title={t("Record")}>{live.recordMissing ? <EmptyState title={t("That record is not in this list")}>{t("It does not exist, or the customer, status or created filter hides it.")}</EmptyState> : <EmptyState title={t("Nothing selected")}>{t("Choose a record.")}</EmptyState>}</Card> : (
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={t("Record {id}", { id: sel.id.slice(0, 8) })} sub={t("{customer} · quote {quote} · version {v}", { customer: sel.customer, quote: sel.r.quoteId.slice(0, 8), v: sel.version })}>
                {sel.stage >= 0 ? <Steps steps={[t("Quoted"), t("Demo requested"), t("Demo purchased"), t("Demo retired")]} current={sel.stage} /> : (
                  <Banner tone="crit" action={live.canWrite && sel.current && <Btn size="sm" disabled={pending} onClick={retry}>{t("Retry")}</Btn>}>{t("{next}. A retry starts a new attempt of the failed stage.", { next: sel.next })}</Banner>
                )}
                <div className="mt-4"><SummaryList items={[
                  [t("Simulated purchase"), t("{amount} — retires as a whole in 1A", { amount: sel.amount })], [t("Provider / scheme"), t("Not selected · demo")],
                  [t("Current attempt"), sel.currentText ?? t("none")],
                  [t("Purchase reference"), sel.r.purchaseRef ?? "—"],
                  [t("Demo certificate"), sel.r.demoCertificateRef ? t("{ref} (demo — not a real certificate)", { ref: sel.r.demoCertificateRef }) : t("Shown only after the demo retirement (DEMO-…)")],
                ]} /></div>
              </Card>
              {live.canWrite && sel.state === "demo_requested" && sel.current && (
                <Card title={t("Next step · demo purchase")}><div className="flex flex-wrap gap-2"><Btn variant="primary" disabled={pending} onClick={() => step("purchase_confirm")}>{t("Confirm simulated purchase")}</Btn><Btn disabled={pending} onClick={() => step("fail")}>{t("Simulate a failure")}</Btn></div></Card>
              )}
              {live.canWrite && sel.state === "demo_purchased" && sel.current && (
                <Card title={t("Next step · demo retirement")}>
                  <Check label={t("I understand this is a demo retirement. It does not retire a real credit.")} checked={ack} onChange={setAck} />
                  <p className="my-1 text-[11px] text-muted">{t("Disabled until confirmed. Retiring before the purchase is not possible.")}</p>
                  <div className="flex flex-wrap gap-2"><Btn variant="primary" disabled={pending || !ack} onClick={() => step("retire")}>{t("Retire whole amount (demo)")}</Btn><Btn disabled={pending} onClick={() => step("fail")}>{t("Simulate a failure")}</Btn></div>
                </Card>
              )}
              <Card title={t("Event history")} action={<Link className="text-xs font-semibold text-primary" href="/admin/audit">{t("Open in audit →")}</Link>}>
                {sel.events.length === 0 ? <p className="text-xs text-muted">{t("No events yet.")}</p> : <Timeline items={sel.events} />}
              </Card>
            </div>
          )}
        </div>
        </>
      )}
      <Modal open={modal} onClose={close} title={t("New demo quote")} wide footer={<><Btn onClick={close}>{t("Cancel")}</Btn>{!quote ? <Btn variant="primary" disabled={pending} onClick={getQuote}>{t("Get demo quote")}</Btn> : <Btn variant="primary" disabled={pending || !confirmed} onClick={request}>{t("Request (demo)")}</Btn>}</>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Field label={t("Customer")} error={tried ? errors.customerId : undefined}><Select value={d.customerId} onChange={(e) => set({ customerId: e.target.value, unitIds: [] })}><option value="">{t("Select…")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label={t("Amount (kgCO₂e)")} error={tried ? errors.amountKg : undefined}><Input type="number" min="0" step="0.001" value={d.amountKg} onChange={(e) => set({ amountKg: e.target.value })} /></Field>
          <Field label={t("Period start (Kuala Lumpur)")} error={tried ? errors.period : undefined}><Input type="datetime-local" value={d.from} onChange={(e) => set({ from: e.target.value })} /></Field>
          <Field label={t("Period end")}><Input type="datetime-local" value={d.to} onChange={(e) => set({ to: e.target.value })} /></Field>
        </div>
        <Field label={t("Purpose")} error={tried ? errors.purpose : undefined}><Textarea value={d.purpose} maxLength={1000} onChange={(e) => set({ purpose: e.target.value })} placeholder={t("Offset the demo office's Scope 2 for August (simulation)")} /></Field>
        <div><p className="mb-1 text-[13px] font-semibold">{t("Units")}</p>{!d.customerId ? <p className="text-xs text-muted">{t("Choose the customer first.")}</p> : <div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{units.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} />)}</div>}{tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}</div>
        {quote && (
          <div className="flex flex-col gap-2 rounded-xl border border-line p-3">
            <div className="flex items-center gap-2"><b className="text-[13px]">{t("Demo quote {id} · v{version}", { id: quote.id.slice(0, 8), version: quote.version })}</b><DemoBadge /></div>
            <SummaryList items={[[t("Amount"), `${kg3(quote.amountKg)} kgCO₂e`], [t("Expires"), showTime(quote.expiresAt, i.display)], [t("Provider / scheme"), t("Not selected · {scheme}", { scheme: quote.scheme })], [t("Price"), t("none — no market price")], [t("Market concept"), conceptText(quote.marketConcept, t)]]} />
            <Check label={t("I confirm no real transaction takes place (demoConfirmed).")} checked={confirmed} onChange={setConfirmed} />
          </div>
        )}
      </Modal>
    </Page>
  );
}
