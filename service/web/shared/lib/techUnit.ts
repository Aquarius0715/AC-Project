// The technician's unit screen (FR-T02, FR-T03, DD-T02, DD-T03, SCR-T02, Figma Technician 02-10…02-13) from the Core
// API: the unit register with “Not registered” for what is missing (DD-T02), the components of its service scope, the
// technician's jobs on it, its open alerts, the latest values with their times — last known, never live, while the
// connection is down (DD-T03) — and one metric over 1 h or 24 h (rolling) or 7 days (Kuala Lumpur calendar days) with
// the other metrics and the events of that period. Pure code shared by the server loader and the view; Vitest covers
// it. Texts in the display language (`i`, IR283); instants in the user's display time zone (IR44).
import { EN, relativeTime, showClock, showDate, showDayMonth, showSpan, type I18n, type T, textOf, byCount } from "@ac/web/lib/i18n";
import { evidenceText, resolutionText } from "@ac/web/lib/evidence";
import { alertTitle, type ApiAlert } from "@ac/web/lib/alerts";
import { metricLabel } from "@ac/web/lib/adminAlerts";
import { airNumber } from "@ac/web/lib/air";
import { bucket, type ApiUnitDetail } from "@ac/web/lib/units";
import { componentGroups, componentLabel, groupLabel, type Group } from "@ac/web/lib/techJob";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import { oneOf } from "@ac/web/lib/oneOf";
import type { Metric } from "@ac/web/lib/contracts.gen";
import { wrapped } from "@ac/web/lib/text";
import { entryOf, lookup } from "@ac/web/lib/lookup";

const TEMPERATURE = "temperature";

/** units.get as the technician reads it (ACUnit.type is part of the unit). */
export type ApiTechUnit = ApiUnitDetail & { type?: string | null };
export interface Point { value: number | null; observedAt: string; quality?: string }
type Tone = "ok" | "warn" | "crit" | "primary" | "muted";

const KL = 8 * 3_600_000, DAY = 86_400_000, HOUR = 3_600_000;
export const PERIODS = ["1h", "24h", "7d"] as const;
export type Period = (typeof PERIODS)[number];
export const periodOf = (v = ""): Period => oneOf(PERIODS, v, "24h");
/** The period of the series (DD-T03): 1 h and 24 h roll back from now; 7 days are the Kuala Lumpur calendar days up to
today. */
export function periodRange(p: Period, nowMs: number): { from: number; to: number } {
  if (p === "1h") return { from: nowMs - HOUR, to: nowMs };
  if (p === "24h") return { from: nowMs - DAY, to: nowMs };
  const today = Math.floor((nowMs + KL) / DAY) * DAY - KL; // 00:00 in Kuala Lumpur
  return { from: today - 6 * DAY, to: nowMs };
}
/** The metric the series shows: the URL's when the unit measures it, else temperature, else its first measured one. */
export function metricOf(d: ApiTechUnit, v?: string): Metric {
  const measured = d.latestMeasurements.map((m) => m.metric);
  const asked = measured.find((m) => m === v);
  if (asked) return asked;
  return measured.includes(TEMPERATURE) ? TEMPERATURE : measured[0] ?? TEMPERATURE;
}
const label = (metric: string, t: T) => (textOf(metricLabel, metric, t) ?? metric.replaceAll('_', " "));

const SCOPE: Record<string, string> = { indoor: "Indoor", outdoor: "Outdoor", electrical: "Electrical" };
const TYPE: Record<string, string> = { split: "Split" };
const COUNT: Record<Group, string> = { indoor: "{n} indoor", outdoor: "{n} outdoor", electrical: "{n} electrical" };
export interface Register { rows: [string, string][]; missing: boolean; note: string }
/** The unit register (Figma: location, manufacturer / model, configuration, installed, capability version, maintenance
scope). A value the register lacks says “Not registered” — never a similar model's value or today's date (DD-T02). */
export function registerRows(d: ApiTechUnit, i: I18n = EN): Register {
  const { t, display } = i;
  const none = t("Not registered");
  const rows: [string, string][] = [
    [t("Location"), d.location.pathLabels.join(" › ") || none],
    [t("Manufacturer / model"), d.capabilities.manufacturer && d.capabilities.model ? `${d.capabilities.manufacturer} / ${d.capabilities.model}` : none],
    [t("Configuration"), d.type ? (textOf(TYPE, d.type, t) ?? d.type) : none],
    [t("Installed"), d.installedAt ? showDate(d.installedAt, display) : none],
    [t("Capability version"), String(d.capabilityVersion)],
    [t("Maintenance scope"), d.serviceScope.map((s) => (textOf(SCOPE, s, t) ?? s)).join(" · ") || none],
  ];
  if (d.location.accessInstructions) rows.push([t("Access"), d.location.accessInstructions]);
  const groups = componentGroups.map(({ group, keys }) => ({ group, n: keys.filter((k) => d.components.includes(k)).length })).filter((g) => g.n);
  const n = groups.reduce((s, g) => s + g.n, 0), list = groups.map((g) => t(COUNT[g.group], { n: g.n })).join(", ");
  return {
    rows, missing: rows.some(([, v]) => v === none),
    note: t(groups.length === 1 ? "{n} components in 1 group ({list}) — inspect them in the job workspace." : "{n} components across {g} groups ({list}) — inspect them in the job workspace.", { g: groups.length, n, list }),
  };
}

export interface ComponentCard { group: Group; title: string; names: string }
/** The components of the service scope by group (IR100), in the canonical order. */
export function componentCards(d: ApiTechUnit, i: I18n = EN): ComponentCard[] {
  const { t } = i;
  return componentGroups.map(({ group, keys }) => ({ group, keys: keys.filter((k) => d.components.includes(k)) })).filter((g) => g.keys.length).map(({ group, keys }) => {
    return {
      group, title: t("{group} · {n} components", { group: t(entryOf(groupLabel, group)), n: keys.length }), names: keys.map((k) => t(lookup(componentLabel, k) ?? k)).join(", "),
    };
  });
}

export interface Tile { label: string; value: string; sub: string; tone?: "crit" | "warn" }
const QUALITY: Record<string, string> = { stale: "stale", suspect: "suspect", missing: "missing" };
const MODE: Record<string, string> = { cool: "Cooling", dry: "Drying", fan: "Fan only" };
const CONNECTION: Record<string, string> = { online: "Online", offline: "Offline", unknown: "Unknown", connecting: "Connecting", error: "Error" };
/** What the AC is doing: off, its mode when known, on, or unknown. */
function operationText(s: ApiTechUnit["observedState"], t: T): string {
  if (s.power === null) return "—";
  if (!s.power) return t("Off");
  return textOf(MODE, s.mode, t) ?? t("On");
}
/** The latest values (Figma tiles): temperature, power, operation and connection, each with its time — “last known”
while the unit is not online, so nothing reads as live (DD-T03). A reading of stale, suspect or missing quality keeps
its quality beside the value (IR213). */
export function liveTiles(d: ApiTechUnit, nowMs: number, i: I18n = EN): Record<"temperature" | "power" | "operation" | "connection", Tile> {
  const { t } = i;
  const isDown = d.connection !== "online";
  const seenText = isDown ? "last known · observed {time}" : "observed {time}";
  const seen = (iso: string | null) => (iso ? t(seenText, { time: relativeTime(iso, nowMs, i) }) : "—");
  const reading = (metric: string): Tile => {
    const m = d.latestMeasurements.find((x) => x.metric === metric);
    if (!m) return { label: label(metric, t), value: "—", sub: t("no reading") };
    if (m.value === null) return { label: label(metric, t), value: "—", sub: seen(m.observedAt) };
    const isBad = m.quality !== "valid";
    return { label: label(metric, t), value: `${airNumber(metric, m.value)} ${m.unit}${isBad ? ` (${t(lookup(QUALITY, m.quality) ?? m.quality)})` : ""}`, sub: seen(m.observedAt), tone: isBad ? "warn" : undefined };
  };
  const s = d.observedState;
  return {
    temperature: reading(TEMPERATURE), power: reading("power"),
    operation: { label: t("Operation"), value: operationText(s, t), sub: seen(s.observedAt) },
    connection: {
      label: t("Connection"), value: t(lookup(CONNECTION, d.connection) ?? d.connection), tone: isDown ? "crit" : undefined,
      sub: d.lastSeenAt ? t("last seen {time}", { time: relativeTime(d.lastSeenAt, nowMs, i) }) : t("never seen"),
    },
  };
}
/** The banner while the connection is down: updates stopped at the last heartbeat; the values shown are the last known. */
export function stoppedBanner(d: ApiTechUnit, nowMs: number, i: I18n = EN): string | null {
  if (d.connection === "online") return null;
  return d.lastSeenAt ? i.t("Updates stopped — communication lost {time}. Showing last known values with their times; nothing here is real-time.", { time: relativeTime(d.lastSeenAt, nowMs, i) })
    : i.t("No data has reached the server from this unit yet — nothing here is real-time.");
}

export interface SeriesChart { title: string; points: (number | null)[]; labels: string[]; gap: string | null; count: string | null; cut: string | null; empty: boolean }
/** The chart's first label: the Kuala Lumpur day for 7 days, the day and time for 24 h, the time for 1 h. */
function windowStart(at: string, p: Period, display: I18n["display"]): string {
  if (p === "7d") return showDayMonth(at, { ...display, timeZone: "Asia/Kuala_Lumpur" });
  return p === "24h" ? `${showDayMonth(at, display)}, ${showClock(at, display, false)}` : showClock(at, display, false);
}
/** One metric over the period: 12 equal slots with the latest valid reading in each (stale, suspect and missing ones
are not plotted), the empty slots between readings named as a gap that stays unconnected. A `complete` series — the
period's averages (IR606) — counts the period's readings (`total`); otherwise, when the period holds more readings
than the screen loaded, it says that the earlier ones are not loaded instead of counting the period (IR598, as IR592
on the customer's air chart). The 24 h axis starts with its day. */
export function seriesChart(metric: string, unit: string, items: Point[], p: Period, nowMs: number, i: I18n = EN, total = items.length, complete = false): SeriesChart {
  const { t, display } = i;
  const { from, to } = periodRange(p, nowMs);
  const points = bucket(items, new Date(from), new Date(to));
  const slot = (to - from) / points.length;
  const first = points.findIndex((v) => v !== null), last = points.findLastIndex((v) => v !== null);
  let gap: string | null = null;
  for (let k = first + 1; first !== -1 && k < last && !gap; k++) {
    if (points.at(k) !== null) continue;
    let end = k;
    while (points[end + 1] === null) end++;
    gap = t("no data {span} (gap not connected)", { span: showSpan(new Date(from + k * slot).toISOString(), new Date(from + (end + 1) * slot).toISOString(), display) });
  }
  const at = new Date(from).toISOString();
  const start = windowStart(at, p, display);
  const isCut = !complete && total > items.length;
  const n = complete ? total : items.length;
  const name = `${label(metric, t)}${wrapped(unit, " (", ")")}`;
  return {
    title: p === "7d" ? t("{metric} · last 7 days (Kuala Lumpur calendar days)", { metric: name }) : t("{metric} · last {period} (rolling)", { metric: name, period: t(p === "1h" ? "1 h" : "24 h") }),
    labels: [start, "", "", "", t("now")],
    empty: points.every((v) => v === null),
    count: isCut ? null : byCount(n, { one: t("1 measurement in this period"), many: t("{n} measurements in this period", { n }) }),
    cut: isCut ? t("Showing the latest {n} of {total} readings — earlier readings in this period are not loaded.", { n: items.length, total }) : null,
    points,
    gap,
  };
}

export interface MiniMetric { metric: string; label: string; value: string; sub: string; points: (number | null)[]; cut: boolean; tone?: "warn" }
/** A reading's quality beside its value, when it is not valid. */
const qualityNote = (quality: string, t: T) => (quality === "valid" ? "" : ` (${textOf(QUALITY, quality, t) ?? quality})`);
/** Up to three other metrics of the unit with their latest value and the period's slots (Figma: humidity, power,
supply air); `cut` when the period holds more readings (`totals`) than were loaded (IR598). */
export function otherMetrics(d: ApiTechUnit, shown: string, series: Record<string, Point[]>, p: Period, nowMs: number, i: I18n = EN, totals: Record<string, number> = {}): MiniMetric[] {
  const { t } = i;
  const { from, to } = periodRange(p, nowMs);
  return d.latestMeasurements.filter((m) => m.metric !== shown).slice(0, 3).map((m) => {
    return {
      metric: m.metric, label: label(m.metric, t), tone: m.quality === "valid" ? undefined : "warn" as const,
      value: m.value === null ? "—" : `${airNumber(m.metric, m.value)} ${m.unit}${qualityNote(m.quality, t)}`,
      sub: t(d.connection === "online" ? "observed {time}" : "last known · observed {time}", { time: relativeTime(m.observedAt, nowMs, i) }),
      points: bucket(lookup(series, m.metric) ?? [], new Date(from), new Date(to)), cut: (lookup(totals, m.metric) ?? 0) > (series[m.metric]?.length ?? 0),
    };
  });
}

export interface WindowEvent { id: string; at: string; time: string; title: string; sub: string; badge: { text: string; tone: Tone } }
const INFO = { text: "Info", tone: "primary" as Tone }; // an alert of an unknown severity reads as information
const SEVERITY: Record<string, { text: string; tone: Tone }> = { critical: { text: "Critical", tone: "crit" }, warning: { text: "Warning", tone: "warn" }, normal: { text: "Info", tone: "primary" } };
const DEVICE: Record<ApiDeviceEventFull["eventType"], [string, string, string, Tone]> = {
  communication_lost: ["Updates stopped", "missed heartbeats — the values are frozen and not shown as live", "Connectivity", "primary"],
  power_lost: ["Power signal lost", "the demo power signal dropped", "Power", "crit"],
  tamper: ["Cover opened", "the device cover was opened while powered", "Tamper", "warn"],
  restored: ["Data resumed", "connection restored — the gap stays unconnected", "Connectivity", "primary"],
  operation_failed: ["Device operation failed", "a device operation did not complete", "Device", "muted"],
};
const RESTORED: Record<string, [string, string, string]> = {
  power: ["Power signal restored", "the demo power signal is back", "Power"],
  tamper: ["Tamper cleared", "the cover is closed — the alert stays until it is handled", "Tamper"],
};
/** The events of the period, newest first: an alert raised, acknowledged or resolved on the unit, and its device's connection, power and tamper events. */
export function windowEvents(alerts: ApiAlert[], events: ApiDeviceEventFull[], p: Period, nowMs: number, i: I18n = EN): WindowEvent[] {
  const { t } = i;
  const { from, to } = periodRange(p, nowMs);
  const inside = (iso?: string | null): iso is string => !!iso && Date.parse(iso) >= from && Date.parse(iso) <= to;
  const out: Omit<WindowEvent, "time">[] = [];
  for (const a of alerts) {
    const title = alertTitle(a, t), sev = lookup(SEVERITY, a.severity) ?? INFO;
    if (inside(a.detectedAt)) out.push({ id: `${a.id}:raised`, at: a.detectedAt, title: t("Alert raised — {title}", { title }), sub: evidenceText(a.evidenceText, t), badge: { text: t(sev.text), tone: sev.tone } });
    if (inside(a.acknowledgedAt)) out.push({ id: `${a.id}:acknowledged`, at: a.acknowledgedAt, title: t("Alert acknowledged — {title}", { title }), sub: "", badge: { text: t("Acknowledged"), tone: "primary" } });
    if (inside(a.resolvedAt)) out.push({ id: `${a.id}:resolved`, at: a.resolvedAt, title: t("Alert resolved — {title}", { title }), sub: a.resolutionReason ? resolutionText(a.resolutionReason, t) : "", badge: { text: t("Resolved"), tone: "ok" } });
  }
  for (const e of events) {
    if (!inside(e.occurredAt)) continue;
    const back = e.eventType === "restored" && e.recovery ? RESTORED[e.recovery.axis] : undefined;
    const [title, sub, kind, tone]: [string, string, string, Tone] = back ? [back[0], back[1], back[2], "ok"] : DEVICE[e.eventType];
    out.push({ id: e.id, at: e.occurredAt, title: t(title), sub: t(sub), badge: { text: t(kind), tone } });
  }
  return out.toSorted((x, y) => y.at.localeCompare(x.at) || x.id.localeCompare(y.id)).map((x) => ({ ...x, time: relativeTime(x.at, nowMs, i) }));
}

export interface HistoryRow { id: string; title: string; sub: string; badge: { text: string; tone: Tone } }
const CURRENT_TONE: Record<string, Tone> = { assigned: "primary", in_progress: "warn", rework_requested: "warn", on_hold: "warn", submitted: "primary", completed: "ok" };
/** The technician's jobs on the unit (Figma: maintenance history): the current ones with their visit, then the ended
ones with the day they were completed, newest first. */
export function jobRows(jobs: ApiTechJobRow[], i: I18n = EN): HistoryRow[] {
  const { t, display } = i;
  const current = jobs.filter((j): j is Extract<ApiTechJobRow, { projection: "summary" }> => j.projection === "summary")
    .toSorted((a, b) => (b.scheduledSlot?.startAt ?? b.dueAt).localeCompare(a.scheduledSlot?.startAt ?? a.dueAt))
    .map((j): HistoryRow => {
      return {
        id: j.id, title: `${j.id.slice(0, 8)} · ${typeLabel(j.type, t)}`, sub: j.scheduledSlot ? showSpan(j.scheduledSlot.startAt, j.scheduledSlot.endAt, display) : t("not scheduled yet"),
        badge: { text: statusWord(j.status, t), tone: lookup(CURRENT_TONE, j.status) ?? "muted" },
      };
    });
  const ended = jobs.filter((j): j is Extract<ApiTechJobRow, { projection: "history" }> => j.projection === "history")
    .toSorted((a, b) => (b.completedAt ?? "").localeCompare(a.completedAt ?? ""))
    .map((j): HistoryRow => {
      return {
        id: j.jobId, title: `${j.jobId.slice(0, 8)} · ${typeLabel(j.type, t)}`, sub: j.completedAt ? t("completed {date}", { date: showDate(j.completedAt, display) }) : t("your assignment ended"),
        badge: { text: statusWord(j.status, t), tone: j.status === "completed" ? "ok" : "muted" },
      };
    });
  return [...current, ...ended];
}

const ACTIVE: Record<string, number> = { in_progress: 0, assigned: 1, rework_requested: 2 };
/** The job diagnostic control works for (DD-T10, IR94): the URL's job when it is one of the technician's active jobs on
the unit, else the one in progress, then assigned or returned for rework; null without one. */
export function controlJob(jobs: ApiTechJobRow[], jobId?: string): string | null {
  const active = jobs.filter((j): j is Extract<ApiTechJobRow, { projection: "summary" }> => j.projection === "summary" && j.status in ACTIVE).toSorted((a, b) => (lookup(ACTIVE, a.status) ?? 9) - (lookup(ACTIVE, b.status) ?? 9));
  return (active.find((j) => j.id === jobId) ?? active[0])?.id ?? null;
}

export interface OpenAlert { id: string; title: string; severity: ApiAlert["severity"]; sub: string; ack: string }
const RANK: Record<string, number> = { critical: 0, warning: 1, normal: 2 };
/** The unit's unresolved alerts, worst and newest first, each with its evidence and who acknowledged it — finishing the job never resolves one. */
export function openAlerts(alerts: ApiAlert[], nowMs: number, i: I18n = EN): OpenAlert[] {
  const { t } = i;
  return alerts.filter((a) => a.status !== "resolved").toSorted((a, b) => (lookup(RANK, a.severity) ?? 9) - (lookup(RANK, b.severity) ?? 9) || b.detectedAt.localeCompare(a.detectedAt)).map((a) => {
    return {
      id: a.id, title: alertTitle(a, t), severity: a.severity, sub: `${evidenceText(a.evidenceText, t)} · ${relativeTime(a.detectedAt, nowMs, i)}`,
      ack: `${a.acknowledgedAt ? t("Acknowledged {time}.", { time: relativeTime(a.acknowledgedAt, nowMs, i) }) : t("Acknowledged by nobody yet.")} ${t("Completing the job does not resolve it.")}`,
    };
  });
}
