"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, cx, DataTable, EmptyState, Field, Input, Modal, Page, Select, SummaryList, Tabs, Textarea, UtilBar } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { klTime, metricUnit, type Metric } from "@ac/web/lib/devices";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import {
  componentLabel, draftFrom, draftInput, groupLabel, progress, readingMetrics, submitIssues, timeRows, versionRows, windowState,
  type ApiTechReport, type Draft, type DraftPart, type DraftRefrigerant, type Group, type Issue, type Result, type TimeOnSite,
} from "@ac/web/lib/techJob";
import { acknowledge, checkIn, pauseWork, resumeRework, saveDraft, signOff, submitReport, uploadPhoto } from "../actions";

export type WorkspaceLive = {
  now: string;
  job: {
    id: string; version: number; status: string; type: string; unitId: string; symptom: string; alertCount: number; origin: string;
    assignment: { scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: "pending" | "accepted" | "cant_make"; cantMakeReason: string | null } | null;
    timeOnSite: TimeOnSite | null; draftReportRef: { reportId: string; reportVersion: number } | null; reportRefs: { reportId: string; reportVersion: number }[];
  };
  unit: { name: string; place: string; model: string; scope: string[] } | null; unitRefused: string | null;
  components: { group: Group; key: string }[];
  report: ApiTechReport | null; reportRefused: string | null;
  parts: { code: string; name: string; vanStockQuantity: number | null }[];
};
type Rep = Pick<ApiTechReport, "id" | "version" | "attachmentRefs" | "signOff">;

const results: { id: NonNullable<Result>; label: string; tone: "ok" | "warn" | "muted" | "crit" }[] = [
  { id: "normal", label: "Normal", tone: "ok" }, { id: "attention", label: "Attention", tone: "warn" }, { id: "not_inspected", label: "Not inspected", tone: "crit" }, { id: "not_applicable", label: "Not applicable", tone: "muted" },
];
const statusText: Record<string, string> = { assigned: "assigned", in_progress: "in progress", on_hold: "on hold", submitted: "submitted", rework_requested: "returned for rework", completed: "completed", cancelled: "cancelled" };
const sources: { id: DraftPart["source"]; label: string }[] = [{ id: "van_stock", label: "Van stock" }, { id: "hq_warehouse", label: "HQ warehouse" }, { id: "bought_locally", label: "Bought locally" }];
const refusal = (f: ActionFailure): string =>
  f.messageKey === "errors.report_version_changed" || f.messageKey === "error.versionConflict" ? "CONFLICT — the report or job changed elsewhere; reload the page to continue from the latest version."
    : f.code === "CONFLICT" ? "CONFLICT — the job is not in a state that allows this (on hold, submitted or cancelled); the page shows the latest state."
    : f.code === "FORBIDDEN" ? `FORBIDDEN — ${f.messageKey.replace(/^errors?\./, "").replace(/_/g, " ")} (outside your work window or assignment).`
    : f.code === "UNAVAILABLE" ? "UNAVAILABLE — not sent; your draft is kept on this page. Try again." : `${f.code} — ${Object.entries(f.fieldErrors).map(([k, v]) => `${k}: ${v}`).join(", ") || f.messageKey}`;
const MAX_PHOTOS = 10, MAX_BYTES = 5 << 20;

/** The technician's job workspace (FR-T04–T06, T08, T09, T13–T15): assignment, check-in, the inspection checklist of the
 * unit's service scope, readings, parts and refrigerant, photos, the work report with its next action, the customer's
 * sign-off and the submission for quality review. The draft lives on this page until saved (autosave after edits). */
export function WorkspaceView({ live }: { live: WorkspaceLive }) {
  const { job } = live;
  const [pending, run] = useAction();
  // the business clock of the server (demo clock), advanced locally for the IR89 warning
  const [now, setNow] = useState(() => Date.parse(live.now));
  useEffect(() => {
    const offset = Date.parse(live.now) - Date.now();
    const t = setInterval(() => setNow(Date.now() + offset), 30_000);
    return () => clearInterval(t);
  }, [live.now]);
  const editableReport = job.draftReportRef && live.report && live.report.version === job.draftReportRef.reportVersion ? live.report : null;
  const [draft, setDraft] = useState<Draft>(() => draftFrom(editableReport ?? (job.status === "in_progress" ? null : live.report), live.components));
  const [rep, setRep] = useState<Rep | null>(editableReport);
  const [dirty, setDirty] = useState(false);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  // validated on the first submit attempt, then re-checked live so fixed rows and counts clear as the draft changes
  const [checked, setChecked] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  const [tab, setTab] = useState<Group>(live.components[0]?.group ?? "indoor");
  const [view, setView] = useState<"readings" | "parts" | "time">("readings");
  const [modal, setModal] = useState<null | "checkin" | "cant" | "part" | "refrigerant" | "sign" | "submit">(null);
  const [photoError, setPhotoError] = useState<string | null>(null);
  const win = windowState(job.assignment, now);
  const inWork = job.status === "in_progress";
  const paused = !!job.timeOnSite?.pauses?.some((p) => !p.to);
  const editable = inWork && win.phase !== "ended" && win.phase !== "before";
  const failed = (f: ActionFailure) => setRefused(refusal(f));
  const edit = (fn: (d: Draft) => Draft) => { setDraft(fn); setDirty(true); };
  const photos = (rep ?? live.report)?.attachmentRefs.filter((a) => a.id !== (rep ?? live.report)?.signOff?.signatureAttachmentId) ?? [];
  const issues: Issue[] | null = checked ? submitIssues(draft, photos, now) : null;
  const photoUrl = (a: { id: string }) => {
    const r = rep ?? live.report;
    return r ? `/technician/jobs/${job.id}/photos/${a.id}?reportId=${r.id}&reportVersion=${r.version}` : "";
  };

  const save = (after?: (r: Rep) => void) => run(() => saveDraft(draftInput(job.id, rep?.id ?? null, draft), rep?.version ?? null), "Draft saved", (r) => {
    setRep(r); setDirty(false); setSavedAt(new Date(now).toISOString()); setRefused(null); after?.(r);
  }, failed);
  // autosave 20 s after the last edit while the work window is open (the API keeps every saved version)
  useEffect(() => {
    if (!dirty || !editable || pending) return;
    const t = setTimeout(() => save(), 20_000);
    return () => clearTimeout(t);
  });
  const withSaved = (fn: (r: Rep) => void) => (dirty || !rep ? save(fn) : fn(rep));
  const addPhotos = (files: FileList | null) => {
    if (!files?.length) return;
    const file = files[0];
    if (!["image/jpeg", "image/png"].includes(file.type) || file.size > MAX_BYTES || photos.length >= MAX_PHOTOS) {
      setPhotoError(`“${file.name}” was not added — only JPEG/PNG up to 5 MiB, max ${MAX_PHOTOS} photos. Saved text and photos are kept.`);
      return;
    }
    setPhotoError(null);
    withSaved((r) => {
      const form = new FormData();
      form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); form.set("file", file);
      run(() => uploadPhoto(form), `Photo ${file.name} added — sign-off cleared`, (next) => { setRep(next); edit((d) => ({ ...d, attachmentIds: next.attachmentRefs.map((a) => a.id) })); setDirty(false); }, failed);
    });
  };
  const trySubmit = () => {
    const found = submitIssues(draft, photos, now);
    setChecked(true);
    if (found.length) {
      const first = found.find((i) => i.group);
      if (first?.group) setTab(first.group);
      return;
    }
    setModal("submit");
  };
  const submit = () => withSaved((r) => run(() => submitReport(job.id, job.version, r.version), `Report v${r.version} submitted`, () => setModal(null), failed));

  const head = (
    <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
      <Link href="/technician" className="font-semibold text-primary">← Overview</Link>
      <b className="text-ink">{job.id.slice(0, 8)} · {live.unit?.name ?? "Unit"} — {statusText[job.status] ?? job.status}</b>
      {dirty && <Badge tone="warn">Unsaved changes</Badge>}
    </div>
  );
  const ack = job.assignment?.acknowledgement ?? "pending";
  const groups = progress(draft, issues ?? []);
  const items = draft.items.filter((i) => i.componentGroup === tab);
  const issueOf = (key: string) => issues?.find((x) => x.key === key)?.text;
  const readOnlyResult = (r: Result) => { const x = results.find((o) => o.id === r); return x ? <Badge tone={x.tone}>{x.label}</Badge> : <span className="text-xs text-muted">— no result</span>; };
  const versions = versionRows(
    rep ? { version: rep.version, results: draft.items.filter((i) => i.result !== null).length, photos: photos.filter((a) => a.status === "ready").length, savedAt } : null,
    job.reportRefs, live.report?.reviewHistory ?? [],
  );
  const netKg = draft.refrigerant.reduce((s, r) => s + r.chargedKg - r.recoveredKg, 0);

  return (
    <Page>
      {head}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {job.status === "assigned" && ack === "pending" && <Banner tone="primary" action={<span className="flex gap-2"><Btn size="sm" variant="primary" disabled={pending} onClick={() => run(() => acknowledge(job.id, job.version, "accept", null, null), "Assignment accepted", undefined, failed)}>✓ Accept assignment</Btn><Btn size="sm" disabled={pending} onClick={() => setModal("cant")}>Can’t make this time…</Btn></span>}>New assignment for {win.text}. Accepting tells your coordinator and HQ you will attend; the client then sees your name.</Banner>}
      {job.status === "assigned" && ack === "cant_make" && <Banner tone="warn">You said you can’t make this time{job.assignment?.cantMakeReason ? ` (“${job.assignment.cantMakeReason}”)` : ""}. Your coordinator reassigns the job or asks the client for another time.</Banner>}
      {job.status === "assigned" && ack === "accepted" && <Banner tone={win.phase === "before" ? "primary" : "ok"} action={<Btn size="sm" variant="primary" disabled={pending || win.phase !== "open" && win.phase !== "ending"} onClick={() => setModal("checkin")}>Start job (check in)</Btn>}>{win.phase === "before" ? `Before the work window (${win.text}): read-only, Start is disabled (IR76).` : win.phase === "ended" ? "The work window has ended — ask your coordinator to reschedule." : "On site: Start job opens Check in (location + unit QR); checking in starts the work (IR111)."}</Banner>}
      {job.status === "on_hold" && <Banner tone="warn">On hold by HQ — saving and submitting are blocked (CONFLICT, IR93). Your saved draft is kept; continue when HQ resumes the job.</Banner>}
      {job.status === "submitted" && <Banner tone="ok">Submitted v{job.reportRefs[job.reportRefs.length - 1]?.reportVersion} — read-only, awaiting quality review.</Banner>}
      {job.status === "completed" && <Banner tone="ok">Accepted — the job is completed and the report is visible to the customer.</Banner>}
      {job.status === "rework_requested" && <Banner tone="warn" action={<Btn size="sm" variant="primary" disabled={pending || win.phase === "ended"} onClick={() => run(() => resumeRework(job.id, job.version), "Rework resumed — continue in the new draft", undefined, failed)}>Resume rework</Btn>}>Returned for rework{live.report?.reviewHistory.at(-1)?.reason ? ` — “${live.report.reviewHistory.at(-1)!.reason}”` : ""}. Resume to continue as a new draft version; the returned version stays as it was.</Banner>}
      {inWork && win.phase === "ended" && <Banner tone="crit">The work window ended — unsaved input was discarded and the job can no longer be edited (FORBIDDEN). Ask your coordinator.</Banner>}
      {live.reportRefused && <Banner tone="warn">The report is not readable now ({live.reportRefused.replace(/^errors?\./, "").replace(/_/g, " ")}).</Banner>}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div className="flex min-w-0 flex-col gap-4 xl:col-span-2">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card title={<Tabs value={tab} onChange={setTab} tabs={groups.map((g) => ({ id: g.group, label: `${g.label} (${g.total})${g.issues ? ` · ${g.issues} issue${g.issues === 1 ? "" : "s"}` : ""}` }))} />}>
              {items.length === 0 ? <EmptyState title="No components">This unit’s service scope has no {groupLabel[tab].toLowerCase()} components.</EmptyState> : (
                <ul className="divide-y divide-line">{items.map((it) => (
                  <li key={it.componentKey} className="py-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-[13px] font-semibold">{componentLabel[it.componentKey] ?? it.componentKey}</span>
                      {editable ? (
                        <div className="w-44"><Select aria-label={`Result of ${componentLabel[it.componentKey]}`} value={it.result ?? ""} onChange={(e) => edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, result: (e.target.value || null) as Result } : x)) }))}>
                          <option value="">— Select result</option>{results.map((r) => <option key={r.id} value={r.id}>{r.label}</option>)}
                        </Select></div>
                      ) : readOnlyResult(it.result)}
                    </div>
                    {it.result && it.result !== "normal" && (editable
                      ? <div className="mt-1.5"><Input placeholder={`Reason (required for ${results.find((r) => r.id === it.result)?.label})`} maxLength={1000} value={it.reason} onChange={(e) => edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, reason: e.target.value } : x)) }))} /></div>
                      : it.reason ? <div className="text-xs text-muted">Reason: {it.reason}</div> : null)}
                    {issueOf(it.componentKey) && <p className="mt-1 text-xs text-crit">{issueOf(it.componentKey)?.replace(/^.*› [^ ]+( [a-z]+)* (has|needs)/, (m) => m)}</p>}
                  </li>
                ))}</ul>
              )}
              <p className="mt-2 text-[11px] text-muted">Initial result is always empty — normal is never preselected. Attention, not inspected and not applicable need a reason.</p>
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title="Checklist progress">
                {groups.map((g) => (
                  <div key={g.group} className="mb-2"><div className="mb-1 flex justify-between text-xs"><b>{g.label}</b><span className="text-muted">{g.done} / {g.total}</span></div><UtilBar pct={g.total ? (g.done / g.total) * 100 : 0} tone={g.issues ? "warn" : "primary"} /></div>
                ))}
                <p className="text-[11px] text-muted">{issues?.length ? `${issues.length} issue${issues.length === 1 ? "" : "s"} block submit — see the summary on the right.` : `${draft.items.filter((i) => i.result === null).length} items still empty — each needs a result, or not inspected / not applicable with a reason, before submit.`}</p>
              </Card>
              <Card title="Job & unit" action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${job.unitId}`}>Unit →</Link>}>
                <SummaryList items={[
                  ["Window", <span key="w" className={win.phase === "ending" || win.phase === "ended" ? "font-semibold text-crit" : undefined}>{job.assignment ? `${klTime(job.assignment.scheduledStart).slice(5)} – ${klTime(job.assignment.scheduledEnd).slice(klTime(job.assignment.scheduledEnd).slice(0, 10) === klTime(job.assignment.scheduledStart).slice(0, 10) ? 11 : 5)}` : "—"}</span>],
                  ["Site", live.unit?.place ?? (live.unitRefused ? "opens at the start of your window" : "—")], ["Unit", live.unit ? `${live.unit.name} · ${live.unit.model}` : job.unitId.slice(0, 8)],
                  ["Job", `${job.type} · ${job.origin.replace(/_/g, " ")}`], ["Request", job.symptom ? `“${job.symptom}”` : "—"],
                  ["Linked alerts", job.alertCount ? <Link key="a" className="font-semibold text-crit" href={`/technician/units/${job.unitId}/alerts?jobId=${job.id}`}>{job.alertCount} alert{job.alertCount === 1 ? "" : "s"} →</Link> : "none"],
                ]} />
                {inWork && <div className="mt-3"><Link className="text-xs font-semibold text-primary" href={`/technician/units/${job.unitId}/control?jobId=${job.id}`}>Diagnostic control →</Link></div>}
              </Card>
            </div>
          </div>
          <Card title={<Tabs value={view} onChange={setView} tabs={[{ id: "readings", label: "Readings", count: draft.readings.length }, { id: "parts", label: "Parts & refrigerant", count: draft.parts.length + draft.refrigerant.length }, { id: "time", label: "Time on site" }]} />}
            action={view === "readings" && editable ? <Btn size="sm" onClick={() => edit((d) => ({ ...d, readings: [...d.readings, { componentKey: tab === "outdoor" ? "refrigerant_pipe" : live.components[0]?.key ?? "filter", metric: "temperature", value: "", observedAt: new Date(now).toISOString() }] }))}>+ Add reading</Btn>
              : view === "parts" && editable ? <span className="flex gap-2"><Btn size="sm" onClick={() => setModal("part")}>+ Add part</Btn><Btn size="sm" onClick={() => setModal("refrigerant")}>+ Refrigerant</Btn></span> : null}>
            {view === "readings" && (draft.readings.length === 0 ? <p className="text-[13px] text-muted">No readings yet. Record each measurement with its unit and the component it belongs to — an unmeasured value is left empty, never 0.</p> : (
              <div className="scroll-x"><table className="w-full min-w-[560px] text-[13px]">
                <thead className="text-left text-[11px] uppercase text-muted"><tr><th className="py-1">Metric</th><th>Value</th><th>Unit</th><th>Linked item</th><th>Observed</th><th /></tr></thead>
                <tbody>{draft.readings.map((m, i) => (
                  <tr key={i} className="border-t border-line align-middle">
                    <td className="py-1.5 pr-2">{editable ? <Select value={m.metric} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, metric: e.target.value as Metric } : x)) }))}>{readingMetrics.map((x) => <option key={x.metric} value={x.metric}>{x.label}</option>)}</Select> : readingMetrics.find((x) => x.metric === m.metric)?.label ?? m.metric}</td>
                    <td className="pr-2">{editable ? <Input inputMode="decimal" value={m.value} aria-invalid={m.value.trim() !== "" && !Number.isFinite(Number(m.value))} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, value: e.target.value } : x)) }))} /> : m.value || "—"}</td>
                    <td className="pr-2 text-muted">{metricUnit[m.metric]}</td>
                    <td className="pr-2">{editable ? <Select value={m.componentKey} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, componentKey: e.target.value } : x)) }))}>{live.components.map((c) => <option key={c.key} value={c.key}>{groupLabel[c.group]} › {componentLabel[c.key]}</option>)}</Select> : componentLabel[m.componentKey]}</td>
                    <td className="pr-2 text-xs text-muted">{klTime(m.observedAt).slice(11)}</td>
                    <td>{editable && <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, readings: d.readings.filter((_, k) => k !== i) }))}>Remove</button>}</td>
                  </tr>
                ))}</tbody>
              </table></div>
            ))}
            {view === "parts" && (
              <div className="flex flex-col gap-3">
                {draft.parts.length === 0 ? <p className="text-[13px] text-muted">No parts used.</p> : (
                  <DataTable rowKey={(p) => `${p.name}-${draft.parts.indexOf(p)}`} rows={draft.parts} cols={[
                    { key: "p", label: "Part", render: (p) => <div><b>{p.name}</b><div className="text-[11px] text-muted">{p.catalogCode ?? "custom"}{p.replacesComponentKey ? ` · replaces ${componentLabel[p.replacesComponentKey]}` : ""}</div></div> },
                    { key: "s", label: "Source", render: (p) => sources.find((s) => s.id === p.source)?.label }, { key: "q", label: "Qty", render: (p) => p.quantity },
                    { key: "l", label: "Lot / serial", render: (p) => p.lotSerial ?? "—" },
                    { key: "x", label: "", render: (p) => editable ? <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, parts: d.parts.filter((x) => x !== p) }))}>Remove</button> : null },
                  ]} />
                )}
                {draft.refrigerant.map((r, i) => (
                  <div key={i}>
                    <div className="mb-1 flex items-center justify-between"><b className="text-[13px]">Refrigerant ({r.refrigerant}) — cylinder {r.cylinderId}</b>{editable && <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, refrigerant: d.refrigerant.filter((_, k) => k !== i) }))}>Remove</button>}</div>
                    <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>{[["Recovered", `${r.recoveredKg.toFixed(2)} kg`], ["Charged", `${r.chargedKg.toFixed(2)} kg`], ["Net added", `${r.chargedKg - r.recoveredKg >= 0 ? "+" : "−"}${Math.abs(r.chargedKg - r.recoveredKg).toFixed(2)} kg`], ["Leak check", `${r.leakCheck.replace(/_/g, " ")}${r.leakCheckMethod ? ` · ${r.leakCheckMethod}` : ""}`]].map(([k, v]) => <div key={k} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{k}</div><div className="font-bold">{v}</div></div>)}</div>
                  </div>
                ))}
                <p className="text-[11px] text-muted">Stock is deducted from your van when the report is accepted. Refrigerant amounts are kept per unit for leak-check and regulatory logs.</p>
              </div>
            )}
            {view === "time" && (
              <div>
                <SummaryList items={timeRows(job.timeOnSite, now)} />
                {inWork && <div className="mt-3"><Btn size="sm" disabled={pending || !editable} onClick={() => withSaved(() => run(() => pauseWork(job.id, job.version, !paused), paused ? "Work resumed" : "Work paused", undefined, failed))}>{paused ? "Resume work" : "Pause work"}</Btn></div>}
                <p className="mt-2 text-[11px] text-muted">Arrival inside the window counts toward the SLA. Times are part of the report and visible to the contractor and HQ.</p>
              </div>
            )}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Work report — ${rep ? `draft v${rep.version}` : live.report ? `v${live.report.version}` : "not started"}`}>
            {issues && issues.length > 0 && <Banner tone="crit"><b>Not submitted (VALIDATION) — {issues.length} issue{issues.length === 1 ? "" : "s"}:</b> {issues.slice(0, 6).map((i) => i.text).join(" · ")}{issues.length > 6 ? " · …" : ""}. The draft is kept.</Banner>}
            {editable && win.phase === "ending" && <div className="mt-2"><Banner tone="warn">{win.text}</Banner></div>}
            {!inWork && !live.report && <p className="text-[13px] text-muted">The report opens when you check in at the site.</p>}
            {(inWork || live.report) && (
              <div className="mt-2 flex flex-col gap-3">
                <Field label="Work performed & next action (10–4000 characters)">{editable ? <Textarea value={draft.workText} maxLength={4000} className="min-h-[110px]" onChange={(e) => edit((d) => ({ ...d, workText: e.target.value }))} /> : <p className="font-normal">{draft.workText || "—"}</p>}</Field>
                <p className="text-[13px] font-semibold">Replacement parts: {draft.parts.length} line{draft.parts.length === 1 ? "" : "s"}{draft.refrigerant.length ? ` · ${draft.refrigerant[0].refrigerant} ${netKg >= 0 ? "+" : "−"}${Math.abs(netKg).toFixed(2)} kg` : ""} <button type="button" className="text-xs text-primary" onClick={() => setView("parts")}>(see Parts & refrigerant)</button></p>
                <div>
                  <p className="mb-1 text-xs font-semibold">Photos (JPEG/PNG, ≤5 MiB, ≤{MAX_PHOTOS})</p>
                  <div className="flex flex-wrap gap-2">
                    {photos.map((a) => <Photo key={a.id} src={photoUrl(a)} name={a.name} />)}
                    {editable && photos.length < MAX_PHOTOS && (
                      <label className={cx("grid h-16 w-20 cursor-pointer place-items-center rounded-lg border border-dashed border-line text-xl text-muted hover:bg-surface2", pending && "pointer-events-none opacity-50")}>
                        +<input type="file" accept="image/jpeg,image/png" className="sr-only" onChange={(e) => { addPhotos(e.target.files); e.target.value = ""; }} />
                      </label>
                    )}
                    {!editable && photos.length === 0 && <span className="text-xs text-muted">No photos.</span>}
                  </div>
                  {photoError && <div className="mt-2"><Banner tone="crit">{photoError}</Banner></div>}
                </div>
                <Field label="Next action">
                  {editable ? (
                    <div className="flex flex-col gap-2">
                      <Select value={draft.nextAction.kind} onChange={(e) => edit((d) => ({ ...d, nextAction: e.target.value === "none" ? { kind: "none" } : { kind: "follow_up", date: "", note: "" } }))}><option value="none">None</option><option value="follow_up">Follow-up visit</option></Select>
                      {draft.nextAction.kind === "follow_up" && <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
                        <Input type="date" value={draft.nextAction.date.slice(0, 10)} onChange={(e) => edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: e.target.value ? new Date(`${e.target.value}T09:00:00+08:00`).toISOString() : "", note: d.nextAction.kind === "follow_up" ? d.nextAction.note : "" } }))} />
                        <Input placeholder="What to do (required)" maxLength={1000} value={draft.nextAction.note} onChange={(e) => edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: d.nextAction.kind === "follow_up" ? d.nextAction.date : "", note: e.target.value } }))} />
                      </div>}
                    </div>
                  ) : <p className="font-normal">{draft.nextAction.kind === "none" ? "None" : `Follow-up ${draft.nextAction.date.slice(0, 10)} — ${draft.nextAction.note}`}</p>}
                </Field>
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3">
                  <div><b className="text-[13px]">Customer sign-off</b><div>{(rep ?? live.report)?.signOff ? <Badge tone="ok">Signed · {(rep ?? live.report)!.signOff!.signerName} · {klTime((rep ?? live.report)!.signOff!.signedAt).slice(11)}</Badge> : <Badge tone="warn">Not signed</Badge>}</div></div>
                  {editable && <Btn size="sm" disabled={pending} onClick={() => setModal("sign")}>{(rep ?? live.report)?.signOff ? "Sign again" : "Get signature"}</Btn>}
                </div>
                {editable && <div className="flex flex-wrap justify-end gap-2"><Btn disabled={pending || !dirty && !!rep} onClick={() => save()}>Save draft</Btn><Btn variant="primary" disabled={pending} onClick={trySubmit}>Submit report</Btn></div>}
                <p className="text-[11px] text-muted">Submitted versions become read-only. Rework resumes from rework_requested as a new draft version. Saving again clears the sign-off.</p>
              </div>
            )}
          </Card>
          <Card title="Versions & autosave">
            <ul className="divide-y divide-line">
              {versions.map((v, i) => <li key={i} className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">{v.title}</b><div className="text-[11px] text-muted">{v.sub}</div></div><Badge tone={v.badge.tone}>{v.badge.text}</Badge></li>)}
              <li className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">Work window</b><div className="text-[11px] text-muted">{win.text}</div></div>{win.phase === "ending" && <Badge tone="warn">ending</Badge>}</li>
            </ul>
          </Card>
        </div>
      </div>
      <CheckInModal open={modal === "checkin"} unitId={job.unitId} pending={pending} onClose={() => setModal(null)} onCheckIn={(method, distance, qr, reason) => run(() => checkIn(job.id, job.version, method, distance, qr, reason), "Checked in — the job has started", () => setModal(null), failed)} />
      <CantMakeModal open={modal === "cant"} pending={pending} onClose={() => setModal(null)} onSend={(reason, slot) => run(() => acknowledge(job.id, job.version, "cant_make", reason, slot), "Sent to your coordinator", () => setModal(null), failed)} />
      <PartModal open={modal === "part"} parts={live.parts} components={live.components} onClose={() => setModal(null)} onAdd={(p) => { edit((d) => ({ ...d, parts: [...d.parts, p] })); setModal(null); }} />
      <RefrigerantModal open={modal === "refrigerant"} onClose={() => setModal(null)} onAdd={(r) => { edit((d) => ({ ...d, refrigerant: [...d.refrigerant, r] })); setModal(null); }} />
      <SignModal open={modal === "sign"} pending={pending} summary={`${rep ? `draft v${rep.version}` : "new draft"} · ${draft.items.filter((i) => i.result).length} / ${draft.items.length} results · ${draft.parts.length} parts`} onClose={() => setModal(null)}
        onSign={(form) => withSaved((r) => { form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); run(() => signOff(form), "Sign-off saved for this report version", (next) => { setRep(next); setModal(null); }, failed); })} />
      <Modal open={modal === "submit"} onClose={() => setModal(null)} title={`Submit report v${rep?.version ?? 1}?`} footer={<><Btn onClick={() => setModal(null)}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={submit}>Submit</Btn></>}>
        <p className="text-[13px]">Submitted versions become read-only; the quality review by your company or HQ follows.</p>
        {!(rep ?? live.report)?.signOff && <Banner tone="warn">No customer sign-off yet — the report can still be submitted; the reviewer sees “not signed”.</Banner>}
      </Modal>
    </Page>
  );
}

function CheckInModal({ open, unitId, pending, onClose, onCheckIn }: { open: boolean; unitId: string; pending: boolean; onClose: () => void; onCheckIn: (method: "location_qr" | "manual", distance: number | null, qr: string | null, reason: string | null) => void }) {
  const [method, setMethod] = useState<"location_qr" | "manual">("location_qr");
  const [distance, setDistance] = useState("40");
  const [qr, setQr] = useState(`ac-unit:${unitId}`);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const go = () => {
    if (method === "manual") {
      if (reason.trim().length < 1 || reason.trim().length > 1000) return setError("A reason is required for a manual check-in (1–1000 characters)");
      return onCheckIn("manual", null, null, reason.trim());
    }
    const d = Number(distance);
    if (!Number.isFinite(d) || d < 0 || d > 200) return setError("Location check-in needs you within 200 m of the site — check in manually with a reason otherwise");
    if (!qr.trim()) return setError("Scan the unit’s QR label");
    onCheckIn("location_qr", d, qr.trim(), null);
  };
  return (
    <Modal open={open} onClose={onClose} title="Check in at site" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={go}>Check in & start</Btn></>}>
      <Field label="How"><Choice value={method} onChange={(v) => { setMethod(v); setError(null); }} options={[{ id: "location_qr", label: "Location + unit QR" }, { id: "manual", label: "Manual (reason)" }]} /></Field>
      {method === "location_qr" ? (
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
          <Field label="Distance to the site (m)" hint="Demo: the device's location service fills this; within 200 m"><Input inputMode="numeric" value={distance} onChange={(e) => setDistance(e.target.value)} /></Field>
          <Field label="Scanned unit QR" hint="Demo: the camera scan fills this"><Input value={qr} onChange={(e) => setQr(e.target.value)} /></Field>
        </div>
      ) : <Field label="Reason (required)"><Textarea value={reason} maxLength={1000} placeholder="No GPS signal in the basement plant room" onChange={(e) => setReason(e.target.value)} /></Field>}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted">Arrival must be inside your work window. Checking in records the arrival and starts the work (IR111).</p>
    </Modal>
  );
}

function CantMakeModal({ open, pending, onClose, onSend }: { open: boolean; pending: boolean; onClose: () => void; onSend: (reason: string, slot: { startAt: string; endAt: string } | null) => void }) {
  const [reason, setReason] = useState("");
  const [date, setDate] = useState("");
  const [window, setWindow] = useState("10:00-12:00");
  const [error, setError] = useState<string | null>(null);
  const send = () => {
    if (reason.trim().length < 1) return setError("Tell your coordinator why (1–1000 characters)");
    const [from, to] = window.split("-");
    onSend(reason.trim(), date ? { startAt: new Date(`${date}T${from}:00+08:00`).toISOString(), endAt: new Date(`${date}T${to}:00+08:00`).toISOString() } : null);
  };
  return (
    <Modal open={open} onClose={onClose} title="Can’t make this time" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={send}>Send to coordinator</Btn></>}>
      <Field label="Reason (required)" error={error ?? undefined}><Textarea value={reason} maxLength={1000} placeholder="Medical appointment on that morning" onChange={(e) => setReason(e.target.value)} /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
        <Field label="A time you could do instead (optional)"><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label="Window"><Select value={window} onChange={(e) => setWindow(e.target.value)}>{["08:00-10:00", "10:00-12:00", "12:00-14:00", "14:00-16:00", "16:00-18:00"].map((w) => <option key={w}>{w}</option>)}</Select></Field>
      </div>
      <Banner>Your coordinator reassigns the job or proposes a new time — the client must approve any change.</Banner>
    </Modal>
  );
}

function PartModal({ open, parts, components, onClose, onAdd }: { open: boolean; parts: WorkspaceLive["parts"]; components: WorkspaceLive["components"]; onClose: () => void; onAdd: (p: DraftPart) => void }) {
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [qty, setQty] = useState("1");
  const [source, setSource] = useState<DraftPart["source"]>("van_stock");
  const [lot, setLot] = useState("");
  const [replaces, setReplaces] = useState("");
  const [disposal, setDisposal] = useState<"" | "disposed_on_site" | "returned">("");
  const [error, setError] = useState<string | null>(null);
  const picked = parts.find((p) => p.code === code);
  const add = () => {
    const n = Number(qty), label = (picked?.name ?? name).trim();
    if (!label) return setError("Choose a catalog part or name it");
    if (!Number.isInteger(n) || n < 1 || n > 999) return setError("Quantity must be 1–999");
    onAdd({ name: label, quantity: n, catalogCode: picked?.code ?? null, source, lotSerial: lot.trim() || null, replacesComponentKey: replaces || null, oldPartDisposal: replaces ? (disposal || "disposed_on_site") : null, receiptAttachmentId: null });
    setCode(""); setName(""); setQty("1"); setLot(""); setReplaces(""); setDisposal(""); setError(null);
  };
  return (
    <Modal open={open} onClose={onClose} title="Add part" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={add}>Add</Btn></>}>
      <Field label="Part (catalog)"><Select value={code} onChange={(e) => setCode(e.target.value)}><option value="">Not in the catalog…</option>{parts.map((p) => <option key={p.code} value={p.code}>{p.name} · {p.code}{p.vanStockQuantity !== null ? ` · ${p.vanStockQuantity} in van` : ""}</option>)}</Select></Field>
      {!picked && <Field label="Part name"><Input value={name} maxLength={120} onChange={(e) => setName(e.target.value)} /></Field>}
      <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
        <Field label="Qty"><Input inputMode="numeric" value={qty} onChange={(e) => setQty(e.target.value)} /></Field>
        <Field label="Source"><Select value={source} onChange={(e) => setSource(e.target.value as DraftPart["source"])}>{sources.map((s) => <option key={s.id} value={s.id}>{s.label}</option>)}</Select></Field>
        <Field label="Lot / serial"><Input value={lot} onChange={(e) => setLot(e.target.value)} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label="Replaces checklist item (optional)"><Select value={replaces} onChange={(e) => setReplaces(e.target.value)}><option value="">—</option>{components.map((c) => <option key={c.key} value={c.key}>{groupLabel[c.group]} › {componentLabel[c.key]}</option>)}</Select></Field>
        {replaces && <Field label="Old part"><Select value={disposal} onChange={(e) => setDisposal(e.target.value as typeof disposal)}><option value="">Disposed on site</option><option value="returned">Returned</option></Select></Field>}
      </div>
      {source === "bought_locally" && <p className="text-[11px] text-muted">Keep the receipt — attach a photo of it with the report photos.</p>}
      {error && <Banner tone="crit">{error}</Banner>}
    </Modal>
  );
}

function RefrigerantModal({ open, onClose, onAdd }: { open: boolean; onClose: () => void; onAdd: (r: DraftRefrigerant) => void }) {
  const [kind, setKind] = useState<DraftRefrigerant["refrigerant"]>("R32");
  const [cylinder, setCylinder] = useState("");
  const [recovered, setRecovered] = useState("0");
  const [charged, setCharged] = useState("0");
  const [leak, setLeak] = useState<DraftRefrigerant["leakCheck"]>("pass");
  const [method, setMethod] = useState("electronic detector");
  const [error, setError] = useState<string | null>(null);
  const add = () => {
    const r = Number(recovered), c = Number(charged);
    if (!cylinder.trim()) return setError("The cylinder ID is required");
    if (!Number.isFinite(r) || !Number.isFinite(c) || r < 0 || c < 0) return setError("Amounts are kilograms of 0 or more");
    onAdd({ refrigerant: kind, cylinderId: cylinder.trim(), recoveredKg: r, chargedKg: c, leakCheck: leak, leakCheckMethod: leak === "not_done" ? null : method.trim() || null });
    setError(null);
  };
  return (
    <Modal open={open} onClose={onClose} title="Refrigerant" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={add}>Add</Btn></>}>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label="Refrigerant"><Select value={kind} onChange={(e) => setKind(e.target.value as DraftRefrigerant["refrigerant"])}><option>R32</option><option>R410A</option></Select></Field>
        <Field label="Cylinder ID"><Input value={cylinder} maxLength={64} placeholder="CYL-R32-0182" onChange={(e) => setCylinder(e.target.value)} /></Field>
        <Field label="Recovered (kg)"><Input inputMode="decimal" value={recovered} onChange={(e) => setRecovered(e.target.value)} /></Field>
        <Field label="Charged (kg)"><Input inputMode="decimal" value={charged} onChange={(e) => setCharged(e.target.value)} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label="Leak check"><Select value={leak} onChange={(e) => setLeak(e.target.value as DraftRefrigerant["leakCheck"])}><option value="pass">Pass</option><option value="fail">Fail</option><option value="not_done">Not done</option></Select></Field>
        {leak !== "not_done" && <Field label="Method"><Input value={method} onChange={(e) => setMethod(e.target.value)} /></Field>}
      </div>
      {error && <Banner tone="crit">{error}</Banner>}
    </Modal>
  );
}

function SignModal({ open, pending, summary, onClose, onSign }: { open: boolean; pending: boolean; summary: string; onClose: () => void; onSign: (form: FormData) => void }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const [drawn, setDrawn] = useState(false);
  const [name, setName] = useState("");
  const [absent, setAbsent] = useState(false);
  const [reason, setReason] = useState("");
  const [photo, setPhoto] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const point = (e: React.PointerEvent<HTMLCanvasElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    return [((e.clientX - r.left) / r.width) * e.currentTarget.width, ((e.clientY - r.top) / r.height) * e.currentTarget.height] as const;
  };
  const clear = () => { canvas.current?.getContext("2d")?.clearRect(0, 0, canvas.current.width, canvas.current.height); setDrawn(false); };
  const sign = () => {
    if (!name.trim()) return setError("The signer's name is required");
    const form = new FormData();
    form.set("signerName", name.trim());
    if (absent) {
      if (!reason.trim()) return setError("Say why the customer could not sign");
      if (!photo || !["image/jpeg", "image/png"].includes(photo.type) || photo.size > 5 << 20) return setError("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available");
      form.set("absentReason", reason.trim()); form.set("sitePhoto", photo);
    } else {
      if (!drawn || !canvas.current) return setError("Ask the customer to sign in the box");
      form.set("signature", canvas.current.toDataURL("image/png").split(",")[1]);
    }
    setError(null);
    onSign(form);
  };
  return (
    <Modal open={open} onClose={onClose} title="Customer sign-off" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={sign}>Save sign-off</Btn></>}>
      <p className="text-xs text-muted">Report {summary}</p>
      <Field label="Signer name"><Input value={name} maxLength={120} placeholder="Ms Tan" onChange={(e) => setName(e.target.value)} /></Field>
      <Field label="How"><Choice value={absent ? "absent" : "sign"} onChange={(v) => setAbsent(v === "absent")} options={[{ id: "sign", label: "Signature" }, { id: "absent", label: "Customer not available" }]} /></Field>
      {absent ? (
        <>
          <Field label="Why the customer could not sign"><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field>
          <Field label="Site photo (JPEG/PNG)"><input type="file" accept="image/jpeg,image/png" onChange={(e) => setPhoto(e.target.files?.[0] ?? null)} /></Field>
        </>
      ) : (
        <div>
          <canvas ref={canvas} width={560} height={160} aria-label="Signature pad" className="h-32 w-full touch-none rounded-xl border border-dashed border-line bg-surface"
            onPointerDown={(e) => { drawing.current = true; const c = e.currentTarget.getContext("2d")!; const [x, y] = point(e); c.lineWidth = 2.5; c.lineCap = "round"; c.strokeStyle = "#0f172a"; c.beginPath(); c.moveTo(x, y); e.currentTarget.setPointerCapture(e.pointerId); }}
            onPointerMove={(e) => { if (!drawing.current) return; const c = e.currentTarget.getContext("2d")!; const [x, y] = point(e); c.lineTo(x, y); c.stroke(); setDrawn(true); }}
            onPointerUp={() => { drawing.current = false; }} />
          <div className="mt-1 flex justify-between text-[11px] text-muted"><span>✍ Sign here</span><button type="button" className="font-semibold text-primary" onClick={clear}>Clear</button></div>
        </div>
      )}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted">The sign-off is bound to this report version; saving the draft again clears it.</p>
    </Modal>
  );
}

/** A report photo streamed through the DAL route; a photo whose file is gone (404) or unreadable shows a tile instead. */
function Photo({ src, name }: { src: string; name: string }) {
  const [broken, setBroken] = useState(false);
  if (broken) return <span title={name} className="grid h-16 w-20 place-items-center rounded-lg border border-dashed border-line bg-surface2 px-1 text-center text-[10px] leading-tight text-muted">Photo unavailable</span>;
  // eslint-disable-next-line @next/next/no-img-element -- streamed through the DAL route (attachments.getContent)
  return <img src={src} alt={name} onError={() => setBroken(true)} className="h-16 w-20 rounded-lg bg-surface2 object-cover" />;
}
