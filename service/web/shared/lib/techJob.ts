// Technician job workspace (FR-T04–T06, T08, T09, T13–T15, DATA_SOURCE=api): the component checklist of the unit's
// service scope, the editable draft and its submit checks (IR100 / IR126), the work-window state (IR76 / IR89), time on
// site and the version rows. Pure code shared by the Server Component and the client view.
import { klTime, metricUnit, type Metric } from "@ac/web/lib/devices";

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
export function draftInput(jobId: string, reportId: string | null, d: Draft) {
  const num = (v: string) => (v.trim() === "" || !Number.isFinite(Number(v)) ? null : Number(v));
  return {
    jobId, ...(reportId ? { reportId } : {}),
    items: d.items.map((it) => ({ componentGroup: it.componentGroup, componentKey: it.componentKey, result: it.result, reason: it.reason.trim() || null, evidenceIds: it.evidenceIds })),
    measurements: d.readings.map((m) => ({ ...(m.id ? { id: m.id } : {}), componentKey: m.componentKey, metric: m.metric, value: num(m.value), unit: metricUnit[m.metric], observedAt: m.observedAt })),
    parts: d.parts.map((p) => ({ ...p, name: p.name.trim() })), refrigerant: d.refrigerant, workText: d.workText,
    nextAction: d.nextAction.kind === "none" ? { kind: "none" as const } : { kind: "follow_up" as const, date: d.nextAction.date, note: d.nextAction.note.trim() },
    attachmentIds: d.attachmentIds,
  };
}

/** What blocks a submit (the IR100 / IR126 submit rules of the Core API, checked first in the browser so the
 * technician sees every issue at once; the API stays the authority). */
export type Issue = { key: string | null; group: Group | null; text: string };
export function submitIssues(d: Draft, attachments: { id: string; name: string; status: string }[], now: number): Issue[] {
  const out: Issue[] = [];
  for (const it of d.items) {
    const where = `${groupLabel[it.componentGroup]} › ${componentLabel[it.componentKey] ?? it.componentKey}`;
    if (it.result === null) out.push({ key: it.componentKey, group: it.componentGroup, text: `${where} has no result` });
    else if (it.result !== "normal" && !it.reason.trim()) out.push({ key: it.componentKey, group: it.componentGroup, text: `${where} needs a reason` });
  }
  for (const m of d.readings) {
    if (m.value.trim() !== "" && !Number.isFinite(Number(m.value))) out.push({ key: m.componentKey, group: groupOf(m.componentKey), text: `${componentLabel[m.componentKey] ?? m.componentKey} reading is not a number` });
  }
  const n = d.workText.trim().length;
  if (n < 10 || n > 4000) out.push({ key: null, group: null, text: "Work performed must be 10–4000 characters" });
  if (d.nextAction.kind === "follow_up" && (!d.nextAction.date || Date.parse(d.nextAction.date) <= now || !d.nextAction.note.trim())) out.push({ key: null, group: null, text: "A follow-up needs a future date and a note" });
  d.parts.forEach((p, i) => { if (p.quantity < 1 || p.quantity > 999) out.push({ key: null, group: null, text: `Part ${i + 1} (${p.name || "unnamed"}) needs a quantity of 1–999` }); });
  for (const a of attachments.filter((x) => d.attachmentIds.includes(x.id) && x.status !== "ready")) out.push({ key: null, group: null, text: `Photo ${a.name} is ${a.status} — remove or retry it` });
  return out;
}

/** Checklist progress per group: results chosen out of the group's components, and the issues of each group. */
export function progress(d: Draft, issues: Issue[]) {
  return componentGroups.map(({ group }) => {
    const items = d.items.filter((i) => i.componentGroup === group);
    return { group, label: groupLabel[group], done: items.filter((i) => i.result !== null).length, total: items.length, issues: issues.filter((x) => x.group === group).length };
  }).filter((g) => g.total > 0);
}

/** The work window (IR76 before the start, IR89 warning 15 minutes before the end and the end itself). */
export type WindowState = { phase: "none" | "before" | "open" | "ending" | "ended"; text: string };
export function windowState(a: { scheduledStart: string; scheduledEnd: string } | null, now: number): WindowState {
  if (!a) return { phase: "none", text: "No work window" };
  const start = Date.parse(a.scheduledStart), end = Date.parse(a.scheduledEnd);
  const range = `${klTime(a.scheduledStart).slice(5)} – ${klTime(a.scheduledEnd).slice(5)}`;
  if (now < start) return { phase: "before", text: `Starts ${klTime(a.scheduledStart).slice(5)} · ${range}` };
  if (now >= end) return { phase: "ended", text: `Ended ${klTime(a.scheduledEnd).slice(5)} · ${range}` };
  const left = Math.ceil((end - now) / 60000);
  return left <= 15 ? { phase: "ending", text: `Ends in ${left} min — unsaved input is discarded at ${klTime(a.scheduledEnd).slice(11)} (IR89)` } : { phase: "open", text: range };
}

/** Time on site (FR-T13 / T14): arrival and check-in evidence, start, pauses, finish and the counted minutes. */
export type TimeOnSite = {
  arrivedAt?: string | null; checkInMethod?: "location_qr" | "manual" | null; checkInReason?: string | null; distanceMeters?: number | null; startedAt?: string | null;
  pauses?: { from: string; to: string | null }[]; finishedAt?: string | null; onSiteMinutes?: number | null;
};
const hhmm = (iso: string) => klTime(iso).slice(11);
const duration = (mins: number) => (mins >= 60 ? `${Math.floor(mins / 60)} h ${mins % 60} min` : `${mins} min`);
export function timeRows(t: TimeOnSite | null | undefined, now: number): [string, string][] {
  if (!t?.arrivedAt && !t?.startedAt) return [["Arrived", "not checked in"]];
  const pauses = t.pauses ?? [];
  const open = pauses.find((p) => !p.to);
  const paused = pauses.reduce((s, p) => s + ((p.to ? Date.parse(p.to) : now) - Date.parse(p.from)), 0);
  const from = Date.parse(t.arrivedAt ?? t.startedAt!);
  const minutes = t.onSiteMinutes ?? Math.max(0, Math.floor((((t.finishedAt ? Date.parse(t.finishedAt) : now) - from) - paused) / 60000));
  return [
    ["Arrived", `${hhmm(t.arrivedAt ?? t.startedAt!)} · ${t.checkInMethod === "manual" ? "manual check-in" : "checked in"}`],
    ["Location", t.checkInMethod === "manual" ? `manual — ${t.checkInReason ?? "no reason"}` : t.distanceMeters != null ? `${Math.round(t.distanceMeters)} m from site · QR matched` : "—"],
    ["Started", t.startedAt ? hhmm(t.startedAt) : "—"],
    ["Paused", open ? `since ${hhmm(open.from)}` : pauses.length ? `${pauses.length} pause${pauses.length === 1 ? "" : "s"} · ${duration(Math.round(paused / 60000))}` : "—"],
    ["Finished", t.finishedAt ? hhmm(t.finishedAt) : "— (on submit)"],
    ["On-site time", duration(minutes)],
  ];
}

/** Versions & autosave (Figma Technician 02): the open draft as the page holds it (its saved version, recorded results
 * and ready photos — the first save creates it without a reload) above the submitted versions with their review. */
export type VersionRow = { title: string; sub: string; badge: { text: string; tone: "primary" | "ok" | "warn" | "muted" } };
export type DraftVersion = { version: number; results: number; photos: number; savedAt: string | null };
export function versionRows(draft: DraftVersion | null, reportRefs: { reportVersion: number }[], reviews: ApiTechReport["reviewHistory"]): VersionRow[] {
  const rows: VersionRow[] = [];
  if (draft) {
    rows.push({ title: `Draft v${draft.version}${draft.savedAt ? ` · saved ${hhmm(draft.savedAt)}` : ""}`, sub: `${draft.results} result${draft.results === 1 ? "" : "s"} · ${draft.photos} photo${draft.photos === 1 ? "" : "s"}`, badge: { text: "Draft", tone: "warn" } });
  }
  for (const ref of [...reportRefs].reverse()) {
    const rv = reviews.filter((h) => h.reportVersion === ref.reportVersion).pop();
    rows.push({
      title: `v${ref.reportVersion} · ${rv ? (rv.decision === "accept" ? "accepted" : "returned") : "submitted"}`, sub: rv ? `${klTime(rv.occurredAt).slice(5)}${rv.reason ? ` — ${rv.reason}` : ""}` : "awaiting quality review",
      badge: rv?.decision === "accept" ? { text: "Accepted", tone: "ok" } : rv?.decision === "return" ? { text: "Returned", tone: "warn" } : { text: "Submitted", tone: "primary" },
    });
  }
  return rows;
}
