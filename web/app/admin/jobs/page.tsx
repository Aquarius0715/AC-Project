"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, Field, Input, ListRow, Modal, Page, Select, Steps, SummaryList, Tabs, Textarea, useToast, cx } from "@/components/ui";

type J = { id: string; unit: string; cust: string; kind: string; status: string; line: string; tone: "primary" | "warn" | "ok" | "crit" | "unknown" };
const seed: J[] = [
  { id: "job-internal-a", unit: "Bedroom AC", cust: "customer-a", kind: "Reactive", status: "requested", line: "Unassigned · due 09-15 12:00", tone: "primary" },
  { id: "job-a11", unit: "Rooftop unit", cust: "customer-b", kind: "Reactive", status: "offered", line: "Offer to contractor-a · expires 09-24 09:00", tone: "primary" },
  { id: "job-contractor-a", unit: "Bedroom AC", cust: "customer-a", kind: "Preventive", status: "in_progress", line: "contractor-a · tech-external-a · window ended 09-20", tone: "crit" },
  { id: "job-c04", unit: "Bedroom AC", cust: "customer-a", kind: "Reactive", status: "assigned", line: "Internal · tech-internal-a", tone: "warn" },
  { id: "job-p09", unit: "Lobby AC", cust: "customer-b", kind: "Periodic", status: "submitted", line: "contractor-a · awaiting contractor review", tone: "warn" },
  { id: "job-a06", unit: "Living room AC", cust: "customer-a", kind: "Periodic", status: "submitted", line: "Internal · tech-internal-a · report v1", tone: "warn" },
  { id: "job-c02", unit: "Living room AC", cust: "customer-a", kind: "Periodic", status: "completed", line: "Internal · completed 09-08", tone: "ok" },
];
const plans = [{ id: "plan-living-a", name: "Living room AC — periodic filter cleaning", rule: "Every month · customer-a · next 2026-10-15" }, { id: "plan-lobby-b", name: "Lobby AC — quarterly inspection", rule: "Every 3 months · customer-b · next 2026-12-01" }];

export default function AdminJobs() {
  const toast = useToast();
  const [tab, setTab] = useState<"jobs" | "plans">("jobs");
  const [list, setList] = useState(seed);
  const [sel, setSel] = useState(seed[5]);
  const [plan, setPlan] = useState(plans[0]);
  const [overdue, setOverdue] = useState(false);
  const [modal, setModal] = useState<null | "new" | "rework" | "hold">(null);
  const [step, setStep] = useState(0);
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [f, setF] = useState({ unit: "Bedroom AC", kind: "Reactive", delivery: "internal", sym: "" });
  const counts = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "rework", "completed", "on_hold"];
  const shown = list.filter((j) => !overdue || j.tone === "crit");
  const setS = (status: string) => { setList((l) => l.map((x) => (x.id === sel.id ? { ...x, status } : x))); setSel({ ...sel, status }); };
  const close = () => { setModal(null); setStep(0); setTried(false); setReason(""); };
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "jobs", label: "Jobs", count: list.length }, { id: "plans", label: "Plans", count: 2 }]} />
      {tab === "jobs" ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap gap-2">{counts.map((c) => <span key={c} className="rounded-full bg-surface2 px-2.5 py-1 text-xs"><span className="text-muted">{c}</span> <b>{list.filter((j) => j.status === c).length}</b></span>)}</div><div className="flex items-center gap-3"><Check label="Overdue only" checked={overdue} onChange={setOverdue} /><Btn size="sm" variant="primary" onClick={() => setModal("new")}>+ New job</Btn></div></div>
          <div className="split-rev">
            <Card title="Jobs · all customers" sub="status ↑" className="self-start"><div className="flex flex-col gap-2">{shown.map((j) => <ListRow key={j.id} selected={sel.id === j.id} onClick={() => setSel(j)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{j.id} · {j.unit}</b><Badge tone={j.tone}>{j.status}</Badge></div><div className="text-[11px] text-muted">{j.cust} · {j.kind}</div><div className="text-[11px] text-muted">{j.line}</div></div></ListRow>)}{shown.length === 0 && <p className="text-xs text-muted">No jobs.</p>}</div></Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={`${sel.id} · ${sel.unit}`} sub={`${sel.kind} · ${sel.cust} · Home A › 1F · ${sel.status === "offered" ? "plan-living-a" : "plan-living-a (Sep occurrence)"}`} action={<Badge tone={sel.tone}>{sel.status}</Badge>}>
                <p className="mb-2 text-xs text-muted">Internal delivery skips Offered / Accepted. Submitted jobs cannot be cancelled — put on hold first (IR56).</p>
                <SummaryList cols={2} items={[["Requested window", "09-15 10:00–12:00 · Asia/Kuala_Lumpur"], ["Due", "09-15 12:00 = requested end"], ["Scheduled", "09-15 10:00–12:00 · tech-internal-a"], ["Symptom", "Periodic filter cleaning · contact 9:00–18:00"]]} />
                {sel.status === "offered" ? <div className="mt-3"><Banner>Contractor offer · contractor-a · expires 09-24 09:00. Re-offer after decline is possible.</Banner></div> : <div className="mt-3 rounded-xl bg-surface2 p-3 text-xs"><b className="text-[11px] tracking-wide">DELIVERY · INTERNAL</b><br />tech-internal-a · assignment valid 09-15 10:00–12:00<br />Qualifications demo_indoor ✓ · no overlapping confirmed schedule</div>}
              </Card>
              {sel.status === "submitted" && <Card title="Work report · version 1" sub="Submitted 09-15 11:48 by tech-internal-a · 6 checks · 3 photos">
                <SummaryList items={[["Filter cleaned", "Normal"], ["Drain line", "Normal"], ["Refrigerant pressure", "Normal · 412 kPa"], ["Airflow", "Needs attention · reduced on Low"]]} />
                <p className="mt-2 text-xs text-ok">You did not contribute to this report, so you can review it (self-approval is rejected).</p>
                <div className="mt-3 flex flex-wrap gap-2"><Btn variant="primary" onClick={() => { setS("completed"); toast("Report accepted — job completed"); }}>Accept report</Btn><Btn onClick={() => setModal("rework")}>Return for rework…</Btn><Btn variant="danger" onClick={() => setModal("hold")}>Put on hold…</Btn></div>
              </Card>}
              <Card title="Costs" action={<Btn size="sm" onClick={() => toast("Cost line added")}>+ Add cost line</Btn>}>
                <div className="scroll-x"><table className="w-full min-w-[480px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Kind</th><th>Description</th><th>Visibility</th><th className="text-right">Amount</th></tr></thead><tbody>{[["Estimate", "Filter cleaning labour", "customer", "80.00 MYR"], ["Estimate", "Replacement filter", "customer", "25.00 MYR"], ["Actual", "Filter cleaning labour", "customer", "80.00 MYR"], ["Actual", "Filter (imported)", "internal", "6.20 USD"]].map((r, i) => <tr key={i} className="border-t border-line">{r.map((c, j) => <td key={j} className={cx("py-1.5", j === 3 && "text-right font-semibold")}>{c}</td>)}</tr>)}</tbody></table></div>
                <p className="mt-2 text-xs">Estimate total <b>105.00 MYR</b> · Actual total <b>80.00 MYR + 6.20 USD</b></p><p className="text-[11px] text-muted">Totals are shown per currency — never converted.</p>
              </Card>
            </div>
          </div>
        </>
      ) : (
        <div className="split-rev">
          <Card title="Maintenance plans" className="self-start"><div className="flex flex-col gap-2">{plans.map((p) => <ListRow key={p.id} selected={plan.id === p.id} onClick={() => setPlan(p)}><div><b className="text-[13px]">{p.id}</b><div className="text-[11px] text-muted">{p.name}</div></div></ListRow>)}</div></Card>
          <Card title={plan.name} sub={plan.rule} action={<Btn size="sm" variant="primary" onClick={() => toast("CONFLICT — an occurrence already exists for this period", "warn")}>Generate jobs</Btn>}>
            <SummaryList items={[["Plan", plan.id], ["Rule", plan.rule], ["Lead time", "7 days"], ["Delivery", "Internal"]]} /><p className="mt-2 text-xs text-muted">Generating twice for the same period returns a CONFLICT instead of duplicating jobs.</p>
          </Card>
        </div>
      )}
      <Modal open={modal === "new"} onClose={close} title={`New job — step ${step + 1} of 2`} footer={step === 0 ? <><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (f.sym.trim().length < 10) return; setTried(false); setStep(1); }}>Next</Btn></> : <><Btn onClick={() => setStep(0)}>← Back</Btn><Btn variant="primary" onClick={() => { setList((l) => [{ id: "job-a12", unit: f.unit, cust: "customer-a", kind: f.kind, status: f.delivery === "internal" ? "assigned" : "offered", line: f.delivery === "internal" ? "Internal · tech-internal-a" : "Offer to contractor-a", tone: "primary" }, ...l]); toast("Job created"); close(); }}>Create job</Btn></>}>
        <Steps steps={["Details", "Delivery"]} current={step} />
        {step === 0 ? <><Field label="Unit"><Select value={f.unit} onChange={(e) => setF({ ...f, unit: e.target.value })}><option>Bedroom AC</option><option>Living room AC</option><option>Rooftop unit</option></Select></Field><Field label="Type"><Choice value={f.kind as "Reactive"} onChange={(v) => setF({ ...f, kind: v })} options={[{ id: "Reactive", label: "Reactive" }, { id: "Preventive" as "Reactive", label: "Preventive" }, { id: "Periodic" as "Reactive", label: "Periodic" }]} /></Field><Field label="Symptom / scope (10+ chars)" error={tried && f.sym.trim().length < 10 ? "Enter at least 10 characters" : undefined}><Textarea value={f.sym} onChange={(e) => setF({ ...f, sym: e.target.value })} /></Field></> : <><Field label="Delivery"><Choice value={f.delivery as "internal"} onChange={(v) => setF({ ...f, delivery: v })} options={[{ id: "internal", label: "Internal technician" }, { id: "contractor" as "internal", label: "Offer to contractor" }]} /></Field><Banner>{f.delivery === "internal" ? "Internal delivery skips Offered / Accepted." : "Offer to contractor-a — they answer within the offer window."}</Banner></>}
      </Modal>
      <Modal open={modal === "rework" || modal === "hold"} onClose={close} title={modal === "rework" ? "Return for rework" : "Put on hold (IR56)"} footer={<><Btn onClick={close}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!reason.trim()) return; setS(modal === "rework" ? "rework" : "on_hold"); toast(modal === "rework" ? "Returned for rework" : "Job put on hold", "warn"); close(); }}>Confirm</Btn></>}><Field label="Reason (required)" error={tried && !reason.trim() ? "A reason is required" : undefined}><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></Modal>
    </Page>
  );
}
