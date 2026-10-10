"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, Modal, Page, Select, SummaryList, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { actionOf } from "@ac/web/lib/clientAutomations";
import { actionCode, actionWord, fieldText, refusal } from "@ac/web/lib/techControl";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { sendDiagnostic, startTestRun } from "../actions";
import type { ControlLive } from "../_lib/load";

/** Diagnostic control (FR-T10, Figma Technician 02-18…02-28) in API mode: one diagnostic command or one test run at a
 * time for the assigned job, each confirmed first; device acknowledgements and the run's end are shown only when the
 * device reports them. Texts in the user's display language; the times come formatted from the loader (IR286). */
export function ControlView({ live }: { live: ControlLive }) {
  const t = useT();
  const router = useRouter();
  const [pending, run] = useAction();
  const options = live.unit.options;
  const first = options[0]?.options[0]?.key ?? "power:on";
  const [action, setAction] = useState(options.find((g) => g.options[0]?.key.startsWith("mode:"))?.options[0]?.key ?? first);
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
    const timer = setInterval(() => router.refresh(), 4000);
    return () => clearInterval(timer);
  }, [live.active, router]);
  const back = <div className="flex flex-wrap items-center gap-2 text-[13px]"><Link href={live.unitHref} className="font-semibold text-primary" aria-label={t("Back")}>‹</Link><h1 className="text-base font-bold">{t("Diagnostic control")}</h1><span className="text-muted">· {live.unit.name}</span></div>;
  if (!live.canDiagnose) return <Page>{back}<EmptyState title={t("No diagnostic permission")}>{t("Diagnostic control needs control.diagnose on an assigned job. Ask HQ if you should have it.")}</EmptyState></Page>;
  if (!live.job) return <Page>{back}<EmptyState title={t("Choose the job first")} action={<Link href={live.unitHref} className="text-xs font-semibold text-primary">{t("← Unit and its jobs")}</Link>}>{t("Diagnostic commands and test runs belong to a job you are assigned to, inside its work window — open diagnostic control from the unit or the job.")}</EmptyState></Page>;
  const job = live.job;
  const busy = live.active || !!live.unit.blocked;
  const failed = (f: ActionFailure) => {
    const fe: Record<string, string> = {};
    for (const [k, v] of Object.entries(f.fieldErrors)) fe[k] = fieldText(v, t);
    setErrors(fe);
    setRefused(refusal(f, t));
  };
  const review = (kind: "cmd" | "run") => {
    const e: Record<string, string> = {};
    if (kind === "cmd" && (reason.trim().length < 1 || reason.trim().length > 1000)) e.reason = t("Reason is required (1–1000 characters)");
    if (kind === "run") {
      const n = Number(minutes);
      if (!Number.isInteger(n) || n < 1 || n > 15) e.durationMinutes = t("VALIDATION — a test run is 1–15 minutes only");
      if (trReason.trim().length < 1 || trReason.trim().length > 1000) e.trReason = t("Reason is required (1–1000 characters)");
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
        (c) => t("{id} sent — waiting for the device", { id: c.id.slice(0, 8) }), () => { setErrors({}); setRefused(null); }, failed);
    } else {
      run(() => startTestRun({ jobId: job.id, unitId: live.unit.id, startAction: actionOf(start), endAction: actionOf(end), durationMinutes: Number(minutes), reason: trReason,
        expectedUnitVersion: live.unit.version, expectedJobVersion: job.version }), t("Test run requested — waiting for the start acknowledgement"), () => { setErrors({}); setRefused(null); },
        (f) => failed({ ...f, fieldErrors: Object.fromEntries(Object.entries(f.fieldErrors).map(([k, v]) => [k === "reason" ? "trReason" : k, v])) }));
    }
  };
  const select = (value: string, set: (v: string) => void, label: string) => (
    <Select aria-label={label} value={value} onChange={(e) => set(e.target.value)}>
      {options.map((g) => <optgroup key={g.group} label={g.group}>{g.options.map((o) => <option key={o.key} value={o.key}>{actionCode(actionOf(o.key))}</option>)}</optgroup>)}
    </Select>
  );
  const latest = live.history.find((h) => !h.run); // this card's own last command
  return (
    <Page>
      {back}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {live.unit.restriction && <Banner tone="warn">{t("Restriction active — {restriction} applies to diagnostics too.", { restriction: live.unit.restriction })}</Banner>}
      {live.unit.blocked && <Banner tone="warn">{t("Control is blocked right now ({reason}) — commands and test runs wait until it clears.", { reason: live.unit.blocked })}</Banner>}
      {!job.inWindow && <Banner tone="warn">{t("Outside the work window {window} — the Core API refuses diagnostic commands now (IR94).", { window: job.window })}</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Normal diagnostic action")}>
            <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
              <Field label={t("Job")}><Input readOnly value={job.id.slice(0, 8)} className="bg-surface2" /></Field>
              <Field label={t("Action")} error={errors.action}>{select(action, setAction, t("Action"))}</Field>
            </div>
            <div className="mt-3"><Field label={t("Reason (required)")} error={errors.reason}><Textarea value={reason} maxLength={1000} placeholder={t("Why this command is needed for the diagnosis")} onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant="primary" disabled={pending || busy || options.length === 0} onClick={() => review("cmd")}>{t("Review & send")}</Btn></div>
            {latest && <p className={`mt-3 rounded-xl px-3 py-2 text-xs font-semibold ${latest.badge.tone === "ok" ? "bg-ok-soft text-ok" : latest.badge.tone === "crit" ? "bg-crit-soft text-crit" : "bg-surface2 text-muted"}`}>{latest.title} — {latest.sub}</p>}
          </Card>
          <Card title={t("Current unit state")} action={<span className="text-xs text-muted">{live.unit.observed}</span>}>
            <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>
              {live.unit.tiles.map((x) => <div key={x.label} className="rounded-xl bg-surface2 px-3 py-2"><div className="text-[11px] text-muted">{x.label}</div><div className={`text-[15px] font-bold ${x.warn ? "text-crit" : ""}`}>{x.value}</div></div>)}
            </div>
            <p className="mt-2 text-[11px] text-muted">{live.unit.capabilityText}</p>
          </Card>
          <Card title={t("Command history — this job")} sub={t("Newest first · the device's acknowledgement is the only proof a setting changed")}>
            {live.history.length === 0 ? <p className="text-[13px] text-muted">{t("No commands for this job yet.")}</p> : live.history.map((h) => (
              <div key={h.id} className="flex flex-wrap items-start justify-between gap-2 border-t border-line py-2 text-[13px] first:border-0">
                <div><b>{h.at} · {h.title}</b><div className={`text-xs ${h.badge.tone === "crit" ? "text-crit" : "text-muted"}`}>{h.sub}</div></div>
                <Badge tone={h.badge.tone}>{h.badge.text}</Badge>
              </div>
            ))}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Test run")}>
            <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
              <Field label={t("Start action")}>{select(start, setStart, t("Start action"))}</Field>
              <Field label={t("Duration (min)")} error={errors.durationMinutes}><Input inputMode="numeric" value={minutes} onChange={(e) => setMinutes(e.target.value)} /></Field>
            </div>
            <div className="mt-3"><Field label={t("End action")}>{select(end, setEnd, t("End action"))}</Field></div>
            <div className="mt-3"><Field label={t("Reason (required)")} error={errors.trReason}><Textarea value={trReason} maxLength={1000} placeholder={t("What the test run should show")} onChange={(e) => setTrReason(e.target.value)} /></Field></div>
            {live.banner && <div className="mt-3"><Banner tone={live.banner.tone}><b>{live.banner.title}</b><div className="text-xs">{live.banner.text}</div></Banner></div>}
            <div className="mt-3"><Btn className="w-full" disabled={pending || busy || options.length === 0} onClick={() => review("run")}>{t(live.run && live.active ? "Start test run (running)" : "Start test run")}</Btn></div>
            <p className="mt-2 text-[11px] text-muted">{t("1–15 minutes only. Reason required. Cannot start during firmware updates or an unfinished command.")}</p>
          </Card>
          <Card title={t("Authorization")}>
            <SummaryList items={[[t("Assignment"), job.id.slice(0, 8)], [t("Window"), <span key="w" className={job.inWindow ? "" : "text-crit"}>{job.window}</span>], [t("Restriction"), live.unit.restriction ?? t("none")], [t("Pending commands"), String(live.unit.pending)]]} />
            <p className="mt-2 text-[11px] text-muted">{t("Checked again just before each command. When the window ends, no end command can be sent (end_blocked).")}</p>
          </Card>
        </div>
      </div>
      <Modal open={confirm !== null} onClose={() => setConfirm(null)} title={t(confirm === "run" ? "Start this test run?" : "Send this diagnostic command?")}
        footer={<><Btn onClick={() => setConfirm(null)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t(confirm === "run" ? "Start test run" : "Send command")}</Btn></>}>
        <SummaryList items={[
          [t("Target"), live.unit.name], [t("Job"), t("{job} · window {window}", { job: job.id.slice(0, 8), window: job.window })],
          [t("Action"), confirm === "run" ? t("{start} for {n} min, then {end}", { start: actionWord(actionOf(start), t), n: minutes, end: actionWord(actionOf(end), t) }) : actionCode(actionOf(action))],
          [t("Current (confirmed)"), live.unit.current], [t("Reason"), (confirm === "run" ? trReason : reason).trim()],
        ]} />
        <p className="text-xs text-muted">{t("Permission, work window, capability and restriction are checked again just before sending. The setting changes only after the device acknowledges the command.")}</p>
      </Modal>
    </Page>
  );
}
