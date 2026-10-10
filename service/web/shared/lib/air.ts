// Customer air quality (FR-C07, DATA_SOURCE=api): the room choices, the metric cards from the unit's latest readings
// (UnitSummary.latestMeasurements with read-time quality, IR213), the IR99 guidance, the IR98 allergen observation, the
// IR41 series window with its 5-minute or hourly averages, and the manual ventilation log (IR110). Pure code shared by
// the Server Component and the client view. Texts in the display language (`t` / `i`); every instant in the user's
// display time zone, the 7-day window's start a Kuala Lumpur calendar day that the chart names (IR266).
import { periodRange } from "@ac/web/lib/clientEnergy";
import { roundAway } from "@ac/web/lib/energy";
import { spacePath, type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow } from "@ac/web/lib/assets";
import { DEFAULT_DISPLAY, EN, relativeTime, showClock, showDate, showDay, showSpan, translator, type Display, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

export type AirMetric = "co2" | "pm25" | "temperature" | "humidity";
export const airMetrics: AirMetric[] = ["co2", "pm25", "temperature", "humidity"];
export type AirPeriod = "1h" | "24h" | "7d";
export const airPeriods: AirPeriod[] = ["1h", "24h", "7d"];
type Tone = "ok" | "warn" | "crit" | "unknown" | "primary" | "muted";

/** Measurement of service-contracts.ts (fields shown here). */
export type ApiMeasurement = {
  id: string; unitId: string; sensorId: string; metric: string; value: number | null; unit: string; observedAt: string;
  origin: "measured" | "estimated" | "inspection"; quality: "valid" | "missing" | "stale" | "suspect"; qualityReason: string | null;
};
/** AllergenObservation of service-contracts.ts (IR98). */
export type ApiAllergen = {
  availability: "not_measured" | "unsupported" | "available"; substance: string | null; value: number | null; unit: string | null;
  sourceLabel: string | null; observedAt: string | null; evidenceText: string | null;
};
/** AirSeries of service-contracts.ts: Page<Measurement> & {allergenObservation}. */
export type ApiAirSeries = { items: ApiMeasurement[]; nextCursor: string | null; total: number; allergenObservation: ApiAllergen | null };
export type VentMethod = "window_opened" | "door_opened" | "ventilation_fan" | "other";
/** VentilationLog of service-contracts.ts. */
export type ApiVentilationLog = {
  id: string; spaceId: string; unitId: string | null; method: VentMethod; durationMinutes: number;
  co2AtLog: { value: number; unit: string; observedAt: string } | null; loggedByMembershipId: string; loggedAt: string;
};

export const metricInfo: Record<AirMetric, { label: string; tab: string; unit: string; guide: number | null }> = {
  co2: { label: "CO2", tab: "CO2", unit: "ppm", guide: 1000 },
  pm25: { label: "PM2.5", tab: "PM2.5", unit: "µg/m³", guide: 35 },
  temperature: { label: "Temperature", tab: "Temp", unit: "°C", guide: null },
  humidity: { label: "Humidity", tab: "Humidity", unit: "%", guide: null },
};

const KL = "Asia/Kuala_Lumpur";
const nf =(digits: number) => new Intl.NumberFormat("en-MY", { minimumFractionDigits: digits, maximumFractionDigits: digits, useGrouping: false });
/** IR44: temperature and humidity with one decimal, ppm and µg/m³ as integers, rounded half away from zero; other
 * metrics as received. */
export function airNumber(metric: string, v: number): string {
  const digits = metric === "temperature" || metric === "humidity" ? 1 : metric === "co2" || metric === "pm25" ? 0 : null;
  return digits === null ? String(v) : nf(digits).format(roundAway(v, digits));
}

/** A choice of the viewing bar (Figma Client 05a “Room: Bedroom · Home A › 1F”): a space with units directly in it, or a
 * property's units outside any room (they have readings but no ventilation log — logs belong to a room). */
export type AirRoom = { key: string; spaceId: string | null; label: string; path: string; units: { id: string; name: string }[] };
export function airRooms(ps: ApiPropertyRow[], ss: ApiSpaceRow[], us: ApiUnitRow[], t: T = en): AirRoom[] {
  const props = new Map(ps.filter((p) => !p.archived).map((p) => [p.id, p.name]));
  const live = us.filter((u) => !u.archived && props.has(u.propertyId));
  const units = (keep: (u: ApiUnitRow) => boolean) => live.filter(keep).map((u) => ({ id: u.id, name: u.displayName })).sort((a, b) => a.name.localeCompare(b.name));
  const out: (AirRoom & { sort: string })[] = [];
  for (const s of ss.filter((x) => !x.archived && props.has(x.propertyId))) {
    const here = units((u) => u.spaceId === s.id);
    const property = props.get(s.propertyId)!;
    if (here.length) out.push({ key: s.id, spaceId: s.id, label: `${s.name} · ${[property, spacePath(s.parentSpaceId, ss)].filter(Boolean).join(" › ")}`, path: `${property} › ${spacePath(s.id, ss)}`, units: here, sort: `${property}\u0000${spacePath(s.id, ss)}` });
  }
  for (const [id, property] of props) {
    const loose = units((u) => u.propertyId === id && u.spaceId === null);
    if (loose.length) out.push({ key: `unassigned:${id}`, spaceId: null, label: t("Not in a room · {property}", { property }), path: property, units: loose, sort: `${property}\u0000￿` });
  }
  return out.sort((a, b) => a.sort.localeCompare(b.sort)).map((r) => ({ key: r.key, spaceId: r.spaceId, label: r.label, path: r.path, units: r.units }));
}
/** The room and unit shown: the URL unit when it is listed, else the URL room's (or the first room's) first unit. */
export function airSelection(rooms: AirRoom[], unitId?: string, spaceId?: string): { room: AirRoom; unitId: string } | null {
  const byUnit = unitId ? rooms.find((r) => r.units.some((u) => u.id === unitId)) : undefined;
  if (byUnit) return { room: byUnit, unitId: unitId! };
  const room = rooms.find((r) => r.spaceId !== null && r.spaceId === spaceId) ?? rooms[0];
  return room ? { room, unitId: room.units[0].id } : null;
}

/** One metric card (Figma Client 05a/05b): the feed badge, value, status, observation line and advice. A stale, null or
 * suspect reading is never shown as a current normal value (D07); humidity 0 is a measured zero. */
export type MetricCard = {
  metric: AirMetric; label: string; unit: string; value: string | null;
  feed: { text: string; tone: Tone; icon: string }; status: { text: string; tone: Tone; icon: string }; sub: string; advice: string; warn: boolean;
};
const reasonText: Record<string, string> = { unit_mismatch: "unit mismatch", non_finite: "not a number", out_of_range: "out of range", invalid_time: "invalid time" };
const originText: Record<string, string> = { estimated: "estimated", inspection: "inspection" };
/** The card in the display language; reading times are the IR44 time of day in the user's display time zone (IR260). */
export function metricCard(metric: AirMetric, m: ApiMeasurement | undefined, hasSensor: boolean, i: I18n = EN): MetricCard {
  const { t, display } = i;
  const { unit, guide } = metricInfo[metric];
  const label = t(metricInfo[metric].label);
  const noAdvice = t(metric === "temperature" ? "Setpoint is on Unit Control" : metric === "humidity" ? "Not enough data for advice" : "Not enough data for {metric} advice", { metric: label });
  const unavailable = (sub: string, feed = { text: t("Unavailable"), tone: "unknown" as Tone, icon: "⊘" }): MetricCard =>
    ({ metric, label, unit, value: null, feed, status: { text: t("Not measured"), tone: "muted", icon: "○" }, sub, advice: noAdvice, warn: false });
  if (!hasSensor) return unavailable(t("No {metric} sensor on this model", { metric: label }), { text: t("Unsupported"), tone: "muted", icon: "⊘" });
  if (!m) return unavailable(t("No reading yet"));
  const at = showClock(m.observedAt, display);
  if (m.quality === "stale") return unavailable(m.value !== null ? t("No reading since {time} (last {value} {unit})", { time: at, value: airNumber(metric, m.value), unit }) : t("No reading since {time}", { time: at }));
  if (m.quality === "suspect") {
    return {
      metric, label, unit, value: m.value === null ? null : airNumber(metric, m.value), feed: { text: t("Suspect"), tone: "warn", icon: "!" },
      status: { text: t("Suspect"), tone: "warn", icon: "!" }, sub: t("Observed {time} · {reason}", { time: at, reason: t(reasonText[m.qualityReason ?? ""] ?? "unreliable reading") }), advice: t("Not used for advice"), warn: false,
    };
  }
  if (m.value === null) return unavailable(t(metric === "humidity" ? "No reading (null) — not 0% · {time}" : "No reading (null) · {time}", { time: at }), { text: t("Unknown"), tone: "unknown", icon: "?" });
  const high = guide !== null && m.value >= guide;
  const status = guide === null ? { text: t("Measured"), tone: "muted" as Tone, icon: "●" } : high ? { text: t("High"), tone: "warn" as Tone, icon: "⚠" } : { text: t("Within guide"), tone: "ok" as Tone, icon: "✓" };
  const advice = t(metric === "co2" ? (high ? "Ventilation recommended (≥ 1000 ppm)" : "No current advice")
    : metric === "pm25" ? (high ? "Filter cleaning recommended (≥ 35 µg/m³)" : "No current advice")
    : metric === "temperature" ? "Setpoint is on Unit Control" : "No current advice");
  return {
    metric, label, unit, value: airNumber(metric, m.value), feed: { text: t("Live"), tone: "ok", icon: "●" }, status,
    sub: `${t("Observed {time}", { time: at })}${m.origin === "measured" ? "" : ` · ${t(originText[m.origin] ?? m.origin)}`}`, advice, warn: high,
  };
}

/** IR99 guidance from the latest valid CO2 and PM2.5 (all that apply); no guidance from temperature or humidity. */
export type Guidance = "ventilate" | "ventilate_manual" | "clean" | "none" | "unavailable";
export const guidanceText: Record<Guidance, string> = {
  ventilate: "Ventilation recommended", ventilate_manual: "Ventilate manually, for example by opening a window", clean: "Filter cleaning and inspection recommended",
  none: "No current guidance", unavailable: "Not enough data to provide guidance",
};
const validValue = (m?: ApiMeasurement) => (m && m.quality === "valid" && m.value !== null ? m.value : null);
export function airGuidance(co2: ApiMeasurement | undefined, pm25: ApiMeasurement | undefined, freshAir: boolean): Guidance[] {
  const c = validValue(co2);
  const p = validValue(pm25);
  const out: Guidance[] = [];
  if (c !== null && c >= 1000) out.push(freshAir ? "ventilate" : "ventilate_manual");
  if (p !== null && p >= 35) out.push("clean");
  if (!out.length) out.push(c === null && p === null ? "unavailable" : "none");
  return out;
}
/** The ventilation strip above the chart (Figma 05a/05b): the CO2 guidance or how ventilation works for this unit. Log
 * ventilation is offered in every case; nothing is ever sent to the AC or to HQ (IR110). */
export function ventStrip(g: Guidance[], co2: ApiMeasurement | undefined, room: string, unit: string, freshAir: boolean, t: T = en): { tone: "warn" | "primary"; title: string; text: string } {
  const v = validValue(co2);
  const reading = v === null ? "" : `${t("CO2 {value} ppm in {room} (≥ 1000 ppm guide).", { value: airNumber("co2", v), room })} `;
  if (g.includes("ventilate")) return { tone: "warn", title: t(guidanceText.ventilate), text: reading + t("Open a window or run your ventilation fan, then log it — the record is kept in this room’s ventilation history.") };
  if (g.includes("ventilate_manual")) return { tone: "warn", title: t(guidanceText.ventilate_manual), text: reading + t("{unit} has no fresh-air function, so nothing is sent to the AC — open a window, then log what you did.", { unit }) };
  if (g.includes("unavailable")) {
    return { tone: "primary", title: t(guidanceText.unavailable), text: `${t("Neither CO2 nor PM2.5 has a current reading — no advice is given from missing data.")} ${freshAir
      ? t("You can still log a manual ventilation.") : t("Manual ventilation: {unit} has no fresh-air function, so open a window or use a fan and log what you did — nothing is sent to the AC.", { unit })}` };
  }
  if (!freshAir) return { tone: "primary", title: t("Manual ventilation"), text: t("Open a window or use a fan. {unit} has no fresh-air function, so nothing is sent to the AC — just log what you did.", { unit }) };
  return { tone: "primary", title: t(guidanceText.none), text: t("After opening a window or running your ventilation fan, log it — nothing is sent to the AC or to HQ.") };
}
/** The PM2.5 cleaning banner (IR99): the latest valid reading against the 35 µg/m³ guide, or null. */
export function cleanNote(g: Guidance[], pm25: ApiMeasurement | undefined, room: string, t: T = en): string | null {
  const v = validValue(pm25);
  return g.includes("clean") && v !== null
    ? t("{guidance} — PM2.5 {value} µg/m³ in {room} (≥ 35 µg/m³ guide). Request a filter clean from Maintenance.", { guidance: t(guidanceText.clean), value: airNumber("pm25", v), room }) : null;
}

/** The allergen strip (IR98): an observation needs substance, source, time and evidence, a number also its unit; missing
 * or unsupported data never reads as “no allergens”, and nothing is derived from PM2.5. */
export function allergenView(a: ApiAllergen | null, i: I18n = EN): { title: string; badge: { text: string; tone: Tone; icon: string }; text: string } {
  const { t, display } = i;
  const title = t("Allergen");
  if (!a) return { title, badge: { text: t("Unknown"), tone: "unknown", icon: "?" }, text: t("No observation for this selection — this does not mean “no allergens”.") };
  if (a.availability === "unsupported") return { title, badge: { text: t("Unsupported"), tone: "muted", icon: "⊘" }, text: t("This unit cannot observe allergens — this does not mean “no allergens”.") };
  if (a.availability === "not_measured") return { title, badge: { text: t("Not measured"), tone: "muted", icon: "○" }, text: t("No observation — this does not mean “no allergens”.") };
  if (!a.substance || !a.sourceLabel || !a.observedAt || !a.evidenceText) return { title, badge: { text: t("Unknown"), tone: "unknown", icon: "?" }, text: t("Incomplete observation — shown as unknown, this does not mean “no allergens”.") };
  const value = a.value === null ? [] : [a.unit ? `${a.value} ${a.unit}` : t("Value unknown (no unit)")];
  return {
    title: t("Allergen ({substance})", { substance: a.substance }), badge: { text: t("Detected"), tone: "warn", icon: "⚠" },
    text: [...value, t("Source: {source}", { source: a.sourceLabel }), showClock(a.observedAt, display), a.evidenceText].join(" · "),
  };
}

/** [from, to) of a preset (IR41): 1h/24h roll back from the business clock floored to the UTC minute; 7d is SR17 calendar
 * days (00:00 six days earlier in Kuala Lumpur). Chart slots: 5-minute averages for 1h/24h, hourly averages for 7d. */
export type AirWindow = { period: AirPeriod; from: string; to: string; slotMs: number };
export function airWindow(period: AirPeriod, now: Date): AirWindow {
  if (period === "7d") {
    const r = periodRange("7d", now);
    return { period, from: r.from, to: r.to, slotMs: 3_600_000 };
  }
  const to = Math.floor(now.getTime() / 60_000) * 60_000;
  return { period, from: new Date(to - (period === "1h" ? 60 : 1440) * 60_000).toISOString(), to: new Date(to).toISOString(), slotMs: 300_000 };
}
export const windowTitle = (p: AirPeriod, t: T = en) => t(p === "1h" ? "last hour" : p === "24h" ? "last 24 hours" : "last 7 days");
/** The chart's sub line: unit, room and the window — its end in the display time zone with the zone's abbreviation, and
 * for 7 days the Kuala Lumpur calendar day it starts on. */
export function windowSub(w: AirWindow, unit: string, room: string, i: I18n = EN): string {
  const { t, display } = i;
  const time = showClock(w.to, display);
  const span = w.period === "7d"
    ? t("calendar days in Asia/Kuala_Lumpur from {date} to {time} now · hourly averages", { date: showDate(w.from, { locale: display.locale, timeZone: KL }), time })
    : t("rolling window ending {time} · 5-min averages", { time });
  return `${unit} · ${room} · ${span}`;
}

/** The averages of valid readings per slot of the window (null = no valid reading: a gap the chart never joins). */
export type AirSlot = { start: string; avg: number | null; n: number };
export function airSlots(items: ApiMeasurement[], w: AirWindow): AirSlot[] {
  const from = Date.parse(w.from);
  const to = Date.parse(w.to);
  const n = Math.max(1, Math.ceil((to - from) / w.slotMs));
  const sum = Array<number>(n).fill(0);
  const cnt = Array<number>(n).fill(0);
  for (const m of items) {
    const t = Date.parse(m.observedAt);
    if (m.quality !== "valid" || m.value === null || t < from || t >= to) continue;
    const i = Math.floor((t - from) / w.slotMs);
    sum[i] += m.value;
    cnt[i] += 1;
  }
  return sum.map((s, i) => ({ start: new Date(from + i * w.slotMs).toISOString(), avg: cnt[i] ? s / cnt[i] : null, n: cnt[i] }));
}
/** Five x-axis labels from the window start to now (Figma 05a “09:12 yesterday … 09:12 now”) in the display time zone,
 * which the sub line names. */
export function axisLabels(w: AirWindow, i: I18n = EN): string[] {
  const { t, display } = i;
  const from = Date.parse(w.from);
  const span = Date.parse(w.to) - from;
  return [0, 1, 2, 3, 4].map((k) => {
    const at = new Date(from + (span * k) / 4).toISOString();
    const time = showClock(at, display, false);
    if (k === 4) return t("{time} now", { time });
    if (w.period === "7d") return showDay(at, display); // "Tue 8"
    return k === 0 && w.period === "24h" ? t("{time} yesterday", { time }) : time;
  });
}
/** The y range of the chart: the values and the guide line with some room around them. */
export function axisRange(points: (number | null)[], guide: number | null): { min: number; max: number } {
  const vals = [...points.filter((p): p is number => p !== null), ...(guide === null ? [] : [guide])];
  if (!vals.length) return { min: 0, max: 1 };
  const lo = Math.min(...vals);
  const hi = Math.max(...vals);
  const pad = Math.max((hi - lo) * 0.15, Math.abs(hi) * 0.05, 1);
  return { min: Math.floor(lo - pad), max: Math.ceil(hi + pad) };
}
/** About four round y-axis grid values inside [min, max] (steps of 1, 2 or 5 × 10ⁿ). */
export function axisTicks(min: number, max: number): number[] {
  const raw = (max - min) / 4;
  const mag = 10 ** Math.floor(Math.log10(raw || 1));
  const stepSize = [1, 2, 5, 10].map((f) => f * mag).find((s) => s >= raw) ?? 10 * mag;
  const out: number[] = [];
  for (let t = Math.ceil(min / stepSize) * stepSize; t <= max; t += stepSize) out.push(roundAway(t, 6));
  return out;
}
/** The table view: newest slot first, consecutive empty slots merged into one gap row; each row's slot as a span in the
 * display time zone. */
export type AirRow = { key: string; time: string; value: string; note: string; gap: boolean };
export function airRows(slots: AirSlot[], metric: AirMetric, slotMs: number, i: I18n = EN): AirRow[] {
  const { t, display } = i;
  const out: AirRow[] = [];
  const end = (s: AirSlot) => new Date(Date.parse(s.start) + slotMs).toISOString();
  let gap: { newest: AirSlot; oldest: AirSlot } | null = null;
  const flush = () => {
    if (gap) out.push({ key: `gap-${gap.oldest.start}`, time: showSpan(gap.oldest.start, end(gap.newest), display), value: t("No data"), note: t("Gap — not joined"), gap: true });
    gap = null;
  };
  for (const s of [...slots].reverse()) {
    if (s.avg === null) {
      gap = gap ? { newest: gap.newest, oldest: s } : { newest: s, oldest: s };
      continue;
    }
    flush();
    out.push({ key: s.start, time: showSpan(s.start, end(s), display), value: `${airNumber(metric, s.avg)} ${metricInfo[metric].unit}`, note: t(s.n === 1 ? "{n} reading" : "{n} readings", { n: s.n }), gap: false });
  }
  flush();
  return out;
}

export const ventMethods: { id: VentMethod; label: string }[] = [
  { id: "window_opened", label: "Window opened" }, { id: "door_opened", label: "Door opened" }, { id: "ventilation_fan", label: "Ventilation fan" }, { id: "other", label: "Other" },
];
/** Durations offered by the Log ventilation modal (Figma 05c); ventilation.log accepts 1–240 minutes (IR110). */
export const ventDurations = [5, 10, 15, 20, 30, 45, 60, 90, 120, 180, 240];
/** The latest valid CO2 as the modal's “Current CO2” (what co2AtLog will record), or null. */
export const co2Now = (co2?: ApiMeasurement, d: Display = DEFAULT_DISPLAY) => (validValue(co2) === null ? null : `${airNumber("co2", co2!.value!)} ppm · ${showClock(co2!.observedAt, d)}`);
/** One ventilation history row: method and duration, the CO2 recorded at logging time, who and when (“today …” on the
 * days next to now). */
export function ventRow(v: ApiVentilationLog, me: string, names: Map<string, string>, nowMs: number, i: I18n = EN) {
  const { t, display } = i;
  const method = ventMethods.find((m) => m.id === v.method);
  return {
    id: v.id, text: [method ? t(method.label) : v.method, t("{n} min", { n: v.durationMinutes }), ...(v.unitId ? [names.get(v.unitId) ?? t("another unit")] : [])].join(" · "),
    co2: v.co2AtLog ? t("CO2 {value} ppm at {time}", { value: airNumber("co2", v.co2AtLog.value), time: showClock(v.co2AtLog.observedAt, display) }) : t("CO2 not measured at logging"),
    by: t(v.loggedByMembershipId === me ? "by you" : "by another member"), when: relativeTime(v.loggedAt, nowMs, i),
  };
}
