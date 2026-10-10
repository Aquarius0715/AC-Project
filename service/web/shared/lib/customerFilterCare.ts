// Customer Filter care (FR-C18, DD-C18, IR134, IR238, Figma Client 07j): one row per AC from filterCare.list (run time
// since the cleaning, % of the threshold, status and the last cleaning), an area holding many ACs folded into one
// summary row, the reminder settings in the client's words (filterCare.getSettings) and the reminder dialog's checks.
// Pure code shared by the Server Component and the client view; texts in the display language and days in the user's
// display time zone (`i`, IR262).
import { EN, intlTag, translator, type Display, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

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

/** “8 Sept” in the user's language and display time zone. */
export function monthDay(iso: string, d: Display = EN.display): string {
  for (const timeZone of [d.timeZone, EN.display.timeZone]) {
    try {
      return new Date(iso).toLocaleDateString(intlTag(d.locale), { timeZone, month: "short", day: "numeric" });
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso.slice(5, 10);
}

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
export function filterRow(s: ApiFilterStatus, u: FilterUnit | undefined, nowMs: number, i: I18n = EN): FilterRow {
  const { t } = i;
  const h = s.runHoursSinceCleaning;
  const days = s.lastCleanedAt ? Math.max(0, Math.floor((nowMs - Date.parse(s.lastCleanedAt)) / 86_400_000)) : null;
  const pct = h != null ? Math.min(100, Math.round((h / s.thresholdHours) * 100)) : days != null ? Math.min(100, Math.round((days / s.fallbackDays) * 100)) : null;
  const progress = h != null ? t("{pct} % of {n} h", { pct: pct!, n: s.thresholdHours }) : days != null ? t("{pct} % of {n} days", { pct: pct!, n: s.fallbackDays }) : t("run time unknown");
  const offline = !!u && u.connection !== "online";
  const last = !s.lastCleanedAt ? t("no cleaning recorded yet") : t(s.lastCleanedBy === "technician" ? "cleaned by technician {date}" : "last cleaned {date}", { date: monthDay(s.lastCleanedAt, i.display) });
  return {
    unitId: s.unitId, unit: u?.name ?? t("Unit"), place: u?.place ?? "", spaceId: u?.spaceId ?? null, offline,
    last, jobId: s.lastCleanedBy === "technician" ? s.lastCleaningJobId : null,
    run: h == null ? "—" : t(s.lastCleanedAt ? "{n} h since cleaning" : "{n} h so far", { n: Math.round(h) }), runNote: h != null ? null : t(offline ? "offline" : "no power readings"),
    pct, progress, state: s.status, threshold: s.thresholdHours, hours: h, days, fallbackDays: s.fallbackDays,
  };
}

/** The summary of an area's ACs: “2 due soon · 1 offline”, the average progress and “2 due”. */
export function filterGroup(key: string, rows: FilterRow[], t: T = en): FilterGroup {
  const n = (st: FilterState) => rows.filter((r) => r.state === st).length;
  const offline = rows.filter((r) => r.offline).length;
  const parts = [n("overdue") && t("{n} overdue", { n: n("overdue") }), n("due_soon") && t("{n} due soon", { n: n("due_soon") }), offline && t("{n} offline", { n: offline })].filter(Boolean) as string[];
  const known = rows.filter((r) => r.hours != null && r.pct != null);
  const pct = known.length ? Math.round(known.reduce((a, r) => a + (r.pct ?? 0), 0) / known.length) : null;
  const due = n("overdue") + n("due_soon");
  const name = rows[0].place.split(" › ").pop() || t("Area");
  return {
    key, title: t("{name} · {n} ACs", { name, n: rows.length }), place: rows[0].place, run: parts.length ? parts.join(" · ") : t("all OK"),
    pct, progress: pct != null ? t("{pct} % of {n} h (average)", { pct, n: rows[0].threshold }) : t("run time unknown"),
    badge: due ? { label: t("{n} due", { n: due }), tone: n("overdue") ? "crit" : "warn" } : n("unknown") === rows.length ? { label: t("Unknown"), tone: "unknown" } : { label: t("OK"), tone: "ok" },
    rows,
  };
}

/** The table in the order filterCare.list returns (overdue, due soon, OK, unknown); an area with AREA_GROUP_AT ACs or
 * more becomes one summary row where its first (most urgent) AC would be. */
export function filterLines(items: ApiFilterStatus[], units: FilterUnit[], nowMs: number, i: I18n = EN): FilterLine[] {
  const byId = new Map(units.map((u) => [u.id, u]));
  const rows = items.map((s) => filterRow(s, byId.get(s.unitId), nowMs, i));
  const size = new Map<string, number>();
  for (const r of rows) if (r.spaceId) size.set(r.spaceId, (size.get(r.spaceId) ?? 0) + 1);
  const out: FilterLine[] = [];
  const placed = new Set<string>();
  for (const r of rows) {
    if (!r.spaceId || (size.get(r.spaceId) ?? 0) < AREA_GROUP_AT) out.push({ kind: "unit", row: r });
    else if (!placed.has(r.spaceId)) {
      placed.add(r.spaceId);
      out.push({ kind: "group", group: filterGroup(r.spaceId, rows.filter((x) => x.spaceId === r.spaceId), i.t) });
    }
  }
  return out;
}

/** The symptom line of a cleaning request opened from the tab (New request prefilled, type preventive), in the
 * display language — the client still edits it before sending. */
export const cleaningSymptom = (r: FilterRow, t: T = en) =>
  r.hours != null ? t("Filter cleaning: {n} h of running since the last cleaning (reminder at {threshold} h).", { n: Math.round(r.hours), threshold: r.threshold })
    : r.days != null ? t("Filter cleaning: {n} days since the last cleaning (reminder every {days} days).", { n: r.days, days: r.fallbackDays }) : t("Filter cleaning: the filter is due for cleaning.");

/** The Reminders card (Figma 07j). */
export function reminderLines(s: ApiFilterSettings, t: T = en): [string, string][] {
  return [
    [t("Remind at"), s.thresholdHours == null ? t("{n} h (model default)", { n: DEFAULT_FILTER_HOURS }) : t("{n} h of running", { n: s.thresholdHours })],
    [t("Also remind"), t("every {n} days if run time unknown", { n: s.fallbackDays })],
    [t("Send by"), t(s.channels.includes("email") ? "App + email" : "App only")],
    [t("Who"), t(s.recipients === "owners" ? "Owners of the location" : "All users of the location")],
  ];
}

/** The reminder dialog: the model default or 50–2000 h; 7–180 days; owners or all users; the app always, e-mail optional. */
export type ReminderForm = { useDefault: boolean; hours: string; days: string; recipients: ApiFilterSettings["recipients"]; email: boolean };
export const reminderForm = (s: ApiFilterSettings): ReminderForm => ({
  useDefault: s.thresholdHours == null, hours: String(s.thresholdHours ?? DEFAULT_FILTER_HOURS), days: String(s.fallbackDays), recipients: s.recipients, email: s.channels.includes("email"),
});
const whole = (v: string, lo: number, hi: number) => /^\d+$/.test(v.trim()) && +v >= lo && +v <= hi;
export function reminderErrors(f: ReminderForm, t: T = en): { hours?: string; days?: string } {
  return {
    ...(!f.useDefault && !whole(f.hours, 50, 2000) ? { hours: t("Whole hours from 50 to 2000.") } : {}),
    ...(!whole(f.days, 7, 180) ? { days: t("Whole days from 7 to 180.") } : {}),
  };
}
/** filterCare.saveSettings input from the dialog. */
export const reminderInput = (f: ReminderForm) => ({
  thresholdHours: f.useDefault ? null : Number(f.hours), fallbackDays: Number(f.days), recipients: f.recipients, channels: f.email ? ["inApp", "email"] as ApiFilterSettings["channels"] : ["inApp"] as ApiFilterSettings["channels"],
});

/** A refused Mark cleaned or reminder save in the client's words. */
export function filterRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string {
  if (f.code === "FORBIDDEN") return t("Not saved: only the account owner can change the reminders.");
  if (f.code === "NOT_FOUND") return t("Not saved: that AC is no longer in your account.");
  const fe = f.fieldErrors ?? {};
  if (fe.unitId === "error.unitArchived") return t("Not saved: that AC is archived.");
  if (fe.thresholdHours) return t("Not saved: remind at 50–2000 h of running, or use the model default.");
  if (fe.fallbackDays) return t("Not saved: the fallback is 7–180 days.");
  if (fe.channels || fe.recipients) return t("Not saved: choose who gets the reminder and keep the app notification on.");
  return `${f.code} — ${f.messageKey}`;
}
