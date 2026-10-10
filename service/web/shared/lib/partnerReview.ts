// Contractor quality review (FR-P05, DATA_SOURCE=api): the submitted WorkReport read as inspection rows, readings,
// the evidence check of DD-P05 step 2, the version / alert rows and the IR31 review availability. Pure code shared by
// the Server Component and the client view. Texts in the display language (`t` / `i`) and times in the user's display
// time zone, formatted by the loader on the server (IR274).
import { EN, showSpan, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

/** WorkReport of service-contracts.ts (the parts the review shows). */
export type ApiInspectionItem = {
  id: string; componentGroup: "indoor" | "outdoor" | "electrical"; componentKey: string;
  result: "normal" | "attention" | "not_inspected" | "not_applicable" | null; reason: string | null; evidenceIds: string[]; authorId: string; observedAt: string;
};
export type ApiReportMeasurement = { id: string; componentKey?: string | null; metric: string; value: number | null; unit: string; quality: string; qualityReason?: string | null; observedAt: string };
export type ApiPart = { name: string; quantity: number; source: string; replacesComponentKey: string | null };
export type ApiRefrigerant = { refrigerant: string; cylinderId: string; recoveredKg: number; chargedKg: number; leakCheck: string };
export type ApiAttachment = { id: string; name: string; mime: string; size: number; status: "processing" | "ready" | "failed" };
export type ApiReview = { reviewerUserId: string; reportVersion: number; decision: "accept" | "return"; reason: string | null; occurredAt: string };
export type ReviewAvailability = { allowed: true; reason: null } | { allowed: false; reason: "permission_denied" | "self_authored" | "not_current" | "not_submitted" };
export type ApiWorkReport = {
  id: string; version: number; jobId: string; authorId: string; reviewAvailability: ReviewAvailability; items: ApiInspectionItem[]; measurements: ApiReportMeasurement[];
  parts: ApiPart[]; refrigerant: ApiRefrigerant[]; signOff: { signerName: string; signedAt: string; absentReason: string | null } | null; workText: string;
  nextAction: { kind: "none" } | { kind: "follow_up"; date: string; note: string } | null; attachmentRefs: ApiAttachment[]; submittedAt: string | null; acceptedAt: string | null;
  reviewHistory: ApiReview[];
};
/** The job the review runs on (jobs.get detail projection; offer / history projections carry no report). */
export type ApiReviewJob = {
  projection: "detail" | "offer" | "history"; id?: string; jobId?: string; version?: number; status: string; type: string; unitId?: string; alertIds?: string[];
  dueAt?: string | null; reportRefs?: { reportId: string; reportVersion: number }[]; draftReportRef?: { reportId: string; reportVersion: number } | null;
  timeOnSite?: { arrivedAt?: string | null; startedAt?: string | null; finishedAt?: string | null; onSiteMinutes?: number | null } | null;
  assignment?: { scheduledStart: string; scheduledEnd: string } | null; completedAt?: string | null;
};
export type ApiAlertLite = { id: string; type: string; severity: string; status: string; evidenceText: string; detectedAt: string };

const groupLabel: Record<ApiInspectionItem["componentGroup"], string> = { indoor: "Indoor unit", outdoor: "Outdoor unit", electrical: "Electrical" };
const componentLabel: Record<string, string> = {
  filter: "filter condition", evaporator_coil: "evaporator coil", blower_motor: "blower motor", blower_fan: "blower fan", drain_pipe: "drain pipe", drain_pan: "drain pan", outlet: "outlet",
  louver: "louver", condenser_coil: "condenser coil", compressor: "compressor", fan: "fan", blade: "fan blade", refrigerant_pipe: "refrigerant pressure", thermostat: "thermostat",
  sensor: "sensor", capacitor: "capacitor", contactor: "contactor", wiring: "wiring insulation",
};
export const resultText: Record<NonNullable<ApiInspectionItem["result"]>, { label: string; tone: "ok" | "warn" | "muted" | "crit" }> = {
  normal: { label: "OK", tone: "ok" }, attention: { label: "Attention", tone: "warn" }, not_inspected: { label: "Not inspected", tone: "crit" }, not_applicable: { label: "Not applicable", tone: "muted" },
};
/** One inspection row (Figma 03 “Indoor unit — filter condition · OK”, reason under not-applicable items). */
export type InspectionRow = { id: string; label: string; result: ApiInspectionItem["result"]; reason: string | null };
export function inspectionRows(items: ApiInspectionItem[], t: T = en): InspectionRow[] {
  const order = { indoor: 0, outdoor: 1, electrical: 2 };
  return [...items].sort((a, b) => order[a.componentGroup] - order[b.componentGroup]).map((it) => ({
    id: it.id, label: `${t(groupLabel[it.componentGroup])} — ${componentLabel[it.componentKey] ? t(componentLabel[it.componentKey]) : it.componentKey.replace(/_/g, " ")}`, result: it.result, reason: it.reason,
  }));
}

const metricLabel: Record<string, string> = { refrigerant_pressure: "Refrigerant pressure", temperature: "Supply air temperature", humidity: "Humidity", vibration: "Vibration", power: "Power draw", co2: "CO₂", pm25: "PM2.5" };
const qualityNote: Record<string, string> = { valid: "recorded", suspect: "suspect reading", missing: "no value" };
// the component in a reading's label, without the part the metric already says (“Refrigerant pressure (refrigerant)”)
const componentShort: Record<string, string> = { filter: "filter", refrigerant_pipe: "refrigerant", wiring: "wiring" };
/** Readings recorded (Figma 03): “Refrigerant pressure · 4.6 bar · within spec” from the inspection measurements. */
export function readingRows(ms: ApiReportMeasurement[], t: T = en): [string, string][] {
  const part = (key: string) => (componentShort[key] ?? componentLabel[key] ? t(componentShort[key] ?? componentLabel[key]) : key);
  return ms.map((m) => [`${metricLabel[m.metric] ? t(metricLabel[m.metric]) : m.metric}${m.componentKey ? ` (${part(m.componentKey)})` : ""}`,
    m.value === null ? t("no value") : `${m.value} ${m.unit} · ${qualityNote[m.quality] ? t(qualityNote[m.quality]) : m.quality}${m.qualityReason ? ` (${m.qualityReason.replace(/_/g, " ")})` : ""}`]);
}
/** “14 Sept, 9:05 – 11:40 am MYT (2 h 35 m)” from the job's time on site, in the display time zone (IR274). */
export function timeOnSiteText(ts: ApiReviewJob["timeOnSite"], i: I18n = EN): string {
  const { t, display } = i;
  if (!ts || !ts.arrivedAt) return t("not recorded");
  const from = ts.arrivedAt ?? ts.startedAt!;
  const span = ts.finishedAt ? showSpan(from, ts.finishedAt, display) : t("{from} – now", { from: showTime(from, display) });
  const mins = ts.onSiteMinutes ?? null;
  return mins !== null ? t("{span} ({h} h {m} m)", { span, h: Math.floor(mins / 60), m: mins % 60 }) : span;
}

const partSource: Record<string, string> = { van_stock: "from van stock", hq_warehouse: "from the HQ warehouse", bought_locally: "bought locally" };
const leakCheckText: Record<string, string> = { pass: "passed", fail: "failed", not_done: "not done" };
/** Parts & refrigerant lines (Figma 03): “Filter × 1 · from van stock · replaces filter”. */
export function partLines(parts: ApiPart[], refrigerant: ApiRefrigerant[], t: T = en): [string, string][] {
  const code = (x: string, map: Record<string, string>) => (map[x] ? t(map[x]) : x.replace(/_/g, " "));
  return [
    ...parts.map((p): [string, string] => [`${p.name} × ${p.quantity}`, [code(p.source, partSource), ...(p.replacesComponentKey ? [t("replaces {part}", { part: componentLabel[p.replacesComponentKey] ? t(componentLabel[p.replacesComponentKey]) : p.replacesComponentKey.replace(/_/g, " ") })] : [])].join(" · ")]),
    ...refrigerant.map((x): [string, string] => [t("{refrigerant} · cylinder {id}", { refrigerant: x.refrigerant, id: x.cylinderId }), t("recovered {recovered} kg · charged {charged} kg · leak check {check}", { recovered: x.recoveredKg, charged: x.chargedKg, check: code(x.leakCheck, leakCheckText) })]),
  ];
}

/** The evidence check of DD-P05 step 2: every required item recorded, every non-OK item with a reason, photos,
 * readings and the contributors of the version. `acceptable` means the two required checks hold. */
export type EvidenceRow = { label: string; value: string; tone: "ok" | "warn" | "muted" };
export function evidenceCheck(r: ApiWorkReport, names: Map<string, string>, t: T = en): { rows: EvidenceRow[]; acceptable: boolean; missing: string[] } {
  const recorded = r.items.filter((x) => x.result !== null).length;
  const needReason = r.items.filter((x) => x.result !== null && x.result !== "normal");
  const withReason = needReason.filter((x) => x.reason && x.reason.trim()).length;
  const photos = r.attachmentRefs.filter((a) => a.status === "ready").length;
  const contributors = [...new Set(r.items.map((x) => x.authorId))].map((id) => names.get(id) ?? id.slice(0, 8));
  const missing: string[] = [];
  const noResult = r.items.length - recorded, noReason = needReason.length - withReason;
  if (noResult > 0) missing.push(t(noResult === 1 ? "{n} inspection item without a result" : "{n} inspection items without a result", { n: noResult }));
  if (noReason > 0) missing.push(t(noReason === 1 ? "{n} reason missing" : "{n} reasons missing", { n: noReason }));
  return {
    rows: [
      { label: t("Required inspection items"), value: t("{n} / {total} recorded", { n: recorded, total: r.items.length }), tone: recorded === r.items.length ? "ok" : "warn" },
      { label: t("Not-applicable reasons"), value: needReason.length ? t("{n} / {total} given", { n: withReason, total: needReason.length }) : t("none needed"), tone: withReason === needReason.length ? "ok" : "warn" },
      { label: t("Photos"), value: t("{n} attached", { n: photos }), tone: photos ? "ok" : "muted" },
      { label: t("Readings"), value: t("{n} recorded", { n: r.measurements.length }), tone: r.measurements.length ? "ok" : "muted" },
      { label: t("Contributors to v{version}", { version: r.version }), value: contributors.length === 1 ? t("{name} only", { name: contributors[0] }) : contributors.join(", ") || "—", tone: "ok" },
    ],
    acceptable: missing.length === 0, missing,
  };
}

/** Why the signed-in reviewer may not decide (IR31 self-authorship, stale version, not submitted, permission). */
export function availabilityText(av: ReviewAvailability, status: string, t: T = en): string | null {
  if (av.allowed) return null;
  switch (av.reason) {
    case "self_authored": return t("You contributed to this report version, so you cannot approve or return it (IR31). Another reviewer of your company or HQ must decide.");
    case "not_current": return t("This is not the current report version — a newer version exists; the page shows the latest refs.");
    case "not_submitted": return t(status === "completed" ? "This report was accepted; the job is completed." : status === "rework_requested" ? "This version was returned for rework; wait for the technician's next version." : "The report has not been submitted yet.");
    default: return t("Reviewing needs the partner.review permission on this delegated job.");
  }
}

/** Versions & linked alert rows (Figma 03): one row per report version with its review, the alerts and the job due;
 * the times in the display time zone. */
export type VersionRow = { title: string; sub: string; badge: { label: string; tone: "primary" | "ok" | "warn" | "crit" | "muted" } | null };
export function versionRows(job: ApiReviewJob, r: ApiWorkReport, author: string, names: Map<string, string>, i: I18n = EN): VersionRow[] {
  const { t, display } = i;
  const refs = job.reportRefs ?? [];
  const rows: VersionRow[] = refs.map((ref) => {
    const rv = r.reviewHistory.filter((h) => h.reportVersion === ref.reportVersion).sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0];
    const current = ref.reportVersion === r.version;
    const by = rv ? names.get(rv.reviewerUserId) ?? t("reviewer") : "";
    if (rv?.decision === "accept") return { title: t("v{version} · accepted {time}", { version: ref.reportVersion, time: showTime(rv.occurredAt, display) }), sub: t("by {name} · visible to the customer", { name: by }), badge: { label: t("Accepted"), tone: "ok" } };
    if (rv?.decision === "return") return { title: t("v{version} · returned {time}", { version: ref.reportVersion, time: showTime(rv.occurredAt, display) }), sub: rv.reason ? t("by {name} — {reason}", { name: by, reason: rv.reason }) : t("by {name}", { name: by }), badge: { label: t("Returned"), tone: "warn" } };
    return {
      title: t("v{version} · submitted {time}", { version: ref.reportVersion, time: r.submittedAt && current ? showTime(r.submittedAt, display) : "—" }),
      sub: t(current && job.status === "submitted" ? "by {name} · reviewing now" : "by {name}", { name: author }), badge: current ? { label: t("Current"), tone: "primary" } : null,
    };
  });
  if (job.draftReportRef) rows.push({ title: t("v{version} · draft", { version: job.draftReportRef.reportVersion }), sub: t("{name} is working on the rework", { name: author }), badge: { label: t("Draft"), tone: "muted" } });
  return rows;
}
export function alertRows(alerts: ApiAlertLite[], i: I18n = EN): VersionRow[] {
  const { t, display } = i;
  return alerts.map((a) => ({
    title: t("{text} (alert)", { text: a.evidenceText || a.type }), sub: `${showTime(a.detectedAt, display)} · ${t(a.status === "resolved" ? "resolved" : "stays open after completion")}`,
    badge: { label: t(a.status === "resolved" ? "Resolved" : a.status === "acknowledged" ? "Acknowledged" : "Open"), tone: a.status === "resolved" ? "ok" : a.severity === "critical" ? "crit" : "warn" },
  }));
}
export function dueRow(job: ApiReviewJob, i: I18n = EN): VersionRow {
  const { t, display } = i;
  const a = job.assignment;
  return { title: t("Job due"), sub: a ? t("work window {span}", { span: showSpan(a.scheduledStart, a.scheduledEnd, display) }) : job.dueAt ? t("due {time}", { time: showTime(job.dueAt, display) }) : t("no window"), badge: null };
}
