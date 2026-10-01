"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, Field, Input, Modal, Page, Select, SummaryList, Tabs, Textarea, UtilBar, cx, useToast } from "@/components/ui";

type R = "normal" | "attention" | "not_inspected" | "not_applicable" | null;
const groups: Record<string, string[]> = {
  indoor: ["Filter", "Evaporator coil", "Blower motor", "Blower fan", "Drain pipe", "Drain pan", "Outlet", "Louver"],
  outdoor: ["Compressor", "Condenser coil", "Fan motor", "Refrigerant line", "Service valve"],
  electrical: ["Wiring insulation", "Capacitor", "Breaker", "Terminals", "Grounding"],
};
const labels = { indoor: "Indoor (8)", outdoor: "Outdoor (5)", electrical: "Electrical (5)" };

export default function Workspace({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const toast = useToast();
  const [tab, setTab] = useState<"indoor" | "outdoor" | "electrical">("indoor");
  const [res, setRes] = useState<Record<string, R>>({ Filter: "attention", "Evaporator coil": "normal", "Blower motor": "normal", "Blower fan": "normal", "Drain pipe": "normal", "Drain pan": "normal", Outlet: "not_applicable" });
  const [why, setWhy] = useState<Record<string, string>>({ Filter: "heavy dust buildup, recommend replacement", Outlet: "no accessible outlet in this installation" });
  const [readings, setReadings] = useState([["Supply air temperature", "14.2", "°C", "Evaporator coil"], ["Return air temperature", "25.8", "°C", "Evaporator coil"], ["Filter pressure drop", "118", "Pa", "Filter"], ["Blower motor current", "1.9", "A", "Blower motor"]]);
  const [work, setWork] = useState("Cleaned filter, inspected coil and blower — all normal except filter (dust buildup, recommend replacement). No parts used yet.");
  const [status, setStatus] = useState(id === "job-t07" ? "Assigned" : id === "job-p09" ? "Submitted" : "In progress");
  const [errs, setErrs] = useState<string[]>([]);
  const [confirm, setConfirm] = useState(false);
  if (id === "job-internal-a") return <Page className="max-w-xl"><Card title="This page isn’t available" sub="Not assigned to you (NOT_FOUND)." /></Page>;
  const readOnly = status === "Submitted" || status === "Assigned";
  const all = Object.values(groups).flat();
  const done = all.filter((k) => res[k]).length;
  const left = (g: string) => groups[g].filter((k) => !res[k]).length;
  const submit = () => {
    const e: string[] = [];
    const nul = all.length - done;
    if (nul) e.push(`${nul} items still null — each needs a result, or not_inspected / not_applicable with a reason.`);
    if (work.trim().length < 10) e.push("Work performed must be 10–4000 characters.");
    Object.entries(res).forEach(([k, v]) => { if ((v === "not_inspected" || v === "not_applicable") && !why[k]?.trim()) e.push(`${k}: reason required.`); });
    setErrs(e);
    if (!e.length) setConfirm(true);
  };
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/technician" className="font-semibold text-primary">← Overview</Link><b className="text-ink">{id} · Bedroom AC</b><Badge tone={status === "Submitted" ? "ok" : status === "Assigned" ? "primary" : "warn"}>{status}</Badge></div>
      {status === "Assigned" && <Banner>Before the work window (starts 14:00): read-only, Start is disabled (IR76). <Btn size="sm" disabled>Start job</Btn></Banner>}
      {status === "Submitted" && <Banner tone="ok">Submitted v1 — read-only, awaiting quality review.</Banner>}
      {status === "In progress" && <Banner tone="warn">Work window ends in 15 min — unsaved input will be discarded at 12:00 (IR89).</Banner>}
      {errs.length > 0 && <Banner tone="crit"><b>Can’t submit:</b><ul className="list-disc pl-5 text-xs">{errs.slice(0, 5).map((e) => <li key={e}>{e}</li>)}</ul></Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Inspection checklist" action={<Tabs value={tab} onChange={setTab} tabs={(Object.keys(groups) as (keyof typeof groups)[]).map((g) => ({ id: g as "indoor", label: labels[g as "indoor"] }))} />}>
            <ul className="divide-y divide-line">
              {groups[tab].map((k) => (
                <li key={k} className="py-2.5">
                  <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-[13px] font-semibold">{k}</span>
                    <div className="flex flex-wrap gap-1">{([["normal", "Normal"], ["attention", "Attention"], ["not_inspected", "Not inspected"], ["not_applicable", "N/A"]] as const).map(([v, l]) => <button key={v} disabled={readOnly} onClick={() => setRes((r) => ({ ...r, [k]: v }))} aria-pressed={res[k] === v} className={cx("rounded-lg border px-2.5 py-1 text-xs font-semibold disabled:opacity-60", res[k] === v ? (v === "attention" ? "border-warn bg-warn-soft text-warn" : v === "normal" ? "border-ok bg-ok-soft text-ok" : "border-primary bg-primary-soft text-primary") : "border-line hover:bg-surface2")}>{l}</button>)}</div></div>
                  {(res[k] === "attention" || res[k] === "not_inspected" || res[k] === "not_applicable") && <div className="mt-1.5"><Input disabled={readOnly} placeholder="Reason…" value={why[k] ?? ""} onChange={(e) => setWhy((w) => ({ ...w, [k]: e.target.value }))} /></div>}
                </li>
              ))}
            </ul>
            <p className="mt-2 text-[11px] text-muted">Initial result is always null — normal is never preselected. Not inspected/not applicable require a reason.</p>
          </Card>
          <Card title="Readings recorded" action={<Btn size="sm" disabled={readOnly} onClick={() => setReadings((r) => [...r, ["New reading", "", "", ""]])}>+ Add reading</Btn>}>
            <div className="scroll-x"><table className="w-full min-w-[460px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Metric</th><th>Value</th><th>Unit</th><th>Linked item</th></tr></thead><tbody>{readings.map((r, i) => <tr key={i} className="border-t border-line">{r.map((c, j) => <td key={j} className="py-1.5 pr-2"><Input disabled={readOnly} value={c} onChange={(e) => setReadings((rs) => rs.map((x, a) => (a === i ? x.map((y, b) => (b === j ? e.target.value : y)) : x)))} /></td>)}</tr>)}</tbody></table></div>
          </Card>
          <Card title="Work report — draft v2">
            <Field label="Work performed & next action (10–4000 characters)"><Textarea disabled={readOnly} value={work} onChange={(e) => setWork(e.target.value)} className="min-h-[110px]" /></Field>
            <div className="grid-fluid mt-3" style={{ ["--min"as string]: "150px" }}><Field label="Part name"><Input disabled={readOnly} defaultValue="Air filter (standard)" /></Field><Field label="Qty"><Input disabled={readOnly} type="number" defaultValue={1} /></Field><Field label="Next action"><Select disabled={readOnly}><option>None</option><option>Re-inspect</option><option>Replace part</option></Select></Field></div>
            <div className="mt-3 rounded-xl border border-dashed border-line p-4 text-center text-xs text-muted">Photos (JPEG/PNG, ≤5 MiB, ≤10) · 1 attached · <button disabled={readOnly} className="font-semibold text-primary disabled:opacity-50" onClick={() => toast("Photo added (demo)")}>+ Add photo</button></div>
            <p className="mt-2 text-[11px] text-muted">Submitted versions become read-only. Rework resumes from rework_requested as a new draft version.</p>
            {!readOnly && <div className="mt-3 flex flex-wrap justify-end gap-2"><Btn onClick={() => toast("Draft autosaved 11:44")}>Save draft</Btn><Btn variant="primary" onClick={submit}>Submit report</Btn></div>}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Checklist progress"><div className="mb-2 flex justify-between text-xs"><b>{done} / {all.length}</b><span className="text-muted">{all.length - done} items still null</span></div><UtilBar pct={(done / all.length) * 100} />{Object.keys(groups).map((g) => <div key={g} className="mt-2 flex justify-between text-xs"><span className="capitalize">{g}</span><span>{groups[g].length - left(g)} / {groups[g].length}</span></div>)}</Card>
          <Card title="Job & unit" action={<Link className="text-xs font-semibold text-primary" href="/technician/units/unit-online-rto">Unit →</Link>}><SummaryList items={[["Window", "10:00–12:00 today"], ["Site", "customer-a · Home A › 1F › Bedroom"], ["Access", "Security desk · show job ID"], ["Unit", "unit-online-rto · Split 2.5 kW"], ["Linked alert", "Bedroom too hot · warning"]]} /><div className="mt-3 flex flex-wrap gap-2"><Link className="text-xs font-semibold text-primary" href="/technician/units/unit-online-rto/alerts?jobId=job-t07">Alert evidence →</Link><Link className="text-xs font-semibold text-primary" href="/technician/units/unit-online-rto/control?jobId=job-contractor-a">Diagnostic control →</Link></div></Card>
          <Card title="Versions & autosave"><SummaryList items={[["Draft v2", "autosaved 11:44 · 7 results · 1 photo"], ["Draft v1", "saved 10:40 · 3 results · no photos"], ["Work window", "10:00–12:00 · warning at 11:45 · discard at 12:00"]]} /></Card>
        </div>
      </div>
      <Modal open={confirm} onClose={() => setConfirm(false)} title="Submit report v2?" footer={<><Btn onClick={() => setConfirm(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setStatus("Submitted"); setConfirm(false); toast("Report v2 submitted"); }}>Submit</Btn></>}><p className="text-[13px]">Submitted versions become read-only. HQ/contractor quality review follows.</p></Modal>
    </Page>
  );
}
