"use client";

import Link from "next/link";
import { useState } from "react";
import { Banner, Btn, Card, Field, Input, Modal, Page, Select, SummaryList, Textarea, useToast } from "@ac/web/components/ui";

/** The Phase 1A demo of diagnostic control (fixtures; DATA_SOURCE=api uses ControlView). */
export function ControlDemo({ id }: { id: string }) {
  const toast = useToast();
  const restricted = id === "unit-limited";
  const [reason, setReason] = useState("Confirming mode change resolves reported airflow complaint.");
  const [mode, setMode] = useState("cool");
  const [temp, setTemp] = useState("24");
  const [dur, setDur] = useState("5");
  const [trReason, setTrReason] = useState("Verify cooling recovers after coil clean.");
  const [confirm, setConfirm] = useState<null | "cmd" | "test">(null);
  const [errs, setErrs] = useState<string[]>([]);
  const [hist, setHist] = useState([["09:41", "test run start (Power ON, 5 min)", "cmd-0418 · acknowledged 09:41:03 · ends 09:46"], ["09:40", "set_mode = cool", "cmd-0417 · acknowledged 09:40:02 · mode confirmed by device"], ["09:32", "set_temperature = 15 °C", "cmd-0416 · rejected — below model minimum 16 °C (VALIDATION)"], ["09:30", "read state", "cmd-0415 · acknowledged 09:30:01"]]);
  const [run, setRun] = useState<"running" | "ended" | null>("running");
  const check = (kind: "cmd" | "test") => {
    const e: string[] = [];
    if (kind === "cmd") { if (!reason.trim()) e.push("Reason is required."); if (+temp < (restricted ? 24 : 16) || +temp > 30) e.push(restricted ? `FORBIDDEN — ${temp} °C is below the restriction minimum 24 °C.` : "VALIDATION — temperature must be 16–30 °C."); }
    else { if (+dur < 1 || +dur > 15) e.push("VALIDATION — test run is 1–15 minutes only."); if (!trReason.trim()) e.push("Reason is required."); }
    setErrs(e); if (!e.length) setConfirm(kind);
  };
  const send = () => {
    const kind = confirm; setConfirm(null);
    setHist((h) => [["now", kind === "cmd" ? `set_mode = ${mode}` : `test run start (Power ON, ${dur} min)`, "cmd-0419 · acknowledged"], ...h]);
    if (kind === "test") setRun("running");
    toast("Command acknowledged by device");
  };
  return (
    <Page>
      <div className="text-[13px] text-muted"><Link href={`/technician/units/${id}`} className="font-semibold text-primary">← Unit</Link> › Diagnostic control · {id}</div>
      {errs.length > 0 && <Banner tone="crit">{errs.map((e) => <div key={e}>{e}</div>)}</Banner>}
      {restricted && <Banner tone="warn">Cooling restriction active — minimum 24 °C applies to diagnostics too.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Normal diagnostic action">
            <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}><Field label="Job"><Input readOnly value="job-contractor-a" /></Field><Field label="Action"><Select value={mode} onChange={(e) => setMode(e.target.value)}><option value="cool">set_mode = cool</option><option value="dry">set_mode = dry</option><option value="fan">set_mode = fan</option></Select></Field><Field label="Set temp (°C)"><Input type="number" value={temp} onChange={(e) => setTemp(e.target.value)} /></Field></div>
            <div className="mt-3"><Field label="Reason (required)"><Textarea value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant="primary" onClick={() => check("cmd")}>Review & send</Btn></div>
          </Card>
          <Card title="Test run" sub="1–15 minutes only. Reason required. Cannot start during firmware updates or an unfinished Command.">
            <div className="grid-fluid" style={{ ["--min"as string]: "150px" }}><Field label="Start action"><Input readOnly value="Power ON" /></Field><Field label="Duration (min)"><Input type="number" value={dur} onChange={(e) => setDur(e.target.value)} /></Field><Field label="End action"><Input readOnly value="Power OFF" /></Field></div>
            <div className="mt-3"><Field label="Reason (required)"><Textarea value={trReason} onChange={(e) => setTrReason(e.target.value)} /></Field></div>
            {run === "running" && <div className="mt-3"><Banner>Running — ends 09:46 (start acknowledged 09:41). DiagnosticRun running · endAt = startedAt + 5 min. The end Command is sent at 09:46. <Btn size="sm" className="ml-2" onClick={() => { setRun("ended"); toast("End acknowledged — Stopped"); }}>Simulate end</Btn></Banner></div>}
            {run === "ended" && <div className="mt-3"><Banner tone="ok">End acknowledged — test run completed, unit Stopped.</Banner></div>}
            <div className="mt-3 flex justify-end"><Btn variant="primary" disabled={run === "running"} onClick={() => check("test")}>Start test run</Btn></div>
          </Card>
          <Card title="Command history — this job">{hist.map(([t, a, b], i) => <div key={i} className="border-t border-line py-2 text-[13px] first:border-0"><b>{t} · {a}</b><div className={`text-xs ${b.includes("rejected") ? "text-crit" : "text-muted"}`}>{b}</div></div>)}</Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Current unit state" sub="observed 09:41:02"><SummaryList items={[["Power", "On"], ["Mode", "Cool"], ["Setpoint", "24 °C"], ["Fan", "Auto"], ["Room", "30.4 °C"]]} /><p className="mt-2 text-[11px] text-muted">Capability v3 allows: set_mode (cool/dry/fan), set_temperature 16–30 °C, fan speed, test run 1–15 min. No firmware update running.</p></Card>
          <Card title="Authorization"><SummaryList items={[["Assignment", "job-contractor-a"], ["Window", "10:00–12:00"], ["Restriction", restricted ? "min 24 °C" : "none"], ["Pending commands", "1 (test run)"]]} /><p className="mt-2 text-[11px] text-muted">Checked again just before each command. When the window ends, no end command can be sent (end_blocked).</p></Card>
        </div>
      </div>
      <Modal open={!!confirm} onClose={() => setConfirm(null)} title="Send this diagnostic command?" footer={<><Btn onClick={() => setConfirm(null)}>Cancel</Btn><Btn variant="primary" onClick={send}>Send command</Btn></>}>
        <SummaryList items={[["Unit", id], ["Job", "job-contractor-a"], ["Action", confirm === "cmd" ? `set_mode = ${mode}` : `test run ${dur} min`], ["Reason", confirm === "cmd" ? reason : trReason]]} />
      </Modal>
    </Page>
  );
}
