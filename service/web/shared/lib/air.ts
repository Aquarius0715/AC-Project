// Customer air quality (FR-C07, DATA_SOURCE=api): the room choices, the metric cards from the unit's latest readings
// (UnitSummary.latestMeasurements with read-time quality, IR213), the IR99 guidance, the IR98 allergen observation, the
// IR41 series window with its 5-minute or hourly averages, and the manual ventilation log (IR110). Pure code shared by
// the Server Component and the client view.
import { periodRange } from "@ac/web/lib/clientEnergy";
import { klStamp, roundAway } from "@ac/web/lib/energy";
import { spacePath, type ApiPropertyRow, type ApiSpaceRow, type ApiUnitRow } from "@ac/web/lib/assets";

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
const hms = (iso: string) => new Date(iso).toLocaleTimeString("en-GB", { timeZone: KL, hour12: false });
const hm = (iso: string) => hms(iso).slice(0, 5);
const nf = (digits: number) => new Intl.NumberFormat("en-MY", { minimumFractionDigits: digits, maximumFractionDigits: digits, useGrouping: false });
/** IR44: temperature and humidity with one decimal, ppm and µg/m³ as integers, rounded half away from zero; other
 * metrics as received. */
export function airNumber(metric: string, v: number): string {
  const digits = metric === "temperature" || metric === "humidity" ? 1 : metric === "co2" || metric === "pm25" ? 0 : null;
  return digits === null ? String(v) : nf(digits).format(roundAway(v, digits));
}

/** A choice of the viewing bar (Figma Client 05a “Room: Bedroom · Home A › 1F”): a space with units directly in it, or a
 * property's units outside any room (they have readings but no ventilation log — logs belong to a room). */
export type AirRoom = { key: string; spaceId: string | null; label: string; path: string; units: { id: string; name: string }[] };
export function airRooms(ps: ApiPropertyRow[], ss: ApiSpaceRow[], us: ApiUnitRow[]): AirRoom[] {
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
    if (loose.length) out.push({ key: `unassigned:${id}`, spaceId: null, label: `Not in a room · ${property}`, path: property, units: loose, sort: `${property}\u0000￿` });
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
export function metricCard(metric: AirMetric, m: ApiMeasurement | undefined, hasSensor: boolean): MetricCard {
  const { label, unit, guide } = metricInfo[metric];
  const noAdvice = metric === "temperature" ? "Setpoint is on Unit Control" : metric === "humidity" ? "Not enough data for advice" : `Not enough data for ${label} advice`;
  const unavailable = (sub: string, feed = { text: "Unavailable", tone: "unknown" as Tone, icon: "⊘" }): MetricCard =>
    ({ metric, label, unit, value: null, feed, status: { text: "Not measured", tone: "muted", icon: "○" }, sub, advice: noAdvice, warn: false });
  if (!hasSensor) return unavailable(`No ${label} sensor on this model`, { text: "Unsupported", tone: "muted", icon: "⊘" });
  if (!m) return unavailable("No reading yet");
  if (m.quality === "stale") return unavailable(`No reading since ${hm(m.observedAt)}${m.value !== null ? ` (last ${airNumber(metric, m.value)} ${unit})` : ""}`);
  if (m.quality === "suspect") {
    return {
      metric, label, unit, value: m.value === null ? null : airNumber(metric, m.value), feed: { text: "Suspect", tone: "warn", icon: "!" },
      status: { text: "Suspect", tone: "warn", icon: "!" }, sub: `Observed ${hm(m.observedAt)} · ${reasonText[m.qualityReason ?? ""] ?? "unreliable reading"}`, advice: "Not used for advice", warn: false,
    };
  }
  if (m.value === null) return unavailable(`No reading (null)${metric === "humidity" ? " — not 0%" : ""} · ${hm(m.observedAt)}`, { text: "Unknown", tone: "unknown", icon: "?" });
  const high = guide !== null && m.value >= guide;
  const status = guide === null ? { text: "Measured", tone: "muted" as Tone, icon: "●" } : high ? { text: "High", tone: "warn" as Tone, icon: "⚠" } : { text: "Within guide", tone: "ok" as Tone, icon: "✓" };
  const advice = metric === "co2" ? (high ? "Ventilation recommended (≥ 1000 ppm)" : "No current advice")
    : metric === "pm25" ? (high ? "Filter cleaning recommended (≥ 35 µg/m³)" : "No current advice")
    : metric === "temperature" ? "Setpoint is on Unit Control" : "No current advice";
  return {
    metric, label, unit, value: airNumber(metric, m.value), feed: { text: "Live", tone: "ok", icon: "●" }, status,
    sub: `Observed ${hms(m.observedAt)}${m.origin === "measured" ? "" : ` · ${m.origin}`}`, advice, warn: high,
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
export function ventStrip(g: Guidance[], co2: ApiMeasurement | undefined, room: string, unit: string, freshAir: boolean): { tone: "warn" | "primary"; title: string; text: string } {
  const v = validValue(co2);
  const reading = v === null ? "" : `CO2 ${airNumber("co2", v)} ppm in ${room} (≥ 1000 ppm guide). `;
  if (g.includes("ventilate")) return { tone: "warn", title: guidanceText.ventilate, text: `${reading}Open a window or run your ventilation fan, then log it — the record is kept in this room’s ventilation history.` };
  if (g.includes("ventilate_manual")) return { tone: "warn", title: guidanceText.ventilate_manual, text: `${reading}${unit} has no fresh-air function, so nothing is sent to the AC — open a window, then log what you did.` };
  if (g.includes("unavailable")) {
    return { tone: "primary", title: guidanceText.unavailable, text: `Neither CO2 nor PM2.5 has a current reading — no advice is given from missing data. ${freshAir
      ? "You can still log a manual ventilation." : `Manual ventilation: ${unit} has no fresh-air function, so open a window or use a fan and log what you did — nothing is sent to the AC.`}` };
  }
  if (!freshAir) return { tone: "primary", title: "Manual ventilation", text: `Open a window or use a fan. ${unit} has no fresh-air function, so nothing is sent to the AC — just log what you did.` };
  return { tone: "primary", title: guidanceText.none, text: "After opening a window or running your ventilation fan, log it — nothing is sent to the AC or to HQ." };
}

/** The allergen strip (IR98): an observation needs substance, source, time and evidence, a number also its unit; missing
 * or unsupported data never reads as “no allergens”, and nothing is derived from PM2.5. */
export function allergenView(a: ApiAllergen | null): { title: string; badge: { text: string; tone: Tone; icon: string }; text: string } {
  const notNone = "this does not mean “no allergens”";
  if (!a) return { title: "Allergen", badge: { text: "Unknown", tone: "unknown", icon: "?" }, text: `No observation for this selection — ${notNone}.` };
  if (a.availability === "unsupported") return { title: "Allergen", badge: { text: "Unsupported", tone: "muted", icon: "⊘" }, text: `This unit cannot observe allergens — ${notNone}.` };
  if (a.availability === "not_measured") return { title: "Allergen", badge: { text: "Not measured", tone: "muted", icon: "○" }, text: `No observation — ${notNone}.` };
  if (!a.substance || !a.sourceLabel || !a.observedAt || !a.evidenceText) return { title: "Allergen", badge: { text: "Unknown", tone: "unknown", icon: "?" }, text: `Incomplete observation — shown as unknown, ${notNone}.` };
  const value = a.value === null ? "" : a.unit ? `${a.value} ${a.unit} · ` : "Value unknown (no unit) · ";
  return { title: `Allergen (${a.substance})`, badge: { text: "Detected", tone: "warn", icon: "⚠" }, text: `${value}Source: ${a.sourceLabel} · ${hm(a.observedAt)} · ${a.evidenceText}` };
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
export const windowTitle = (p: AirPeriod) => (p === "1h" ? "last hour" : p === "24h" ? "last 24 hours" : "last 7 days");
export function windowSub(w: AirWindow, unit: string, room: string): string {
  const span = w.period === "7d" ? `calendar days from ${klStamp(w.from).slice(0, 10)} to ${hm(w.to)} now · hourly averages` : `rolling window ending ${hm(w.to)} · 5-min averages`;
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
/** Five x-axis labels from the window start to now (Figma 05a “09:12 yesterday … 09:12 now”). */
export function axisLabels(w: AirWindow): string[] {
  const from = Date.parse(w.from);
  const span = Date.parse(w.to) - from;
  return [0, 1, 2, 3, 4].map((k) => {
    const t = new Date(from + (span * k) / 4).toISOString();
    const day = (o: Intl.DateTimeFormatOptions) => new Date(t).toLocaleDateString("en-US", { timeZone: KL, ...o });
    if (w.period === "7d") return k === 4 ? `${hm(t)} now` : `${day({ weekday: "short" })} ${day({ day: "numeric" })}`; // "Tue 8", not the en-US "8 Tue"
    return k === 4 ? `${hm(t)} now` : k === 0 && w.period === "24h" ? `${hm(t)} yesterday` : hm(t);
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
/** The table view: newest slot first, consecutive empty slots merged into one gap row. */
export type AirRow = { key: string; time: string; value: string; note: string; gap: boolean };
export function airRows(slots: AirSlot[], metric: AirMetric, slotMs: number): AirRow[] {
  const out: AirRow[] = [];
  const end = (s: AirSlot) => new Date(Date.parse(s.start) + slotMs).toISOString();
  let gap: { newest: AirSlot; oldest: AirSlot } | null = null;
  const flush = () => {
    if (gap) out.push({ key: `gap-${gap.oldest.start}`, time: `${klStamp(gap.oldest.start)} – ${klStamp(end(gap.newest)).slice(11)}`, value: "No data", note: "Gap — not joined", gap: true });
    gap = null;
  };
  for (const s of [...slots].reverse()) {
    if (s.avg === null) {
      gap = gap ? { newest: gap.newest, oldest: s } : { newest: s, oldest: s };
      continue;
    }
    flush();
    out.push({ key: s.start, time: klStamp(s.start), value: `${airNumber(metric, s.avg)} ${metricInfo[metric].unit}`, note: `${s.n} reading${s.n === 1 ? "" : "s"}`, gap: false });
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
export const co2Now = (co2?: ApiMeasurement) => (validValue(co2) === null ? null : `${airNumber("co2", co2!.value!)} ppm · ${hm(co2!.observedAt)}`);
/** One ventilation history row: method and duration, the CO2 recorded at logging time, who and when. */
export function ventRow(v: ApiVentilationLog, me: string, names: Map<string, string>) {
  const method = ventMethods.find((m) => m.id === v.method)?.label ?? v.method;
  return {
    id: v.id, text: `${method} · ${v.durationMinutes} min${v.unitId ? ` · ${names.get(v.unitId) ?? "another unit"}` : ""}`,
    co2: v.co2AtLog ? `CO2 ${airNumber("co2", v.co2AtLog.value)} ppm at ${hm(v.co2AtLog.observedAt)}` : "CO2 not measured at logging",
    by: v.loggedByMembershipId === me ? "you" : "another member", when: klStamp(v.loggedAt),
  };
}
export const updatedAt = (now: Date) => hms(now.toISOString());
