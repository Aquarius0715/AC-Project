// The contractor's unit view (FR-P04, FR-P08, DD-P04, Figma Contractor 04-x Unit View) from the Core API: the unit
// register, the accepted job's context with its access window, the company's past work on the unit, the alert
// evidence, the latest readings and the last 24 hours of two diagnosis metrics — read only, no control — and, after
// the delegation, the job's history snapshot. Pure code shared by the server loader and the view; Vitest covers it.
// Texts in the display language (`t` / `i`, IR280); every time in the user's display time zone (IR44).
import { EN, showClock, showDate, showSpan, showTime, type I18n, type T } from "@ac/web/lib/i18n";
import { alertTitle, type ApiAlert } from "@ac/web/lib/alerts";
import { metricLabel } from "@ac/web/lib/adminAlerts";
import { airNumber } from "@ac/web/lib/air";
import { until, type Slot } from "@ac/web/lib/partnerOverview";
import { statusWord, typeLabel, type ApiHistory } from "@ac/web/lib/partnerJobDetail";
import { bucket, type ApiUnitDetail } from "@ac/web/lib/units";

/** jobs.list rows on the unit (the company's own: summaries inside the access window, history after it). */
export type ApiUnitJob =
  | { projection: "summary"; id: string; type: string; status: string; scheduledSlot: Slot | null; technicianMembershipId: string | null; accessValidFrom?: string | null; accessValidUntil?: string | null }
  | { projection: "history"; jobId: string; type: string; status: string; completedAt: string | null }
  | { projection: "offer"; jobId: string; type: string; status: string };

const SCOPE: Record<string, string> = { indoor: "Indoor unit", outdoor: "Outdoor unit", electrical: "Electrical / control" };
const QUALITY: Record<string, { text: string; tone?: "warn" | "crit" }> = { valid: { text: "good" }, stale: { text: "stale", tone: "warn" }, suspect: { text: "suspect", tone: "crit" }, missing: { text: "missing", tone: "warn" } };
const EVIDENCE: Record<string, string> = { inferred: "Suspected", inspection: "Inspection record", demo_observation: "Observed (demo)" };
/** The diagnosis metrics a chart shows first, in order (Figma: vibration and refrigerant pressure). */
export const CHART_METRICS = ["vibration", "refrigerant_pressure", "temperature", "humidity", "power"];
const label = (metric: string, t: T) => (metricLabel[metric] ? t(metricLabel[metric]) : metric.replace(/_/g, " "));

/** The unit register (Figma: location, model, maintenance scope; the connection — a badge — and the last seen time). */
export function registerRows(d: ApiUnitDetail, i: I18n = EN): { connection: ApiUnitDetail["connection"]; rows: [string, string][]; lastSeen: string } {
  const { t, display } = i;
  return {
    connection: d.connection, lastSeen: showTime(d.lastSeenAt, display),
    rows: [
      [t("Location"), d.location.pathLabels.join(" › ") || "—"], [t("Model"), `${d.capabilities.manufacturer} ${d.capabilities.model}`],
      [t("Maintenance scope"), d.serviceScope.map((s) => (SCOPE[s] ? t(SCOPE[s]) : s)).join(" · ") || "—"],
    ],
  };
}

/** The job the page is for: the URL's job when it is one of the company's current jobs on the unit, else the first. */
export const contextJob = (jobs: ApiUnitJob[], jobId?: string) => {
  const current = jobs.filter((j): j is Extract<ApiUnitJob, { projection: "summary" }> => j.projection === "summary");
  return current.find((j) => j.id === jobId) ?? current[0] ?? null;
};

export type JobContext = { id: string; rows: [string, string][]; techMissing: boolean; pct: number | null; note: string };
/** The job context (Figma): the job and its type, the technician or “Not assigned yet”, the access window with its
 * elapsed share and when access ends. */
export function jobContext(j: Extract<ApiUnitJob, { projection: "summary" }>, names: Map<string, string>, nowMs: number, i: I18n = EN): JobContext {
  const { t, display } = i;
  const tech = j.technicianMembershipId ? names.get(j.technicianMembershipId) ?? t("technician") : null;
  const from = j.accessValidFrom ?? null, to = j.accessValidUntil ?? null;
  const span = from && to ? Date.parse(to) - Date.parse(from) : 0;
  const pct = span > 0 ? Math.min(100, Math.max(0, Math.round(((nowMs - Date.parse(from!)) / span) * 100))) : null;
  return {
    id: j.id, techMissing: !tech, pct,
    rows: [[t("Job"), `${j.id.slice(0, 8)} · ${typeLabel(j.type, t)}`], [t("Technician"), tech ?? t("Not assigned yet")], [t("Access window"), from && to ? showSpan(from, to, display) : "—"]],
    note: to && Date.parse(to) > nowMs ? t("Access ends in {left}. After that, live values are cleared and only a history snapshot remains.", { left: until(to, nowMs, t) })
      : t("Access ends with the accepted offer’s access window; after that only a history snapshot remains."),
  };
}

/** The company's past work on the unit: its history snapshots, newest first, with the completion day. */
export function pastWork(jobs: ApiUnitJob[], i: I18n = EN) {
  const { t, display } = i;
  return jobs.filter((j): j is Extract<ApiUnitJob, { projection: "history" }> => j.projection === "history")
    .sort((a, b) => (b.completedAt ?? "").localeCompare(a.completedAt ?? ""))
    .map((j) => ({
      id: j.jobId, title: `${j.jobId.slice(0, 8)} · ${typeLabel(j.type, t)}`, sub: j.completedAt ? showDate(j.completedAt, display) : t("not completed"),
      badge: { text: statusWord(j.status, t), tone: (j.status === "completed" ? "ok" : "muted") as "ok" | "muted" },
    }));
}

/** The alert evidence: the title, the kind of evidence with its text and when it was detected. */
export function evidenceRows(alerts: ApiAlert[], i: I18n = EN) {
  const { t, display } = i;
  return [...alerts].sort((a, b) => b.detectedAt.localeCompare(a.detectedAt)).map((a) => ({
    id: a.id, title: alertTitle(a, t), severity: a.severity, resolved: a.status === "resolved",
    sub: `${t(EVIDENCE[a.evidenceKind] ?? "Evidence")} — ${a.evidenceText} · ${showTime(a.detectedAt, display)}`,
  }));
}

/** The latest readings (diagnosis only) with their quality in words, and the power state. */
export function readingRows(d: ApiUnitDetail, i: I18n = EN): { rows: { label: string; value: string; tone?: "warn" | "crit" }[]; observed: string } {
  const { t, display } = i;
  const rows = d.latestMeasurements.map((m) => {
    const q = QUALITY[m.quality] ?? { text: m.quality };
    return { label: label(m.metric, t), value: m.value === null ? `— · ${t(q.text)}` : `${airNumber(m.metric, m.value)} ${m.unit} · ${t(q.text)}`, tone: q.tone };
  });
  const s = d.observedState;
  if (s.power !== null) rows.push({ label: t("Power state"), value: s.power ? `${t("On")}${s.mode ? ` · ${t(s.mode === "cool" ? "cooling" : s.mode === "dry" ? "drying" : "fan only")}` : ""}` : t("Off"), tone: undefined });
  return { rows, observed: s.observedAt ? showTime(s.observedAt, display) : "—" };
}

/** The metrics the 24-hour charts show: two of the unit's latest readings, diagnosis metrics first. */
export const chartMetrics = (d: ApiUnitDetail) => CHART_METRICS.filter((m) => d.latestMeasurements.some((x) => x.metric === m)).slice(0, 2);
export type Chart = { metric: string; title: string; now: string; labels: string[]; values: number[] };
/** One 24-hour chart: 12 two-hour buckets (the latest valid value in each, 0 where none) labelled with the bucket's
 * start in the display time zone. */
export function chart(metric: string, unit: string, items: { value: number | null; observedAt: string; quality?: string }[], nowMs: number, i: I18n = EN): Chart {
  const { t, display } = i;
  const from = new Date(nowMs - 86_400_000), to = new Date(nowMs);
  const values = bucket(items, from, to, 12);
  const latest = [...values].reverse().find((v) => v !== null);
  return {
    metric, title: `${label(metric, t)} (${unit})`, now: latest === undefined || latest === null ? t("no reading") : t("now {value}", { value: airNumber(metric, latest) }),
    labels: values.map((_, k) => showClock(new Date(from.getTime() + k * 2 * 3600_000).toISOString(), display, false)), values: values.map((v) => v ?? 0),
  };
}

/** The history snapshot after the delegation (Figma: ended while open): the company's decision, the work, the report
 * and that live values are gone. */
export function snapshotRows(h: ApiHistory, i: I18n = EN): [string, string][] {
  const { t, display } = i;
  const decision = [...h.ownDecisionEvents].filter((e) => e.action === "offer.accepted" || e.action === "offer.declined").sort((a, b) => b.occurredAt.localeCompare(a.occurredAt))[0];
  const report = (h.redactedReportSummary ?? null) as { hasReport?: boolean; acceptance?: string } | null;
  return [
    [t("Your decision"), decision ? `${t(decision.action === "offer.accepted" ? "Accepted" : "Declined")} · ${showTime(decision.occurredAt, display)}` : "—"],
    [t("Work"), `${statusWord(h.status, t)}${h.completedAt ? ` · ${showDate(h.completedAt, display)}` : ""}`],
    [t("Report"), report?.hasReport ? t(report.acceptance === "accepted" ? "Submitted · accepted" : "Submitted · not accepted") : t("No report")],
    [t("Live unit values"), t("Not available after the period")],
  ];
}
