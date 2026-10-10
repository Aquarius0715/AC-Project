// Contractor quality review (FR-P05, DATA_SOURCE=api): the submitted WorkReport read as inspection rows, readings,
// the evidence check of DD-P05 step 2, the version / alert rows and the IR31 review availability. Pure code shared by
// the Server Component and the client view.
import { klTime } from "@ac/web/lib/devices";
import { translator, type T } from "@ac/web/lib/i18n";

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
const hhmm = (iso: string) => klTime(iso).slice(11);
/** “09:05 – 11:40 (2 h 35 m)” from the job's time on site. */
export function timeOnSiteText(t: ApiReviewJob["timeOnSite"]): string {
  if (!t || !t.arrivedAt) return "not recorded";
  const from = t.arrivedAt ?? t.startedAt!;
  const to = t.finishedAt;
  const mins = t.onSiteMinutes ?? null;
  return `${hhmm(from)} – ${to ? hhmm(to) : "now"}${mins !== null ? ` (${Math.floor(mins / 60)} h ${mins % 60} m)` : ""}`;
}

/** The evidence check of DD-P05 step 2: every required item recorded, every non-OK item with a reason, photos,
 * readings and the contributors of the version. `acceptable` means the two required checks hold. */
export type EvidenceRow = { label: string; value: string; tone: "ok" | "warn" | "muted" };
export function evidenceCheck(r: ApiWorkReport, names: Map<string, string>): { rows: EvidenceRow[]; acceptable: boolean; missing: string[] } {
  const recorded = r.items.filter((i) => i.result !== null).length;
  const needReason = r.items.filter((i) => i.result !== null && i.result !== "normal");
  const withReason = needReason.filter((i) => i.reason && i.reason.trim()).length;
  const photos = r.attachmentRefs.filter((a) => a.status === "ready").length;
  const contributors = [...new Set(r.items.map((i) => i.authorId))].map((id) => names.get(id) ?? id.slice(0, 8));
  const missing: string[] = [];
  if (recorded < r.items.length) missing.push(`${r.items.length - recorded} inspection item${r.items.length - recorded === 1 ? "" : "s"} without a result`);
  if (withReason < needReason.length) missing.push(`${needReason.length - withReason} reason${needReason.length - withReason === 1 ? "" : "s"} missing`);
  return {
    rows: [
      { label: "Required inspection items", value: `${recorded} / ${r.items.length} recorded`, tone: recorded === r.items.length ? "ok" : "warn" },
      { label: "Not-applicable reasons", value: needReason.length ? `${withReason} / ${needReason.length} given` : "none needed", tone: withReason === needReason.length ? "ok" : "warn" },
      { label: "Photos", value: `${photos} attached`, tone: photos ? "ok" : "muted" },
      { label: "Readings", value: `${r.measurements.length} recorded`, tone: r.measurements.length ? "ok" : "muted" },
      { label: `Contributors to v${r.version}`, value: contributors.length === 1 ? `${contributors[0]} only` : contributors.join(", ") || "—", tone: "ok" },
    ],
    acceptable: missing.length === 0, missing,
  };
}

/** Why the signed-in reviewer may not decide (IR31 self-authorship, stale version, not submitted, permission). */
export function availabilityText(av: ReviewAvailability, status: string): string | null {
  if (av.allowed) return null;
  switch (av.reason) {
    case "self_authored": return "You contributed to this report version, so you cannot approve or return it (IR31). Another reviewer of your company or HQ must decide.";
    case "not_current": return "This is not the current report version — a newer version exists; the page shows the latest refs.";
    case "not_submitted": return status === "completed" ? "This report was accepted; the job is completed." : status === "rework_requested" ? "This version was returned for rework; wait for the technician's next version." : "The report has not been submitted yet.";
    default: return "Reviewing needs the partner.review permission on this delegated job.";
  }
}

/** Versions & linked alert rows (Figma 03): one row per report version with its review, the alerts and the job due. */
export type VersionRow = { title: string; sub: string; badge: { label: string; tone: "primary" | "ok" | "warn" | "crit" | "muted" } | null };
export function versionRows(job: ApiReviewJob, r: ApiWorkReport, author: string, names: Map<string, string>): VersionRow[] {
  const refs = job.reportRefs ?? [];
  const rows: VersionRow[] = refs.map((ref) => {
    const rv = r.reviewHistory.filter((h) => h.reportVersion === ref.reportVersion).sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0];
    const current = ref.reportVersion === r.version;
    const by = rv ? names.get(rv.reviewerUserId) ?? "reviewer" : null;
    if (rv?.decision === "accept") return { title: `v${ref.reportVersion} · accepted ${klTime(rv.occurredAt)}`, sub: `by ${by} · visible to the customer`, badge: { label: "Accepted", tone: "ok" } };
    if (rv?.decision === "return") return { title: `v${ref.reportVersion} · returned ${klTime(rv.occurredAt)}`, sub: `by ${by}${rv.reason ? ` — ${rv.reason}` : ""}`, badge: { label: "Returned", tone: "warn" } };
    return { title: `v${ref.reportVersion} · submitted ${r.submittedAt && current ? klTime(r.submittedAt) : "—"}`, sub: `by ${author}${current && job.status === "submitted" ? " · reviewing now" : ""}`, badge: current ? { label: "Current", tone: "primary" } : null };
  });
  if (job.draftReportRef) rows.push({ title: `v${job.draftReportRef.reportVersion} · draft`, sub: `${author} is working on the rework`, badge: { label: "Draft", tone: "muted" } });
  return rows;
}
export function alertRows(alerts: ApiAlertLite[]): VersionRow[] {
  return alerts.map((a) => ({
    title: `${a.evidenceText || a.type} (alert)`, sub: `${klTime(a.detectedAt).slice(5)} · ${a.status === "resolved" ? "resolved" : "stays open after completion"}`,
    badge: { label: a.status === "resolved" ? "Resolved" : a.status === "acknowledged" ? "Acknowledged" : "Open", tone: a.status === "resolved" ? "ok" : a.severity === "critical" ? "crit" : "warn" },
  }));
}
export function dueRow(job: ApiReviewJob): VersionRow {
  const a = job.assignment;
  return { title: "Job due", sub: a ? `work window ${klTime(a.scheduledStart).slice(5)} → ${klTime(a.scheduledEnd).slice(5)}` : job.dueAt ? `due ${klTime(job.dueAt)}` : "no window", badge: null };
}
