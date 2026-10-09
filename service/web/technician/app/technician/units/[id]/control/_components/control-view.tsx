"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { actionOf, actionOptions, type Caps } from "@ac/web/lib/clientAutomations";
import { actionCode, actionWord, type ApiRun, type HistoryRow } from "@ac/web/lib/techControl";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { sendDiagnostic, startTestRun } from "../actions";

export type ControlLive = {
  unit: {
    id: string; name: string; version: number; connection: string; tiles: { label: string; value: string; warn?: boolean }[]; observed: string; pending: number;
    caps: Caps | null; capabilityText: string; restriction: string; blocked: string | null;
  };
  job: { id: string; version: number; status: string; window: string; inWindow: boolean } | null;
  run: ApiRun | null; banner: { tone: "primary" | "warn" | "ok" | "crit"; title: string; text: string } | null; active: boolean;
  history: HistoryRow[]; canDiagnose: boolean;
};

/** The DomainError of a refused diagnostic write, in the words of Figma Technician 02 (IR46 / IR47 / D04). */
function refusal(f: ActionFailure): string | null {
  if (f.messageKey === "errors.restriction_active") return "FORBIDDEN — the active restriction does not allow this action (for example a setpoint below its minimum).";
  if (f.messageKey === "errors.unit_busy") return "CONFLICT — the unit is busy: a firmware update or an unfinished command must finish first.";
  if (f.messageKey === "errors.reconciliation_required") return "CONFLICT — the unit must be reconciled after a restriction before it can be controlled.";
  if (f.code === "OFFLINE") return "OFFLINE — the device is not reachable (connection or power signal), so nothing was sent.";
  if (f.code === "FORBIDDEN") return "FORBIDDEN — outside your assignment or work window, or without diagnostic permission.";
  if (f.code === "CONFLICT") return "CONFLICT — the unit or job changed; the page now shows the latest version.";
  return null;
}
const fieldText: Record<string, string> = { "error.length": "1–1000 characters", "error.required": "Required", "error.range": "1–15 minutes", "error.unsupportedAction": "Not supported by this AC", "error.invalid": "Not valid" };

/** Diagnostic control (FR-T10) in API mode: one diagnostic command or one test run at a time for the assigned job,
 * each confirmed first; device acknowledgements and the run's end are shown only when the device reports them. */
export function ControlView({ live }: { live: ControlLive }) {
  const router = useRouter();
  const [pending, run] = useAction();
  const options = actionOptions(live.unit.caps);
  const first = options[0]?.options[0]?.key ?? "power:on";
  const [action, setAction] = useState(options.find((g) => g.group === "Mode")?.options[0]?.key ?? first);
  const [reason, setReason] = useState("");
  const [start, setStart] = useState("power:on");
  const [end, setEnd] = useState("power:off");
  const [minutes, setMinutes] = useState("5");
  const [trReason, setTrReason] = useState("");
  const [confirm, setConfirm] = useState<null | "cmd" | "run">(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [refused, setRefused] = useState<string | null>(null);
  useEffect(() => { // device acknowledgements and run states arrive on the server: refresh while something is open
    if (!live.active) return;
    const t = setInterval(() => router.refresh(), 4000);
    return () => clearInterval(t);
  }, [live.active, router]);
  const back = <div className="text-[13px] text-muted"><Link href={`/technician/units/${live.unit.id}`} className="font-semibold text-primary">← Unit</Link> › Diagnostic control · {live.unit.name}</div>;
  if (!live.canDiagnose) return <Page>{back}<EmptyState title="No diagnostic permission">Diagnostic control needs control.diagnose on an assigned job. Ask HQ if you should have it.</EmptyState></Page>;
  if (!live.job) return <Page>{back}<EmptyState title="Choose the job first" action={<Link href={`/technician/units/${live.unit.id}`} className="text-xs font-semibold text-primary">← Unit and its jobs</Link>}>Diagnostic commands and test runs belong to a job you are assigned to, inside its work window — open diagnostic control from the unit or the job.</EmptyState></Page>;
  const job = live.job;
  const busy = live.active || !!live.unit.blocked;
  const failed = (f: ActionFailure) => {
    const fe: Record<string, string> = {};
    for (const [k, v] of Object.entries(f.fieldErrors)) fe[k] = fieldText[v] ?? v.replace(/^errors?\./, "").replace(/_/g, " ");
    setErrors(fe);
    setRefused(refusal(f));
  };
  const review = (kind: "cmd" | "run") => {
    const e: Record<string, string> = {};
    if (kind === "cmd" && (reason.trim().length < 1 || reason.trim().length > 1000)) e.reason = "Reason is required (1–1000 characters)";
    if (kind === "run") {
      const n = Number(minutes);
      if (!Number.isInteger(n) || n < 1 || n > 15) e.durationMinutes = "VALIDATION — a test run is 1–15 minutes only";
      if (trReason.trim().length < 1 || trReason.trim().length > 1000) e.trReason = "Reason is required (1–1000 characters)";
    }
    setErrors(e);
    setRefused(null);
    if (!Object.keys(e).length) setConfirm(kind);
  };
  const send = () => {
    const kind = confirm;
    setConfirm(null);
    if (kind === "cmd") {
      run(() => sendDiagnostic({ unitId: live.unit.id, jobId: job.id, action: actionOf(action), reason, expectedUnitVersion: live.unit.version }),
        (c) => `${c.id.slice(0, 8)} sent — waiting for the device`, () => { setErrors({}); setRefused(null); }, failed);
    } else {
      run(() => startTestRun({ jobId: job.id, unitId: live.unit.id, startAction: actionOf(start), endAction: actionOf(end), durationMinutes: Number(minutes), reason: trReason,
        expectedUnitVersion: live.unit.version, expectedJobVersion: job.version }), "Test run requested — waiting for the start acknowledgement", () => { setErrors({}); setRefused(null); },
        (f) => failed({ ...f, fieldErrors: Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [k === "reason" ? "trReason" : k, v])) }));
    }
  };
  const select = (value: string, set: (v: string) => void, label: string) => (
    <Select aria-label={label} value={value} onChange={(e) => set(e.target.value)}>
      {options.map((g) => <optgroup key={g.group} label={g.group}>{g.options.map((o) => <option key={o.key} value={o.key}>{actionCode(actionOf(o.key))}</option>)}</optgroup>)}
    </Select>
  );
  const latest = live.history.find((h) => !h.title.startsWith("test run")); // this card's own last command
  return (
    <Page>
      {back}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {live.unit.restriction !== "none" && <Banner tone="warn">Restriction active — {live.unit.restriction} applies to diagnostics too.</Banner>}
      {live.unit.blocked && <Banner tone="warn">Control is blocked right now ({live.unit.blocked}) — commands and test runs wait until it clears.</Banner>}
      {!job.inWindow && <Banner tone="warn">Outside the work window {job.window} — the Core API refuses diagnostic commands now (IR94).</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Normal diagnostic action">
            <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
              <Field label="Job"><Input readOnly value={job.id.slice(0, 8)} className="bg-surface2" /></Field>
              <Field label="Action" error={errors.action}>{select(action, setAction, "Action")}</Field>
            </div>
            <div className="mt-3"><Field label="Reason (required)" error={errors.reason}><Textarea value={reason} maxLength={1000} placeholder="Why this command is needed for the diagnosis" onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant="primary" disabled={pending || busy || options.length === 0} onClick={() => review("cmd")}>Review & send</Btn></div>
            {latest && <p className={`mt-3 rounded-xl px-3 py-2 text-xs font-semibold ${latest.badge.tone === "ok" ? "bg-ok-soft text-ok" : latest.badge.tone === "crit" ? "bg-crit-soft text-crit" : "bg-surface2 text-muted"}`}>{latest.title} — {latest.sub}</p>}
          </Card>
          <Card title="Current unit state" action={<span className="text-xs text-muted">{live.unit.observed}</span>}>
            <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>
              {live.unit.tiles.map((t) => <div key={t.label} className="rounded-xl bg-surface2 px-3 py-2"><div className="text-[11px] text-muted">{t.label}</div><div className={`text-[15px] font-bold ${t.warn ? "text-crit" : ""}`}>{t.value}</div></div>)}
            </div>
            <p className="mt-2 text-[11px] text-muted">{live.unit.capabilityText} Connection {live.unit.connection}.</p>
          </Card>
          <Card title="Command history — this job" sub="Newest first · the device's acknowledgement is the only proof a setting changed">
            {live.history.length === 0 ? <p className="text-[13px] text-muted">No commands for this job yet.</p> : live.history.map((h) => (
              <div key={h.id} className="flex flex-wrap items-start justify-between gap-2 border-t border-line py-2 text-[13px] first:border-0">
                <div><b>{h.at} · {h.title}</b><div className={`text-xs ${h.badge.tone === "crit" ? "text-crit" : "text-muted"}`}>{h.sub}</div></div>
                <Badge tone={h.badge.tone}>{h.badge.text}</Badge>
              </div>
            ))}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Test run">
            <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
              <Field label="Start action">{select(start, setStart, "Start action")}</Field>
              <Field label="Duration (min)" error={errors.durationMinutes}><Input inputMode="numeric" value={minutes} onChange={(e) => setMinutes(e.target.value)} /></Field>
            </div>
            <div className="mt-3"><Field label="End action">{select(end, setEnd, "End action")}</Field></div>
            <div className="mt-3"><Field label="Reason (required)" error={errors.trReason}><Textarea value={trReason} maxLength={1000} placeholder="What the test run should show" onChange={(e) => setTrReason(e.target.value)} /></Field></div>
            {live.banner && <div className="mt-3"><Banner tone={live.banner.tone}><b>{live.banner.title}</b><div className="text-xs">{live.banner.text}</div></Banner></div>}
            <div className="mt-3"><Btn className="w-full" disabled={pending || busy || options.length === 0} onClick={() => review("run")}>{live.run && live.active ? "Start test run (running)" : "Start test run"}</Btn></div>
            <p className="mt-2 text-[11px] text-muted">1–15 minutes only. Reason required. Cannot start during firmware updates or an unfinished command.</p>
          </Card>
          <Card title="Authorization">
            <SummaryList items={[["Assignment", job.id.slice(0, 8)], ["Window", <span key="w" className={job.inWindow ? "" : "text-crit"}>{job.window}</span>], ["Restriction", live.unit.restriction], ["Pending commands", String(live.unit.pending)]]} />
            <p className="mt-2 text-[11px] text-muted">Checked again just before each command. When the window ends, no end command can be sent (end_blocked).</p>
          </Card>
        </div>
      </div>
      <Modal open={confirm !== null} onClose={() => setConfirm(null)} title={confirm === "run" ? "Start this test run?" : "Send this diagnostic command?"}
        footer={<><Btn onClick={() => setConfirm(null)}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={send}>{confirm === "run" ? "Start test run" : "Send command"}</Btn></>}>
        <SummaryList items={[
          ["Target", live.unit.name], ["Job", `${job.id.slice(0, 8)} · window ${job.window}`],
          ["Action", confirm === "run" ? `${actionWord(actionOf(start))} for ${minutes} min, then ${actionWord(actionOf(end))}` : actionCode(actionOf(action))],
          ["Current (confirmed)", live.unit.tiles.filter((t) => t.label !== "Room").map((t) => t.value).join(" · ")], ["Reason", (confirm === "run" ? trReason : reason).trim()],
        ]} />
        <p className="text-xs text-muted">Permission, work window, capability and restriction are checked again just before sending. The setting changes only after the device acknowledges the command.</p>
      </Modal>
    </Page>
  );
}
