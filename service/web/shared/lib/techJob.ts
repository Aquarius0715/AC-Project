// Technician job workspace (FR-T04–T06, T08, T09, T13–T15, DATA_SOURCE=api): the component checklist of the unit's
// service scope, the editable draft and its submit checks (IR100 / IR126), the work-window state (IR76 / IR89), time on
// site and the version rows. Pure code shared by the Server Component and the client view. Texts in the display
// language (`t`, IR282); instants through a formatter (`Fmt`) that reads the loader's server-formatted strings, so the
// first render matches the server's, and formats in the browser only what appears after the page loaded.
import { metricUnit, type Metric } from "@ac/web/lib/devices";
import { EN, showClock, showDate, showSpan, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { ComponentKey } from "@ac/web/lib/contracts.gen";

const en = translator("en");

/** How the workspace writes an instant: a time of day, a date with its time and a span — the user's language and
 * display time zone (IR44). */
export type Fmt = { t: T; clock: (iso: string) => string; stamp: (iso: string) => string; span: (from: string, to: string) => string };
/** The instants the loader formatted on the server, keyed by the ISO string (spans by “from|to”). */
export type Known = { clock: Record<string, string>; stamp: Record<string, string>; span: Record<string, string> };
export const fmtOf = (i: I18n, known?: Known): Fmt => ({
  t: i.t,
  clock: (iso) => known?.clock[iso] ?? showClock(iso, i.display),
  stamp: (iso) => known?.stamp[iso] ?? showTime(iso, i.display),
  span: (from, to) => known?.span[`${from}|${to}`] ?? showSpan(from, to, i.display),
});
/** The server's formatting of the instants a page shows: every time of day, stamp and span it will ask for. */
export function knownOf(i: I18n, clocks: (string | null | undefined)[], stamps: (string | null | undefined)[], spans: [string, string][]): Known {
  const f = fmtOf(i);
  const keep = (xs: (string | null | undefined)[], fn: (iso: string) => string) => Object.fromEntries(xs.filter((x): x is string => !!x).map((x) => [x, fn(x)]));
  return { clock: keep(clocks, f.clock), stamp: keep(stamps, f.stamp), span: Object.fromEntries(spans.map(([a, b]) => [`${a}|${b}`, f.span(a, b)])) };
}
const EN_FMT = fmtOf(EN);

export type Group = "indoor" | "outdoor" | "electrical";
/** The component keys of each group in canonical order (maintenance.ComponentGroups of the Core API). */
export const componentGroups: { group: Group; keys: string[] }[] = [
  { group: "indoor", keys: ["filter", "evaporator_coil", "blower_motor", "blower_fan", "drain_pipe", "drain_pan", "outlet", "louver"] },
  { group: "outdoor", keys: ["condenser_coil", "compressor", "fan", "blade", "refrigerant_pipe"] },
  { group: "electrical", keys: ["thermostat", "sensor", "capacitor", "contactor", "wiring"] },
];
export const groupLabel: Record<Group, string> = { indoor: "Indoor", outdoor: "Outdoor", electrical: "Electrical" };
export const componentLabel: Record<string, string> = {
  filter: "Filter", evaporator_coil: "Evaporator coil", blower_motor: "Blower motor", blower_fan: "Blower fan", drain_pipe: "Drain pipe", drain_pan: "Drain pan",
  outlet: "Outlet", louver: "Louver", condenser_coil: "Condenser coil", compressor: "Compressor", fan: "Fan", blade: "Fan blade", refrigerant_pipe: "Refrigerant pipe",
  thermostat: "Thermostat", sensor: "Sensor", capacitor: "Capacitor", contactor: "Contactor", wiring: "Wiring insulation",
};
export const groupOf = (key: string): Group | null => componentGroups.find((g) => g.keys.includes(key))?.group ?? null;
/** The components a report of this unit must cover: every key of the groups in the unit's service scope (IR100). */
export const componentsFor = (scope: string[]) => componentGroups.filter((g) => scope.includes(g.group)).flatMap((g) => g.keys.map((key) => ({ group: g.group, key })));

/** The readings a technician can record, each in its fixed unit (MetricUnits of the Core API). */
export const readingMetrics: { metric: Metric; label: string }[] = [
  { metric: "temperature", label: "Temperature" }, { metric: "humidity", label: "Humidity" }, { metric: "refrigerant_pressure", label: "Refrigerant pressure" },
  { metric: "vibration", label: "Vibration" }, { metric: "power", label: "Power draw" }, { metric: "compressor_cycles", label: "Compressor cycles" },
  { metric: "airflow_drop", label: "Airflow drop" }, { metric: "co2", label: "CO₂" }, { metric: "pm25", label: "PM2.5" },
];

/** WorkReport of service-contracts.ts as the technician reads it. */
export type ApiTechReport = {
  id: string; version: number; jobId: string; authorId: string; state?: string;
  items: { id: string; componentGroup: Group; componentKey: string; result: Result; reason: string | null; evidenceIds: string[]; authorId: string; observedAt: string }[];
  measurements: { id: string; componentKey?: string | null; metric: Metric; value: number | null; unit: string; observedAt: string; quality: string; qualityReason?: string | null }[];
  parts: DraftPart[]; refrigerant: DraftRefrigerant[]; signOff: { signerName: string; signedAt: string; signatureAttachmentId: string | null; absentReason: string | null; reportVersion: number } | null;
  workText: string; nextAction: { kind: "none" } | { kind: "follow_up"; date: string; note: string } | null;
  attachmentRefs: { id: string; name: string; mime: string; size: number; status: "processing" | "ready" | "failed" }[]; submittedAt: string | null; acceptedAt: string | null;
  reviewHistory: { reviewerUserId: string; reportVersion: number; decision: "accept" | "return"; reason: string | null; occurredAt: string }[];
};
export type Result = "normal" | "attention" | "not_inspected" | "not_applicable" | null;
export type DraftItem = { componentGroup: Group; componentKey: string; result: Result; reason: string; evidenceIds: string[] };
export type DraftReading = { id?: string; componentKey: string; metric: Metric; value: string; observedAt: string };
export type DraftPart = {
  name: string; quantity: number; catalogCode: string | null; source: "van_stock" | "hq_warehouse" | "bought_locally"; lotSerial: string | null;
  replacesComponentKey: string | null; oldPartDisposal: "disposed_on_site" | "returned" | null; receiptAttachmentId: string | null;
};
export type DraftRefrigerant = { refrigerant: "R32" | "R410A"; cylinderId: string; recoveredKg: number; chargedKg: number; leakCheck: "pass" | "fail" | "not_done"; leakCheckMethod: string | null };
export type NextAction = { kind: "none" } | { kind: "follow_up"; date: string; note: string };
export type Draft = { items: DraftItem[]; readings: DraftReading[]; parts: DraftPart[]; refrigerant: DraftRefrigerant[]; workText: string; nextAction: NextAction; attachmentIds: string[] };

/** The editable draft of a report: every required component in canonical order (null until a result is chosen — normal
 * is never preselected, DD-T04), the readings as text so a half-typed value survives, and the other sections as saved. */
export function draftFrom(r: ApiTechReport | null, components: { group: Group; key: string }[]): Draft {
  const items = components.map(({ group, key }) => {
    const it = r?.items.find((x) => x.componentKey === key);
    return { componentGroup: group, componentKey: key, result: it?.result ?? null, reason: it?.reason ?? "", evidenceIds: it?.evidenceIds ?? [] };
  });
  return {
    items,
    readings: (r?.measurements ?? []).map((m) => ({ id: m.id, componentKey: m.componentKey ?? components[0]?.key ?? "", metric: m.metric, value: m.value === null ? "" : String(m.value), observedAt: m.observedAt })),
    parts: r?.parts ?? [], refrigerant: r?.refrigerant ?? [], workText: r?.workText ?? "",
    nextAction: r?.nextAction ?? { kind: "none" }, attachmentIds: (r?.attachmentRefs ?? []).filter((a) => a.status !== "failed").map((a) => a.id),
  };
}

/** The jobs.saveDraft body of a draft: trimmed reasons (empty → null), numeric readings in their metric's unit. */
export function draftInput(jobId: string, reportId: string | null, d: Draft): OpInput<"jobs.saveDraft"> {
  const num = (v: string) => (v.trim() === "" || !Number.isFinite(Number(v)) ? null : Number(v));
  return {
    jobId, ...(reportId ? { reportId } : {}),
    items: d.items.map((it) => ({ componentGroup: it.componentGroup, componentKey: it.componentKey as ComponentKey, result: it.result, reason: it.reason.trim() || null, evidenceIds: it.evidenceIds })),
    measurements: d.readings.map((m) => ({ ...(m.id ? { id: m.id } : {}), componentKey: m.componentKey as ComponentKey, metric: m.metric, value: num(m.value), unit: metricUnit[m.metric], observedAt: m.observedAt })),
    parts: d.parts.map((p) => ({ ...p, name: p.name.trim(), replacesComponentKey: p.replacesComponentKey as ComponentKey | null })), refrigerant: d.refrigerant, workText: d.workText,
    nextAction: d.nextAction.kind === "none" ? { kind: "none" as const } : { kind: "follow_up" as const, date: d.nextAction.date, note: d.nextAction.note.trim() },
    attachmentIds: d.attachmentIds,
  };
}

/** What blocks a submit (the IR100 / IR126 submit rules of the Core API, checked first in the browser so the
 * technician sees every issue at once; the API stays the authority). */
export type Issue = { key: string | null; group: Group | null; text: string };
const PHOTO_STATE: Record<string, string> = { processing: "processing", failed: "failed", ready: "ready" };
export function submitIssues(d: Draft, attachments: { id: string; name: string; status: string }[], now: number, t: T = en): Issue[] {
  const out: Issue[] = [];
  const label = (key: string) => t(componentLabel[key] ?? key);
  for (const it of d.items) {
    const where = `${t(groupLabel[it.componentGroup])} › ${label(it.componentKey)}`;
    if (it.result === null) out.push({ key: it.componentKey, group: it.componentGroup, text: t("{where} has no result", { where }) });
    else if (it.result !== "normal" && !it.reason.trim()) out.push({ key: it.componentKey, group: it.componentGroup, text: t("{where} needs a reason", { where }) });
  }
  for (const m of d.readings) {
    if (m.value.trim() !== "" && !Number.isFinite(Number(m.value))) out.push({ key: m.componentKey, group: groupOf(m.componentKey), text: t("{name} reading is not a number", { name: label(m.componentKey) }) });
  }
  const n = d.workText.trim().length;
  if (n < 10 || n > 4000) out.push({ key: null, group: null, text: t("Work performed must be 10–4000 characters") });
  if (d.nextAction.kind === "follow_up" && (!d.nextAction.date || Date.parse(d.nextAction.date) <= now || !d.nextAction.note.trim())) out.push({ key: null, group: null, text: t("A follow-up needs a future date and a note") });
  d.parts.forEach((p, k) => { if (p.quantity < 1 || p.quantity > 999) out.push({ key: null, group: null, text: t("Part {n} ({name}) needs a quantity of 1–999", { n: k + 1, name: p.name || t("unnamed") }) }); });
  for (const a of attachments.filter((x) => d.attachmentIds.includes(x.id) && x.status !== "ready")) out.push({ key: null, group: null, text: t("Photo {name} is {state} — remove or retry it", { name: a.name, state: PHOTO_STATE[a.status] ? t(PHOTO_STATE[a.status]) : a.status }) });
  return out;
}

/** Checklist progress per group: results chosen out of the group's components, and the issues of each group. */
export function progress(d: Draft, issues: Issue[], t: T = en) {
  return componentGroups.map(({ group }) => {
    const items = d.items.filter((i) => i.componentGroup === group);
    return { group, label: t(groupLabel[group]), done: items.filter((i) => i.result !== null).length, total: items.length, issues: issues.filter((x) => x.group === group).length };
  }).filter((g) => g.total > 0);
}

/** The work window (IR76 before the start, IR89 warning 15 minutes before the end and the end itself). */
export type WindowState = { phase: "none" | "before" | "open" | "ending" | "ended"; text: string };
export function windowState(a: { scheduledStart: string; scheduledEnd: string } | null, now: number, f: Fmt = EN_FMT): WindowState {
  const { t } = f;
  if (!a) return { phase: "none", text: t("No work window") };
  const start = Date.parse(a.scheduledStart), end = Date.parse(a.scheduledEnd);
  const range = f.span(a.scheduledStart, a.scheduledEnd);
  if (now < start) return { phase: "before", text: t("Starts {time} · {range}", { time: f.stamp(a.scheduledStart), range }) };
  if (now >= end) return { phase: "ended", text: t("Ended {time} · {range}", { time: f.stamp(a.scheduledEnd), range }) };
  const left = Math.ceil((end - now) / 60000);
  return left <= 15 ? { phase: "ending", text: t("Ends in {n} min — unsaved input is discarded at {time} (IR89)", { n: left, time: f.clock(a.scheduledEnd) }) } : { phase: "open", text: range };
}

/** Time on site (FR-T13 / T14): arrival and check-in evidence, start, pauses, finish and the counted minutes. */
export type TimeOnSite = {
  arrivedAt?: string | null; checkInMethod?: "location_qr" | "manual" | null; checkInReason?: string | null; distanceMeters?: number | null; startedAt?: string | null;
  pauses?: { from: string; to: string | null }[]; finishedAt?: string | null; onSiteMinutes?: number | null;
};
const duration = (mins: number, t: T) => (mins >= 60 ? t("{h} h {m} min", { h: Math.floor(mins / 60), m: mins % 60 }) : t("{n} min", { n: mins }));
/** The instants of a time on site, for the loader to format. */
export const timeInstants = (ts: TimeOnSite | null | undefined) => [ts?.arrivedAt, ts?.startedAt, ts?.finishedAt, ...(ts?.pauses ?? []).flatMap((p) => [p.from, p.to])];
export function timeRows(ts: TimeOnSite | null | undefined, now: number, f: Fmt = EN_FMT): [string, string][] {
  const { t } = f;
  if (!ts?.arrivedAt && !ts?.startedAt) return [[t("Arrived"), t("not checked in")]];
  const pauses = ts.pauses ?? [];
  const open = pauses.find((p) => !p.to);
  const paused = pauses.reduce((sum, p) => sum + ((p.to ? Date.parse(p.to) : now) - Date.parse(p.from)), 0);
  const from = Date.parse(ts.arrivedAt ?? ts.startedAt!);
  const minutes = ts.onSiteMinutes ?? Math.max(0, Math.floor((((ts.finishedAt ? Date.parse(ts.finishedAt) : now) - from) - paused) / 60000));
  return [
    [t("Arrived"), `${f.clock(ts.arrivedAt ?? ts.startedAt!)} · ${t(ts.checkInMethod === "manual" ? "manual check-in" : "checked in")}`],
    [t("Location"), ts.checkInMethod === "manual" ? t("manual — {reason}", { reason: ts.checkInReason ?? t("no reason") }) : ts.distanceMeters != null ? t("{m} m from site · QR matched", { m: Math.round(ts.distanceMeters) }) : "—"],
    [t("Started"), ts.startedAt ? f.clock(ts.startedAt) : "—"],
    [t("Paused"), open ? t("since {time}", { time: f.clock(open.from) }) : pauses.length ? `${t(pauses.length === 1 ? "1 pause" : "{n} pauses", { n: pauses.length })} · ${duration(Math.round(paused / 60000), t)}` : "—"],
    [t("Finished"), ts.finishedAt ? f.clock(ts.finishedAt) : t("— (on submit)")],
    [t("On-site time"), duration(minutes, t)],
  ];
}

/** Versions & autosave (Figma Technician 02): the open draft as the page holds it (its saved version, recorded results
 * and ready photos — the first save creates it without a reload) above the submitted versions with their review. */
export type VersionRow = { title: string; sub: string; badge: { text: string; tone: "primary" | "ok" | "warn" | "muted" } };
export type DraftVersion = { version: number; results: number; photos: number; savedAt: string | null };
export function versionRows(draft: DraftVersion | null, reportRefs: { reportVersion: number }[], reviews: ApiTechReport["reviewHistory"], f: Fmt = EN_FMT): VersionRow[] {
  const { t } = f;
  const rows: VersionRow[] = [];
  if (draft) {
    rows.push({
      title: draft.savedAt ? t("Draft v{version} · saved {time}", { version: draft.version, time: f.clock(draft.savedAt) }) : t("Draft v{version}", { version: draft.version }),
      sub: `${t(draft.results === 1 ? "1 result" : "{n} results", { n: draft.results })} · ${t(draft.photos === 1 ? "1 photo" : "{n} photos", { n: draft.photos })}`, badge: { text: t("Draft"), tone: "warn" },
    });
  }
  for (const ref of [...reportRefs].reverse()) {
    const rv = reviews.filter((h) => h.reportVersion === ref.reportVersion).pop();
    rows.push({
      title: t("v{version} · {state}", { version: ref.reportVersion, state: t(rv ? (rv.decision === "accept" ? "accepted" : "returned") : "submitted") }),
      sub: rv ? `${f.stamp(rv.occurredAt)}${rv.reason ? ` — ${rv.reason}` : ""}` : t("awaiting quality review"),
      badge: rv?.decision === "accept" ? { text: t("Accepted"), tone: "ok" } : rv?.decision === "return" ? { text: t("Returned"), tone: "warn" } : { text: t("Submitted"), tone: "primary" },
    });
  }
  return rows;
}

/** The follow-up of a next action as text: the Kuala Lumpur calendar date the technician picked and the note. */
export const followUpText = (n: NextAction, i: I18n = EN) => (n.kind === "none" ? i.t("None") : i.t("Follow-up {date} — {note}", { date: n.date ? businessDay(n.date, i.display.locale) : "—", note: n.note }));

/** JobHistorySnapshot of service-contracts.ts as the technician reads it (IR124). */
export type ApiTechHistory = {
  projection: "history"; jobId: string; type: string; status: string; asOf: string; completedAt: string | null;
  redactedReportSummary: { hasReport: boolean; acceptance: "accepted" | "not_accepted" };
};
/** A job whose viewing window ended (IR49): completed — the assignment ended with it (IR234) — or reassigned / cancelled,
 * read as the snapshot frozen when it ended (IR124). */
export function historyCard(h: ApiTechHistory, i: I18n = EN) {
  const { t, display } = i;
  const done = h.status === "completed";
  const report = !h.redactedReportSummary.hasReport ? t("No report from your assignment") : t(h.redactedReportSummary.acceptance === "accepted" ? "Accepted in the quality review" : "Submitted — not accepted while you were assigned");
  return {
    title: `${h.jobId.slice(0, 8)} · ${typeLabel(h.type, t)}`, sub: t(done ? "Completed — your assignment ended with the job" : "Your assignment on this job has ended"),
    rows: [
      [t("Status"), done ? (h.completedAt ? t("Completed {time}", { time: showDate(h.completedAt, display) }) : t("Completed")) : t("{status} when your assignment ended", { status: statusWord(h.status, t) })],
      [t("Your report"), report], [t("Assignment ended"), showTime(h.asOf, display)],
    ] as [string, string][],
    note: t(done ? "Your time is free for other jobs. The report, the unit and its devices stay with HQ and the customer." : "HQ or your coordinator changed the assignment, so the job details and the unit are no longer shown to you."),
    back: t("← Overview"),
  };
}
