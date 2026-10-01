"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Choice, Field, Input, Modal, Page, PageHead, Select, SummaryList, Tabs, Textarea, ListRow, cx, useToast } from "@/components/ui";

type Job = { id: string; unit: string; loc: string; status: "Requested" | "Scheduled" | "In progress" | "Completed" | "Cancelled"; line: string; who: string; extra?: string };
const seed: Job[] = [
  { id: "job-c04", unit: "Bedroom AC", loc: "Home A › 1F › Bedroom", status: "Scheduled", line: "Confirmed Sep 15 · 10:00–12:00", who: "Technician: tech-internal-a (HQ)" },
  { id: "job-c05", unit: "Kitchen AC", loc: "Home A › 1F › Kitchen", status: "Scheduled", line: "Confirmed Sep 25 · 09:00–11:00", who: "Contractor: CoolFix Sdn Bhd" },
  { id: "job-c03", unit: "Living room AC", loc: "Home A › 1F › Living room", status: "In progress", line: "Confirmed Aug 22 · 14:00–16:00", who: "Report returned by HQ for rework" },
  { id: "job-c02", unit: "Living room AC", loc: "Home A › 1F › Living room", status: "Completed", line: "Completed Sep 8 · report available", who: "Technician: tech-internal-b" },
  { id: "job-c01", unit: "Study AC", loc: "Home A › 2F › Study", status: "Cancelled", line: "Cancelled Sep 2 · reason: resolved itself", who: "Cancelled before assignment" },
];
type Tab = "all" | Job["status"];

export default function Maintenance() {
  const toast = useToast();
  const [jobs, setJobs] = useState(seed);
  const [tab, setTab] = useState<Tab>("all");
  const [sel, setSel] = useState<Job | null>(null);
  const [nw, setNw] = useState(false);
  const [f, setF] = useState({ unit: "Bedroom AC #2", type: "Reactive", sym: "", date: "2026-09-28", win: "14:00–16:00" });
  const [tried, setTried] = useState(false);
  const symErr = tried && (f.sym.trim().length < 10 || f.sym.length > 2000) ? "Symptoms must be 10–2000 characters" : undefined;
  const shown = jobs.filter((j) => tab === "all" || j.status === tab);
  const count = (s: Tab) => (s === "all" ? jobs.length : jobs.filter((j) => j.status === s).length);
  const submit = () => {
    setTried(true);
    if (f.sym.trim().length < 10) return;
    setJobs((j) => [{ id: "job-c06", unit: f.unit, loc: "Home A › 1F › Bedroom", status: "Requested", line: `Requested ${f.date} · ${f.win} (not confirmed)`, who: "Awaiting HQ" }, ...j]);
    toast("Request sent to HQ"); setNw(false); setTried(false); setF({ ...f, sym: "" });
  };
  return (
    <Page>
      <PageHead title="My requests" sub={`${jobs.length} requests · sorted by status (business order)`} action={<Btn variant="primary" onClick={() => setNw(true)}>+ New maintenance request</Btn>} />
      <Tabs value={tab} onChange={setTab} tabs={(["all", "Requested", "Scheduled", "In progress", "Completed", "Cancelled"] as Tab[]).map((t) => ({ id: t, label: t === "all" ? "All" : t, count: count(t) }))} />
      <div className={cx("grid gap-4", sel ? "@container grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" : "grid-cols-1")}>
        <div className="flex min-w-0 flex-col gap-2">
          {shown.map((j) => (
            <ListRow key={j.id} selected={sel?.id === j.id} onClick={() => setSel(j)}>
              <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><b>{j.id}</b><span className="font-semibold">{j.unit}</span><Badge tone={j.status === "Completed" ? "ok" : j.status === "Cancelled" ? "unknown" : j.status === "Requested" ? "primary" : "warn"}>{j.status}</Badge></div><div className="text-xs text-muted">{j.loc}</div><div className="text-xs">◷ {j.line}</div><div className="text-xs text-muted">{j.who}</div></div><span className="text-muted">›</span>
            </ListRow>
          ))}
          {shown.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">No requests in this status.</p>}
          <p className="text-[11px] text-muted">“Scheduled” = assigned with a confirmed slot. Requested slots are not bookings until HQ confirms them.</p>
        </div>
        {sel && (
          <Card title={`${sel.id}`} sub={`${sel.unit} — ${sel.id === "job-c02" ? "periodic service" : "request"}`} action={<Btn size="sm" variant="ghost" onClick={() => setSel(null)}>✕ Close</Btn>} className="self-start">
            <SummaryList items={[["Unit", `${sel.unit} · ${sel.loc}`], ["Type", sel.id === "job-c02" ? "Periodic" : "Reactive"], ["Status", sel.status], ["Schedule", sel.line], ["Handled by", sel.who]]} />
            {sel.status === "Completed" && (
              <>
                <p className="mt-3 text-[13px] font-bold">Completed report <span className="text-xs font-normal text-muted">Accepted by HQ Sep 8 15:00 (visible after review)</span></p>
                <SummaryList items={[["Filter", "Cleaned — normal"], ["Evaporator coil", "Normal"], ["Drain pipe", "Attention — partly blocked, cleared"], ["Parts", "None"], ["Supply air", "12.5 °C"], ["Return air", "25.0 °C"], ["Current draw", "5.2 A"]]} />
                <p className="mt-2 text-xs text-muted">Photos (2) · Attachments: ⎙ service-report.pdf</p>
                <p className="text-xs text-muted">Next action: none</p>
              </>
            )}
            {sel.status === "Requested" && <div className="mt-3"><Btn size="sm" variant="danger" onClick={() => { setJobs((j) => j.map((x) => (x.id === sel.id ? { ...x, status: "Cancelled", line: "Cancelled by you · just now" } : x))); setSel(null); toast("Request cancelled", "warn"); }}>Cancel request</Btn></div>}
          </Card>
        )}
      </div>
      <Modal open={nw} onClose={() => setNw(false)} title="New maintenance request" footer={<><Btn onClick={() => setNw(false)}>Cancel</Btn><Btn variant="primary" onClick={submit}>Submit request</Btn></>}>
        <Field label="Unit"><Select value={f.unit} onChange={(e) => setF({ ...f, unit: e.target.value })}>{["Bedroom AC", "Bedroom AC #2", "Living room AC", "Kitchen AC", "Study AC"].map((u) => <option key={u}>{u}</option>)}</Select></Field>
        <Field label="Type"><Choice value={f.type as "Reactive"} onChange={(v) => setF({ ...f, type: v })} options={[{ id: "Reactive", label: "Reactive" }, { id: "Preventive" as "Reactive", label: "Preventive" }]} /></Field>
        <Field label="Symptoms (10–2000 characters)" error={symErr} hint={`${f.sym.length} / 2000`}><Textarea value={f.sym} onChange={(e) => setF({ ...f, sym: e.target.value })} placeholder="Cold air is weak and there is water dripping from the indoor unit." /></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}><Field label="Requested date"><Input type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} /></Field><Field label="Time window"><Select value={f.win} onChange={(e) => setF({ ...f, win: e.target.value })}><option>09:00–11:00</option><option>11:00–13:00</option><option>14:00–16:00</option></Select></Field></div>
        <Banner>Requested times are not confirmed bookings. HQ confirms the schedule and assigns a technician.</Banner>
      </Modal>
    </Page>
  );
}
