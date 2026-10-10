// Customer automations & schedules (FR-C04, FR-C05, DATA_SOURCE=api): the rule cards of the list (Figma Client 03a,
// 03d, 03f, 03i), the editor draft with its automations.save input and checks (03b, 03e), the schedule preview and the
// event test (03c), and the location consent card. Pure code shared by the Server Component and the client view.
import { EN, intlTag, showDate, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import type { UnitAction } from "@ac/web/lib/units";
import type { OpInput } from "@ac/web/lib/opTypes";

const en = translator("en");

export type Compare = "gt" | "gte" | "lt" | "lte";
export type ClientCondition =
  | { type: "occupancy"; occupied: boolean } | { type: "location"; event: "arrival" | "departure" }
  | { type: "pattern"; localTime: string } | { type: "weather"; metric: "temperature"; operator: Compare; value: number };
/** ExtraCondition of service-contracts.ts (IR215, Figma 03b “Only if …”). */
export type ExtraCondition = { type: "weekday"; weekdays: number[] } | { type: "occupancy"; occupied: boolean } | { type: "weather"; metric: "temperature"; operator: Compare; value: number };
/** AutomationRun of service-contracts.ts: the latest Command or skip of the rule (IR215). */
export type ApiRun = { at: string; outcome: "command_created" | "skipped"; reason: string | null; commandId: string | null };
type RuleBase = {
  id: string; version: number; createdAt: string; updatedAt: string; name: string; unitIds: string[]; ownerMembershipId: string; timezone: string;
  enabled: boolean; priority: number; disabledReason: "capability_changed" | "unit_archived" | "consent_revoked" | null; onlyIf: ExtraCondition[]; lastRun: ApiRun | null;
};
/** Automation of service-contracts.ts. */
export type ApiAutomation = RuleBase & (
  | { kind: "schedule"; weekdays: number[]; startLocal: string; endLocal: string; endsNextDay: boolean; startAction: UnitAction; endAction: UnitAction }
  | { kind: "event"; condition: ClientCondition; action: UnitAction });
/** Consent of service-contracts.ts. */
export type ApiConsent = { id: string; version: number; granted: boolean; grantedAt: string | null; revokedAt: string | null };
/** ScheduledOccurrence of service-contracts.ts (automationId null for a draft preview, IR214). */
export type ApiOccurrence = { automationId: string | null; phase: "schedule_start" | "schedule_end"; at: string; action: UnitAction };
/** Decision of automations.simulate (SimulationResult.results). */
export type ApiDecision = { unitId: string; decision: "selected" | "suppressed"; ruleId: string | null; reason: string | null };

export type Trigger = "schedule" | "presence" | "location" | "routine" | "weather";
export const triggers: Trigger[] = ["schedule", "presence", "location", "routine", "weather"];
export const triggerInfo: Record<Trigger, { label: string; sub: string; icon: string }> = {
  schedule: { label: "Schedule", sub: "Days & time", icon: "◷" },
  presence: { label: "Presence", sub: "Room occupied / empty", icon: "●" },
  location: { label: "Location", sub: "Arrive / leave (consent)", icon: "⌖" },
  routine: { label: "Routine", sub: "Your usual pattern (demo)", icon: "↻" },
  weather: { label: "Weather", sub: "Outdoor conditions", icon: "☀" },
};
const conditionTrigger = { occupancy: "presence", location: "location", pattern: "routine", weather: "weather" } as const;
export const triggerOf = (a: ApiAutomation): Trigger => (a.kind === "schedule" ? "schedule" : conditionTrigger[a.condition.type]);

export const dayShort = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
export const dayLong = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];
/** "Mon, Wed" (lists) or "Mon & Wed" / "Mon, Tue & Sun" (sentences); every day, weekdays and weekends by name. */
export function weekdaysText(ds: number[], sentence = false, t: T = en): string {
  const s = [...ds].sort((a, b) => a - b);
  if (s.length === 7) return t("Every day");
  if (s.join() === "1,2,3,4,5") return t("Weekdays");
  if (s.join() === "6,7") return t("Weekends");
  const names = s.map((d) => t(dayShort[d - 1]));
  return sentence && names.length > 1 ? t("{list} & {last}", { list: names.slice(0, -1).join(", "), last: names[names.length - 1] }) : names.join(", ");
}
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const modeWord: Record<string, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
const fanWord: Record<string, string> = { low: "Low", mid: "Mid", high: "High" };
export function actionLabel(a: UnitAction | { kind: "ventilate"; level: string }, t: T = en): string {
  switch (a.kind) {
    case "set_power": return t(a.power ? "Power ON" : "Power OFF");
    case "set_temperature": return t("Set temperature {celsius}°C", { celsius: a.celsius });
    case "set_mode": return t("Mode {mode}", { mode: modeWord[a.mode] ? t(modeWord[a.mode]) : cap(a.mode) });
    case "set_fan": return t("Fan {level}", { level: fanWord[a.fanLevel] ? t(fanWord[a.fanLevel]) : cap(a.fanLevel) });
    default: return t("Ventilate {level}", { level: fanWord[a.level] ? t(fanWord[a.level]).toLowerCase() : a.level });
  }
}
const cmp: Record<Compare, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
export const compares: Compare[] = ["gte", "gt", "lte", "lt"];
export const compareText = (c: Compare) => cmp[c];
/** The “When …” sentence of a rule; a location event happens where the target AC is (its property). */
export function whenText(a: ApiAutomation, place: string, t: T = en): string {
  if (a.kind === "schedule") {
    return (a.endsNextDay ? t("{days} {start} → {end} next day (overnight)", { days: weekdaysText(a.weekdays, false, t), start: a.startLocal, end: a.endLocal })
      : t("{days} at {start} ({zone})", { days: weekdaysText(a.weekdays, false, t), start: a.startLocal, zone: a.timezone })) + onlyIfText(a.onlyIf, t);
  }
  const c = a.condition;
  const when = c.type === "location" ? (c.event === "arrival" ? t("Arrival at {place}", { place }) : t("Everyone leaves {place} (departure event)", { place }))
    : c.type === "occupancy" ? t(c.occupied ? "Someone is in the room (demo occupancy)" : "No one detected (demo occupancy)")
    : c.type === "pattern" ? t("Your usual time {time} (demo routine)", { time: c.localTime }) : t("Outdoor temperature {op} {value}°C (demo weather)", { op: cmp[c.operator], value: c.value });
  return when + onlyIfText(a.onlyIf, t);
}
/** “someone is home”, “outdoor ≥ 30°C”, “weekdays” — the “Only if” clause of a sentence. */
export function extraText(x: ExtraCondition, t: T = en): string {
  if (x.type === "weekday") {
    const s = [...x.weekdays].sort((a, b) => a - b).join();
    const days = weekdaysText(x.weekdays, false, t);
    return s === "1,2,3,4,5" || s === "6,7" || x.weekdays.length === 7 ? days.toLowerCase() : days;
  }
  if (x.type === "occupancy") return t(x.occupied ? "someone is home" : "no one is home");
  return t("outdoor {op} {value}°C", { op: cmp[x.operator], value: x.value });
}
export const onlyIfText = (xs: ExtraCondition[], t: T = en) => (xs.length ? t(" · Only if: {conditions}", { conditions: xs.map((x) => extraText(x, t)).join(t(" and ")) }) : "");
export function thenText(a: ApiAutomation, t: T = en): string {
  return a.kind === "schedule" ? t("{start} · at {end} → {endAction} (end action)", { start: actionLabel(a.startAction, t), end: a.endLocal, endAction: actionLabel(a.endAction, t) }) : actionLabel(a.action, t);
}

const KL = "Asia/Kuala_Lumpur";
/** "Mon, 28 Sept, 18:00 MYT": a rule's time in the rule's own time zone (a schedule keeps its local time, IR44 adds the
 * zone's abbreviation), in the display language. */
export function runText(iso: string, tz = KL, i: I18n = EN): string {
  const d = new Date(iso);
  const tag = intlTag(i.display.locale);
  try {
    const zone = new Intl.DateTimeFormat(tag, { timeZone: tz, timeZoneName: "short" }).formatToParts(d).find((p) => p.type === "timeZoneName")?.value;
    return `${d.toLocaleString(tag, { timeZone: tz, weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })}${zone ? ` ${zone}` : ""}`;
  } catch {
    return runText(iso, KL, i);
  }
}
/** “22:00 MYT”: only the time of a rule's run, in the rule's own time zone (the end of a schedule run). */
export function runClock(iso: string, tz = KL, i: I18n = EN): string {
  const d = new Date(iso);
  const tag = intlTag(i.display.locale);
  try {
    const zone = new Intl.DateTimeFormat(tag, { timeZone: tz, timeZoneName: "short" }).formatToParts(d).find((p) => p.type === "timeZoneName")?.value;
    return `${d.toLocaleTimeString(tag, { timeZone: tz, hour: "2-digit", minute: "2-digit", hourCycle: "h23" })}${zone ? ` ${zone}` : ""}`;
  } catch {
    return runClock(iso, KL, i);
  }
}
const reasonText: Record<string, string> = { consent_revoked: "consent revoked", capability_changed: "capability changed", unit_archived: "unit archived" };

/** One rule card of the list. */
export type RuleCard = {
  id: string; version: number; name: string; trigger: Trigger; when: string; then: string; place: string; enabled: boolean;
  status: { text: string; tone: "ok" | "warn" | "muted" }; note: string | null; skipped: string | null; toggle: { allowed: boolean; hint: string | null };
};
/** One rule card in the display language (IR260); a rule's own times stay in its time zone. */
export function ruleCard(a: ApiAutomation, unit: { name: string; path: string; property: string } | null, nextStart: string | null, consentGranted: boolean, i: I18n = EN): RuleCard {
  const { t } = i;
  const trigger = triggerOf(a);
  const place = unit?.property ?? t("the AC's property");
  const extra = a.unitIds.length - 1;
  const more = extra > 0 ? t(extra === 1 ? " (+{n} more AC)" : " (+{n} more ACs)", { n: extra }) : "";
  const where = unit ? `${unit.name}${more} · ${unit.path}` : t("AC no longer available");
  const status = a.enabled ? { text: t("On"), tone: "ok" as const } : a.disabledReason ? { text: t("Disabled · {reason}", { reason: t(reasonText[a.disabledReason]) }), tone: "warn" as const } : { text: t("Off"), tone: "muted" as const };
  const note = a.disabledReason === "consent_revoked" ? t("Will not run: location consent withdrawn {date}. Existing commands are not cancelled.", { date: showDate(a.updatedAt, i.display) })
    : a.disabledReason === "capability_changed" ? t("Will not run: the AC's capabilities changed — edit the actions, then switch it on again.")
    : a.disabledReason === "unit_archived" ? t("Will not run: the AC was archived.") : null;
  const skipped = !note && a.lastRun?.outcome === "skipped" ? t("Last evaluation skipped {when} — {reason}", { when: runText(a.lastRun.at, a.timezone, i), reason: decisionText[a.lastRun.reason ?? ""] ? t(decisionText[a.lastRun.reason ?? ""]) : a.lastRun.reason ?? t("no reason recorded") }) : null;
  const needsConsent = trigger === "location" && !consentGranted;
  return {
    id: a.id, version: a.version, name: a.name, trigger, when: whenText(a, place, t), then: thenText(a, t), enabled: a.enabled, status, note, skipped,
    place: `${where}${a.enabled && nextStart ? t(" · next run {when}", { when: runText(nextStart, a.timezone, i) }) : ""}`,
    toggle: { allowed: !(needsConsent && !a.enabled) && !!unit, hint: needsConsent && !a.enabled ? t("Grant location consent first") : unit ? null : t("The AC is no longer available") },
  };
}

/** The editor draft (DD-C04 / DD-C05 fields); actions are option keys of actionOptions. */
export type Draft = {
  id: string | null; version: number | null; name: string; trigger: Trigger; unitId: string; others: string[]; timezone: string; enabled: boolean; priority: number;
  weekdays: number[]; startLocal: string; endLocal: string; endsNextDay: boolean; startAction: string; endAction: string;
  occupied: boolean; event: "arrival" | "departure"; localTime: string; operator: Compare; value: string; action: string; extras: ExtraDraft[];
};
/** One “Only if” row of the editor (weather value kept as typed). */
export type ExtraDraft = { type: "weekday"; weekdays: number[] } | { type: "occupancy"; occupied: boolean } | { type: "weather"; operator: Compare; value: string };
export const extraLabel: Record<ExtraDraft["type"], string> = { weekday: "Weekdays", occupancy: "Occupancy", weather: "Outdoor temperature" };
/** The “Only if” types a draft may still add (IR215): no weekday on a schedule, not the trigger's own type, each once, at most 3. */
export function extraChoices(d: Draft): ExtraDraft["type"][] {
  if (d.extras.length >= 3) return [];
  const own = d.trigger === "presence" ? "occupancy" : d.trigger === "weather" ? "weather" : null;
  return (["weekday", "occupancy", "weather"] as const).filter((t) => !(t === "weekday" && d.trigger === "schedule") && t !== own && !d.extras.some((x) => x.type === t));
}
export const newExtra = (t: ExtraDraft["type"]): ExtraDraft => (t === "weekday" ? { type: t, weekdays: [1, 2, 3, 4, 5] } : t === "occupancy" ? { type: t, occupied: true } : { type: t, operator: "gte", value: "30" });
/** Extras that no longer fit the trigger (switching the trigger) are dropped. */
export function fitExtras(d: Draft): ExtraDraft[] {
  const own = d.trigger === "presence" ? "occupancy" : d.trigger === "weather" ? "weather" : null;
  return d.extras.filter((x) => !(x.type === "weekday" && d.trigger === "schedule") && x.type !== own);
}
const extraOf = (x: ExtraDraft): ExtraCondition => (x.type === "weather" ? { type: "weather", metric: "temperature", operator: x.operator, value: Number(x.value) } : x);
const extraDraftOf = (x: ExtraCondition): ExtraDraft => (x.type === "weather" ? { type: "weather", operator: x.operator, value: String(x.value) } : x);
export const actionKey = (a: UnitAction): string =>
  a.kind === "set_power" ? `power:${a.power ? "on" : "off"}` : a.kind === "set_temperature" ? `temp:${a.celsius}` : a.kind === "set_mode" ? `mode:${a.mode}` : `fan:${a.fanLevel}`;
export function actionOf(key: string): UnitAction {
  const [k, v] = key.split(":");
  if (k === "power") return { kind: "set_power", power: v === "on" };
  if (k === "temp") return { kind: "set_temperature", celsius: Number(v) };
  if (k === "mode") return { kind: "set_mode", mode: v as "cool" | "dry" | "fan" };
  return { kind: "set_fan", fanLevel: v as "low" | "mid" | "high" };
}
export function newDraft(unitId: string, timezone: string): Draft {
  return {
    id: null, version: null, name: "", trigger: "schedule", unitId, others: [], timezone, enabled: false, priority: 50,
    weekdays: [1, 2, 3, 4, 5], startLocal: "18:00", endLocal: "22:00", endsNextDay: false, startAction: "temp:25", endAction: "power:off",
    occupied: false, event: "arrival", localTime: "18:00", operator: "gte", value: "33", action: "temp:25", extras: [],
  };
}
export function draftOf(a: ApiAutomation): Draft {
  const d = newDraft(a.unitIds[0] ?? "", a.timezone);
  const base = { ...d, id: a.id, version: a.version, name: a.name, enabled: a.enabled, priority: a.priority, trigger: triggerOf(a), others: a.unitIds.slice(1), // rules saved elsewhere may target more ACs: kept
    extras: (a.onlyIf ?? []).map(extraDraftOf) };
  if (a.kind === "schedule") {
    return { ...base, weekdays: a.weekdays, startLocal: a.startLocal, endLocal: a.endLocal, endsNextDay: a.endsNextDay, startAction: actionKey(a.startAction), endAction: actionKey(a.endAction) };
  }
  const c = a.condition;
  return {
    ...base, action: actionKey(a.action), occupied: c.type === "occupancy" ? c.occupied : d.occupied, event: c.type === "location" ? c.event : d.event,
    localTime: c.type === "pattern" ? c.localTime : d.localTime, operator: c.type === "weather" ? c.operator : d.operator, value: c.type === "weather" ? String(c.value) : d.value,
  };
}
export function conditionOf(d: Draft): ClientCondition {
  switch (d.trigger) {
    case "presence": return { type: "occupancy", occupied: d.occupied };
    case "location": return { type: "location", event: d.event };
    case "routine": return { type: "pattern", localTime: d.localTime };
    default: return { type: "weather", metric: "temperature", operator: d.operator, value: Number(d.value) };
  }
}
/** automations.save input (one AC per rule on this screen, Figma 03b “Action for one AC”). */
export function saveInput(d: Draft): OpInput<"automations.save"> {
  const base = { ...(d.id ? { id: d.id } : {}), name: d.name.trim(), unitIds: [d.unitId, ...d.others.filter((u) => u !== d.unitId)], timezone: d.timezone, enabled: d.enabled, priority: d.priority,
    onlyIf: fitExtras(d).map(extraOf) };
  return d.trigger === "schedule"
    ? { ...base, kind: "schedule", weekdays: [...d.weekdays].sort((a, b) => a - b), startLocal: d.startLocal, endLocal: d.endLocal, endsNextDay: d.endsNextDay, startAction: actionOf(d.startAction), endAction: actionOf(d.endAction) }
    : { ...base, kind: "event", condition: conditionOf(d), action: actionOf(d.action) };
}
/** The schedule part of a draft for the automations.nextRuns draft preview. */
export const scheduleDraft = (d: Draft) => ({
  timezone: d.timezone, weekdays: [...d.weekdays].sort((a, b) => a - b), startLocal: d.startLocal, endLocal: d.endLocal, endsNextDay: d.endsNextDay,
  startAction: actionOf(d.startAction), endAction: actionOf(d.endAction),
});
const hhmm = /^([01]\d|2[0-3]):[0-5]\d$/;
const minutes = (s: string) => Number(s.slice(0, 2)) * 60 + Number(s.slice(3, 5));
/** The checks automations.save makes, shown before saving (the API checks again). */
export function draftErrors(d: Draft, tr: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  const n = d.name.trim().length;
  if (n < 1 || n > 120) e.name = tr("1–120 characters");
  if (!d.unitId) e.unitId = tr("Choose the AC");
  if (d.trigger === "schedule") {
    if (d.weekdays.length === 0) e.weekdays = tr("Choose at least one weekday");
    if (!hhmm.test(d.startLocal)) e.startLocal = "HH:mm";
    if (!hhmm.test(d.endLocal)) e.endLocal = "HH:mm";
    if (!e.startLocal && !e.endLocal) {
      const s = minutes(d.startLocal);
      const t = minutes(d.endLocal);
      if (s === t) e.endLocal = tr("Start and end cannot be equal — not 24-hour operation");
      else if (t < s && !d.endsNextDay) e.endsNextDay = tr("The end is before the start — tick “Ends next day” for an overnight schedule");
      else if (t > s && d.endsNextDay) e.endsNextDay = tr("With “Ends next day” the end must be at or before the start (at most 24 hours)");
    }
  }
  if (d.trigger === "routine" && !hhmm.test(d.localTime)) e.localTime = "HH:mm";
  if (d.trigger === "weather" && !(d.value.trim() !== "" && Number(d.value) >= -50 && Number(d.value) <= 100)) e.value = tr("−50 to 100 °C");
  fitExtras(d).forEach((x, i) => {
    if (x.type === "weekday" && x.weekdays.length === 0) e[`extra${i}`] = tr("Choose at least one weekday");
    if (x.type === "weather" && !(x.value.trim() !== "" && Number(x.value) >= -50 && Number(x.value) <= 100)) e[`extra${i}`] = tr("−50 to 100 °C");
  });
  return e;
}
const fieldName: Record<string, string> = {
  name: "name", onlyIf: "onlyIf", weekdays: "weekdays", startLocal: "startLocal", endLocal: "endLocal", endsNextDay: "endsNextDay", condition: "condition", unitIds: "unitId",
  "draft.weekdays": "weekdays", "draft.startLocal": "startLocal", "draft.endLocal": "endLocal", "draft.endsNextDay": "endsNextDay", startAction: "startAction", endAction: "endAction", action: "action",
};
const keyText: Record<string, string> = {
  "error.length": "1–120 characters", "error.required": "Required", "error.invalid": "Not valid", "errors.same_as_start": "Start and end cannot be equal",
  "errors.overnight_required": "The end is before the start — tick “Ends next day”", "errors.over_24_hours": "At most 24 hours", "errors.local_time_invalid": "This local time does not exist or is ambiguous (daylight saving)",
  "errors.consent_required": "Location consent is required for a location trigger", "error.unsupportedAction": "The AC does not support this action",
  "errors.duplicate_condition": "Each “Only if” type once, and not the trigger's own type", "errors.weekday_on_schedule": "A schedule has its own weekdays",
};
/** API field errors of automations.save / nextRuns draft mapped to the editor fields. */
export function apiErrors(fe: Record<string, string>, t: T = en): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(fe)) out[fieldName[k] ?? k] = keyText[v] ? t(keyText[v]) : v.replace(/^errors?\./, "").replace(/_/g, " ");
  return out;
}

/** Action options for an AC (Figma 03b one select): power, the capability temperatures, modes and fan levels. */
export type Caps = { control: boolean; modeControl: boolean; fanControl: boolean; temperature: { min: number; max: number; step: number } | null; modes: ("cool" | "dry" | "fan")[]; fanLevels: ("low" | "mid" | "high")[] };
export function actionOptions(c: Caps | null, t: T = en): { group: string; options: { key: string; label: string }[] }[] {
  if (!c || !c.control) return [];
  const temps: { key: string; label: string }[] = [];
  if (c.temperature) for (let x = c.temperature.min; x <= c.temperature.max; x += c.temperature.step || 1) temps.push({ key: `temp:${x}`, label: t("Set temperature {celsius}°C", { celsius: x }) });
  return [
    { group: t("Power"), options: [{ key: "power:on", label: t("Power ON") }, { key: "power:off", label: t("Power OFF") }] },
    ...(temps.length ? [{ group: t("Temperature"), options: temps }] : []),
    ...(c.modeControl && c.modes.length ? [{ group: t("Mode"), options: c.modes.map((m) => ({ key: `mode:${m}`, label: actionLabel({ kind: "set_mode", mode: m }, t) })) }] : []),
    ...(c.fanControl && c.fanLevels.length ? [{ group: t("Fan"), options: c.fanLevels.map((f) => ({ key: `fan:${f}`, label: actionLabel({ kind: "set_fan", fanLevel: f }, t) })) }] : []),
  ];
}

/** The editor summary (Figma 03b/03e right panel). */
export function summaryText(d: Draft, unit: string, place: string, t: T = en): string {
  const set = (k: string) => {
    const a = actionOf(k);
    switch (a.kind) {
      case "set_power": return t(a.power ? "{unit} power ON" : "{unit} power OFF", { unit });
      case "set_temperature": return t("set {unit} to {value}", { unit, value: `${a.celsius}°C` });
      case "set_mode": return t("set {unit} to {value}", { unit, value: t("mode {mode}", { mode: (modeWord[a.mode] ? t(modeWord[a.mode]) : a.mode).toLowerCase() }) });
      default: return t("set {unit} to {value}", { unit, value: t("fan {level}", { level: (fanWord[a.fanLevel] ? t(fanWord[a.fanLevel]) : a.fanLevel).toLowerCase() }) });
    }
  };
  if (d.trigger === "schedule") {
    const days = d.weekdays.length ? weekdaysText(d.weekdays, true, t) : t("no weekday");
    return t(d.endsNextDay ? "When {days} at {start} → {startAction}. At {end} (next day) → {endAction}.{onlyIf}" : "When {days} at {start} → {startAction}. At {end} → {endAction}.{onlyIf}",
      { days, start: d.startLocal, startAction: set(d.startAction), end: d.endLocal, endAction: set(d.endAction), onlyIf: onlyIfSummary(d, t) });
  }
  const when = d.trigger === "location" ? t(d.event === "arrival" ? "you arrive at {place}" : "everyone leaves {place}", { place })
    : d.trigger === "presence" ? t(d.occupied ? "someone is in the room" : "no one is detected") : d.trigger === "routine" ? t("it is your usual time {time}", { time: d.localTime }) : t("the outdoor temperature is {op} {value}°C", { op: cmp[d.operator], value: d.value });
  return t("When {when} → {action}.{onlyIf}", { when, action: set(d.action), onlyIf: onlyIfSummary(d, t) });
}
function onlyIfSummary(d: Draft, t: T): string {
  const xs = fitExtras(d).filter((x) => x.type !== "weather" || x.value.trim() !== "").map((x) => extraText(extraOf(x), t));
  return xs.length ? t(" Only if {conditions}.", { conditions: xs.join(t(" and ")) }) : "";
}
export const summaryNote: Record<Trigger, string> = {
  schedule: "Weekdays are the start day. At run time the automation is re-checked (permissions, capabilities, restrictions).",
  presence: "Does not run if occupancy data is missing — missing data never counts as a match.",
  location: "Does not run if consent is withdrawn or location data is missing.",
  routine: "A demo estimate of your routine — no real behaviour history is collected.",
  weather: "Does not run if weather data is missing — the evaluation is skipped with a reason.",
};

/** One row of the test result (Figma 03c). */
export type TestRow = { at: string; title: string; detail: string; tone: "ok" | "muted" };
/** The schedule test: the next start and end from the preview and the first following day that is not selected. */
export function scheduleTest(runs: ApiOccurrence[], d: Draft, unit: string, i: I18n = EN): TestRow[] {
  const { t } = i;
  const start = runs.find((r) => r.phase === "schedule_start");
  const end = start ? runs.find((r) => r.phase === "schedule_end" && r.at > start.at) : undefined;
  const rows: TestRow[] = [];
  const would = (a: UnitAction) => t("Would send 1 command: {unit} · {action}", { unit, action: actionLabel(a, t) });
  if (start) rows.push({ at: start.at, title: t("{when} — matches", { when: runText(start.at, d.timezone, i) }), detail: would(start.action), tone: "ok" });
  if (end) rows.push({ at: end.at, title: t("{when} — end action", { when: runText(end.at, d.timezone, i) }), detail: would(end.action), tone: "ok" });
  if (start && d.weekdays.length < 7) {
    for (let k = 1; k <= 7; k++) {
      const at = new Date(Date.parse(start.at) + k * 86_400_000).toISOString();
      const wd = new Date(at).toLocaleDateString("en-US", { timeZone: d.timezone, weekday: "short" }); // the index, not the label
      const iso = dayShort.indexOf(wd) + 1;
      if (iso > 0 && !d.weekdays.includes(iso)) {
        rows.push({ at, title: t("{when} — no match", { when: runText(at, d.timezone, i) }), detail: t("{day} is not selected — nothing would be sent", { day: t(dayLong[iso - 1]) }), tone: "muted" });
        break;
      }
    }
  }
  return rows;
}
const decisionText: Record<string, string> = {
  missing_data: "condition data is missing — never counted as a match", stale: "condition data is out of date", no_match: "the condition does not match",
  consent_revoked: "location consent is withdrawn", owner_forbidden: "your permission to control this AC has ended", restricted: "an active restriction blocks this action",
  disabled: "the automation is off", busy: "the AC is busy with another command", offline: "the AC is offline", invalid_capability: "the AC does not support the action",
  no_control_action: "only a notification would be sent", reconciliation_required: "the AC must be reconciled first",
};
/** The event test: what automations.simulate decided for the AC at the current tick (all the AC's rules arbitrated). */
export function eventTest(dec: ApiDecision | undefined, ruleId: string | null, names: Map<string, string>, unit: string, action: UnitAction, at: string, i: I18n = EN): TestRow {
  const { t } = i;
  const when = runText(at, KL, i);
  if (!dec) return { at, title: t("No result"), detail: t("The simulation returned no decision for this AC"), tone: "muted" };
  if (dec.decision === "selected" && dec.ruleId === ruleId) return { at, title: t("{when} — matches", { when }), detail: t("Would send 1 command: {unit} · {action}", { unit, action: actionLabel(action, t) }), tone: "ok" };
  if (dec.decision === "selected") return { at, title: t("{when} — another rule wins", { when }), detail: t("“{rule}” has priority for {unit} — this one would not send", { rule: names.get(dec.ruleId ?? "") ?? t("Another automation"), unit }), tone: "muted" };
  return { at, title: t("{when} — not sent", { when }), detail: t("Nothing would be sent: {reason}", { reason: decisionText[dec.reason ?? ""] ? t(decisionText[dec.reason ?? ""]) : dec.reason ?? t("no reason") }), tone: "muted" };
}
/** The synthetic fact a test sends for an event rule (Phase 1A demo events; the action adds the AC and the tick). */
export function testFact(d: Draft): Omit<OpInput<"automations.simulate">["facts"][number], "unitId" | "observedAt">[] {
  switch (d.trigger) {
    case "location": return [{ metric: "location", value: d.event as "arrival" | "departure", unit: "event", quality: "valid" }];
    case "presence": return [{ metric: "occupied", value: d.occupied, unit: "boolean", quality: "valid" }];
    case "weather": return [{ metric: "weather_temperature", value: Number(d.value), unit: "°C", quality: "valid" }];
    default: return []; // a routine matches only at its local time (IR52)
  }
}

/** The location consent card (Figma 03a/03f); its times in the user's display time zone (IR44). */
export function consentCard(c: ApiConsent | null, locationRules: number, disabledByConsent: number, i: I18n = EN): { granted: boolean; title: string; text: string; tone: "ok" | "warn" | "muted" } {
  const { t } = i;
  if (!c) return { granted: false, title: t("Not recorded"), text: t("No location consent record exists for your account — location automations cannot be used."), tone: "muted" };
  if (c.granted) {
    return { granted: true, title: t("Granted"), tone: "ok", text: t(locationRules === 1 ? "Granted {when} · used only by {n} location automation · demo location events (no real GPS history)." : "Granted {when} · used only by {n} location automations · demo location events (no real GPS history).", { when: showTime(c.grantedAt, i.display), n: locationRules }) };
  }
  if (c.revokedAt) {
    return { granted: false, title: t("Revoked"), tone: "warn", text: t(disabledByConsent === 1 ? "Withdrawn {when} · {n} location automation was turned off and will not run. Granting consent again does NOT turn them back on automatically — you re-enable each one. Manual control is unaffected." : "Withdrawn {when} · {n} location automations were turned off and will not run. Granting consent again does NOT turn them back on automatically — you re-enable each one. Manual control is unaffected.", { when: showTime(c.revokedAt, i.display), n: disabledByConsent }) };
  }
  return { granted: false, title: t("Not granted"), tone: "muted", text: t("Location automations need your consent. It is used only for arrival/departure automations — demo location events, no real GPS history.") };
}
