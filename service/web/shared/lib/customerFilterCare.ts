// Customer Filter care (FR-C18, DD-C18, IR134, IR238, Figma Client 07j): one row per AC from filterCare.list (run time
// since the cleaning, % of the threshold, status and the last cleaning), an area holding many ACs folded into one
// summary row, the reminder settings in the client's words (filterCare.getSettings) and the reminder dialog's checks.
// Pure code shared by the Server Component and the client view.
import { klTime } from "@ac/web/lib/devices";

export type FilterState = "ok" | "due_soon" | "overdue" | "unknown";
/** FilterCareStatus of service-contracts.ts. */
export type ApiFilterStatus = {
  unitId: string; runHoursSinceCleaning: number | null; thresholdHours: number; fallbackDays: number; lastCleanedAt: string | null;
  lastCleanedBy: "customer" | "technician" | null; lastCleaningJobId: string | null; status: FilterState;
};
/** FilterCareSettings of service-contracts.ts; version 0 = never saved, the defaults. */
export type ApiFilterSettings = {
  id: string; version: number; customerId: string; thresholdHours: number | null; fallbackDays: number; recipients: "owners" | "all_users"; channels: ("inApp" | "email")[];
};
/** A unit of the customer as the tab needs it: its name and place (unitPlaces), its space and its connection. */
export type FilterUnit = { id: string; name: string; place: string; spaceId: string | null; connection: string };

/** The model default threshold (DD-C18). */
export const DEFAULT_FILTER_HOURS = 250;
/** A space with this many ACs or more shows one summary row with “View n” (DD-C18 step 1, IR238). */
export const AREA_GROUP_AT = 4;

type Tone = "ok" | "warn" | "crit" | "unknown" | "primary" | "muted";
export const filterStatus: Record<FilterState, { label: string; badge: Tone; bar: Tone }> = {
  overdue: { label: "Overdue", badge: "crit", bar: "crit" },
  due_soon: { label: "Due soon", badge: "warn", bar: "warn" },
  ok: { label: "OK", badge: "ok", bar: "primary" },
  unknown: { label: "Unknown", badge: "unknown", bar: "muted" },
};

const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
/** “Sep 8” in Kuala Lumpur time. */
export const monthDay = (iso: string) => {
  const d = klTime(iso);
  return `${months[+d.slice(5, 7) - 1]} ${+d.slice(8, 10)}`;
};

export type FilterRow = {
  unitId: string; unit: string; place: string; spaceId: string | null; offline: boolean;
  /** “last cleaned Jul 21” · “cleaned by technician Sep 8” (with jobId for the link) · “no cleaning recorded yet” */
  last: string; jobId: string | null;
  /** “268 h since cleaning” (“… so far” before the first recorded cleaning), or “—” with the reason while the run time
   * is unknown: offline, or no power readings (IR134 item 1). */
  run: string; runNote: string | null;
  /** The bar (0–100, capped) and its text: % of the threshold, else days of the fallback period, else unknown. */
  pct: number | null; progress: string;
  state: FilterState; threshold: number; hours: number | null; days: number | null; fallbackDays: number;
};
export type FilterGroup = {
  key: string; title: string; place: string; run: string; pct: number | null; progress: string;
  badge: { label: string; tone: Tone }; rows: FilterRow[];
};
export type FilterLine = { kind: "unit"; row: FilterRow } | { kind: "group"; group: FilterGroup };

/** One row per AC (IR134 items 1–3): run time and % of the threshold, or the fallback days since the last cleaning. */
export function filterRow(s: ApiFilterStatus, u: FilterUnit | undefined, nowMs: number): FilterRow {
  const h = s.runHoursSinceCleaning;
  const days = s.lastCleanedAt ? Math.max(0, Math.floor((nowMs - Date.parse(s.lastCleanedAt)) / 86_400_000)) : null;
  const pct = h != null ? Math.min(100, Math.round((h / s.thresholdHours) * 100)) : days != null ? Math.min(100, Math.round((days / s.fallbackDays) * 100)) : null;
  const progress = h != null ? `${pct} % of ${s.thresholdHours} h` : days != null ? `${pct} % of ${s.fallbackDays} days` : "run time unknown";
  const offline = !!u && u.connection !== "online";
  const last = !s.lastCleanedAt ? "no cleaning recorded yet" : s.lastCleanedBy === "technician" ? `cleaned by technician ${monthDay(s.lastCleanedAt)}` : `last cleaned ${monthDay(s.lastCleanedAt)}`;
  return {
    unitId: s.unitId, unit: u?.name ?? "Unit", place: u?.place ?? "", spaceId: u?.spaceId ?? null, offline,
    last, jobId: s.lastCleanedBy === "technician" ? s.lastCleaningJobId : null,
    run: h == null ? "—" : `${Math.round(h)} h ${s.lastCleanedAt ? "since cleaning" : "so far"}`, runNote: h != null ? null : offline ? "offline" : "no power readings",
    pct, progress, state: s.status, threshold: s.thresholdHours, hours: h, days, fallbackDays: s.fallbackDays,
  };
}

/** The summary of an area's ACs: “2 due soon · 1 offline”, the average progress and “2 due”. */
export function filterGroup(key: string, rows: FilterRow[]): FilterGroup {
  const n = (st: FilterState) => rows.filter((r) => r.state === st).length;
  const offline = rows.filter((r) => r.offline).length;
  const parts = [n("overdue") && `${n("overdue")} overdue`, n("due_soon") && `${n("due_soon")} due soon`, offline && `${offline} offline`].filter(Boolean) as string[];
  const known = rows.filter((r) => r.hours != null && r.pct != null);
  const pct = known.length ? Math.round(known.reduce((a, r) => a + (r.pct ?? 0), 0) / known.length) : null;
  const due = n("overdue") + n("due_soon");
  const name = rows[0].place.split(" › ").pop() || "Area";
  return {
    key, title: `${name} · ${rows.length} ACs`, place: rows[0].place, run: parts.length ? parts.join(" · ") : "all OK",
    pct, progress: pct != null ? `${pct} % of ${rows[0].threshold} h (average)` : "run time unknown",
    badge: due ? { label: `${due} due`, tone: n("overdue") ? "crit" : "warn" } : n("unknown") === rows.length ? { label: "Unknown", tone: "unknown" } : { label: "OK", tone: "ok" },
    rows,
  };
}

/** The table in the order filterCare.list returns (overdue, due soon, OK, unknown); an area with AREA_GROUP_AT ACs or
 * more becomes one summary row where its first (most urgent) AC would be. */
export function filterLines(items: ApiFilterStatus[], units: FilterUnit[], nowMs: number): FilterLine[] {
  const byId = new Map(units.map((u) => [u.id, u]));
  const rows = items.map((s) => filterRow(s, byId.get(s.unitId), nowMs));
  const size = new Map<string, number>();
  for (const r of rows) if (r.spaceId) size.set(r.spaceId, (size.get(r.spaceId) ?? 0) + 1);
  const out: FilterLine[] = [];
  const placed = new Set<string>();
  for (const r of rows) {
    if (!r.spaceId || (size.get(r.spaceId) ?? 0) < AREA_GROUP_AT) out.push({ kind: "unit", row: r });
    else if (!placed.has(r.spaceId)) {
      placed.add(r.spaceId);
      out.push({ kind: "group", group: filterGroup(r.spaceId, rows.filter((x) => x.spaceId === r.spaceId)) });
    }
  }
  return out;
}

/** The symptom line of a cleaning request opened from the tab (New request prefilled, type preventive). */
export const cleaningSymptom = (r: FilterRow) =>
  `Filter cleaning: ${r.hours != null ? `${Math.round(r.hours)} h of running since the last cleaning (reminder at ${r.threshold} h)`
    : r.days != null ? `${r.days} days since the last cleaning (reminder every ${r.fallbackDays} days)` : "the filter is due for cleaning"}.`;

/** The Reminders card (Figma 07j). */
export function reminderLines(s: ApiFilterSettings): [string, string][] {
  return [
    ["Remind at", s.thresholdHours == null ? `${DEFAULT_FILTER_HOURS} h (model default)` : `${s.thresholdHours} h of running`],
    ["Also remind", `every ${s.fallbackDays} days if run time unknown`],
    ["Send by", s.channels.includes("email") ? "App + email" : "App only"],
    ["Who", s.recipients === "owners" ? "Owners of the location" : "All users of the location"],
  ];
}

/** The reminder dialog: the model default or 50–2000 h; 7–180 days; owners or all users; the app always, e-mail optional. */
export type ReminderForm = { useDefault: boolean; hours: string; days: string; recipients: ApiFilterSettings["recipients"]; email: boolean };
export const reminderForm = (s: ApiFilterSettings): ReminderForm => ({
  useDefault: s.thresholdHours == null, hours: String(s.thresholdHours ?? DEFAULT_FILTER_HOURS), days: String(s.fallbackDays), recipients: s.recipients, email: s.channels.includes("email"),
});
const whole = (v: string, lo: number, hi: number) => /^\d+$/.test(v.trim()) && +v >= lo && +v <= hi;
export function reminderErrors(f: ReminderForm): { hours?: string; days?: string } {
  return {
    ...(!f.useDefault && !whole(f.hours, 50, 2000) ? { hours: "Whole hours from 50 to 2000." } : {}),
    ...(!whole(f.days, 7, 180) ? { days: "Whole days from 7 to 180." } : {}),
  };
}
/** filterCare.saveSettings input from the dialog. */
export const reminderInput = (f: ReminderForm) => ({
  thresholdHours: f.useDefault ? null : Number(f.hours), fallbackDays: Number(f.days), recipients: f.recipients, channels: f.email ? ["inApp", "email"] as ApiFilterSettings["channels"] : ["inApp"] as ApiFilterSettings["channels"],
});

/** A refused Mark cleaned or reminder save in the client's words. */
export function filterRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  if (f.code === "FORBIDDEN") return "Not saved: only the account owner can change the reminders.";
  if (f.code === "NOT_FOUND") return "Not saved: that AC is no longer in your account.";
  const fe = f.fieldErrors ?? {};
  if (fe.unitId === "error.unitArchived") return "Not saved: that AC is archived.";
  if (fe.thresholdHours) return "Not saved: remind at 50–2000 h of running, or use the model default.";
  if (fe.fallbackDays) return "Not saved: the fallback is 7–180 days.";
  if (fe.channels || fe.recipients) return "Not saved: choose who gets the reminder and keep the app notification on.";
  return `${f.code} — ${f.messageKey}`;
}
