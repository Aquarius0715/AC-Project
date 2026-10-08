"use client";

import Link from "next/link";
import { use, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Badge, Banner, Btn, Card, Choice, DataTable, Field, Input, Modal, Page, Select, SummaryList, Tabs, Textarea, UtilBar, cx, useToast } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { Job, fmt, jobActions, longDate, useJobs } from "@ac/web/lib/jobs";

type R = "normal" | "attention" | "not_inspected" | "not_applicable" | null;
const groups: Record<string, string[]> = {
  indoor: ["Filter", "Evaporator coil", "Blower motor", "Blower fan", "Drain pipe", "Drain pan", "Outlet", "Louver"],
  outdoor: ["Compressor", "Condenser coil", "Fan motor", "Refrigerant line", "Service valve"],
  electrical: ["Wiring insulation", "Capacitor", "Breaker", "Terminals", "Grounding"],
};
const labels = { indoor: "Indoor (8)", outdoor: "Outdoor (5)", electrical: "Electrical (5)" };

export default function WorkspaceRoute({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const live = useJobs().find((j) => j.id === id && j.technician === "tech-external-a" && !["job-contractor-a", "job-t07", "job-p05", "job-p09"].includes(j.id));
  return live ? <LiveAssignment j={live} /> : <Workspace id={id} />;
}

/** Assignment from the shared store (IR113): accept (受領) or “can’t make this time”. */
function LiveAssignment({ j }: { j: Job }) {
  const toast = useToast();
  const [cant, setCant] = useState(false);
  const [reason, setReason] = useState<"training" | "overrun" | "sick" | "other">("other");
  const [comment, setComment] = useState("");
  const [alt, setAlt] = useState("Fri 10-02 14:00–16:00");
  const [tried, setTried] = useState(false);
  const ack = j.techAck?.status ?? "pending";
  const coord = j.delivery === "contractor" ? j.contractor : "HQ";
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href="/technician" className="font-semibold text-primary">← Overview</Link><b className="text-ink">{j.id} · {j.unit} — {ack === "pending" ? "new assignment" : "assigned"}</b></div>
      {ack === "accepted" && <Banner tone="ok">You accepted this assignment. Start job (with check-in) unlocks at the start of the window on {j.scheduled?.date.slice(5)}.</Banner>}
      {ack === "cant_make" && <Banner tone="warn">You told {coord} you can’t make it (“{j.techAck?.reason}”). They will reassign the job or ask the client for a new time.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Inspection checklist" sub="Read-only until you accept and the work window opens."><p className="text-xs text-muted">Indoor (8) · Outdoor (5) · Electrical (5) — all results start empty.</p></Card>
          <Card title="Job & unit" action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${j.unitId}`}>Unit →</Link>}><SummaryList items={[["Window", j.scheduled ? longDate(j.scheduled) : "—"], ["Site", `${j.customer} · ${j.loc}`], ["Access", "Security desk · show job ID"], ["Unit", `${j.unitId} · Split 2.5 kW`], ["Request", `“${j.symptom}”`]]} /></Card>
        </div>
        <Card title={ack === "pending" ? "New assignment — please accept" : "Job — assigned to you"} className="self-start">
          <SummaryList items={[["Job", `${j.type} · ${j.customer} · ${j.unitId}`], ["Origin", <OriginBadge key="o" origin={j.origin} />], ["Visit time", <b key="v">{j.scheduled ? longDate(j.scheduled) : "—"}</b>], ["Assigned by", coord ?? "HQ"], ["Agreed with", j.origin === "plan" ? "periodic plan (client notified)" : "the client (via HQ)"]]} />
          <div className="mt-3"><Banner>Accepting tells {coord} and HQ that you will attend at this time. The client then sees your name.</Banner></div>
          {ack === "pending" && <div className="mt-3 flex flex-col gap-2"><Btn variant="primary" onClick={() => { jobActions.techAccept(j.id); toast("Assignment accepted"); }}>✓ Accept assignment</Btn><Btn variant="danger" onClick={() => setCant(true)}>Can’t make this time…</Btn></div>}
          {ack === "accepted" && <div className="mt-3"><Btn variant="primary" disabled className="w-full">Start job (opens check-in)</Btn></div>}
        </Card>
      </div>
      <Modal open={cant} onClose={() => setCant(false)} title="Can’t make this time" footer={<><Btn onClick={() => setCant(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!comment.trim()) return; jobActions.techCantMake(j.id, comment.trim(), alt); setCant(false); toast(`Sent to ${coord}`, "warn"); }}>Send to {coord}</Btn></>}>
        <p className="text-xs text-muted">{j.id} · {fmt(j.scheduled)} · assigned by {coord}</p>
        <Field label="Reason"><Choice value={reason} onChange={setReason} options={[{ id: "training", label: "Training / course" }, { id: "overrun", label: "Another job runs over" }, { id: "sick", label: "Sick" }, { id: "other", label: "Other" }]} /></Field>
        <Field label="Comment (required)" error={tried && !comment.trim() ? "Tell your coordinator why" : undefined}><Input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Medical appointment on 10-01 morning." /></Field>
        <Field label="A time you could do instead (optional)"><Select value={alt} onChange={(e) => setAlt(e.target.value)}><option>Fri 10-02 14:00–16:00</option><option>Mon 10-05 10:00–12:00</option><option value="">—</option></Select></Field>
        <Banner>{coord} reassigns the job or proposes a new time — the client must approve any change. Until then the job stays on your list as “Can’t make it”.</Banner>
      </Modal>
    </Page>
  );
}

function Workspace({ id }: { id: string }) {
  const toast = useToast();
  // URL key `tab` selects the checklist group (SCR-T04: indoor default · outdoor · electrical, IR115).
  const sp = useSearchParams();
  const initialTab = sp.get("tab");
  const [tab, setTabState] = useState<"indoor" | "outdoor" | "electrical">(initialTab === "outdoor" || initialTab === "electrical" ? initialTab : "indoor");
  const setTab = (t: "indoor" | "outdoor" | "electrical") => {
    setTabState(t);
    const u = new URL(window.location.href);
    u.searchParams.set("tab", t);
    window.history.replaceState(null, "", u);
  };
  const [res, setRes] = useState<Record<string, R>>({ Filter: "attention", "Evaporator coil": "normal", "Blower motor": "normal", "Blower fan": "normal", "Drain pipe": "normal", "Drain pan": "normal", Outlet: "not_applicable" });
  const [why, setWhy] = useState<Record<string, string>>({ Filter: "heavy dust buildup, recommend replacement", Outlet: "no accessible outlet in this installation" });
  const [readings, setReadings] = useState([["Supply air temperature", "14.2", "°C", "Evaporator coil"], ["Return air temperature", "25.8", "°C", "Evaporator coil"], ["Filter pressure drop", "118", "Pa", "Filter"], ["Blower motor current", "1.9", "A", "Blower motor"]]);
  const [work, setWork] = useState("Cleaned filter, inspected coil and blower — all normal except filter (dust buildup, recommend replacement). No parts used yet.");
  const [status, setStatus] = useState(id === "job-t07" ? "Assigned" : id === "job-p09" ? "Submitted" : "In progress");
  const [errs, setErrs] = useState<string[]>([]);
  const [confirm, setConfirm] = useState(false);
  const [view, setView] = useState<"readings" | "parts" | "time">("readings");
  const [parts, setParts] = useState([{ p: "Air filter (standard)", c: "AF-STD-2.5 · replaces Filter (Attention)", src: "Van stock", q: 1, lot: "lot 2609A" }, { p: "Drain pan tablet", c: "DP-TAB-10", src: "Van stock", q: 2, lot: "—" }]);
  const [partModal, setPartModal] = useState(false);
  const [signModal, setSignModal] = useState(false);
  const [signed, setSigned] = useState<string | null>(null);
  const [checkIn, setCheckIn] = useState(false);
  if (id === "job-internal-a") return <Page className="max-w-xl"><Card title="This page isn’t available" sub="Not assigned to you (NOT_FOUND)." /></Page>;
  const readOnly = status === "Submitted" || status === "Assigned" || status === "On hold";
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
      {status === "Assigned" && <Banner action={<span className="flex gap-2"><Btn size="sm" disabled>Start job</Btn><Btn size="sm" variant="ghost" onClick={() => setCheckIn(true)}>Preview check-in</Btn></span>}>Before the work window (starts 14:00): read-only, Start is disabled (IR76). On site, Start job opens Check in (location + unit QR).</Banner>}
      {status === "Submitted" && <Banner tone="ok">Submitted v1 — read-only, awaiting quality review.</Banner>}
      {status === "In progress" && <Banner tone="warn">Work window ends in 15 min — unsaved input will be discarded at 12:00 (IR89).</Banner>}
      {status === "On hold" && <Banner tone="warn" action={<Btn size="sm" variant="ghost" onClick={() => setStatus("In progress")}>Demo: HQ resumes</Btn>}>On hold by HQ — save and submit are blocked (CONFLICT, IR93). Your draft v2 is kept; you can continue when HQ resumes the job.</Banner>}
      {status === "In progress" && <p className="text-right text-[11px] text-muted"><button className="underline" onClick={() => setStatus("On hold")}>Demo: HQ puts the job on hold</button></p>}
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
          <Card title={<Tabs value={view} onChange={setView} tabs={[{ id: "readings", label: "Readings", count: readings.length }, { id: "parts", label: "Parts & refrigerant", count: parts.length }, { id: "time", label: "Time on site" }]} />} action={view === "readings" ? <Btn size="sm" disabled={readOnly} onClick={() => setReadings((r) => [...r, ["New reading", "", "", ""]])}>+ Add reading</Btn> : view === "parts" ? <Btn size="sm" disabled={readOnly} onClick={() => setPartModal(true)}>+ Add part</Btn> : null}>
            {view === "parts" ? <PartsView parts={parts} readOnly={readOnly} onRemove={(i) => setParts((p) => p.filter((_, k) => k !== i))} /> : view === "time" ? <TimeView /> : <div className="scroll-x"><table className="w-full min-w-[460px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Metric</th><th>Value</th><th>Unit</th><th>Linked item</th></tr></thead><tbody>{readings.map((r, i) => <tr key={i} className="border-t border-line">{r.map((c, j) => <td key={j} className="py-1.5 pr-2"><Input disabled={readOnly} value={c} onChange={(e) => setReadings((rs) => rs.map((x, a) => (a === i ? x.map((y, b) => (b === j ? e.target.value : y)) : x)))} /></td>)}</tr>)}</tbody></table></div>}
          </Card>
          <Card title="Work report — draft v2">
            <Field label="Work performed & next action (10–4000 characters)"><Textarea disabled={readOnly} value={work} onChange={(e) => setWork(e.target.value)} className="min-h-[110px]" /></Field>
            <p className="mt-3 text-[13px] font-semibold">Replacement parts: {parts.length} lines · R32 +0.25 kg <button className="text-xs text-primary" onClick={() => setView("parts")}>(see Parts & refrigerant)</button></p>
            <div className="grid-fluid mt-2" style={{ ["--min"as string]: "150px" }}><Field label="Next action"><Select disabled={readOnly}><option>None</option><option>Re-inspect</option><option>Replace part</option></Select></Field></div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3"><div><b className="text-[13px]">Customer sign-off</b><div>{signed ? <Badge tone="ok">Signed · {signed}</Badge> : <Badge tone="warn">Not signed</Badge>}</div></div><Btn size="sm" disabled={readOnly} onClick={() => setSignModal(true)}>{signed ? "Sign again" : "Get signature"}</Btn></div>
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
      <Modal open={confirm} onClose={() => setConfirm(false)} title="Submit report v2?" footer={<><Btn onClick={() => setConfirm(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setStatus("Submitted"); setConfirm(false); toast("Report v2 submitted"); }}>Submit</Btn></>}><p className="text-[13px]">Submitted versions become read-only. HQ/contractor quality review follows.</p>{!signed && <Banner tone="warn">No customer sign-off yet — the report can still be submitted; HQ sees “not signed”.</Banner>}</Modal>
      <Modal open={partModal} onClose={() => setPartModal(false)} title="Add part" footer={<><Btn onClick={() => setPartModal(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setParts((p) => [...p, { p: "Capacitor 35 µF", c: "CAP-35", src: "Van stock", q: 1, lot: "lot 2608C" }]); setPartModal(false); toast("Part added — van stock is deducted when the report is accepted"); }}>Add</Btn></>}>
        <Field label="Part (van stock)"><Select><option>Capacitor 35 µF · CAP-35 · 3 in van</option><option>Air filter (standard) · AF-STD-2.5 · 1 in van</option></Select></Field>
        <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}><Field label="Qty"><Input type="number" defaultValue={1} /></Field><Field label="Lot / serial"><Input defaultValue="lot 2608C" /></Field></div>
        <Field label="Replaces checklist item (optional)"><Select><option>—</option><option>Filter (Attention)</option></Select></Field>
      </Modal>
      <Modal open={signModal} onClose={() => setSignModal(false)} title="Customer sign-off" footer={<><Btn onClick={() => setSignModal(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setSigned("Ms Tan · 11:52"); setSignModal(false); toast("Signature saved for report v2"); }}>Save signature</Btn></>}>
        <SummaryList items={[["Report", "draft v2 · 7 / 18 results"], ["Parts", `${parts.length} lines`], ["Time on site", "10:05 – now"]]} />
        <Field label="Signer name"><Input defaultValue="Ms Tan" /></Field>
        <div className="grid h-32 place-items-center rounded-xl border border-dashed border-line text-xs text-muted">✍ Sign here</div>
        <p className="text-[11px] text-muted">The signature is bound to this report version; saving the draft again clears it.</p>
      </Modal>
      <Modal open={checkIn} onClose={() => setCheckIn(false)} title="Check in at site" footer={<><Btn onClick={() => setCheckIn(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setCheckIn(false); toast("Checked in — job started (demo)"); }}>Check in & start</Btn></>}>
        <p className="text-xs text-muted">{id} · Bedroom AC · Now 14:02</p>
        <SummaryList items={[["Location", <Badge key="l" tone="ok">✓ 78 m from site (within 200 m)</Badge>], ["Unit QR", <Badge key="q" tone="ok">✓ unit-online-rto matched</Badge>], ["Arrival", <Badge key="a" tone="ok">✓ inside work window</Badge>]]} />
        <Banner>If location is unavailable or the QR code is missing, check in manually with a reason.</Banner>
      </Modal>
    </Page>
  );
}

function PartsView({ parts, readOnly, onRemove }: { parts: { p: string; c: string; src: string; q: number; lot: string }[]; readOnly: boolean; onRemove: (i: number) => void }) {
  return (
    <div className="flex flex-col gap-3">
      <DataTable rowKey={(r) => r.p} rows={parts} cols={[{ key: "p", label: "Part", render: (r) => <div><b>{r.p}</b><div className="text-[11px] text-muted">{r.c}</div></div> }, { key: "s", label: "Source", render: (r) => r.src }, { key: "q", label: "Qty", render: (r) => r.q }, { key: "l", label: "Lot / serial", render: (r) => r.lot }, { key: "x", label: "", render: (r) => <button disabled={readOnly} className="text-xs font-semibold text-primary" onClick={() => onRemove(parts.indexOf(r))}>Remove</button> }]} />
      <b className="text-[13px]">Refrigerant (R32) — cylinder CYL-R32-0182</b>
      <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>{[["Recovered", "0.40 kg"], ["Charged", "0.65 kg"], ["Net added", "+0.25 kg"], ["Leak check", "Pass · electronic detector"]].map(([k, v]) => <div key={k} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{k}</div><div className="font-bold">{v}</div></div>)}</div>
      <p className="text-[11px] text-muted">Stock is deducted from your van when the report is accepted. Refrigerant amounts are kept per unit for leak-check logs.</p>
    </div>
  );
}
function TimeView() {
  return (
    <div>
      <SummaryList items={[["Arrived", <Badge key="a" tone="ok">10:05 · checked in</Badge>], ["Location", "78 m from site · QR matched"], ["Started", "10:12"], ["Paused", "—"], ["Finished", "— (on submit)"], ["On-site time", "1 h 33 min"]]} />
      <p className="mt-2 text-[11px] text-muted">Arrival inside the agreed window counts toward SLA. Times are part of the report and visible to the contractor and HQ.</p>
    </div>
  );
}
