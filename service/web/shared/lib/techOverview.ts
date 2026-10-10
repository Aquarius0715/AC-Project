// The technician overview (FR-T01, DD-T01, Figma Technician 01-1…01-5) from the Core API: the KPI tiles of
// summaries.get, the assigned jobs of today or all of them with their state (not accepted yet, read-only until the
// work window opens, in progress, ended window, returned, submitted), today's timeline, the alerts on the assigned
// units and the reports by state. Pure code shared by the server loader and the client view; Vitest covers it.
// Texts in the display language (`t` / `i`, IR281). Today and its 08:00–18:00 timeline are Kuala Lumpur, where the
// work happens; a job's window and an alert's time are instants in the user's display time zone (IR44).
import { klTime } from "@ac/web/lib/devices";
import { EN, relativeTime, showSpan, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { alertTitle, type ApiAlert } from "@ac/web/lib/alerts";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { Slot } from "@ac/web/lib/partnerOverview";

const en = translator("en");

/** A technician's jobs.list row (IR23): the summary while the viewing window is open, the history after it. */
export type ApiTechJobRow =
  | { projection: "summary"; id: string; version: number; unitId: string; type: string; status: string; severity: "normal" | "warning" | "critical"; dueAt: string; requestedSlot: Slot; scheduledSlot: Slot | null; origin?: string; assignmentAcknowledgement?: "pending" | "accepted" | "cant_make" | null }
  | { projection: "history"; jobId: string; type: string; status: string; completedAt: string | null };
export type ApiTechCounts = { total: number; scheduledCount: number; inProgressCount: number; overdueCount: number; assignedCount: number };

export const TABS = [{ id: "today", label: "Today" }, { id: "all", label: "All assigned" }] as const;
export type TechTab = (typeof TABS)[number]["id"];
export const tabOf = (v?: string): TechTab => (v === "all" ? "all" : "today");
export const SORTS = [
  { id: "severity", text: "Severity ↓", word: "severity", field: "severity", direction: "desc" }, { id: "dueAt", text: "Deadline ↑", word: "deadline", field: "dueAt", direction: "asc" },
  { id: "status", text: "Progress ↑", word: "progress", field: "status", direction: "asc" },
] as const;
export type TechSort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): TechSort => (SORTS.some((s) => s.id === v) ? (v as TechSort) : "severity");

type Tone = "ok" | "warn" | "crit" | "primary" | "muted";
const KL = 8 * 3600_000, DAY = 86_400_000;
const klDate = (ms: number) => new Date(ms + KL).toISOString().slice(0, 10);
const hhmm = (iso: string) => klTime(iso).slice(11);
/** Whether a slot touches today's Kuala Lumpur calendar date. */
export function isToday(slot: Slot | null, now: number): boolean {
  if (!slot) return false;
  const start = Date.parse(`${klDate(now)}T00:00:00Z`) - KL, end = start + DAY;
  return Date.parse(slot.startAt) < end && start < Date.parse(slot.endAt);
}
/** “14:00–16:00” today, “09-22 10:00–12:00” on another day, “09-14 08:00 → 09-20 08:00” across days — Kuala Lumpur
 * clock times, for the 08:00–18:00 timeline and the screens not on the display time zone yet. */
export function windowText(s: Slot, now: number): string {
  const d1 = klDate(Date.parse(s.startAt)), d2 = klDate(Date.parse(s.endAt));
  if (d1 !== d2) return `${klTime(s.startAt).slice(5)} → ${klTime(s.endAt).slice(5)}`;
  return d1 === klDate(now) ? `${hhmm(s.startAt)}–${hhmm(s.endAt)}` : `${klTime(s.startAt).slice(5, 10)} ${hhmm(s.startAt)}–${hhmm(s.endAt)}`;
}

export type TechJobRow = {
  id: string; short: string; title: string; origin: string | null; line: string; badge: { text: string; tone: Tone }; today: boolean;
  slot: Slot | null; slotText: string; ackPending: boolean; opensAt: string | null; opens: string | null; status: string; severity: string;
};
/** One assigned job: what the technician can do now, read from the status, the acknowledgement and the work window. */
export function techRow(j: ApiTechJobRow, now: number, units: Map<string, string>, i: I18n = EN): TechJobRow | null {
  const { t, display } = i;
  if (j.projection === "history") return null; // past assignments are not part of today's work
  const s = j.scheduledSlot;
  const before = !!s && Date.parse(s.startAt) > now;
  const ended = !!s && Date.parse(s.endAt) <= now && (j.status === "assigned" || j.status === "in_progress");
  const ack = j.assignmentAcknowledgement ?? null;
  const opens = before && s ? relativeTime(s.startAt, now, i) : null;
  let badge: TechJobRow["badge"], note: string;
  if (j.status === "assigned" && ack === "pending") [badge, note] = [{ text: t("Not accepted"), tone: "warn" }, t("new assignment — review and accept it")];
  else if (j.status === "assigned" && ack === "cant_make") [badge, note] = [{ text: t("Can’t make it"), tone: "warn" }, t("you said you can’t make it — waiting for your contractor or HQ")];
  else if (ended) [badge, note] = [{ text: t("Work window ended"), tone: "crit" }, t("ask your contractor or HQ to extend or reassign (IR89)")];
  else if (j.status === "assigned") [badge, note] = [{ text: t("Work not started"), tone: "primary" }, opens ? t("opens {time} (read-only until then)", { time: opens }) : t("check in to start")];
  else if (j.status === "in_progress") [badge, note] = [{ text: t("In progress"), tone: "warn" }, t("report draft open")];
  else if (j.status === "on_hold") [badge, note] = [{ text: t("On hold"), tone: "warn" }, t("held by HQ — actions are blocked")];
  else if (j.status === "rework_requested") [badge, note] = [{ text: t("Rework requested"), tone: "warn" }, t("returned — resume in a new draft version")];
  else if (j.status === "submitted") [badge, note] = [{ text: t("Submitted"), tone: "primary" }, t("awaiting quality review (read-only)")];
  else if (j.status === "completed") [badge, note] = [{ text: t("Completed"), tone: "ok" }, t("accepted in the quality review")];
  else [badge, note] = [{ text: statusWord(j.status, t), tone: "muted" }, ""];
  const alert = j.severity === "critical" ? t("critical alert on the unit") : j.severity === "warning" ? t("warning on the unit") : "";
  const slotText = s ? showSpan(s.startAt, s.endAt, display) : "";
  return {
    id: j.id, short: j.id.slice(0, 8), title: units.get(j.unitId) ?? t("Unit"), origin: j.origin ?? null, status: j.status, severity: j.severity,
    line: [typeLabel(j.type, t), s ? t("window {slot}", { slot: slotText }) : "", alert, note].filter(Boolean).join(" · "), badge, today: isToday(s, now), slot: s, slotText,
    ackPending: j.status === "assigned" && ack === "pending", opensAt: before && s ? s.startAt : null, opens,
  };
}

export type KpiTile = { label: string; value: number; sub: string; tone?: "crit" | "warn" | "ok" };
/** The three tiles of FR-T01: assigned units, jobs not started, overdue jobs (summaries.get kind=technician). */
export function kpis(c: ApiTechCounts, rows: TechJobRow[], t: T = en): KpiTile[] {
  const notStarted = rows.filter((r) => r.status === "assigned");
  return [
    { label: t("Assigned units"), value: c.total, sub: t("in your assignments") },
    { label: t("Not started"), value: c.scheduledCount, sub: notStarted.length ? notStarted.slice(0, 3).map((r) => r.short).join(", ") : "—", tone: c.scheduledCount ? "warn" : undefined },
    { label: t("Overdue"), value: c.overdueCount, sub: t(c.overdueCount ? "past the due date" : "No overdue jobs"), tone: c.overdueCount ? "crit" : "ok" },
  ];
}

export type TimelineBlock = { id: string; short: string; left: number; width: number; text: string; current: boolean };
const START_H = 8, END_H = 18;
/** Today's timeline from 08:00 to 18:00 (KL): one block per job slot and the position of now (null outside). */
export function dayTimeline(rows: TechJobRow[], now: number): { hours: string[]; blocks: TimelineBlock[]; nowPct: number | null; now: string } {
  const day = Date.parse(`${klDate(now)}T00:00:00Z`) - KL;
  const pct = (ms: number) => Math.min(100, Math.max(0, ((ms - day) / 3600_000 - START_H) / (END_H - START_H) * 100));
  const blocks = rows.filter((r) => r.today && r.slot).map((r) => {
    const left = pct(Date.parse(r.slot!.startAt)), right = pct(Date.parse(r.slot!.endAt));
    return { id: r.id, short: r.short, left, width: Math.max(right - left, 2), text: `${windowText(r.slot!, now)} · ${r.short}`, current: r.status === "in_progress" };
  });
  const n = ((now - day) / 3600_000);
  return { hours: ["08:00", "10:00", "12:00", "14:00", "16:00", "18:00"], blocks, nowPct: n >= START_H && n <= END_H ? pct(now) : null, now: hhmm(new Date(now).toISOString()) };
}

export type AlertRow = { id: string; title: string; sub: string; severity: ApiAlert["severity"]; href: string };
const ALERT_STATE: Record<string, string> = { open: "open", acknowledged: "acknowledged", resolved: "resolved" };
/** Open alerts on the assigned units, worst first; each opens the unit's alerts with its job. */
export function alertRows(alerts: ApiAlert[], units: Map<string, string>, jobOfUnit: Map<string, string>, i: I18n = EN): AlertRow[] {
  const { t, display } = i;
  const rank = { critical: 0, warning: 1, normal: 2 } as const;
  return alerts.filter((a) => a.status !== "resolved").sort((a, b) => rank[a.severity] - rank[b.severity] || Date.parse(b.detectedAt) - Date.parse(a.detectedAt)).map((a) => {
    const job = jobOfUnit.get(a.unitId);
    return {
      id: a.id, title: alertTitle(a, t), severity: a.severity, href: `/technician/units/${a.unitId}/alerts${job ? `?jobId=${job}` : ""}`,
      sub: `${units.get(a.unitId) ?? t("unit")} · ${ALERT_STATE[a.status] ? t(ALERT_STATE[a.status]) : a.status} · ${showTime(a.detectedAt, display)}`,
    };
  });
}

export type ReportRow = { id: string; title: string; sub: string; badge: { text: string; tone: Tone } };
const REPORTS: Record<string, [string, string, Tone]> = {
  in_progress: ["draft in progress", "Draft", "warn"], rework_requested: ["returned — resume as a new version", "Rework", "warn"], submitted: ["awaiting quality review · read-only", "Submitted", "primary"],
};
/** The reports by state: drafts in progress, returned for rework, submitted for review. */
export function reportRows(rows: TechJobRow[], t: T = en): ReportRow[] {
  return rows.filter((r) => REPORTS[r.status]).map((r) => ({ id: r.id, title: `${r.short} · ${r.title}`, sub: t(REPORTS[r.status][0]), badge: { text: t(REPORTS[r.status][1]), tone: REPORTS[r.status][2] } }));
}
