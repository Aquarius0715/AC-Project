"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Badge, Banner, Btn, Card, Choice, cx, DataTable, EmptyState, Field, Input, Modal, Page, Select, SummaryList, Tabs, Textarea, UtilBar } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { metricUnit, type Metric } from "@ac/web/lib/devices";
import { zonedInstant, type T } from "@ac/web/lib/i18n";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import {
  componentLabel, draftFrom, draftInput, fmtOf, followUpText, groupLabel, progress, readingMetrics, submitIssues, timeRows, versionRows, windowState,
  type ApiTechReport, type Draft, type DraftPart, type DraftRefrigerant, type Group, type Issue, type Known, type Result, type TimeOnSite,
} from "@ac/web/lib/techJob";
import { acknowledge, checkIn, pauseWork, resumeRework, saveDraft, signOff, submitReport, uploadPhoto } from "../actions";

export type WorkspaceLive = {
  now: string; known: Known; zone: string;
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
const FORBIDDEN: Record<string, string> = {
  "errors.assignment_not_started": "your work window has not started", "errors.assignment_ended": "your work window has ended", "errors.assignment_required": "you are not assigned to this job",
  "errors.qr_unit_mismatch": "the scanned QR label is not this job's unit",
};
const refusal = (f: ActionFailure, t: T): string =>
  f.messageKey === "errors.report_version_changed" || f.messageKey === "error.versionConflict" ? t("CONFLICT — the report or job changed elsewhere; reload the page to continue from the latest version.")
    : f.code === "CONFLICT" ? t("CONFLICT — the job is not in a state that allows this (on hold, submitted or cancelled); the page shows the latest state.")
    : f.code === "FORBIDDEN" ? t("FORBIDDEN — {reason} (outside your work window or assignment).", { reason: FORBIDDEN[f.messageKey] ? t(FORBIDDEN[f.messageKey]) : f.messageKey.replace(/^errors?\./, "").replace(/_/g, " ") })
    : f.code === "UNAVAILABLE" ? t("UNAVAILABLE — not sent; your draft is kept on this page. Try again.") : `${f.code} — ${Object.entries(f.fieldErrors).map(([k, v]) => `${k}: ${v}`).join(", ") || f.messageKey}`;
const LEAK: Record<string, string> = { pass: "passed", fail: "failed", not_done: "not done" };
const MAX_PHOTOS = 10, MAX_BYTES = 5 << 20;

/** The technician's job workspace (FR-T04–T06, T08, T09, T13–T15): assignment, check-in, the inspection checklist of the
 * unit's service scope, readings, parts and refrigerant, photos, the work report with its next action, the customer's
 * sign-off and the submission for quality review. The draft lives on this page until saved (autosave after edits).
 * Texts in the user's display language; the times the page loaded with come formatted from the loader (IR282). */
export function WorkspaceView({ live }: { live: WorkspaceLive }) {
  const { job } = live;
  const i18n = useI18n();
  const { t } = i18n;
  const f = fmtOf(i18n, live.known);
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
  const win = windowState(job.assignment, now, f);
  const inWork = job.status === "in_progress";
  const paused = !!job.timeOnSite?.pauses?.some((p) => !p.to);
  const editable = inWork && win.phase !== "ended" && win.phase !== "before";
  const failed = (x: ActionFailure) => setRefused(refusal(x, t));
  const edit = (fn: (d: Draft) => Draft) => { setDraft(fn); setDirty(true); };
  const photos = (rep ?? live.report)?.attachmentRefs.filter((a) => a.id !== (rep ?? live.report)?.signOff?.signatureAttachmentId) ?? [];
  const issues: Issue[] | null = checked ? submitIssues(draft, photos, now, t) : null;
  const photoUrl = (a: { id: string }) => {
    const r = rep ?? live.report;
    return r ? `/technician/jobs/${job.id}/photos/${a.id}?reportId=${r.id}&reportVersion=${r.version}` : "";
  };

  const save = (after?: (r: Rep) => void) => run(() => saveDraft(draftInput(job.id, rep?.id ?? null, draft), rep?.version ?? null), t("Draft saved"), (r) => {
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
      setPhotoError(t("“{name}” was not added — only JPEG/PNG up to 5 MiB, max {max} photos. Saved text and photos are kept.", { name: file.name, max: MAX_PHOTOS }));
      return;
    }
    setPhotoError(null);
    withSaved((r) => {
      const form = new FormData();
      form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); form.set("file", file);
      run(() => uploadPhoto(form), t("Photo {name} added — sign-off cleared", { name: file.name }), (next) => { setRep(next); edit((d) => ({ ...d, attachmentIds: next.attachmentRefs.map((a) => a.id) })); setDirty(false); }, failed);
    });
  };
  const trySubmit = () => {
    const found = submitIssues(draft, photos, now, t);
    setChecked(true);
    if (found.length) {
      const first = found.find((i) => i.group);
      if (first?.group) setTab(first.group);
      return;
    }
    setModal("submit");
  };
  const submit = () => withSaved((r) => run(() => submitReport(job.id, job.version, r.version), t("Report v{version} submitted", { version: r.version }), () => setModal(null), failed));

  const head = (
    <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
      <Link href="/technician" className="font-semibold text-primary">{t("← Overview")}</Link>
      <b className="text-ink">{job.id.slice(0, 8)} · {live.unit?.name ?? t("Unit")} — {statusText[job.status] ? t(statusText[job.status]) : job.status}</b>
      {dirty && <Badge tone="warn">{t("Unsaved changes")}</Badge>}
    </div>
  );
  const ack = job.assignment?.acknowledgement ?? "pending";
  const groups = progress(draft, issues ?? [], t);
  const items = draft.items.filter((i) => i.componentGroup === tab);
  const issueOf = (key: string) => issues?.find((x) => x.key === key)?.text;
  const readOnlyResult = (r: Result) => { const x = results.find((o) => o.id === r); return x ? <Badge tone={x.tone}>{t(x.label)}</Badge> : <span className="text-xs text-muted">{t("— no result")}</span>; };
  const versions = versionRows(
    rep ? { version: rep.version, results: draft.items.filter((x) => x.result !== null).length, photos: photos.filter((a) => a.status === "ready").length, savedAt } : null,
    job.reportRefs, live.report?.reviewHistory ?? [], f,
  );
  const label = (key: string) => t(componentLabel[key] ?? key);
  const netKg = draft.refrigerant.reduce((s, r) => s + r.chargedKg - r.recoveredKg, 0);

  return (
    <Page>
      {head}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {job.status === "assigned" && ack === "pending" && <Banner tone="primary" action={<span className="flex gap-2"><Btn size="sm" variant="primary" disabled={pending} onClick={() => run(() => acknowledge(job.id, job.version, "accept", null, null), t("Assignment accepted"), undefined, failed)}>{t("✓ Accept assignment")}</Btn><Btn size="sm" disabled={pending} onClick={() => setModal("cant")}>{t("Can’t make this time…")}</Btn></span>}>{t("New assignment for {window}. Accepting tells your coordinator and HQ you will attend; the client then sees your name.", { window: win.text })}</Banner>}
      {job.status === "assigned" && ack === "cant_make" && <Banner tone="warn">{job.assignment?.cantMakeReason ? t("You said you can’t make this time (“{reason}”). Your coordinator reassigns the job or asks the client for another time.", { reason: job.assignment.cantMakeReason }) : t("You said you can’t make this time. Your coordinator reassigns the job or asks the client for another time.")}</Banner>}
      {job.status === "assigned" && ack === "accepted" && <Banner tone={win.phase === "before" ? "primary" : "ok"} action={<Btn size="sm" variant="primary" disabled={pending || win.phase !== "open" && win.phase !== "ending"} onClick={() => setModal("checkin")}>{t("Start job (check in)")}</Btn>}>{win.phase === "before" ? t("Before the work window ({window}): read-only, Start is disabled (IR76).", { window: win.text }) : win.phase === "ended" ? t("The work window has ended — ask your coordinator to reschedule.") : t("On site: Start job opens Check in (location + unit QR); checking in starts the work (IR111).")}</Banner>}
      {job.status === "on_hold" && <Banner tone="warn">{t("On hold by HQ — saving and submitting are blocked (CONFLICT, IR93). Your saved draft is kept; continue when HQ resumes the job.")}</Banner>}
      {job.status === "submitted" && <Banner tone="ok">{t("Submitted v{version} — read-only, awaiting quality review.", { version: job.reportRefs[job.reportRefs.length - 1]?.reportVersion ?? "" })}</Banner>}
      {job.status === "completed" && <Banner tone="ok">{t("Accepted — the job is completed and the report is visible to the customer.")}</Banner>}
      {job.status === "rework_requested" && <Banner tone="warn" action={<Btn size="sm" variant="primary" disabled={pending || win.phase === "ended"} onClick={() => run(() => resumeRework(job.id, job.version), t("Rework resumed — continue in the new draft"), undefined, failed)}>{t("Resume rework")}</Btn>}>{live.report?.reviewHistory.at(-1)?.reason ? t("Returned for rework — “{reason}”. Resume to continue as a new draft version; the returned version stays as it was.", { reason: live.report.reviewHistory.at(-1)!.reason! }) : t("Returned for rework. Resume to continue as a new draft version; the returned version stays as it was.")}</Banner>}
      {inWork && win.phase === "ended" && <Banner tone="crit">{t("The work window ended — unsaved input was discarded and the job can no longer be edited (FORBIDDEN). Ask your coordinator.")}</Banner>}
      {live.reportRefused && <Banner tone="warn">{t("The report is not readable now ({reason}).", { reason: FORBIDDEN[live.reportRefused] ? t(FORBIDDEN[live.reportRefused]) : live.reportRefused.replace(/^errors?\./, "").replace(/_/g, " ") })}</Banner>}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div className="flex min-w-0 flex-col gap-4 xl:col-span-2">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card title={<Tabs value={tab} onChange={setTab} tabs={groups.map((g) => ({ id: g.group, label: `${g.label} (${g.total})${g.issues ? ` · ${t(g.issues === 1 ? "1 issue" : "{n} issues", { n: g.issues })}` : ""}` }))} />}>
              {items.length === 0 ? <EmptyState title={t("No components")}>{t("This unit’s service scope has no components in {group}.", { group: t(groupLabel[tab]) })}</EmptyState> : (
                <ul className="divide-y divide-line">{items.map((it) => (
                  <li key={it.componentKey} className="py-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-[13px] font-semibold">{label(it.componentKey)}</span>
                      {editable ? (
                        <div className="w-44"><Select aria-label={t("Result of {item}", { item: label(it.componentKey) })} value={it.result ?? ""} onChange={(e) => edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, result: (e.target.value || null) as Result } : x)) }))}>
                          <option value="">{t("— Select result")}</option>{results.map((r) => <option key={r.id} value={r.id}>{t(r.label)}</option>)}
                        </Select></div>
                      ) : readOnlyResult(it.result)}
                    </div>
                    {it.result && it.result !== "normal" && (editable
                      ? <div className="mt-1.5"><Input placeholder={t("Reason (required for {result})", { result: t(results.find((r) => r.id === it.result)?.label ?? "") })} maxLength={1000} value={it.reason} onChange={(e) => edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, reason: e.target.value } : x)) }))} /></div>
                      : it.reason ? <div className="text-xs text-muted">{t("Reason: {reason}", { reason: it.reason })}</div> : null)}
                    {issueOf(it.componentKey) && <p className="mt-1 text-xs text-crit">{issueOf(it.componentKey)}</p>}
                  </li>
                ))}</ul>
              )}
              <p className="mt-2 text-[11px] text-muted">{t("Initial result is always empty — normal is never preselected. Attention, not inspected and not applicable need a reason.")}</p>
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <Card title={t("Checklist progress")}>
                {groups.map((g) => (
                  <div key={g.group} className="mb-2"><div className="mb-1 flex justify-between text-xs"><b>{g.label}</b><span className="text-muted">{g.done} / {g.total}</span></div><UtilBar pct={g.total ? (g.done / g.total) * 100 : 0} tone={g.issues ? "warn" : "primary"} /></div>
                ))}
                <p className="text-[11px] text-muted">{issues?.length ? t(issues.length === 1 ? "1 issue blocks submit — see the summary on the right." : "{n} issues block submit — see the summary on the right.", { n: issues.length }) : t("{n} items still empty — each needs a result, or not inspected / not applicable with a reason, before submit.", { n: draft.items.filter((x) => x.result === null).length })}</p>
              </Card>
              <Card title={t("Job & unit")} action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${job.unitId}?jobId=${job.id}`}>{t("Unit →")}</Link>}>
                <SummaryList items={[
                  [t("Window"), <span key="w" className={win.phase === "ending" || win.phase === "ended" ? "font-semibold text-crit" : undefined}>{job.assignment ? f.span(job.assignment.scheduledStart, job.assignment.scheduledEnd) : "—"}</span>],
                  [t("Site"), live.unit?.place ?? (live.unitRefused ? t("opens at the start of your window") : "—")], [t("Unit"), live.unit ? `${live.unit.name} · ${live.unit.model}` : job.unitId.slice(0, 8)],
                  [t("Job"), `${typeLabel(job.type, t)} · ${t(job.origin === "periodic_plan" ? "periodic plan" : "client request")}`], [t("Request"), job.symptom ? `“${job.symptom}”` : "—"],
                  [t("Linked alerts"), job.alertCount ? <Link key="a" className="font-semibold text-crit" href={`/technician/units/${job.unitId}/alerts?jobId=${job.id}`}>{t(job.alertCount === 1 ? "1 alert →" : "{n} alerts →", { n: job.alertCount })}</Link> : t("none")],
                ]} />
                {inWork && <div className="mt-3"><Link className="text-xs font-semibold text-primary" href={`/technician/units/${job.unitId}/control?jobId=${job.id}`}>{t("Diagnostic control →")}</Link></div>}
              </Card>
            </div>
          </div>
          <Card title={<Tabs value={view} onChange={setView} tabs={[{ id: "readings", label: t("Readings"), count: draft.readings.length }, { id: "parts", label: t("Parts & refrigerant"), count: draft.parts.length + draft.refrigerant.length }, { id: "time", label: t("Time on site") }]} />}
            action={view === "readings" && editable ? <Btn size="sm" onClick={() => edit((d) => ({ ...d, readings: [...d.readings, { componentKey: tab === "outdoor" ? "refrigerant_pipe" : live.components[0]?.key ?? "filter", metric: "temperature", value: "", observedAt: new Date(now).toISOString() }] }))}>{t("+ Add reading")}</Btn>
              : view === "parts" && editable ? <span className="flex gap-2"><Btn size="sm" onClick={() => setModal("part")}>{t("+ Add part")}</Btn><Btn size="sm" onClick={() => setModal("refrigerant")}>{t("+ Refrigerant")}</Btn></span> : null}>
            {view === "readings" && (draft.readings.length === 0 ? <p className="text-[13px] text-muted">{t("No readings yet. Record each measurement with its unit and the component it belongs to — an unmeasured value is left empty, never 0.")}</p> : (
              <div className="scroll-x"><table className="w-full min-w-[560px] text-[13px]">
                <thead className="text-left text-[11px] uppercase text-muted"><tr><th className="py-1">{t("Metric")}</th><th>{t("Value")}</th><th>{t("Unit")}</th><th>{t("Linked item")}</th><th>{t("Observed")}</th><th /></tr></thead>
                <tbody>{draft.readings.map((m, i) => (
                  <tr key={i} className="border-t border-line align-middle">
                    <td className="py-1.5 pr-2">{editable ? <Select aria-label={t("Metric")} value={m.metric} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, metric: e.target.value as Metric } : x)) }))}>{readingMetrics.map((x) => <option key={x.metric} value={x.metric}>{t(x.label)}</option>)}</Select> : t(readingMetrics.find((x) => x.metric === m.metric)?.label ?? m.metric)}</td>
                    <td className="pr-2">{editable ? <Input inputMode="decimal" value={m.value} aria-invalid={m.value.trim() !== "" && !Number.isFinite(Number(m.value))} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, value: e.target.value } : x)) }))} /> : m.value || "—"}</td>
                    <td className="pr-2 text-muted">{metricUnit[m.metric]}</td>
                    <td className="pr-2">{editable ? <Select aria-label={t("Linked item")} value={m.componentKey} onChange={(e) => edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, componentKey: e.target.value } : x)) }))}>{live.components.map((c) => <option key={c.key} value={c.key}>{t(groupLabel[c.group])} › {label(c.key)}</option>)}</Select> : label(m.componentKey)}</td>
                    <td className="pr-2 text-xs text-muted">{f.clock(m.observedAt)}</td>
                    <td>{editable && <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, readings: d.readings.filter((_, k) => k !== i) }))}>{t("Remove")}</button>}</td>
                  </tr>
                ))}</tbody>
              </table></div>
            ))}
            {view === "parts" && (
              <div className="flex flex-col gap-3">
                {draft.parts.length === 0 ? <p className="text-[13px] text-muted">{t("No parts used.")}</p> : (
                  <DataTable rowKey={(p) => `${p.name}-${draft.parts.indexOf(p)}`} rows={draft.parts} cols={[
                    { key: "p", label: t("Part"), render: (p) => <div><b>{p.name}</b><div className="text-[11px] text-muted">{p.catalogCode ?? t("custom")}{p.replacesComponentKey ? ` · ${t("replaces {part}", { part: label(p.replacesComponentKey) })}` : ""}</div></div> },
                    { key: "s", label: t("Source"), render: (p) => t(sources.find((x) => x.id === p.source)?.label ?? p.source) }, { key: "q", label: t("Qty"), render: (p) => p.quantity },
                    { key: "l", label: t("Lot / serial"), render: (p) => p.lotSerial ?? "—" },
                    { key: "x", label: "", render: (p) => editable ? <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, parts: d.parts.filter((x) => x !== p) }))}>{t("Remove")}</button> : null },
                  ]} />
                )}
                {draft.refrigerant.map((r, i) => (
                  <div key={i}>
                    <div className="mb-1 flex items-center justify-between"><b className="text-[13px]">{t("Refrigerant ({kind}) — cylinder {id}", { kind: r.refrigerant, id: r.cylinderId })}</b>{editable && <button type="button" className="text-xs font-semibold text-primary" onClick={() => edit((d) => ({ ...d, refrigerant: d.refrigerant.filter((_, k) => k !== i) }))}>{t("Remove")}</button>}</div>
                    <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>{[[t("Recovered"), `${r.recoveredKg.toFixed(2)} kg`], [t("Charged"), `${r.chargedKg.toFixed(2)} kg`], [t("Net added"), `${r.chargedKg - r.recoveredKg >= 0 ? "+" : "−"}${Math.abs(r.chargedKg - r.recoveredKg).toFixed(2)} kg`], [t("Leak check"), `${t(LEAK[r.leakCheck] ?? r.leakCheck)}${r.leakCheckMethod ? ` · ${r.leakCheckMethod}` : ""}`]].map(([k, v]) => <div key={k} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{k}</div><div className="font-bold">{v}</div></div>)}</div>
                  </div>
                ))}
                <p className="text-[11px] text-muted">{t("Stock is deducted from your van when the report is accepted. Refrigerant amounts are kept per unit for leak-check and regulatory logs.")}</p>
              </div>
            )}
            {view === "time" && (
              <div>
                <SummaryList items={timeRows(job.timeOnSite, now, f)} />
                {inWork && <div className="mt-3"><Btn size="sm" disabled={pending || !editable} onClick={() => withSaved(() => run(() => pauseWork(job.id, job.version, !paused), t(paused ? "Work resumed" : "Work paused"), undefined, failed))}>{t(paused ? "Resume work" : "Pause work")}</Btn></div>}
                <p className="mt-2 text-[11px] text-muted">{t("Arrival inside the window counts toward the SLA. Times are part of the report and visible to the contractor and HQ.")}</p>
              </div>
            )}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Work report — {version}", { version: rep ? t("draft v{version}", { version: rep.version }) : live.report ? `v${live.report.version}` : t("not started") })}>
            {issues && issues.length > 0 && <Banner tone="crit"><b>{t(issues.length === 1 ? "Not submitted (VALIDATION) — 1 issue:" : "Not submitted (VALIDATION) — {n} issues:", { n: issues.length })}</b> {issues.slice(0, 6).map((x) => x.text).join(" · ")}{issues.length > 6 ? " · …" : ""}. {t("The draft is kept.")}</Banner>}
            {editable && win.phase === "ending" && <div className="mt-2"><Banner tone="warn">{win.text}</Banner></div>}
            {!inWork && !live.report && <p className="text-[13px] text-muted">{t("The report opens when you check in at the site.")}</p>}
            {(inWork || live.report) && (
              <div className="mt-2 flex flex-col gap-3">
                <Field label={t("Work performed & next action (10–4000 characters)")}>{editable ? <Textarea value={draft.workText} maxLength={4000} className="min-h-[110px]" onChange={(e) => edit((d) => ({ ...d, workText: e.target.value }))} /> : <p className="font-normal">{draft.workText || "—"}</p>}</Field>
                <p className="text-[13px] font-semibold">{t(draft.parts.length === 1 ? "Replacement parts: 1 line" : "Replacement parts: {n} lines", { n: draft.parts.length })}{draft.refrigerant.length ? ` · ${draft.refrigerant[0].refrigerant} ${netKg >= 0 ? "+" : "−"}${Math.abs(netKg).toFixed(2)} kg` : ""} <button type="button" className="text-xs text-primary" onClick={() => setView("parts")}>{t("(see Parts & refrigerant)")}</button></p>
                <div>
                  <p className="mb-1 text-xs font-semibold">{t("Photos (JPEG/PNG, ≤5 MiB, ≤{max})", { max: MAX_PHOTOS })}</p>
                  <div className="flex flex-wrap gap-2">
                    {photos.map((a) => <Photo key={a.id} src={photoUrl(a)} name={a.name} />)}
                    {editable && photos.length < MAX_PHOTOS && (
                      <label className={cx("grid h-16 w-20 cursor-pointer place-items-center rounded-lg border border-dashed border-line text-xl text-muted hover:bg-surface2", pending && "pointer-events-none opacity-50")}>
                        +<input type="file" accept="image/jpeg,image/png" className="sr-only" onChange={(e) => { addPhotos(e.target.files); e.target.value = ""; }} />
                      </label>
                    )}
                    {!editable && photos.length === 0 && <span className="text-xs text-muted">{t("No photos.")}</span>}
                  </div>
                  {photoError && <div className="mt-2"><Banner tone="crit">{photoError}</Banner></div>}
                </div>
                <Field label={t("Next action")}>
                  {editable ? (
                    <div className="flex flex-col gap-2">
                      <Select value={draft.nextAction.kind} onChange={(e) => edit((d) => ({ ...d, nextAction: e.target.value === "none" ? { kind: "none" } : { kind: "follow_up", date: "", note: "" } }))}><option value="none">{t("None")}</option><option value="follow_up">{t("Follow-up visit")}</option></Select>
                      {draft.nextAction.kind === "follow_up" && <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
                        <Input type="date" value={draft.nextAction.date.slice(0, 10)} onChange={(e) => edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: e.target.value ? new Date(`${e.target.value}T09:00:00+08:00`).toISOString() : "", note: d.nextAction.kind === "follow_up" ? d.nextAction.note : "" } }))} />
                        <Input placeholder={t("What to do (required)")} maxLength={1000} value={draft.nextAction.note} onChange={(e) => edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: d.nextAction.kind === "follow_up" ? d.nextAction.date : "", note: e.target.value } }))} />
                      </div>}
                    </div>
                  ) : <p className="font-normal">{followUpText(draft.nextAction, i18n)}</p>}
                </Field>
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3">
                  <div><b className="text-[13px]">{t("Customer sign-off")}</b><div>{(rep ?? live.report)?.signOff ? <Badge tone="ok">{t("Signed · {name} · {time}", { name: (rep ?? live.report)!.signOff!.signerName, time: f.clock((rep ?? live.report)!.signOff!.signedAt) })}</Badge> : <Badge tone="warn">{t("Not signed")}</Badge>}</div></div>
                  {editable && <Btn size="sm" disabled={pending} onClick={() => setModal("sign")}>{t((rep ?? live.report)?.signOff ? "Sign again" : "Get signature")}</Btn>}
                </div>
                {editable && <div className="flex flex-wrap justify-end gap-2"><Btn disabled={pending || !dirty && !!rep} onClick={() => save()}>{t("Save draft")}</Btn><Btn variant="primary" disabled={pending} onClick={trySubmit}>{t("Submit report")}</Btn></div>}
                <p className="text-[11px] text-muted">{t("Submitted versions become read-only. Rework resumes from rework_requested as a new draft version. Saving again clears the sign-off.")}</p>
              </div>
            )}
          </Card>
          <Card title={t("Versions & autosave")}>
            <ul className="divide-y divide-line">
              {versions.map((v, k) => <li key={k} className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">{v.title}</b><div className="text-[11px] text-muted">{v.sub}</div></div><Badge tone={v.badge.tone}>{v.badge.text}</Badge></li>)}
              <li className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">{t("Work window")}</b><div className="text-[11px] text-muted">{win.text}</div></div>{win.phase === "ending" && <Badge tone="warn">{t("ending")}</Badge>}</li>
            </ul>
          </Card>
        </div>
      </div>
      <CheckInModal open={modal === "checkin"} unitId={job.unitId} pending={pending} onClose={() => setModal(null)} onCheckIn={(method, distance, qr, reason) => run(() => checkIn(job.id, job.version, method, distance, qr, reason), t("Checked in — the job has started"), () => setModal(null), failed)} />
      <CantMakeModal open={modal === "cant"} pending={pending} zone={live.zone} onClose={() => setModal(null)} onSend={(reason, slot) => run(() => acknowledge(job.id, job.version, "cant_make", reason, slot), t("Sent to your coordinator"), () => setModal(null), failed)} />
      <PartModal open={modal === "part"} parts={live.parts} components={live.components} onClose={() => setModal(null)} onAdd={(p) => { edit((d) => ({ ...d, parts: [...d.parts, p] })); setModal(null); }} />
      <RefrigerantModal open={modal === "refrigerant"} onClose={() => setModal(null)} onAdd={(r) => { edit((d) => ({ ...d, refrigerant: [...d.refrigerant, r] })); setModal(null); }} />
      <SignModal open={modal === "sign"} pending={pending} summary={t("{version} · {done} / {total} results · {parts} parts", { version: rep ? t("draft v{version}", { version: rep.version }) : t("new draft"), done: draft.items.filter((x) => x.result).length, total: draft.items.length, parts: draft.parts.length })} onClose={() => setModal(null)}
        onSign={(form) => withSaved((r) => { form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); run(() => signOff(form), t("Sign-off saved for this report version"), (next) => { setRep(next); setModal(null); }, failed); })} />
      <Modal open={modal === "submit"} onClose={() => setModal(null)} title={t("Submit report v{version}?", { version: rep?.version ?? 1 })} footer={<><Btn onClick={() => setModal(null)}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={submit}>{t("Submit")}</Btn></>}>
        <p className="text-[13px]">{t("Submitted versions become read-only; the quality review by your company or HQ follows.")}</p>
        {!(rep ?? live.report)?.signOff && <Banner tone="warn">{t("No customer sign-off yet — the report can still be submitted; the reviewer sees “not signed”.")}</Banner>}
      </Modal>
    </Page>
  );
}

function CheckInModal({ open, unitId, pending, onClose, onCheckIn }: { open: boolean; unitId: string; pending: boolean; onClose: () => void; onCheckIn: (method: "location_qr" | "manual", distance: number | null, qr: string | null, reason: string | null) => void }) {
  const t = useT();
  const [method, setMethod] = useState<"location_qr" | "manual">("location_qr");
  const [distance, setDistance] = useState("40");
  const [qr, setQr] = useState(`ac-unit:${unitId}`);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const go = () => {
    if (method === "manual") {
      if (reason.trim().length < 1 || reason.trim().length > 1000) return setError(t("A reason is required for a manual check-in (1–1000 characters)"));
      return onCheckIn("manual", null, null, reason.trim());
    }
    const d = Number(distance);
    if (!Number.isFinite(d) || d < 0 || d > 200) return setError(t("Location check-in needs you within 200 m of the site — check in manually with a reason otherwise"));
    if (!qr.trim()) return setError(t("Scan the unit’s QR label"));
    onCheckIn("location_qr", d, qr.trim(), null);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Check in at site")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={go}>{t("Check in & start")}</Btn></>}>
      <Field label={t("How")}><Choice value={method} onChange={(v) => { setMethod(v); setError(null); }} options={[{ id: "location_qr", label: t("Location + unit QR") }, { id: "manual", label: t("Manual (reason)") }]} /></Field>
      {method === "location_qr" ? (
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
          <Field label={t("Distance to the site (m)")} hint={t("Demo: the device's location service fills this; within 200 m")}><Input inputMode="numeric" value={distance} onChange={(e) => setDistance(e.target.value)} /></Field>
          <Field label={t("Scanned unit QR")} hint={t("Demo: the camera scan fills this")}><Input value={qr} onChange={(e) => setQr(e.target.value)} /></Field>
        </div>
      ) : <Field label={t("Reason (required)")}><Textarea value={reason} maxLength={1000} placeholder={t("No GPS signal in the basement plant room")} onChange={(e) => setReason(e.target.value)} /></Field>}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted">{t("Arrival must be inside your work window. Checking in records the arrival and starts the work (IR111).")}</p>
    </Modal>
  );
}

function CantMakeModal({ open, pending, zone, onClose, onSend }: { open: boolean; pending: boolean; zone: string; onClose: () => void; onSend: (reason: string, slot: { startAt: string; endAt: string } | null) => void }) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [date, setDate] = useState("");
  const [window, setWindow] = useState("10:00-12:00");
  const [error, setError] = useState<string | null>(null);
  const send = () => {
    if (reason.trim().length < 1) return setError(t("Tell your coordinator why (1–1000 characters)"));
    const [from, to] = window.split("-");
    onSend(reason.trim(), date ? { startAt: zonedInstant(date, from, zone), endAt: zonedInstant(date, to, zone) } : null); // typed in the display time zone (NFR-08)
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Can’t make this time")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t("Send to coordinator")}</Btn></>}>
      <Field label={t("Reason (required)")} error={error ?? undefined}><Textarea value={reason} maxLength={1000} placeholder={t("Medical appointment on that morning")} onChange={(e) => setReason(e.target.value)} /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
        <Field label={t("A time you could do instead (optional)")} hint={t("Times in {zone}.", { zone })}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label={t("Window")}><Select value={window} onChange={(e) => setWindow(e.target.value)}>{["08:00-10:00", "10:00-12:00", "12:00-14:00", "14:00-16:00", "16:00-18:00"].map((w) => <option key={w}>{w}</option>)}</Select></Field>
      </div>
      <Banner>{t("Your coordinator reassigns the job or proposes a new time — the client must approve any change.")}</Banner>
    </Modal>
  );
}

function PartModal({ open, parts, components, onClose, onAdd }: { open: boolean; parts: WorkspaceLive["parts"]; components: WorkspaceLive["components"]; onClose: () => void; onAdd: (p: DraftPart) => void }) {
  const t = useT();
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
    if (!label) return setError(t("Choose a catalog part or name it"));
    if (!Number.isInteger(n) || n < 1 || n > 999) return setError(t("Quantity must be 1–999"));
    onAdd({ name: label, quantity: n, catalogCode: picked?.code ?? null, source, lotSerial: lot.trim() || null, replacesComponentKey: replaces || null, oldPartDisposal: replaces ? (disposal || "disposed_on_site") : null, receiptAttachmentId: null });
    setCode(""); setName(""); setQty("1"); setLot(""); setReplaces(""); setDisposal(""); setError(null);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Add part")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" onClick={add}>{t("Add")}</Btn></>}>
      <Field label={t("Part (catalog)")}><Select value={code} onChange={(e) => setCode(e.target.value)}><option value="">{t("Not in the catalog…")}</option>{parts.map((p) => <option key={p.code} value={p.code}>{p.name} · {p.code}{p.vanStockQuantity !== null ? ` · ${t("{n} in van", { n: p.vanStockQuantity })}` : ""}</option>)}</Select></Field>
      {!picked && <Field label={t("Part name")}><Input value={name} maxLength={120} onChange={(e) => setName(e.target.value)} /></Field>}
      <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
        <Field label={t("Qty")}><Input inputMode="numeric" value={qty} onChange={(e) => setQty(e.target.value)} /></Field>
        <Field label={t("Source")}><Select value={source} onChange={(e) => setSource(e.target.value as DraftPart["source"])}>{sources.map((x) => <option key={x.id} value={x.id}>{t(x.label)}</option>)}</Select></Field>
        <Field label={t("Lot / serial")}><Input value={lot} onChange={(e) => setLot(e.target.value)} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Replaces checklist item (optional)")}><Select value={replaces} onChange={(e) => setReplaces(e.target.value)}><option value="">—</option>{components.map((c) => <option key={c.key} value={c.key}>{t(groupLabel[c.group])} › {t(componentLabel[c.key] ?? c.key)}</option>)}</Select></Field>
        {replaces && <Field label={t("Old part")}><Select value={disposal} onChange={(e) => setDisposal(e.target.value as typeof disposal)}><option value="">{t("Disposed on site")}</option><option value="returned">{t("Returned")}</option></Select></Field>}
      </div>
      {source === "bought_locally" && <p className="text-[11px] text-muted">{t("Keep the receipt — attach a photo of it with the report photos.")}</p>}
      {error && <Banner tone="crit">{error}</Banner>}
    </Modal>
  );
}

function RefrigerantModal({ open, onClose, onAdd }: { open: boolean; onClose: () => void; onAdd: (r: DraftRefrigerant) => void }) {
  const t = useT();
  const [kind, setKind] = useState<DraftRefrigerant["refrigerant"]>("R32");
  const [cylinder, setCylinder] = useState("");
  const [recovered, setRecovered] = useState("0");
  const [charged, setCharged] = useState("0");
  const [leak, setLeak] = useState<DraftRefrigerant["leakCheck"]>("pass");
  const [method, setMethod] = useState("electronic detector");
  const [error, setError] = useState<string | null>(null);
  const add = () => {
    const r = Number(recovered), c = Number(charged);
    if (!cylinder.trim()) return setError(t("The cylinder ID is required"));
    if (!Number.isFinite(r) || !Number.isFinite(c) || r < 0 || c < 0) return setError(t("Amounts are kilograms of 0 or more"));
    onAdd({ refrigerant: kind, cylinderId: cylinder.trim(), recoveredKg: r, chargedKg: c, leakCheck: leak, leakCheckMethod: leak === "not_done" ? null : method.trim() || null });
    setError(null);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Refrigerant")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" onClick={add}>{t("Add")}</Btn></>}>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label={t("Refrigerant")}><Select value={kind} onChange={(e) => setKind(e.target.value as DraftRefrigerant["refrigerant"])}><option>R32</option><option>R410A</option></Select></Field>
        <Field label={t("Cylinder ID")}><Input value={cylinder} maxLength={64} placeholder="CYL-R32-0182" onChange={(e) => setCylinder(e.target.value)} /></Field>
        <Field label={t("Recovered (kg)")}><Input inputMode="decimal" value={recovered} onChange={(e) => setRecovered(e.target.value)} /></Field>
        <Field label={t("Charged (kg)")}><Input inputMode="decimal" value={charged} onChange={(e) => setCharged(e.target.value)} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Leak check")}><Select value={leak} onChange={(e) => setLeak(e.target.value as DraftRefrigerant["leakCheck"])}><option value="pass">{t("Pass")}</option><option value="fail">{t("Fail")}</option><option value="not_done">{t("Not done")}</option></Select></Field>
        {leak !== "not_done" && <Field label={t("Method")}><Input value={method} onChange={(e) => setMethod(e.target.value)} /></Field>}
      </div>
      {error && <Banner tone="crit">{error}</Banner>}
    </Modal>
  );
}

function SignModal({ open, pending, summary, onClose, onSign }: { open: boolean; pending: boolean; summary: string; onClose: () => void; onSign: (form: FormData) => void }) {
  const t = useT();
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
    if (!name.trim()) return setError(t("The signer's name is required"));
    const form = new FormData();
    form.set("signerName", name.trim());
    if (absent) {
      if (!reason.trim()) return setError(t("Say why the customer could not sign"));
      if (!photo || !["image/jpeg", "image/png"].includes(photo.type) || photo.size > 5 << 20) return setError(t("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available"));
      form.set("absentReason", reason.trim()); form.set("sitePhoto", photo);
    } else {
      if (!drawn || !canvas.current) return setError(t("Ask the customer to sign in the box"));
      form.set("signature", canvas.current.toDataURL("image/png").split(",")[1]);
    }
    setError(null);
    onSign(form);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Customer sign-off")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={sign}>{t("Save sign-off")}</Btn></>}>
      <p className="text-xs text-muted">{t("Report {summary}", { summary })}</p>
      <Field label={t("Signer name")}><Input value={name} maxLength={120} placeholder="Ms Tan" onChange={(e) => setName(e.target.value)} /></Field>
      <Field label={t("How")}><Choice value={absent ? "absent" : "sign"} onChange={(v) => setAbsent(v === "absent")} options={[{ id: "sign", label: t("Signature") }, { id: "absent", label: t("Customer not available") }]} /></Field>
      {absent ? (
        <>
          <Field label={t("Why the customer could not sign")}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field>
          <Field label={t("Site photo (JPEG/PNG)")}><input type="file" accept="image/jpeg,image/png" onChange={(e) => setPhoto(e.target.files?.[0] ?? null)} /></Field>
        </>
      ) : (
        <div>
          <canvas ref={canvas} width={560} height={160} aria-label={t("Signature pad")} className="h-32 w-full touch-none rounded-xl border border-dashed border-line bg-surface"
            onPointerDown={(e) => { drawing.current = true; const c = e.currentTarget.getContext("2d")!; const [x, y] = point(e); c.lineWidth = 2.5; c.lineCap = "round"; c.strokeStyle = "#0f172a"; c.beginPath(); c.moveTo(x, y); e.currentTarget.setPointerCapture(e.pointerId); }}
            onPointerMove={(e) => { if (!drawing.current) return; const c = e.currentTarget.getContext("2d")!; const [x, y] = point(e); c.lineTo(x, y); c.stroke(); setDrawn(true); }}
            onPointerUp={() => { drawing.current = false; }} />
          <div className="mt-1 flex justify-between text-[11px] text-muted"><span>{t("✍ Sign here")}</span><button type="button" className="font-semibold text-primary" onClick={clear}>{t("Clear")}</button></div>
        </div>
      )}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted">{t("The sign-off is bound to this report version; saving the draft again clears it.")}</p>
    </Modal>
  );
}

/** A report photo streamed through the DAL route; a photo whose file is gone (404) or unreadable shows a tile instead. */
function Photo({ src, name }: { src: string; name: string }) {
  const t = useT();
  const [broken, setBroken] = useState(false);
  if (broken) return <span title={name} className="grid h-16 w-20 place-items-center rounded-lg border border-dashed border-line bg-surface2 px-1 text-center text-[10px] leading-tight text-muted">{t("Photo unavailable")}</span>;
  // eslint-disable-next-line @next/next/no-img-element -- streamed through the DAL route (attachments.getContent)
  return <img src={src} alt={name} onError={() => setBroken(true)} className="h-16 w-20 rounded-lg bg-surface2 object-cover" />;
}
