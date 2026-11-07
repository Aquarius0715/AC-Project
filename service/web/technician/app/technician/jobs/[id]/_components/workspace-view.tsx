"use client";
const IN_PROGRESS = "in_progress";
const REFRIGERANT = "Refrigerant";
const LOCATION_QR = "location_qr";

import Link from "next/link";
import { useEffect, useRef, useState, useEffectEvent } from "react";
import { Badge, Banner, Btn, Card, Choice, cx, DataTable, EmptyState, Field, Input, Modal, Page, Select, SummaryList, Tabs, Textarea, UtilizationBar, Plain, ScrollX } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { focusField } from "@ac/web/lib/focusField";
import { metricUnit } from "@ac/web/lib/devices";
import { oneOf } from "@ac/web/lib/oneOf";
import { byCount, zonedInstant, type T, textOf } from "@ac/web/lib/i18n";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import { actionMessage, fieldErrorTexts, type ActionFailure } from "@ac/web/lib/actionMessage";
import {
  CHECK_IN_API_FIELDS, CHECK_IN_FIELDS, checkInErrors, checkResults, componentLabel, PART_FIELDS, partErrors, REFRIGERANT_FIELDS, refrigerantErrors, draftFrom, draftInput, fmtOf, followUpText, groupLabel, itemPhotos, linkPhoto, progress, readingMetrics, submitIssues, timeRows, versionRows, windowState,
  type ApiTechReport, type CheckInField, type Draft, type DraftPart, type DraftRefrigerant, type Group, type Issue, type Known, type Result, type TimeOnSite,
} from "@ac/web/lib/techJob";
import { acknowledge, checkIn, pauseWork, resumeRework, saveDraft, signOff, submitReport, uploadPhoto } from "../actions";
import { Icon } from "@ac/web/components/Icon";
import type { ComponentKey } from "@ac/web/lib/contracts.gen";
import { without } from "@ac/web/lib/omit";
import { lookup } from "@ac/web/lib/lookup";

export interface WorkspaceLive {
  now: string; known: Known; zone: string;
  job: {
    id: string; version: number; status: string; type: string; unitId: string; symptom: string; alertCount: number; origin: string;
    assignment: { scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: "pending" | "accepted" | "cant_make"; cantMakeReason: string | null } | null;
    timeOnSite: TimeOnSite | null; draftReportRef: { reportId: string; reportVersion: number } | null; reportRefs: { reportId: string; reportVersion: number }[];
    problemPhotos: { id: string; name: string }[];
  };
  unit: { name: string; place: string; access: string | null; model: string; scope: string[] } | null; unitRefused: string | null;
  unitName: string | null; // also before the window, when the unit itself is not open yet
  components: { group: Group; key: ComponentKey }[];
  report: ApiTechReport | null; reportRefused: string | null;
  parts: { code: string; name: string; vanStockQuantity: number | null }[];
}

const results: { id: NonNullable<Result>; label: string; tone: "ok" | "warn" | "muted" | "crit" }[] = [
  { id: "normal", label: "Normal", tone: "ok" }, { id: "attention", label: "Attention", tone: "warn" }, { id: "not_inspected", label: "Not inspected", tone: "crit" }, { id: "not_applicable", label: "Not applicable", tone: "muted" },
];
const statusText: Record<string, string> = { assigned: "assigned", in_progress: "in progress", on_hold: "on hold", submitted: "submitted", rework_requested: "returned for rework", completed: "completed", cancelled: "cancelled" };
const metricIds = readingMetrics.map((r) => r.metric);
const sources: { id: DraftPart["source"]; label: string }[] = [{ id: "van_stock", label: "Van stock" }, { id: "hq_warehouse", label: "HQ warehouse" }, { id: "bought_locally", label: "Bought locally" }];
const FORBIDDEN: Record<string, string> = {
  "errors.assignment_not_started": "your work window has not started", "errors.assignment_ended": "your work window has ended", "errors.assignment_required": "you are not assigned to this job",
};
const refusal = (f: ActionFailure, t: T): string => {
  if (f.messageKey === "errors.report_version_changed" || f.messageKey === "error.versionConflict") return t("The report or job changed elsewhere; reload the page to continue from the latest version.");
  if (f.code === "CONFLICT") return t("The job is not in a state that allows this (on hold, submitted or cancelled); the page shows the latest state.");
  if (f.code === "FORBIDDEN") return t("Not allowed: {reason}.", { reason: textOf(FORBIDDEN, f.messageKey, t) ?? f.messageKey.replace(/^errors?\./, "").replaceAll('_', " ") });
  if (f.code === "UNAVAILABLE") return t("Not sent; your draft is kept on this page. Try again.");
  return actionMessage(f, t);
}; // field errors, the write limit's wait and other codes (IR380)
const LEAK: Record<string, string> = { pass: "passed", fail: "failed", not_done: "not done" };
const MAX_PHOTOS = 10, MAX_BYTES = 5 << 20;

/** The technician's job workspace (FR-T04–T06, T08, T09, T13–T15): assignment, check-in, the inspection checklist of the
unit's service scope, readings, parts and refrigerant, photos, the work report with its next action, the customer's
sign-off and the submission for quality review. The draft lives on this page until saved (autosave after edits).
Texts in the user's display language; the times the page loaded with come formatted from the loader (IR282). */
type WindowInfo = ReturnType<typeof windowState>;

/** The checklist progress per group and what still blocks submitting. */
function ChecklistProgressCard({ groups, issues, isEditable, emptyCount }: Readonly<{ groups: ReturnType<typeof progress>; issues: Issue[] | null; isEditable: boolean; emptyCount: number }>) {
  const { t } = useI18n();
  return (
      <Card title={t("Checklist progress")}>
        {groups.map((g) => (
            <div key={g.group} className="mb-2"><div className="mb-1 flex justify-between text-xs"><b>{g.label}</b><span className="text-muted-foreground">{g.done} / {g.total}</span></div><UtilizationBar pct={g.total ? (g.done / g.total) * 100 : 0} tone={g.issues ? "warn" : "primary"}  label={`${g.label} ${String(g.done)} / ${String(g.total)}`} /></div>
          ))}
        {isEditable && <p className="text-[11px] text-muted-foreground">{issues?.length ? byCount(issues.length, { one: t("1 issue blocks submit — it is listed above Submit report."), many: t("{n} issues block submit — they are listed above Submit report.", { n: issues.length }) }) : t("{n} items still empty — each needs a result, or not inspected / not applicable with a reason, before submit.", { n: emptyCount })}</p>}
      </Card>
  );
}

/** The job and its unit: window, site, access, request, photos and links to the unit's pages. */
function JobUnitCard({ live, win, isUnitOpen }: Readonly<{ live: WorkspaceLive; win: WindowInfo; isUnitOpen: boolean }>) {
  const i18n = useI18n();
  const { t } = i18n;
  const { job } = live;
  const f = fmtOf(i18n, live.known);
  return (
      <Card title={t("Job & unit")} action={<Link className="text-xs font-semibold text-primary" href={`/technician/units/${job.unitId}?jobId=${job.id}`}>{t("Unit")} <Icon name="arrowRight" /></Link>}>
        <SummaryList items={[
          [t("Window"), <span key="w" className={win.phase === "ending" || win.phase === "ended" ? "font-semibold text-crit" : undefined}>{job.assignment ? f.span(job.assignment.scheduledStart, job.assignment.scheduledEnd) : "—"}</span>],
          [t("Site"), live.unit?.place ?? (live.unitRefused ? t("opens at the start of your window") : "—")], [t("Access"), live.unit?.access ?? "—"], [t("Unit"), live.unit ? `${live.unit.name} · ${live.unit.model}` : live.unitName ?? job.unitId.slice(0, 8)],
          [t("Job"), `${typeLabel(job.type, t)} · ${t(job.origin === "periodic_plan" ? "periodic plan" : "client request")}`], [t("Request"), job.symptom ? `“${job.symptom}”` : "—"],
          ...(job.problemPhotos.length > 0 ? [[t("Client’s photos"), <span key="p" className="flex flex-wrap gap-x-3">{job.problemPhotos.map((p) => <a key={p.id} href={`/technician/jobs/${job.id}/problem-photos/${p.id}`} target="_blank" rel="noreferrer" className="font-semibold text-primary">{p.name}</a>)}</span>] as [string, React.ReactNode]] : []),
          [t("Linked alerts"), job.alertCount ? <Link key="a" className="font-semibold text-crit" href={`/technician/units/${job.unitId}/alerts?jobId=${job.id}`}>{t(job.alertCount === 1 ? "1 alert" : "{n} alerts", { n: job.alertCount })} <Icon name="arrowRight" /></Link> : t("none")],
        ]} />
        {isUnitOpen && (
          <div className="mt-3 flex flex-wrap gap-3 text-xs font-semibold">
            <Link className="text-primary" href={`/technician/units/${job.unitId}/alerts?jobId=${job.id}`}>{t("Alert evidence")} <Icon name="arrowRight" /></Link>
            <Link className="text-primary" href={`/technician/units/${job.unitId}/control?jobId=${job.id}`}>{t("Diagnostics")} <Icon name="arrowRight" /></Link>
          </div>
        )}
      </Card>
  );
}

/** The saved report versions and the work window. */
function VersionsCard({ versions, win }: Readonly<{ versions: ReturnType<typeof versionRows>; win: WindowInfo }>) {
  const { t } = useI18n();
  return (
      <Card title={t("Versions & autosave")}>
        <ul className="divide-y divide-line">
          {versions.map((v, k) => <li key={k} className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">{v.title}</b><div className="text-[11px] text-muted-foreground">{v.sub}</div></div><Badge tone={v.badge.tone}>{v.badge.text}</Badge></li>)}
          <li className="flex items-start justify-between gap-2 py-2"><div><b className="text-[13px]">{t("Work window")}</b><div className="text-[11px] text-muted-foreground">{win.text}</div></div>{win.phase === "ending" && <Badge tone="warn">{t("ending")}</Badge>}</li>
        </ul>
      </Card>
  );
}

type EditDraft = (fn: (d: Draft) => Draft) => void;

/** The measurements of the report, one editable row each while the report is editable. */
function ReadingsTable({ draft, edit, isEditable, components, f }: Readonly<{ draft: Draft; edit: EditDraft; isEditable: boolean; components: WorkspaceLive["components"]; f: ReturnType<typeof fmtOf> }>) {
  const { t } = useI18n();
  const label = (key: string) => t(componentLabel[key] ?? key);
  const componentKeys = components.map((c) => c.key);
  return (
draft.readings.length === 0 ? <p className="text-[13px] text-muted-foreground">{t("No readings yet. Record each measurement with its unit and the component it belongs to — an unmeasured value is left empty, never 0.")}</p> : (
        <ScrollX><table className="w-full min-w-[560px] text-[13px]">
          <thead className="text-left text-[11px] uppercase text-muted-foreground"><tr><th className="py-1">{t("Metric")}</th><th>{t("Value")}</th><th>{t("Unit")}</th><th>{t("Linked item")}</th><th>{t("Observed")}</th><th /></tr></thead>
          <tbody>{draft.readings.map((m, i) => (
            <tr key={i} className="border-t border-line align-middle">
              <td className="py-1.5 pr-2">{isEditable ? <Select aria-label={t("Metric")} value={m.metric} onChange={(e) => { edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, metric: oneOf(metricIds, e.target.value, x.metric) } : x)) })); }}>{readingMetrics.map((x) => <option key={x.metric} value={x.metric}>{t(x.label)}</option>)}</Select> : t(readingMetrics.find((x) => x.metric === m.metric)?.label ?? m.metric)}</td>
              <td className="pr-2">{isEditable ? <Input inputMode="decimal" value={m.value} aria-invalid={m.value.trim() !== "" && !Number.isFinite(Number(m.value))} onChange={(e) => { edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, value: e.target.value } : x)) })); }} /> : m.value || "—"}</td>
              <td className="pr-2 text-muted-foreground">{metricUnit[m.metric]}</td>
              <td className="pr-2">{isEditable ? <Select aria-label={t("Linked item")} value={m.componentKey} onChange={(e) => { edit((d) => ({ ...d, readings: d.readings.map((x, k) => (k === i ? { ...x, componentKey: oneOf(componentKeys, e.target.value, x.componentKey) } : x)) })); }}>{components.map((c) => <option key={c.key} value={c.key}>{t(groupLabel[c.group])} › {label(c.key)}</option>)}</Select> : label(m.componentKey)}</td>
              <td className="pr-2 text-xs text-muted-foreground">{f.clock(m.observedAt)}</td>
              <td>{isEditable && <Plain type="button" className="text-xs font-semibold text-primary" onClick={() => { edit((d) => ({ ...d, readings: d.readings.filter((_, k) => k !== i) })); }}>{t("Remove")}</Plain>}</td>
            </tr>
          ))}</tbody>
        </table></ScrollX>
      )
  );
}

/** The parts used and the refrigerant handled. */
function PartsView({ draft, edit, isEditable }: Readonly<{ draft: Draft; edit: EditDraft; isEditable: boolean }>) {
  const { t } = useI18n();
  const label = (key: string) => t(componentLabel[key] ?? key);
  return (
        <div className="flex flex-col gap-3">
          {draft.parts.length === 0 ? <p className="text-[13px] text-muted-foreground">{t("No parts used.")}</p> : (
            <DataTable rowKey={(p) => `${p.name}-${String(draft.parts.indexOf(p))}`} rows={draft.parts} cols={[
              { key: "p", label: t("Part"), render: (p) => <div><b>{p.name}</b><div className="text-[11px] text-muted-foreground">{p.catalogCode ?? t("custom")}{p.replacesComponentKey ? ` · ${t("replaces {part}", { part: label(p.replacesComponentKey) })}` : ""}</div></div> },
              { key: "s", label: t("Source"), render: (p) => t(sources.find((x) => x.id === p.source)?.label ?? p.source) }, { key: "q", label: t("Qty"), render: (p) => p.quantity },
              { key: "l", label: t("Lot / serial"), render: (p) => p.lotSerial ?? "—" },
              { key: "x", label: "", render: (p) => isEditable ? <Plain type="button" className="text-xs font-semibold text-primary" onClick={() => { edit((d) => ({ ...d, parts: d.parts.filter((x) => x !== p) })); }}>{t("Remove")}</Plain> : null },
            ]} />
          )}
          {draft.refrigerant.map((r, i) => (
            <div key={i}>
              <div className="mb-1 flex items-center justify-between"><b className="text-[13px]">{t("Refrigerant ({kind}) — cylinder {id}", { kind: r.refrigerant, id: r.cylinderId })}</b>{isEditable && <Plain type="button" className="text-xs font-semibold text-primary" onClick={() => { edit((d) => ({ ...d, refrigerant: d.refrigerant.filter((_, k) => k !== i) })); }}>{t("Remove")}</Plain>}</div>
              <div className="grid-fluid" style={{ ["--min" as string]: "120px" }}>{[[t("Recovered"), `${r.recoveredKg.toFixed(2)} kg`], [t("Charged"), `${r.chargedKg.toFixed(2)} kg`], [t("Net added"), `${r.chargedKg >= r.recoveredKg ? "+" : "−"}${Math.abs(r.chargedKg - r.recoveredKg).toFixed(2)} kg`], [t("Leak check"), `${t(LEAK[r.leakCheck] ?? r.leakCheck)}${r.leakCheckMethod ? ` · ${r.leakCheckMethod}` : ""}`]].map(([k, v]) => <div key={k} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted-foreground">{k}</div><div className="font-bold">{v}</div></div>)}</div>
            </div>
          ))}
          <p className="text-[11px] text-muted-foreground">{t("Stock is deducted from your van when the report is accepted. Refrigerant amounts are kept per unit for leak-check and regulatory logs.")}</p>
        </div>
  );
}

interface WorkReportProps {
  versionText: string; issues: Issue[] | null; isEditable: boolean; notice: string | null | undefined; isInWork: boolean; hasReport: boolean; hasDraftReport: boolean;
  draft: Draft; edit: EditDraft; photos: ApiTechReport["attachmentRefs"]; photoUrl: (a: { id: string }) => string; photoError: string | null; addPhotos: (files: FileList | null) => void;
  pending: boolean; dirty: boolean; signature: ApiTechReport["signOff"] | undefined; f: ReturnType<typeof fmtOf>;
  onViewParts: () => void; onSign: () => void; onSave: () => void; onSubmit: () => void;
}

/** The work report card: the text, the parts summary, the photos, the next action, the sign-off and the buttons. */
function WorkReportCard({ versionText, issues, isEditable, notice, isInWork, hasReport, hasDraftReport, draft, edit, photos, photoUrl, photoError, addPhotos, pending, dirty, signature, f, onViewParts, onSign, onSave, onSubmit }: Readonly<WorkReportProps>) {
  const i18n = useI18n();
  const { t } = i18n;
  const netKg = draft.refrigerant.reduce((sum, r) => sum + r.chargedKg - r.recoveredKg, 0);
  const netSign = netKg >= 0 ? "+" : "−";
  return (
        <Card title={t("Work report — {version}", { version: versionText })}>
          {issues && issues.length > 0 && <Banner tone="crit"><b>{t(issues.length === 1 ? "Not submitted — 1 issue:" : "Not submitted — {n} issues:", { n: issues.length })}</b> {issues.slice(0, 6).map((x) => x.text).join(" · ")}{issues.length > 6 ? " · …" : ""}. {t("The draft is kept.")}</Banner>}
          {isEditable && notice && <div className="mt-2"><Banner tone="warn">{notice}</Banner></div>}
          {!isInWork && !hasReport && <p className="text-[13px] text-muted-foreground">{t("The report opens when you check in at the site.")}</p>}
          {(isInWork || hasReport) && (
            <div className="mt-2 flex flex-col gap-3">
              <Field label={t("Work performed & next action (10–4000 characters)")}>{isEditable ? <Textarea value={draft.workText} maxLength={4000} className="min-h-[110px]" onChange={(e) => { edit((d) => ({ ...d, workText: e.target.value })); }} /> : <p className="font-normal">{draft.workText || "—"}</p>}</Field>
              <p className="text-[13px] font-semibold">{t(draft.parts.length === 1 ? "Replacement parts: 1 line" : "Replacement parts: {n} lines", { n: draft.parts.length })}{draft.refrigerant.length > 0 ? ` · ${draft.refrigerant[0]?.refrigerant ?? ""} ${netSign}${Math.abs(netKg).toFixed(2)} kg` : ""} <Plain type="button" className="text-xs text-primary" onClick={onViewParts}>{t("(see Parts & refrigerant)")}</Plain></p>
              <div>
                <p className="mb-1 text-xs font-semibold">{t("Photos (JPEG/PNG, ≤5 MiB, ≤{max})", { max: MAX_PHOTOS })}</p>
                <div className="flex flex-wrap gap-2">
                  {photos.map((a) => <Photo key={a.id} src={photoUrl(a)} name={a.name} />)}
                  {isEditable && photos.length < MAX_PHOTOS && (
                    <label className={cx("grid h-16 w-20 cursor-pointer place-items-center rounded-lg border border-dashed border-line text-xl text-muted-foreground hover:bg-surface2 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-primary", pending && "pointer-events-none opacity-50")}>
                      +<Input type="file" aria-label={t("Add photos")} accept="image/jpeg,image/png" className="sr-only h-px w-px" onChange={(e) => { addPhotos(e.target.files); e.target.value = ""; }} />
                    </label>
                  )}
                  {!isEditable && photos.length === 0 && <span className="text-xs text-muted-foreground">{t("No photos.")}</span>}
                </div>
                {photoError && <div className="mt-2"><Banner tone="crit">{photoError}</Banner></div>}
              </div>
              <Field label={t("Next action")}>
                {isEditable ? (
                  <div className="flex flex-col gap-2">
                    <Select aria-label={t("Next action")} value={draft.nextAction.kind} onChange={(e) => { edit((d) => ({ ...d, nextAction: e.target.value === "none" ? { kind: "none" } : { kind: "follow_up", date: "", note: "" } })); }}><option value="none">{t("None")}</option><option value="follow_up">{t("Follow-up visit")}</option></Select>
                    {draft.nextAction.kind === "follow_up" && <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
                      <Input type="date" aria-label={t("Follow-up date")} value={draft.nextAction.date.slice(0, 10)} onChange={(e) => { edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: e.target.value ? new Date(`${e.target.value}T09:00:00+08:00`).toISOString() : "", note: d.nextAction.kind === "follow_up" ? d.nextAction.note : "" } })); }} />
                      <Input aria-label={t("What to do (required)")} placeholder={t("What to do (required)")} maxLength={1000} value={draft.nextAction.note} onChange={(e) => { edit((d) => ({ ...d, nextAction: { kind: "follow_up", date: d.nextAction.kind === "follow_up" ? d.nextAction.date : "", note: e.target.value } })); }} />
                    </div>}
                  </div>
                ) : <p className="font-normal">{followUpText(draft.nextAction, i18n)}</p>}
              </Field>
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl bg-surface2 p-3">
                <div><b className="text-[13px]">{t("Customer sign-off")}</b><div>{signature ? <Badge tone="ok">{t("Signed · {name} · {time}", { name: signature.signerName, time: f.clock(signature.signedAt) })}</Badge> : <Badge tone="warn">{t("Not signed")}</Badge>}</div></div>
                {isEditable && <Btn size="sm" disabled={pending} onClick={onSign}>{t(signature ? "Sign again" : "Get signature")}</Btn>}
              </div>
              {isEditable && <div className="flex flex-wrap justify-end gap-2"><Btn disabled={pending || !dirty && hasDraftReport} onClick={onSave}>{t("Save draft")}</Btn><Btn variant="primary" disabled={pending} onClick={onSubmit}>{t("Submit report")}</Btn></div>}
              <p className="text-[11px] text-muted-foreground">{t("Submitted versions become read-only. A report returned for rework continues as a new draft version. Saving again clears the sign-off.")}</p>
            </div>
          )}
        </Card>
  );
}

export function WorkspaceView({ live }: Readonly<{ live: WorkspaceLive }>) {
  type Rep = Pick<ApiTechReport, "id" | "version" | "attachmentRefs" | "signOff">;
  const { job } = live;
  const i18n = useI18n();
  const { t } = i18n;
  const f = fmtOf(i18n, live.known);
  const [pending, run] = useAction();
  // the business clock of the server (demo clock), advanced locally for the IR89 warning
  const [now, setNow] = useState(() => Date.parse(live.now));
  useEffect(() => {
    const offset = Date.parse(live.now) - Date.now();
    const t = setInterval(() => { setNow(Date.now() + offset); }, 30_000);
    return () => { clearInterval(t); };
  }, [live.now]);
  const editableReport = job.draftReportRef && live.report?.version === job.draftReportRef.reportVersion ? live.report : null;
  const [draft, setDraft] = useState<Draft>(() => draftFrom(editableReport ?? (job.status === IN_PROGRESS ? null : live.report), live.components));
  const [rep, setRep] = useState<Rep | null>(editableReport);
  const [dirty, setDirty] = useState(false);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  // validated on the first submit attempt, then re-checked live so fixed rows and counts clear as the draft changes
  const [checked, setChecked] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  // the open checklist group and view outlive the remount a new job version causes (IR625)
  const [tab, setTabState] = useState<Group>(openTab.get(live.job.id) ?? live.components[0]?.group ?? "indoor");
  const [view, setViewState] = useState<View>(openView.get(live.job.id) ?? "readings");
  const setTab = (g: Group) => { openTab.set(live.job.id, g); setTabState(g); };
  const setView = (v: View) => { openView.set(live.job.id, v); setViewState(v); };
  const [modal, setModal] = useState<null | "checkin" | "cant" | "part" | "refrigerant" | "sign" | "submit">(null);
  const [checkInError, setCheckInError] = useState<CheckInErrors>({});
  // an action that moves the job on mounts the view again for the new version: the focus goes where it said (IR621, IR625)
  useEffect(() => {
    if (remountFocus?.jobId !== live.job.id) return;
    const id = remountFocus.id;
    remountFocus = null;
    document.getElementById(id)?.focus();
  }, [live.job.id]);
  const [photoError, setPhotoError] = useState<string | null>(null);
  const [linking, setLinking] = useState<string | null>(null); // the component whose “+ Link photo” list is open
  const win = windowState(job.assignment, now, f);
  // the visit and its start alone: the banners name the time once (UX-03); the Work window line keeps the full state
  const visit = job.assignment ? f.span(job.assignment.scheduledStart, job.assignment.scheduledEnd) : "";
  const opens = job.assignment ? f.stamp(job.assignment.scheduledStart) : "";
  // the accepted assignment's banner: before the window, after it, or on site
  const acceptedText = (): string => {
    if (win.phase === "before") return t("Read-only until the window opens at {time} — Start is disabled before then.", { time: opens });
    if (win.phase === "ended") return t("The work window has ended — ask your coordinator to reschedule.");
    return t("On site: Start job opens Check in (location + unit QR); checking in starts the work.");
  };
  const isInWork = job.status === IN_PROGRESS;
  const isPaused = !!job.timeOnSite?.pauses?.some((p) => !p.to);
  const isEditable = isInWork && win.phase !== "ended" && win.phase !== "before";
  // the unit's alert evidence and diagnostics open for an active job inside its window: in progress, assigned or
  // returned for rework (IR94, IR283; Figma Technician 423:604)
  const isUnitOpen = ["assigned", IN_PROGRESS, "rework_requested"].includes(job.status) && win.phase !== "ended" && win.phase !== "before";
  const failed = (x: ActionFailure) => { setRefused(refusal(x, t)); };
  const edit = (fn: (d: Draft) => Draft) => { setDraft(fn); setDirty(true); };
  const photos = (rep ?? live.report)?.attachmentRefs.filter((a) => a.id !== (rep ?? live.report)?.signOff?.signatureAttachmentId) ?? [];
  const issues: Issue[] | null = checked ? submitIssues(draft, photos, now, t) : null;
  const photoUrl = (a: { id: string }) => {
    const r = rep ?? live.report;
    return r ? `/technician/jobs/${job.id}/photos/${a.id}?reportId=${r.id}&reportVersion=${String(r.version)}` : "";
  };

  const save = (after?: (r: Rep) => void) => {
    run(() => saveDraft(draftInput(job.id, rep?.id ?? null, draft), rep?.version ?? null), t("Draft saved"), (r) => {
      setRep(r); setDirty(false); setSavedAt(new Date(now).toISOString()); setRefused(null); after?.(r);
    }, failed);
  };
  // autosave 20 s after the last edit while the work window is open (the API keeps every saved version)
  // the timer restarts with each edit, not with every render; the save it runs reads the latest draft (useEffectEvent)
  const autosave = useEffectEvent(() => { save(); });
  useEffect(() => {
    const t = dirty && isEditable && !pending ? setTimeout(() => { autosave(); }, 20_000) : null;
    return () => { if (t) clearTimeout(t); };
  }, [draft, dirty, isEditable, pending]);
  const withSaved = (fn: (r: Rep) => void) => { if (dirty || !rep) save(fn); else fn(rep); };
  const photoAdded = (next: Rep) => { setRep(next); edit((d) => ({ ...d, attachmentIds: next.attachmentRefs.map((a) => a.id) })); setDirty(false); };
  const addPhotos = (files: FileList | null) => {
    const file = files?.[0];
    if (!file) return;
    if (!["image/jpeg", "image/png"].includes(file.type) || file.size > MAX_BYTES || photos.length >= MAX_PHOTOS) {
      setPhotoError(t("“{name}” was not added — only JPEG/PNG up to 5 MiB, max {max} photos. Saved text and photos are kept.", { name: file.name, max: MAX_PHOTOS }));
      return;
    }
    setPhotoError(null);
    withSaved((r) => {
      const form = new FormData();
      form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); form.set("file", file);
      run(() => uploadPhoto(form), t("Photo {name} added — sign-off cleared", { name: file.name }), photoAdded, failed);
    });
  };
  const trySubmit = () => {
    const found = submitIssues(draft, photos, now, t);
    setChecked(true);
    if (found.length > 0) {
      const first = found.find((i) => i.group);
      if (first?.group) setTab(first.group);
      // focus the first item to fix once its tab shows it, so a phone does not have to scroll up to it (IR594)
      if (first?.key) focusField(`result-${first.key}`);
      return;
    }
    setModal("submit");
  };
  const submit = () => { withSaved((r) => { run(() => submitReport(job.id, job.version, r.version), t("Report v{version} submitted", { version: r.version }), () => { setModal(null); }, failed); }); };

  const head = (
    <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground">
      <Link href="/technician" className="font-semibold text-primary"><Icon name="arrowLeft" /> {t("Overview")}</Link>
      <h2 id="job-title" tabIndex={-1} className="rounded font-bold text-ink outline-none focus-visible:ring-2 focus-visible:ring-ring/50">{job.id.slice(0, 8)} · {live.unitName ?? t("Unit")} — {textOf(statusText, job.status, t) ?? job.status}</h2>
      {dirty && <Badge tone="warn">{t("Unsaved changes")}</Badge>}
    </div>
  );
  const ack = job.assignment?.acknowledgement ?? "pending";
  const groups = progress(draft, issues ?? [], t);
  const items = draft.items.filter((i) => i.componentGroup === tab);
  const issueOf = (key: string) => issues?.find((x) => x.key === key)?.text;
  const readOnlyResult = (r: Result) => { const x = results.find((o) => o.id === r); return x ? <Badge tone={x.tone}>{t(x.label)}</Badge> : <span className="text-xs text-muted-foreground">{t("— no result")}</span>; };
  const versions = versionRows(
    rep ? { version: rep.version, results: draft.items.filter((x) => x.result !== null).length, photos: photos.filter((a) => a.status === "ready").length, savedAt } : null,
    job.reportRefs, live.report?.reviewHistory ?? [], f,
  );
  const label = (key: string) => t(componentLabel[key] ?? key);
  // the report version: the editable draft's, else the last report's
  const versionText = (): string => {
    if (rep) return t("draft v{version}", { version: rep.version });
    if (live.report) return `v${String(live.report.version)}`;
    return t("not started");
  };
  // the add buttons of the readings and parts tabs while the report is editable
  const viewAction = () => {
    if (!isEditable) return null;
    if (view === "readings") return <Btn size="sm" onClick={() => { addReading(tab === "outdoor" ? "refrigerant_pipe" : live.components[0]?.key ?? "filter"); }}><Icon name="add" /> {t("Add reading")}</Btn>;
    if (view === "parts") return <span className="flex gap-2"><Btn size="sm" onClick={() => { setModal("part"); }}><Icon name="add" /> {t("Add part")}</Btn><Btn size="sm" onClick={() => { setModal("refrigerant"); }}><Icon name="add" /> {t(REFRIGERANT)}</Btn></span>;
    return null;
  };
  const addReading = (key: ComponentKey) => { edit((d) => ({ ...d, readings: [...d.readings, { componentKey: key, metric: "temperature", value: "", observedAt: new Date(now).toISOString() }] })); };
  // a finding's own readings and linked report photos, with “+ Add reading” and “+ Link photo” on a finding that needs
  // attention or already has evidence (Figma Technician 423:204, DD-T04–T06 step 1, IR352)
  const evidence = (it: Draft["items"][number]) => {
    const own = draft.readings.filter((m) => m.componentKey === it.componentKey);
    const ph = itemPhotos(draft, it.componentKey, photos);
    const isActions = isEditable && (it.result === "attention" || own.length > 0 || ph.linked.length > 0);
    if (!isActions && own.length === 0 && ph.linked.length === 0) return null;
    const name = label(it.componentKey);
    return (
      <div className="mt-1.5 flex flex-col gap-1.5">
        {own.map((m, k) => (
          <div key={k} className="flex flex-wrap items-baseline gap-x-2 rounded-lg bg-primary-soft/60 px-2.5 py-1.5 text-xs">
            <b>{t("Reading")}</b><span>{t(readingMetrics.find((x) => x.metric === m.metric)?.label ?? m.metric)} · {m.value.trim() || "—"} {metricUnit[m.metric]}</span><span className="text-muted-foreground">{t("observed {time}", { time: f.clock(m.observedAt) })}</span>
          </div>
        ))}
        {ph.linked.length > 0 && (
          <div className="flex flex-wrap gap-1.5">{ph.linked.map((x) => (
            <span key={x.id} className="inline-flex items-center gap-1 rounded-full bg-surface2 px-2 py-0.5 text-xs">
              <Icon name="camera" /> {x.status === "missing" ? x.name : <a href={photoUrl(x)} target="_blank" rel="noreferrer" className="font-semibold text-primary">{x.name}</a>}
              {isEditable && <Plain type="button" aria-label={t("Unlink {photo} from {item}", { photo: x.name, item: name })} className="text-muted-foreground hover:text-ink" onClick={() => { edit((d) => linkPhoto(d, it.componentKey, x.id, false)); }}><Icon name="close" /></Plain>}
            </span>
          ))}</div>
        )}
        {isActions && (
          <div className="flex flex-wrap gap-2">
            <Btn size="sm" onClick={() => { addReading(it.componentKey); setView("readings"); }}><Icon name="add" /> {t("Add reading")}</Btn>
            <Btn size="sm" disabled={ph.free.length === 0} aria-expanded={linking === it.componentKey} onClick={() => { setLinking((k) => (k === it.componentKey ? null : it.componentKey)); }}><Icon name="add" /> {t("Link photo")}</Btn>
          </div>
        )}
        {isActions && photos.every((x) => x.status !== "ready") && <p className="text-[11px] text-muted-foreground">{t("Add a photo under Photos first, then link it to this finding.")}</p>}
        {isActions && linking === it.componentKey && ph.free.length > 0 && (
          <div role="group" aria-label={t("Report photos to link to {item}", { item: name })} className="flex flex-wrap gap-1.5 rounded-lg border border-line p-2">
            {ph.free.map((x) => <Btn key={x.id} size="sm" variant="ghost" onClick={() => { edit((d) => linkPhoto(d, it.componentKey, x.id, true)); setLinking(null); }}><Icon name="camera" />{x.name}</Btn>)}
          </div>
        )}
      </div>
    );
  };

  const returnReason = live.report?.reviewHistory.at(-1)?.reason; // HQ's reason for the last return, if it gave one
  const signature = (rep ?? live.report)?.signOff;
  return (
    <Page>
      {head}
      {refused && <Banner tone="crit">{refused}</Banner>}
      {job.status === "assigned" && ack === "pending" && <Banner tone="primary" action={<span className="flex gap-2"><Btn size="sm" variant="primary" disabled={pending} onClick={() => { run(() => acknowledge(job.id, job.version, "accept", null, null), t("Assignment accepted"), () => { remountFocus = { jobId: job.id, id: "job-title" }; }, failed); }}><Icon name="check" />{t("Accept assignment")}</Btn><Btn size="sm" disabled={pending} onClick={() => { setModal("cant"); }}>{t("Can’t make this time…")}</Btn></span>}>{t("New assignment for {window}. Accepting tells your coordinator and HQ you will attend; the client then sees your name.", { window: visit })}</Banner>}
      {job.status === "assigned" && ack === "cant_make" && <Banner tone="warn">{job.assignment?.cantMakeReason ? t("You said you can’t make this time (“{reason}”). Your coordinator reassigns the job or asks the client for another time.", { reason: job.assignment.cantMakeReason }) : t("You said you can’t make this time. Your coordinator reassigns the job or asks the client for another time.")}</Banner>}
      {job.status === "assigned" && ack === "accepted" && <Banner tone={win.phase === "before" ? "primary" : "ok"} action={<Btn size="sm" variant="primary" disabled={pending || win.phase !== "open" && win.phase !== "ending"} onClick={() => { setCheckInError({}); setModal("checkin"); }}>{t("Start job (check in)")}</Btn>}>{acceptedText()}</Banner>}
      {job.status === "on_hold" && <Banner tone="warn">{t("On hold by HQ — saving and submitting are blocked. Your saved draft is kept; continue when HQ resumes the job.")}</Banner>}
      {job.status === "submitted" && <Banner tone="ok">{t("Submitted v{version} — read-only, awaiting quality review.", { version: job.reportRefs.at(-1)?.reportVersion ?? "" })}</Banner>}
      {job.status === "completed" && <Banner tone="ok">{t("Accepted — the job is completed and the report is visible to the customer.")}</Banner>}
      {job.status === "rework_requested" && <Banner tone="warn" action={<Btn size="sm" variant="primary" disabled={pending || win.phase === "ended"} onClick={() => { run(() => resumeRework(job.id, job.version), t("Rework resumed — continue in the new draft"), () => { remountFocus = { jobId: job.id, id: "job-title" }; }, failed); }}>{t("Resume rework")}</Btn>}>{returnReason ? t("Returned for rework — “{reason}”. Resume to continue as a new draft version; the returned version stays as it was.", { reason: returnReason }) : t("Returned for rework. Resume to continue as a new draft version; the returned version stays as it was.")}</Banner>}
      {isInWork && win.phase === "ended" && <Banner tone="crit">{t("The work window ended — unsaved input was discarded and the job can no longer be edited. Ask your coordinator.")}</Banner>}
      {live.reportRefused && <Banner tone="warn">{t("The report is not readable now ({reason}).", { reason: textOf(FORBIDDEN, live.reportRefused, t) ?? live.reportRefused.replace(/^errors?\./, "").replaceAll('_', " ") })}</Banner>}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div className="flex min-w-0 flex-col gap-4 xl:col-span-2">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card title={<Tabs value={tab} onChange={setTab} tabs={groups.map((g) => ({ id: g.group, label: `${g.label} (${String(g.total)})${g.issues ? ` · ${byCount(g.issues, { one: t("1 issue"), many: t("{n} issues", { n: g.issues }) })}` : ""}` }))} />}>
              {items.length === 0 ? <EmptyState title={t("No components")}>{t("This unit’s service scope has no components in {group}.", { group: t(groupLabel[tab]) })}</EmptyState> : (
                <ul className="divide-y divide-line">{items.map((it) => (
                  <li key={it.componentKey} className="py-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-[13px] font-semibold">{label(it.componentKey)}</span>
                      {isEditable ? (
                        <div className="w-44"><Select id={`result-${it.componentKey}`} aria-label={t("Result of {item}", { item: label(it.componentKey) })} value={it.result ?? ""} onChange={(e) => { edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, result: oneOf(checkResults, e.target.value, null) } : x)) })); }}>
                          <option value="">{t("— Select result")}</option>{results.map((r) => <option key={r.id} value={r.id}>{t(r.label)}</option>)}
                        </Select></div>
                      ) : readOnlyResult(it.result)}
                    </div>
                    {isEditable && it.result && it.result !== "normal" ? <div className="mt-1.5"><Input placeholder={t("Reason (required for {result})", { result: t(results.find((r) => r.id === it.result)?.label ?? "") })} maxLength={1000} value={it.reason} onChange={(e) => { edit((d) => ({ ...d, items: d.items.map((x) => (x.componentKey === it.componentKey ? { ...x, reason: e.target.value } : x)) })); }} /></div> : null}
                    {!isEditable && it.result && it.result !== "normal" && it.reason ? <div className="text-xs text-muted-foreground">{t("Reason: {reason}", { reason: it.reason })}</div> : null}
                    {evidence(it)}
                    {issueOf(it.componentKey) && <p className="mt-1 text-xs text-crit">{issueOf(it.componentKey)}</p>}
                  </li>
                ))}</ul>
              )}
              <p className="mt-2 text-[11px] text-muted-foreground">{t("Initial result is always empty — normal is never preselected. Attention, not inspected and not applicable need a reason.")}</p>
            </Card>
            <div className="flex min-w-0 flex-col gap-4">
              <ChecklistProgressCard groups={groups} issues={issues} isEditable={isEditable} emptyCount={draft.items.filter((x) => x.result === null).length} />
              <JobUnitCard live={live} win={win} isUnitOpen={isUnitOpen} />
            </div>
          </div>
          <Card title={<Tabs value={view} onChange={setView} tabs={[{ id: "readings", label: t("Readings"), count: draft.readings.length }, { id: "parts", label: t("Parts & refrigerant"), count: draft.parts.length + draft.refrigerant.length }, { id: "time", label: t("Time on site") }]} />}
            action={viewAction()}>
            {view === "readings" && <ReadingsTable draft={draft} edit={edit} isEditable={isEditable} components={live.components} f={f} />}
            {view === "parts" && <PartsView draft={draft} edit={edit} isEditable={isEditable} />}
            {view === "time" && (
              <div>
                <SummaryList items={timeRows(job.timeOnSite, now, f)} />
                {isInWork && <div className="mt-3"><Btn id="pause-toggle" size="sm" disabled={pending || !isEditable} onClick={() => { withSaved(() => { run(() => pauseWork(job.id, job.version, !isPaused), t(isPaused ? "Work resumed" : "Work paused"), () => { remountFocus = { jobId: job.id, id: "pause-toggle" }; }, failed); }); }}>{t(isPaused ? "Resume work" : "Pause work")}</Btn></div>}
                <p className="mt-2 text-[11px] text-muted-foreground">{t("Arrival inside the window counts toward the SLA. Times are part of the report and visible to the contractor and HQ.")}</p>
              </div>
            )}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <WorkReportCard versionText={versionText()} issues={issues} isEditable={isEditable} notice={win.notice} isInWork={isInWork} hasReport={!!live.report} hasDraftReport={!!rep} draft={draft} edit={edit} photos={photos} photoUrl={photoUrl} photoError={photoError} addPhotos={addPhotos} pending={pending} dirty={dirty} signature={signature} f={f} onViewParts={() => { setView("parts"); }} onSign={() => { setModal("sign"); }} onSave={() => { save(); }} onSubmit={trySubmit} />
          <VersionsCard versions={versions} win={win} />
        </div>
      </div>
      <CheckInModal open={modal === "checkin"} unitId={job.unitId} pending={pending} errors={checkInError} setErrors={setCheckInError} onClose={() => { setModal(null); }}
        onCheckIn={(method, distance, qr, reason) => { run(() => checkIn(job.id, job.version, method, distance, qr, reason), t("Checked in — the job has started"), () => { remountFocus = { jobId: job.id, id: "job-title" }; setModal(null); }, (f) => {
          // a refusal of a field (too far, another unit's label) shows under that field, which takes the focus (IR621)
          const fields = fieldErrorTexts(f, CHECK_IN_API_FIELDS, t);
          const first = CHECK_IN_FIELDS.find((k) => fields[k]);
          if (!first) { failed(f); return; }
          setCheckInError(fields);
          focusField(CHECK_IN_IDS[first]);
        }); }} />
      <CantMakeModal open={modal === "cant"} pending={pending} zone={live.zone} onClose={() => { setModal(null); }} onSend={(reason, slot) => { run(() => acknowledge(job.id, job.version, "cant_make", reason, slot), t("Sent to your coordinator"), () => { setModal(null); }, failed); }} />
      <PartModal open={modal === "part"} parts={live.parts} components={live.components} onClose={() => { setModal(null); }} onAdd={(p) => { edit((d) => ({ ...d, parts: [...d.parts, p] })); setModal(null); }} />
      <RefrigerantModal open={modal === "refrigerant"} onClose={() => { setModal(null); }} onAdd={(r) => { edit((d) => ({ ...d, refrigerant: [...d.refrigerant, r] })); setModal(null); }} />
      <SignModal open={modal === "sign"} pending={pending} summary={t("{version} · {done} / {total} results · {parts} parts", { version: rep ? t("draft v{version}", { version: rep.version }) : t("new draft"), done: draft.items.filter((x) => x.result).length, total: draft.items.length, parts: draft.parts.length })} onClose={() => { setModal(null); }}
        onSign={(form) => { withSaved((r) => { form.set("jobId", job.id); form.set("reportId", r.id); form.set("reportVersion", String(r.version)); run(() => signOff(form), t("Sign-off saved for this report version"), (next) => { setRep(next); setModal(null); }, failed); }); }} />
      <Modal open={modal === "submit"} onClose={() => { setModal(null); }} title={t("Submit report v{version}?", { version: rep?.version ?? 1 })} footer={<><Btn onClick={() => { setModal(null); }}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={submit}>{t("Submit")}</Btn></>}>
        <p className="text-[13px]">{t("Submitted versions become read-only; the quality review by your company or HQ follows.")}</p>
        {!(rep ?? live.report)?.signOff && <Banner tone="warn">{t("No customer sign-off yet — the report can still be submitted; the reviewer sees “not signed”.")}</Banner>}
      </Modal>
    </Page>
  );
}

type CheckInErrors = Partial<Record<CheckInField, string>>;
/** The page keys the view by the job's version, so an action that moves the job on mounts it again and drops the focus:
the check-in, accepting and resuming rework send it to the job's title, pausing to the same Pause / Resume button
(IR621, IR625). The job's next view takes it. */
let remountFocus: { jobId: string; id: string } | null = null;
type View = "readings" | "parts" | "time";
/** The checklist group and the view each job had open, for its next view after a remount (IR625). */
const openTab = new Map<string, Group>();
const openView = new Map<string, View>();
/** The check-in form's fields by id: one dialog per page, and a refusal from the Core API focuses its field. */
const CHECK_IN_IDS: Record<CheckInField, string> = { distance: "check-in-distance", qr: "check-in-qr", reason: "check-in-reason" };

function CheckInModal({ open, unitId, pending, errors, setErrors, onClose, onCheckIn }: Readonly<{ open: boolean; unitId: string; pending: boolean; errors: CheckInErrors; setErrors: (e: CheckInErrors) => void; onClose: () => void; onCheckIn: (method: "location_qr" | "manual", distance: number | null, qr: string | null, reason: string | null) => void }>) {
  const t = useT();
  const [method, setMethod] = useState<"location_qr" | "manual">(LOCATION_QR);
  const [distance, setDistance] = useState("40");
  const [qr, setQr] = useState(`ac-unit:${unitId}`);
  const [reason, setReason] = useState("");
  const clear = (k: CheckInField) => { if (lookup(errors, k)) setErrors(without(errors, k)); };
  const go = () => {
    const e = checkInErrors(method, distance, qr, reason, t);
    setErrors(e);
    // a refused check-in takes the focus to the first field to fix, top to bottom (IR586, IR621)
    const bad = CHECK_IN_FIELDS.find((k) => e[k]);
    if (bad) { focusField(CHECK_IN_IDS[bad]); return; }
    if (method === "manual") onCheckIn("manual", null, null, reason.trim());
    else onCheckIn(LOCATION_QR, Number(distance), qr.trim(), null);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Check in at site")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={go}>{t("Check in & start")}</Btn></>}>
      <Field label={t("How")}><Choice value={method} onChange={(v) => { setMethod(v); setErrors({}); }} options={[{ id: LOCATION_QR, label: t("Location + unit QR") }, { id: "manual", label: t("Manual (reason)") }]} /></Field>
      {method === LOCATION_QR ? (
        <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
          <Field label={t("Distance to the site (m)")} hint={t("Demo: the device's location service fills this; within 200 m")} error={errors.distance}><Input id={CHECK_IN_IDS.distance} inputMode="numeric" value={distance} onChange={(e) => { setDistance(e.target.value); clear("distance"); }} /></Field>
          <Field label={t("Scanned unit QR")} hint={t("Demo: the camera scan fills this")} error={errors.qr}><Input id={CHECK_IN_IDS.qr} value={qr} onChange={(e) => { setQr(e.target.value); clear("qr"); }} /></Field>
        </div>
      ) : <Field label={t("Reason (required)")} error={errors.reason}><Textarea id={CHECK_IN_IDS.reason} value={reason} maxLength={1000} placeholder={t("No GPS signal in the basement plant room")} onChange={(e) => { setReason(e.target.value); clear("reason"); }} /></Field>}
      <p className="text-[11px] text-muted-foreground">{t("Arrival must be inside your work window. Checking in records the arrival and starts the work.")}</p>
    </Modal>
  );
}

function CantMakeModal({ open, pending, zone, onClose, onSend }: Readonly<{ open: boolean; pending: boolean; zone: string; onClose: () => void; onSend: (reason: string, slot: { startAt: string; endAt: string } | null) => void }>) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [date, setDate] = useState("");
  const [window, setWindow] = useState("10:00-12:00");
  const [error, setError] = useState<string | null>(null);
  const send = () => {
    if (reason.trim().length === 0) { setError(t("Tell your coordinator why (1–1000 characters)")); return; }
    const [from = "", to = ""] = window.split("-", 2);
    onSend(reason.trim(), date ? { startAt: zonedInstant(date, from, zone), endAt: zonedInstant(date, to, zone) } : null); // typed in the display time zone (NFR-08)
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Can’t make this time")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={send}>{t("Send to coordinator")}</Btn></>}>
      <Field label={t("Reason (required)")} error={error ?? undefined}><Textarea value={reason} maxLength={1000} placeholder={t("Medical appointment on that morning")} onChange={(e) => { setReason(e.target.value); }} /></Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "150px" }}>
        <Field label={t("A time you could do instead (optional)")} hint={t("Times in {zone}.", { zone })}><Input type="date" value={date} onChange={(e) => { setDate(e.target.value); }} /></Field>
        <Field label={t("Window")}><Select value={window} onChange={(e) => { setWindow(e.target.value); }}>{["08:00-10:00", "10:00-12:00", "12:00-14:00", "14:00-16:00", "16:00-18:00"].map((w) => <option key={w}>{w}</option>)}</Select></Field>
      </div>
      <Banner>{t("Your coordinator reassigns the job or proposes a new time — the client must approve any change.")}</Banner>
    </Modal>
  );
}

function PartModal({ open, parts, components, onClose, onAdd }: Readonly<{ open: boolean; parts: WorkspaceLive["parts"]; components: WorkspaceLive["components"]; onClose: () => void; onAdd: (p: DraftPart) => void }>) {
  const t = useT();
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [qty, setQty] = useState("1");
  const [source, setSource] = useState<DraftPart["source"]>("van_stock");
  const [lot, setLot] = useState("");
  const [replaces, setReplaces] = useState<ComponentKey | "">("");
  const [disposal, setDisposal] = useState<"" | "disposed_on_site" | "returned">("");
  const [errors, setErrors] = useState<Partial<Record<(typeof PART_FIELDS)[number], string>>>({});
  const picked = parts.find((p) => p.code === code);
  const add = () => {
    const label = (picked?.name ?? name).trim();
    const e = partErrors(label, qty, t);
    setErrors(e);
    // a refused part takes the focus to the field to fix (IR625, as IR621)
    const bad = PART_FIELDS.find((k) => e[k]);
    if (bad) { focusField(`part-${bad}`); return; }
    onAdd({ name: label, quantity: Number(qty), catalogCode: picked?.code ?? null, lotSerial: lot.trim() || null, replacesComponentKey: replaces || null, oldPartDisposal: replaces ? (disposal || "disposed_on_site") : null, receiptAttachmentId: null, source });
    setCode(""); setName(""); setQty("1"); setLot(""); setReplaces(""); setDisposal("");
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Add part")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" onClick={add}>{t("Add")}</Btn></>}>
      <Field label={t("Part (catalog)")}><Select value={code} onChange={(e) => { setCode(e.target.value); }}><option value="">{t("Not in the catalog…")}</option>{parts.map((p) => <option key={p.code} value={p.code}>{p.name} · {p.code}{p.vanStockQuantity === null ? "" : ` · ${t("{n} in van", { n: p.vanStockQuantity })}`}</option>)}</Select></Field>
      {!picked && <Field label={t("Part name")} error={errors.name}><Input id="part-name" value={name} maxLength={120} onChange={(e) => { setName(e.target.value); setErrors(without(errors, "name")); }} /></Field>}
      <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
        <Field label={t("Qty")} error={errors.qty}><Input id="part-qty" inputMode="numeric" value={qty} onChange={(e) => { setQty(e.target.value); setErrors(without(errors, "qty")); }} /></Field>
        <Field label={t("Source")}><Select value={source} onChange={(e) => { setSource(oneOf(sources.map((x) => x.id), e.target.value, source)); }}>{sources.map((x) => <option key={x.id} value={x.id}>{t(x.label)}</option>)}</Select></Field>
        <Field label={t("Lot / serial")}><Input value={lot} onChange={(e) => { setLot(e.target.value); }} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Replaces checklist item (optional)")}><Select value={replaces} onChange={(e) => { setReplaces(oneOf(components.map((c) => c.key), e.target.value, "")); }}><option value="">—</option>{components.map((c) => <option key={c.key} value={c.key}>{t(groupLabel[c.group])} › {t(componentLabel[c.key] ?? c.key)}</option>)}</Select></Field>
        {replaces && <Field label={t("Old part")}><Select value={disposal} onChange={(e) => { setDisposal(oneOf(["disposed_on_site", "returned"] as const, e.target.value, "")); }}><option value="">{t("Disposed on site")}</option><option value="returned">{t("Returned")}</option></Select></Field>}
      </div>
      {source === "bought_locally" && <p className="text-[11px] text-muted-foreground">{t("Keep the receipt — attach a photo of it with the report photos.")}</p>}
    </Modal>
  );
}

function RefrigerantModal({ open, onClose, onAdd }: Readonly<{ open: boolean; onClose: () => void; onAdd: (r: DraftRefrigerant) => void }>) {
  const t = useT();
  const [kind, setKind] = useState<DraftRefrigerant["refrigerant"]>("R32");
  const [cylinder, setCylinder] = useState("");
  const [recovered, setRecovered] = useState("0");
  const [charged, setCharged] = useState("0");
  const [leak, setLeak] = useState<DraftRefrigerant["leakCheck"]>("pass");
  const [method, setMethod] = useState("electronic detector");
  const [errors, setErrors] = useState<Partial<Record<(typeof REFRIGERANT_FIELDS)[number], string>>>({});
  const clear = (k: (typeof REFRIGERANT_FIELDS)[number]) => { setErrors((x) => without(x, k)); };
  const add = () => {
    const e = refrigerantErrors(cylinder, recovered, charged, t);
    setErrors(e);
    // a refused line takes the focus to the field to fix (IR625, as IR621)
    const bad = REFRIGERANT_FIELDS.find((k) => e[k]);
    if (bad) { focusField(`refrigerant-${bad}`); return; }
    onAdd({ refrigerant: kind, cylinderId: cylinder.trim(), recoveredKg: Number(recovered), chargedKg: Number(charged), leakCheck: leak, leakCheckMethod: leak === "not_done" ? null : method.trim() || null });
  };
  return (
    <Modal open={open} onClose={onClose} title={t(REFRIGERANT)} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" onClick={add}>{t("Add")}</Btn></>}>
      <div className="grid-fluid" style={{ ["--min" as string]: "140px" }}>
        <Field label={t(REFRIGERANT)}><Select value={kind} onChange={(e) => { setKind(oneOf(["R32", "R410A"] as const, e.target.value, kind)); }}><option>R32</option><option>R410A</option></Select></Field>
        <Field label={t("Cylinder ID")} error={errors.cylinder}><Input id="refrigerant-cylinder" value={cylinder} maxLength={64} placeholder="CYL-R32-0182" onChange={(e) => { setCylinder(e.target.value); clear("cylinder"); }} /></Field>
        <Field label={t("Recovered (kg)")} error={errors.recovered}><Input id="refrigerant-recovered" inputMode="decimal" value={recovered} onChange={(e) => { setRecovered(e.target.value); clear("recovered"); }} /></Field>
        <Field label={t("Charged (kg)")} error={errors.charged}><Input id="refrigerant-charged" inputMode="decimal" value={charged} onChange={(e) => { setCharged(e.target.value); clear("charged"); }} /></Field>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>
        <Field label={t("Leak check")}><Select value={leak} onChange={(e) => { setLeak(oneOf(["pass", "fail", "not_done"] as const, e.target.value, leak)); }}><option value="pass">{t("Pass")}</option><option value="fail">{t("Fail")}</option><option value="not_done">{t("Not done")}</option></Select></Field>
        {leak !== "not_done" && <Field label={t("Method")}><Input value={method} onChange={(e) => { setMethod(e.target.value); }} /></Field>}
      </div>
    </Modal>
  );
}

function SignModal({ open, pending, summary, onClose, onSign }: Readonly<{ open: boolean; pending: boolean; summary: string; onClose: () => void; onSign: (form: FormData) => void }>) {
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
    if (!name.trim()) { setError(t("The signer's name is required")); return; }
    const form = new FormData();
    form.set("signerName", name.trim());
    if (absent) {
      if (!reason.trim()) { setError(t("Say why the customer could not sign")); return; }
      if (!photo || !["image/jpeg", "image/png"].includes(photo.type) || photo.size > 5 << 20) { setError(t("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available")); return; }
      form.set("absentReason", reason.trim()); form.set("sitePhoto", photo);
    } else {
      if (!drawn || !canvas.current) { setError(t("Ask the customer to sign in the box")); return; }
      form.set("signature", canvas.current.toDataURL("image/png").split(",", 2)[1] ?? "");
    }
    setError(null);
    onSign(form);
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Customer sign-off")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={sign}>{t("Save sign-off")}</Btn></>}>
      <p className="text-xs text-muted-foreground">{t("Report {summary}", { summary })}</p>
      <Field label={t("Signer name")}><Input value={name} maxLength={120} placeholder="Ms Tan" onChange={(e) => { setName(e.target.value); }} /></Field>
      <Field label={t("How")}><Choice value={absent ? "absent" : "sign"} onChange={(v) => { setAbsent(v === "absent"); }} options={[{ id: "sign", label: t("Signature") }, { id: "absent", label: t("Customer not available") }]} /></Field>
      {absent ? (
        <>
          <Field label={t("Why the customer could not sign")}><Textarea value={reason} maxLength={1000} onChange={(e) => { setReason(e.target.value); }} /></Field>
          <Field label={t("Site photo (JPEG/PNG)")}><Input type="file" accept="image/jpeg,image/png" onChange={(e) => { setPhoto(e.target.files?.[0] ?? null); }} /></Field>
        </>
      ) : (
        <div>
          <canvas ref={canvas} width={560} height={160} aria-label={t("Signature pad")} className="h-32 w-full touch-none rounded-xl border border-dashed border-line bg-surface"
            onPointerDown={(e) => { drawing.current = true; const c = e.currentTarget.getContext("2d"); if (!c) { return; } const [x, y] = point(e); c.lineWidth = 2.5; c.lineCap = "round"; c.strokeStyle = "#0f172a"; c.beginPath(); c.moveTo(x, y); e.currentTarget.setPointerCapture(e.pointerId); }}
            onPointerMove={(e) => { const c = e.currentTarget.getContext("2d"); if (!c || !drawing.current) { return; } const [x, y] = point(e); c.lineTo(x, y); c.stroke(); setDrawn(true); }}
            onPointerUp={() => { drawing.current = false; }} />
          <div className="mt-1 flex justify-between text-[11px] text-muted-foreground"><span className="flex items-center gap-1"><Icon name="signature" />{t("Sign here")}</span><Plain type="button" className="font-semibold text-primary" onClick={clear}>{t("Clear")}</Plain></div>
        </div>
      )}
      {error && <Banner tone="crit">{error}</Banner>}
      <p className="text-[11px] text-muted-foreground">{t("The sign-off is bound to this report version; saving the draft again clears it.")}</p>
    </Modal>
  );
}

/** A report photo streamed through the DAL route; a photo whose file is gone (404) or unreadable shows a tile instead. */
function Photo({ src, name }: Readonly<{ src: string; name: string }>) {
  const t = useT();
  const [broken, setBroken] = useState(false);
  if (broken) return <span title={name} className="grid h-16 w-20 place-items-center rounded-lg border border-dashed border-line bg-surface2 px-1 text-center text-[10px] leading-tight text-muted-foreground">{t("Photo unavailable")}</span>;
  // eslint-disable-next-line @next/next/no-img-element -- streamed through the DAL route (attachments.getContent)
  return <img src={src} alt={name} onError={() => { setBroken(true); }} className="h-16 w-20 rounded-lg bg-surface2 object-cover" />;
}
