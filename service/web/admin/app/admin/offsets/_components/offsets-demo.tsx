"use client";

import { useState } from "react";
import { Banner, Btn, Card, Check, DemoBadge, Field, Input, ListRow, Modal, Page, Steps, SummaryList, Tabs, Timeline, useToast } from "@ac/web/components/ui";
import { useUrlTab } from "@ac/web/lib/useUrlTab";

type O = { id: string; cust: string; kind: string; amt: string; st: string; stage: number; tone: "primary" | "ok" | "warn" | "crit" };
const seed: O[] = [
  { id: "offset-0231", cust: "customer-a", kind: "Demo offset", amt: "1.000", st: "DEMO-CERT-0231 · 09-12 14:20", stage: 4, tone: "ok" },
  { id: "offset-0198", cust: "customer-a", kind: "Demo offset", amt: "2.500", st: "Next: demo retirement · 09-14 10:05", stage: 3, tone: "primary" },
  { id: "offset-0212", cust: "customer-b", kind: "Pilot offset", amt: "0.750", st: "Awaiting demo purchase confirmation", stage: 2, tone: "warn" },
  { id: "offset-0175", cust: "customer-a", kind: "Demo offset", amt: "0.500", st: "Purchase failed · previous: Demo requested", stage: -1, tone: "crit" },
];
const stages = ["Quoted", "Demo requested", "Demo purchased", "Demo retired"];

/** The Phase 1A demo of the offset registry (fixture records). */
export function OffsetsDemo() {
  const toast = useToast();
  const [tab, setTab] = useUrlTab<"records" | "market">({ records: "records", market: "market" }, "records");
  const [list, setList] = useState(seed);
  const [sel, setSel] = useState(seed[1]);
  const [ack, setAck] = useState(false);
  const [quote, setQuote] = useState(false);
  const [q, setQ] = useState({ cust: "customer-a", amt: "1.0" });
  const upd = (patch: Partial<O>) => { setList((l) => l.map((x) => (x.id === sel.id ? { ...x, ...patch } : x))); setSel({ ...sel, ...patch }); };
  return (
    <Page>
      <Banner tone="warn" action={<DemoBadge />}>Demo only — quotes, purchases, and retirements are simulated. No real credit, certificate, balance, or market is involved.</Banner>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "records", label: "Demo records", count: list.length }, { id: "market", label: "Market concept" }]} />
      {tab === "records" ? (
        <div className="split-rev">
          <Card title="Demo offset records" action={<Btn size="sm" variant="primary" onClick={() => setQuote(true)}>+ New demo quote</Btn>} className="self-start"><div className="flex flex-col gap-2">{list.map((x) => <ListRow key={x.id} selected={sel.id === x.id} onClick={() => { setSel(x); setAck(false); }}><div className="min-w-0"><b className="text-[13px]">{x.id}</b><div className="text-[11px] text-muted">{x.cust} · {x.kind} · {x.amt} kgCO₂e</div><div className="text-[11px] text-muted">{x.st}</div></div></ListRow>)}</div><p className="mt-3 text-[11px] text-muted">Energy savings and MRV estimates are never converted into credits.</p></Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={sel.id} sub={`${sel.cust} · “${sel.kind}” · quote-${sel.id.slice(-4)}`}>
              {sel.stage >= 0 ? <Steps steps={stages} current={sel.stage} /> : <Banner tone="crit" action={<Btn size="sm" onClick={() => { upd({ stage: 2, st: "Retry sent", tone: "primary" }); toast("Retry started (attempt-02)"); }}>Retry purchase</Btn>}>Purchase failed · previous: Demo request. Retry creates a new attempt.</Banner>}
              <div className="mt-4 grid-fluid" style={{ ["--min"as string]: "190px" }}><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Energy saved (reference)</div><b>20.0 kWh</b><div className="text-[10px] text-muted">From energy analysis — not a credit</div></div><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Est. emission reduction (reference)</div><b>10.0 kgCO₂e</b><div className="text-[10px] text-muted">MRV estimate — never added to a balance</div></div><div className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">Simulated purchase (this record)</div><b>{sel.amt} kgCO₂e</b><div className="text-[10px] text-muted">Retires as a whole in 1A</div></div></div>
            </Card>
            <Card title="Record details"><SummaryList items={[["Quote", `quote-${sel.id.slice(-4)} · version 1 · expires 09-14 10:15`], ["Amount", `${sel.amt} kgCO₂e`], ["Provider / scheme", "Not selected · demo"], ["Current attempt", "attempt-01"], ["Demo certificate", sel.stage === 4 ? "DEMO-CERT-0231 (demo — not a real certificate)" : "Shown only after demo retirement (DEMO-…)"]]} /></Card>
            {sel.stage === 3 && <Card title="Next step · Demo retirement"><Check label="I understand this is a demo retirement. It does not retire a real credit." checked={ack} onChange={setAck} /><p className="my-1 text-[11px] text-muted">Disabled until confirmed. Retiring before purchase is not possible.</p><Btn variant="primary" disabled={!ack} onClick={() => { upd({ stage: 4, st: "DEMO-CERT-" + sel.id.slice(-4) + " · just now", tone: "ok" }); toast("Demo retired"); }}>Retire (demo)</Btn></Card>}
            {sel.stage === 2 && <Card title="Next step · Demo purchase"><Btn variant="primary" onClick={() => { upd({ stage: 3, st: "Next: demo retirement" }); toast("Demo purchase confirmed"); }}>Confirm simulated purchase</Btn></Card>}
            <Card title="Event history" action={<a className="text-xs font-semibold text-primary" href="/admin/audit">Open in audit →</a>}><Timeline items={[{ time: "09-14 10:05", title: "purchase_confirm · evt-0198-03 · attempt-01", detail: "hq-operator · demoConfirmed" }, { time: "09-14 10:02", title: "request · quote-0198 v1", detail: "hq-operator · demoConfirmed" }, { time: "09-14 10:00", title: "offsets.preview · 2.500 kgCO₂e", detail: "hq-operator · demoConfirmed" }]} /></Card>
          </div>
        </div>
      ) : (
        <Card title="Market concept" sub="Not available · no prices, balances or trading"><p className="text-[13px]">A future carbon market is a concept only. Nothing here is tradable, priced or balance-carrying.</p></Card>
      )}
      <Modal open={quote} onClose={() => setQuote(false)} title="New demo quote" footer={<><Btn onClick={() => setQuote(false)}>Cancel</Btn><Btn variant="primary" disabled={!(+q.amt > 0)} onClick={() => { setList((l) => [{ id: "offset-0240", cust: q.cust, kind: "Demo offset", amt: (+q.amt).toFixed(3), st: "Quoted", stage: 0, tone: "primary" }, ...l]); toast("Demo quote created"); setQuote(false); }}>Create quote</Btn></>}><Field label="Customer"><Input value={q.cust} onChange={(e) => setQ({ ...q, cust: e.target.value })} /></Field><Field label="Amount (kgCO₂e, > 0)" error={+q.amt > 0 ? undefined : "Positive numbers only"}><Input type="number" value={q.amt} onChange={(e) => setQ({ ...q, amt: e.target.value })} /></Field></Modal>
    </Page>
  );
}
