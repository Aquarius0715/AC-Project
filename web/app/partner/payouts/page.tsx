"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, Kpi, ListRow, Modal, Page, Select, SummaryList, Textarea, useToast } from "@/components/ui";

const statements = [
  { id: "stmt-2026-09", period: "September 2026", st: "Approved", total: "2,690.00 MYR", pay: "Pay date 10-10" },
  { id: "stmt-2026-08", period: "August 2026", st: "Paid", total: "3,120.00 MYR", pay: "Paid 09-10" },
];
const lines = [
  { job: "job-p03", d: "Repair — base visit + parts", amt: "450.00", note: "Rework 1× (free)" },
  { job: "job-p04", d: "Periodic inspection", amt: "380.00", note: "" },
  { job: "job-p09", d: "Periodic inspection (plan-lobby-b)", amt: "380.00", note: "" },
  { job: "job-p05", d: "Repair — base visit", amt: "450.00", note: "" },
  { job: "job-p07", d: "Rework deduction (2nd return)", amt: "− 120.00", note: "Ask HQ" },
  { job: "job-contractor-a", d: "Repair + refrigerant 0.25 kg", amt: "1,150.00", note: "" },
];

export default function Payouts() {
  const toast = useToast();
  const [sel, setSel] = useState(statements[0]);
  const [ask, setAsk] = useState(false);
  const [msg, setMsg] = useState("");
  const [tried, setTried] = useState(false);
  return (
    <Page>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}><Kpi label="Next payout" value="2,690.00" unit="MYR" sub="stmt-2026-09 · pay date 10-10" /><Kpi label="Jobs in statement" value={6} sub="accepted in September" /><Kpi label="Open questions" value={0} sub="ask HQ about any line" /></div>
      <div className="split-rev">
        <Card title="Statements" className="self-start"><div className="flex flex-col gap-2">{statements.map((s) => <ListRow key={s.id} selected={sel.id === s.id} onClick={() => setSel(s)}><div className="min-w-0 flex-1"><b className="text-[13px]">{s.period}</b><div className="text-[11px] text-muted">{s.id} · {s.pay}</div></div><Badge tone={s.st === "Paid" ? "ok" : "primary"}>{s.st}</Badge></ListRow>)}</div><p className="mt-2 text-[11px] text-muted">You see approved and paid statements only. Drafts stay with HQ.</p></Card>
        <Card title={`${sel.period} · ${sel.id}`} sub={`Rate card rc-contractor-a v3 · ${sel.pay}`} action={<Badge tone={sel.st === "Paid" ? "ok" : "primary"}>{sel.st}</Badge>}>
          <DataTable rowKey={(r) => r.job} rows={lines} cols={[{ key: "j", label: "Job", render: (r) => <b>{r.job}</b> }, { key: "d", label: "Work type", render: (r) => r.d }, { key: "a", label: "MYR", render: (r) => <span className="font-semibold">{r.amt}</span>, className: "text-right" }, { key: "n", label: "", render: (r) => (r.note === "Ask HQ" ? <Btn size="sm" onClick={() => setAsk(true)}>Ask HQ</Btn> : <span className="text-[11px] text-muted">{r.note}</span>) }]} />
          <div className="mt-3"><SummaryList items={[["Total", <b key="t">{sel.total}</b>], ["Priced by", "the rate card effective when each job was accepted"]]} /></div>
        </Card>
      </div>
      <Modal open={ask} onClose={() => setAsk(false)} title="Ask HQ about the job-p07 deduction" footer={<><Btn onClick={() => setAsk(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!msg.trim()) return; setAsk(false); toast("Question sent — HQ answers on the statement"); }}>Send</Btn></>}>
        <Field label="Line"><Select disabled><option>job-p07 · Rework deduction (2nd return) · − 120.00 MYR</option></Select></Field>
        <Field label="Question (required)" error={tried && !msg.trim() ? "Enter your question" : undefined}><Textarea value={msg} onChange={(e) => setMsg(e.target.value)} placeholder="The 2nd return was caused by a missing part from HQ stock." /></Field>
        <Banner>If HQ agrees, an adjustment line is added to the next statement.</Banner>
      </Modal>
    </Page>
  );
}
