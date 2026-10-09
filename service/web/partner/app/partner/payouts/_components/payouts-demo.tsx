"use client";

// The Phase 1A demo view (fixture rows) of this screen; the page renders it outside DATA_SOURCE=api.

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Kpi, ListRow, Modal, Page, Select, SummaryList, Textarea, useToast } from "@ac/web/components/ui";

const statements = [
  { id: "stmt-2026-09", period: "September 2026", st: "Approved", total: "1,990.00 MYR", pay: "Pays 10-15" },
  { id: "stmt-2026-08", period: "August 2026", st: "Paid", total: "1,760.00 MYR", pay: "Paid 09-15" },
  { id: "stmt-2026-07", period: "July 2026", st: "Paid", total: "2,040.00 MYR", pay: "Paid 08-15" },
];
const lines = [
  { job: "job-contractor-a", d: "Repair · Bedroom AC", amt: "450.00", note: "" },
  { job: "job-p02", d: "Periodic inspection · unit-p-rooftop", amt: "380.00", note: "" },
  { job: "job-p04", d: "Periodic inspection · Meeting room AC", amt: "380.00", note: "" },
  { job: "job-p03", d: "Periodic inspection · Office AC", amt: "380.00", note: "" },
  { job: "job-p07", d: "Repair · Server room AC", amt: "520.00", note: "" },
  { job: "job-p07", d: "Rework deduction (report returned twice)", amt: "− 120.00", note: "Ask HQ" },
];

export function PayoutsDemo() {
  const toast = useToast();
  const [sel, setSel] = useState(statements[0]);
  const [ask, setAsk] = useState(false);
  const [msg, setMsg] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Page>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}><Kpi label="Next payout" value="1,990.00" unit="MYR" sub="stmt-2026-09 · pays 10-15" /><Kpi label="Jobs in statement" value={5} sub="accepted in September" /><Kpi label="Open questions" value={0} sub="ask HQ about any line" /></div>
      <div className="split-rev">
        <Card title="Statements" className="self-start"><div className="flex flex-col gap-2">{statements.map((s) => <ListRow key={s.id} selected={sel.id === s.id} onClick={() => setSel(s)}><div className="min-w-0 flex-1"><b className="text-[13px]">{s.period}</b><div className="text-[11px] text-muted">{s.id} · {s.pay}</div></div><Badge tone={s.st === "Paid" ? "ok" : "primary"}>{s.st}</Badge></ListRow>)}</div><p className="mt-2 text-[11px] text-muted">You see approved and paid statements only. Drafts stay with HQ.</p></Card>
        <Card title={`${sel.period} · ${sel.id}`} sub={`Rate card rc-contractor-a v3 · ${sel.pay}`} action={<Badge tone={sel.st === "Paid" ? "ok" : "primary"}>{sel.st}</Badge>}>
          <DataTable rowKey={(r) => r.job} rows={lines} cols={[{ key: "j", label: "Job", render: (r) => <b>{r.job}</b> }, { key: "d", label: "Work type", render: (r) => r.d }, { key: "a", label: "MYR", render: (r) => <span className="font-semibold">{r.amt}</span>, className: "text-right" }, { key: "n", label: "", render: (r) => (r.note === "Ask HQ" ? <Btn size="sm" onClick={() => setAsk(true)}>Ask HQ</Btn> : <span className="text-[11px] text-muted">{r.note}</span>) }]} />
          <div className="mt-3"><SummaryList items={[["Total", <b key="t">{sel.total}</b>], ["Priced by", "the rate card effective when each job was accepted"]]} /></div>
        </Card>
      </div>
      <Modal open={ask} onClose={() => setAsk(false)} title="Ask HQ about this line" footer={<><Btn onClick={() => setAsk(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!msg.trim()) return; setAsk(false); toast("Question sent — HQ answers on the statement"); }}>Send</Btn></>}>
        <Field label="Line"><Select disabled><option>job-p07 · Rework deduction (2nd return) · − 120.00 MYR</option></Select></Field>
        <Field label="Question (required)" error={tried && !msg.trim() ? "Enter your question" : undefined}><Textarea value={msg} onChange={(e) => setMsg(e.target.value)} placeholder="The 2nd return was caused by a missing part from HQ stock." /></Field>
        <Banner>If HQ agrees, an adjustment line is added to the next statement.</Banner>
      </Modal>
    </Page>
  );
}
