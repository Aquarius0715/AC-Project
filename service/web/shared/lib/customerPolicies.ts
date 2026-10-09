// Customer alert policies (FR-C15, DD-C15, Figma Client 06e/06f, IR243): the default policy's rules with the
// customer's on / off settings (policies.setDefaultRule, owner only, IR115), the customer's own policies with their
// When / Then sentences and attached ACs, and the editor's form, checks, summary sentence and policies.save input
// (hidden fields: recipients = the session membership, escalation 60, cooldown 5, Preferences.timezone, priority 50 —
// kept on edit, IR120). Pure code shared by the Server Component and the client view.
import { metricUnit, opSymbol, recoveryError, type Channel, type Operator, type Severity } from "@ac/web/lib/adminAlerts";

export type PolicyWindow = { weekdays: number[]; startLocal: string; endLocal: string };
/** DefaultAlertRule / DefaultRuleSetting / Policy (alert and default_alert kinds) of service-contracts.ts. */
export type ApiRule = {
  ruleKey: string; name: string; category: "air_quality" | "fault" | "maintenance" | "connection"; metric: string; operator: Operator; threshold: number;
  recoveryThreshold: number; durationSeconds: number; activeWindow: PolicyWindow | null; severity: Severity;
};
export type ApiRuleSetting = { id: string; version: number; ruleKey: string; customerId: string; enabled: boolean; changedByMembershipId: string; reason: string | null; updatedAt: string };
export type ApiDefaultPolicy = { id: string; version: number; kind: "default_alert"; name: string; rules: ApiRule[]; ruleSettings: ApiRuleSetting[] };
export type ApiAlertPolicy = {
  id: string; version: number; kind: "alert"; name: string; unitIds: string[]; timezone: string; enabled: boolean; priority: number; disabledReason: string | null;
  customerId: string; recipientMembershipIds: string[]; channels: Channel[]; escalateAfterMinutes: number; cooldownMinutes: number;
  metric: string; operator: Operator; threshold: number; recoveryThreshold: number; durationSeconds: number; activeWindow: PolicyWindow | null; severity: Severity;
};

const KL = "Asia/Kuala_Lumpur";
const md = (iso: string) => new Date(iso).toLocaleDateString("en-US", { timeZone: KL, month: "short", day: "numeric" });
const label: Record<string, string> = {
  temperature: "Room temperature", humidity: "Humidity", co2: "CO₂", pm25: "PM2.5", power: "Power", refrigerant_pressure: "Refrigerant pressure",
  compressor_cycles: "Compressor cycles", airflow_drop: "Airflow drop", heartbeat_gap: "Heartbeat gap", vibration: "Vibration",
};
export const severityText: Record<Severity, string> = { normal: "Info", warning: "Warning", critical: "Critical" };
const categoryText: Record<ApiRule["category"], string> = { air_quality: "Air quality", fault: "Fault cause", maintenance: "Maintenance", connection: "Connection" };
const dayNames = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/** “60 s”, “15 min”, “3 h”. */
export function durationText(s: number): string {
  if (s >= 3600 && s % 3600 === 0) return `${s / 3600} h`;
  if (s >= 60 && s % 60 === 0) return `${s / 60} min`;
  return `${s} s`;
}
/** “Room temperature ≥ 30 °C for 60 s”, “No heartbeat for 15 min”. */
export function conditionText(c: { metric: string; operator: Operator; threshold: number; durationSeconds: number }): string {
  if (c.metric === "heartbeat_gap") return `No heartbeat for ${c.threshold} min`;
  const unit = metricUnit[c.metric] ?? "";
  const value = unit === "%" || unit === "cycles/h" ? `${c.threshold}${unit === "%" ? " %" : " cycles / h"}` : `${c.threshold}${unit ? ` ${unit}` : ""}`;
  return `${label[c.metric] ?? c.metric} ${opSymbol[c.operator]} ${value} for ${durationText(c.durationSeconds)}`;
}
/** “on weekdays”, “on weekends”, “on Mon, Wed”, or "" for every day. */
const daysText = (weekdays: number[]) => {
  const days = [...weekdays].sort();
  return days.length === 7 ? "" : days.join() === "1,2,3,4,5" ? "on weekdays" : days.join() === "6,7" ? "on weekends" : `on ${days.map((d) => dayNames[d - 1]).join(", ")}`;
};
/** “only 08:00–19:00 on weekdays”, “only 22:00–06:00”, “only 09:00–12:00 on Sat, Sun”. */
export function windowText(w: PolicyWindow): string {
  const d = daysText(w.weekdays);
  return `only ${w.startLocal}–${w.endLocal}${d ? ` ${d}` : ""}`;
}
const recoverText = (op: Operator, v: number, metric: string) => `recover ${op === "gt" || op === "gte" ? "below" : "above"} ${v} ${metricUnit[metric] ?? ""}`.trim();
const channelText = (cs: Channel[]) => cs.map((c) => (c === "inApp" ? "in-app" : c === "email" ? "email" : "WhatsApp")).join(" + ");

export type DefaultRuleRow = { ruleKey: string; name: string; condition: string; type: string; severity: Severity; enabled: boolean; version: number };
/** The default policy's rules with this customer's settings (a rule without a setting is on, version 0). */
export function defaultRuleRows(p: ApiDefaultPolicy, customerId: string | null, me: string): { rows: DefaultRuleRow[]; on: number; notes: string[] } {
  const mine = new Map(p.ruleSettings.filter((s) => s.customerId === customerId).map((s) => [s.ruleKey, s]));
  const rows = p.rules.map((r) => {
    const s = mine.get(r.ruleKey);
    return { ruleKey: r.ruleKey, name: r.name, condition: conditionText(r), type: categoryText[r.category] ?? r.category, severity: r.severity, enabled: s?.enabled ?? true, version: s?.version ?? 0 };
  });
  const notes = rows.filter((r) => !r.enabled).map((r) => {
    const s = mine.get(r.ruleKey)!;
    return `Turned off: “${r.name}” — no alerts from this rule on any of your ACs (turned off ${s.changedByMembershipId === me ? "by you" : "by the account owner"}, ${md(s.updatedAt)}). HQ still sees device status.`;
  });
  return { rows, on: rows.filter((r) => r.enabled).length, notes };
}

export type PolicyCard = {
  id: string; version: number; name: string; badge: string; enabled: boolean; when: string; then: string;
  attached: string[]; attachedText: string; note: string | null;
};
const groupOf = (metric: string) => (metric === "co2" || metric === "pm25" ? "Air quality" : metric === "temperature" ? "Temperature" : metric === "humidity" ? "Humidity" : metric === "power" ? "Power" : metric);
/** One card per own policy (Figma 06e): When / Then sentences and the attached ACs (attaching happens on each AC). */
export function policyCards(ps: ApiAlertPolicy[], unitName: (id: string) => string | undefined): PolicyCard[] {
  return [...ps].sort((a, b) => a.name.localeCompare(b.name)).map((p) => {
    const names = p.unitIds.map((id) => unitName(id) ?? "AC");
    return {
      id: p.id, version: p.version, name: p.name, badge: groupOf(p.metric), enabled: p.enabled,
      when: [conditionText(p), recoverText(p.operator, p.recoveryThreshold, p.metric), p.activeWindow ? windowText(p.activeWindow) : null].filter(Boolean).join(" · "),
      then: `${severityText[p.severity]} · notify ${channelText(p.channels)}`, attached: names,
      attachedText: names.length ? `Attached to ${names.length} AC${names.length === 1 ? "" : "s"}: ${names.join(", ")}` : "Not attached to any AC yet — attach it from an AC’s page",
      note: p.disabledReason === "unit_archived" ? "An attached AC was archived." : p.disabledReason === "capability_changed" ? "An attached AC's capabilities changed — check the condition." : null,
    };
  });
}

export type MetricGroup = "temperature" | "humidity" | "air" | "power";
export type PolicyForm = {
  name: string; group: MetricGroup; metric: string; operator: Operator; threshold: string; recovery: string; duration: string; unit: "s" | "min";
  windowOn: boolean; weekdays: number[]; start: string; end: string; severity: Severity; email: boolean;
};
export const metricGroups: { id: MetricGroup; label: string; sub: string }[] = [
  { id: "temperature", label: "Temperature", sub: "°C" }, { id: "humidity", label: "Humidity", sub: "%" }, { id: "air", label: "Air quality", sub: "CO₂ ppm · PM2.5" }, { id: "power", label: "Power", sub: "kW" },
];
const groupMetric: Record<MetricGroup, string> = { temperature: "temperature", humidity: "humidity", air: "co2", power: "power" };
export const groupOfMetric = (m: string): MetricGroup => (m === "co2" || m === "pm25" ? "air" : m === "humidity" ? "humidity" : m === "power" ? "power" : "temperature");
export const metricOfGroup = (g: MetricGroup) => groupMetric[g];
/** The editor's values: an existing policy's, or a new one (room temperature ≥ 30 °C for 1 min, recovering below 28). */
export function policyForm(p?: ApiAlertPolicy | null): PolicyForm {
  if (!p) return { name: "", group: "temperature", metric: "temperature", operator: "gte", threshold: "30", recovery: "28", duration: "1", unit: "min", windowOn: false, weekdays: [1, 2, 3, 4, 5], start: "08:00", end: "19:00", severity: "warning", email: false };
  const min = p.durationSeconds % 60 === 0;
  return {
    name: p.name, group: groupOfMetric(p.metric), metric: p.metric, operator: p.operator, threshold: String(p.threshold), recovery: String(p.recoveryThreshold),
    duration: String(min ? p.durationSeconds / 60 : p.durationSeconds), unit: min ? "min" : "s", windowOn: !!p.activeWindow, weekdays: p.activeWindow?.weekdays ?? [1, 2, 3, 4, 5],
    start: p.activeWindow?.startLocal ?? "08:00", end: p.activeWindow?.endLocal ?? "19:00", severity: p.severity, email: p.channels.includes("email"),
  };
}
const num = (v: string) => (v.trim() !== "" && Number.isFinite(Number(v)) ? Number(v) : null);
const seconds = (f: PolicyForm) => { const d = num(f.duration); return d === null ? null : Math.round(f.unit === "min" ? d * 60 : d); };
/** The editor's checks (DD-C15; the API checks them again): name 1–120, numbers, the recovery direction, 1 s–24 h, a
 * window with days and different times. */
export function policyErrors(f: PolicyForm): Partial<Record<"name" | "threshold" | "recovery" | "duration" | "window", string>> {
  const e: Partial<Record<"name" | "threshold" | "recovery" | "duration" | "window", string>> = {};
  const name = f.name.trim();
  if (!name || name.length > 120) e.name = "Name is 1–120 characters.";
  const t = num(f.threshold), r = num(f.recovery), s = seconds(f);
  if (t === null) e.threshold = "Enter a number.";
  if (r === null) e.recovery = "Enter a number.";
  else if (t !== null) { const bad = recoveryError(f.operator, t, r); if (bad) e.recovery = bad; }
  if (s === null || s < 1 || s > 86400) e.duration = "Between 1 s and 24 h.";
  if (f.windowOn && (!f.weekdays.length || !/^([01]\d|2[0-3]):[0-5]\d$/.test(f.start) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(f.end) || f.start === f.end)) e.window = "Pick at least one day and two different times.";
  return e;
}
/** The live summary sentence (Figma 06f): “Warn me when CO₂ stays ≥ 1200 ppm for 15 min on weekdays 08:00–19:00. Clears below 1000 ppm.” */
export function summaryText(f: PolicyForm): string {
  const verb = f.severity === "critical" ? "Alert me (critical)" : f.severity === "warning" ? "Warn me" : "Tell me";
  const unit = metricUnit[f.metric] ?? "";
  const s = seconds(f);
  const d = f.windowOn && f.weekdays.length ? daysText(f.weekdays) : "";
  const win = f.windowOn && f.weekdays.length ? ` ${d ? `${d} ` : ""}${f.start}–${f.end}` : "";
  const what = f.metric === "co2" || f.metric === "pm25" ? label[f.metric] : (label[f.metric] ?? f.metric).toLowerCase();
  return `${verb} when ${what} stays ${opSymbol[f.operator]} ${f.threshold || "…"} ${unit} for ${s === null ? "…" : durationText(s)}${win}. Clears ${f.operator === "gt" || f.operator === "gte" ? "below" : "above"} ${f.recovery || "…"} ${unit}.`.replace(/ +/g, " ");
}
/** policies.save input (AlertPolicyInput): the form plus the hidden fields — new: the session membership as recipient,
 * escalation 60, cooldown 5, the Preferences time zone, priority 50; edit: the policy's own (IR120). */
export function policyInput(f: PolicyForm, existing: ApiAlertPolicy | null, ctx: { customerId: string; membershipId: string; timezone: string }) {
  return {
    ...(existing ? { id: existing.id } : {}), kind: "alert" as const, name: f.name.trim(), customerId: ctx.customerId,
    timezone: existing?.timezone ?? ctx.timezone, enabled: existing?.enabled ?? true, priority: existing?.priority ?? 50,
    recipientMembershipIds: existing?.recipientMembershipIds ?? [ctx.membershipId], escalateAfterMinutes: existing?.escalateAfterMinutes ?? 60, cooldownMinutes: existing?.cooldownMinutes ?? 5,
    channels: (f.email ? ["inApp", "email"] : ["inApp"]) as Channel[], metric: f.metric, operator: f.operator, threshold: Number(f.threshold), recoveryThreshold: Number(f.recovery),
    durationSeconds: seconds(f) ?? 0, activeWindow: f.windowOn ? { weekdays: [...f.weekdays].sort(), startLocal: f.start, endLocal: f.end } : null, severity: f.severity,
  };
}
/** The on / off switch of a policy card: the saved policy with only `enabled` changed. */
export const toggledInput = (p: ApiAlertPolicy, enabled: boolean) => ({ ...policyInput(policyForm(p), p, { customerId: p.customerId, membershipId: "", timezone: p.timezone }), enabled, channels: p.channels });

/** A refused policy write in the client's words. */
export function policyRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  if (f.code === "FORBIDDEN") return "Not saved: only the account owner can switch default rules.";
  if (f.code === "CONFLICT") return "Not saved: this policy changed meanwhile — the latest version is shown.";
  if (f.code === "NOT_FOUND") return "Not saved: this policy no longer exists.";
  const fe = f.fieldErrors ?? {};
  if (fe.recoveryThreshold) return "Not saved: the recovery value must be on the safe side of the limit.";
  if (fe.durationSeconds) return "Not saved: the duration is 1 s to 24 h.";
  if (fe["activeWindow"] || fe["activeWindow.weekdays"]) return "Not saved: check the days and times of “Only if”.";
  if (fe.channels) return "Not saved: choose how to be notified.";
  if (fe.name) return "Not saved: the name is 1–120 characters.";
  return `Not saved (${f.code}).`;
}
