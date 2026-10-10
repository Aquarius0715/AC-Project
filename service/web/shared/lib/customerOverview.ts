// Customer overview (FR-C01, DD-C01, Figma Client 01a, IR240): the scope (property, unit) and period from the URL, the
// unit table from the latest readings (UnitSummary.latestMeasurements, IR213 — the measured room temperature, never the
// AC setting), the energy and emissions cards (energy.summary of the period against the previous one, the last 7 days
// against the 7 before), the air-quality card, Needs attention from the unresolved alerts (IR51) and the automations
// card. Pure code shared by the Server Component and the client view; texts and times in the user's display language and
// time zone (`i`, IR260), the period note in Asia/Kuala_Lumpur, where the periods are cut.
import { alertTitle, type ApiAlert } from "@ac/web/lib/alerts";
import { EN, intlTag, relativeTime, showClock, showTime, type I18n } from "@ac/web/lib/i18n";
import { airNumber, type ApiMeasurement } from "@ac/web/lib/air";
import type { ApiPropertyRow, ApiUnitRow } from "@ac/web/lib/assets";
import type { ApiEnergySummary } from "@ac/web/lib/energy";

export type OverviewPeriod = "today" | "7d" | "30d";
export const OVERVIEW_PERIODS: { id: OverviewPeriod; label: string }[] = [{ id: "today", label: "Today" }, { id: "7d", label: "7d" }, { id: "30d", label: "30d" }];
export const periodOf = (v?: string): OverviewPeriod => (v === "7d" || v === "30d" ? v : "today");
export type OverviewUnit = ApiUnitRow & { latestMeasurements: ApiMeasurement[] };

const KL = "Asia/Kuala_Lumpur";
const hm = (iso: string) => new Date(iso).toLocaleTimeString("en-GB", { timeZone: KL, hour: "2-digit", minute: "2-digit", hour12: false });
const md = (iso: string, i: I18n) => new Date(iso).toLocaleDateString(intlTag(i.display.locale), { timeZone: KL, month: "short", day: "numeric" });
const one = (v: number) => (Math.round(v * 10) / 10).toFixed(1);

/** The units shown: the URL property's (all properties by default) and, inside it, the URL unit. */
export function overviewScope(units: OverviewUnit[], properties: ApiPropertyRow[], q: { propertyId?: string; unitId?: string }) {
  const props = properties.filter((p) => !p.archived);
  const live = units.filter((u) => !u.archived && props.some((p) => p.id === u.propertyId));
  const propertyId = props.some((p) => p.id === q.propertyId) ? q.propertyId! : null;
  const inProperty = live.filter((u) => !propertyId || u.propertyId === propertyId).sort((a, b) => a.displayName.localeCompare(b.displayName));
  const unitId = inProperty.some((u) => u.id === q.unitId) ? q.unitId! : null;
  return {
    propertyId, unitId, inProperty, scope: unitId ? inProperty.filter((u) => u.id === unitId) : inProperty,
    propertyName: propertyId ? props.find((p) => p.id === propertyId)!.name : null,
  };
}

export const periodLabel: Record<OverviewPeriod, string> = { today: "Today", "7d": "Last 7 days", "30d": "Last 30 days" };

/** “Today = 00:00–09:12 · Asia/Kuala_Lumpur” (the period the energy figures cover; the periods are Kuala Lumpur days). */
export function periodNote(kind: OverviewPeriod, p: { from: string; to: string }, i: I18n = EN): string {
  return kind === "today" ? i.t("Today = {from}–{to} · {zone}", { from: hm(p.from), to: hm(p.to), zone: KL })
    : i.t("{period} = {from} – {to} · {zone}", { period: i.t(periodLabel[kind]), from: `${md(p.from, i)} ${hm(p.from)}`, to: `${md(p.to, i)} ${hm(p.to)}`, zone: KL });
}
/** The period of the same length that ends where this one starts. */
export function previousRange(p: { from: string; to: string }): { from: string; to: string } {
  const span = Date.parse(p.to) - Date.parse(p.from);
  return { from: new Date(Date.parse(p.from) - span).toISOString(), to: p.from };
}

const latestOf = (u: OverviewUnit, metric: string) => u.latestMeasurements.find((m) => m.metric === metric);
const current = (m?: ApiMeasurement) => (m && m.quality === "valid" && m.value !== null ? m.value : null);

export type OverviewRow = {
  id: string; name: string; place: string; observed: string; temp: string | null; hum: string | null; power: string | null;
  state: "running" | "stopped" | "unknown"; conn: "online" | "offline" | "connecting" | "unknown" | "error";
};
/** One row per unit: the latest valid room temperature, humidity and power (W); missing readings stay empty. */
export function overviewRows(units: OverviewUnit[], place: (u: OverviewUnit) => string, i: I18n = EN): OverviewRow[] {
  return units.map((u) => {
    const t = latestOf(u, "temperature"), h = latestOf(u, "humidity"), p = latestOf(u, "power");
    const last = [t, h, p].filter((m): m is ApiMeasurement => !!m).map((m) => m.observedAt).sort().at(-1);
    const watts = current(p) === null ? null : Math.round(p!.unit === "kW" ? p!.value! * 1000 : p!.value!);
    return {
      id: u.id, name: u.displayName, place: place(u),
      observed: u.connection === "online" ? (last ? showClock(last, i.display) : i.t("no reading yet")) : i.t("last seen {when}", { when: u.lastSeenAt ? showTime(u.lastSeenAt, i.display) : i.t("never") }),
      temp: current(t) === null ? null : airNumber("temperature", current(t)!), hum: current(h) === null ? null : airNumber("humidity", current(h)!),
      power: watts === null ? null : String(watts),
      state: u.effectivePowerState === "on" ? "running" : u.effectivePowerState === "off" ? "stopped" : "unknown", conn: u.connection,
    };
  });
}

export type EnergyCard = { sub: string; total: string | null; delta: { text: string; tone: "ok" | "warn" } | null; labels: string[]; series: number[][]; gaps: boolean };
const previousName: Record<OverviewPeriod, string> = { today: "yesterday at this time", "7d": "the 7 days before", "30d": "the 30 days before" };
/** The energy card: the period's kWh and its change against the previous period; bars for the last 7 days (dark)
 * against the 7 days before (light). A missing value is never a 0 kWh total. */
export function energyCard(kind: OverviewPeriod, cur: ApiEnergySummary | null, prev: ApiEnergySummary | null, week: { label: string; kWh: number | null }[], before: (number | null)[], scope: string, i: I18n = EN): EnergyCard {
  const total = cur?.totals.kWh ?? null;
  const was = prev?.totals.kWh ?? null;
  const delta = total !== null && was !== null && was > 0 ? Math.round(((total - was) / was) * 100) : null;
  return {
    sub: `${i.t(periodLabel[kind])} · ${scope}`, total: total === null ? null : one(total),
    delta: delta === null ? null : { text: i.t("{arrow} {pct}% vs {previous} ({kWh} kWh)", { arrow: delta < 0 ? "↓" : delta > 0 ? "↑" : "±", pct: Math.abs(delta), previous: i.t(previousName[kind]), kWh: one(was!) }), tone: delta <= 0 ? "ok" : "warn" },
    labels: week.map((d) => d.label), series: [before.map((v) => v ?? 0), week.map((d) => d.kWh ?? 0)], gaps: week.some((d) => d.kWh === null) || before.some((v) => v === null),
  };
}

export type EmissionsCard = { total: string | null; factor: string | null; baseline: string | null; saved: string | null };
/** Estimated emissions of the period (the factor of energy.summary) and, with a comparable baseline, its emissions and
 * the estimated saving — an estimate, never a tradable balance. */
export function emissionsCard(cur: ApiEnergySummary | null, baselineLabel: string | null, i: I18n = EN): EmissionsCard {
  const e = cur?.totals.emissionsKg ?? null;
  const saved = cur?.totals.savedEmissionsKg ?? null;
  const f = cur?.factorSnapshot ?? null;
  return {
    total: e === null ? null : one(e), factor: f ? i.t("{region} {year} · {factor} kgCO₂e/kWh (demo factor)", { region: f.region, year: f.year, factor: f.kgCO2ePerKWh }) : null,
    baseline: e !== null && saved !== null ? `${one(e + saved)} kgCO₂e${baselineLabel ? ` (${baselineLabel})` : ""}` : null, saved: saved === null ? null : `${one(saved)} kgCO₂e`,
  };
}

/** “today 9:12 am MYT”, “yesterday 10:40 pm MYT”, else the date and time — in the user's display time zone (IR260). */
export const whenText = (iso: string, nowMs: number, i: I18n = EN) => relativeTime(iso, nowMs, i);

export type AttentionCard = { items: { id: string; title: string; severity: "critical" | "warning"; where: string }[]; more: number; info: { count: number; text: string } };
const rank = { critical: 0, warning: 1, normal: 2 };
/** Needs attention: the unresolved critical and warning alerts of the units shown, most severe and newest first (IR51);
 * unresolved information (normal severity) is counted separately and never in the attention count. */
export function attentionCard(alerts: ApiAlert[], units: OverviewUnit[], nowMs: number, max = 3, i: I18n = EN): AttentionCard {
  const names = new Map(units.map((u) => [u.id, u.displayName]));
  const open = alerts.filter((a) => names.has(a.unitId) && a.status !== "resolved");
  const warn = open.filter((a) => a.severity !== "normal").sort((a, b) => rank[a.severity] - rank[b.severity] || b.detectedAt.localeCompare(a.detectedAt));
  const info = open.filter((a) => a.severity === "normal");
  const titles = [...new Set(info.map((a) => alertTitle(a, i.t).toLowerCase()))];
  return {
    items: warn.slice(0, max).map((a) => ({ id: a.id, title: alertTitle(a, i.t), severity: a.severity as "critical" | "warning", where: `${names.get(a.unitId)} · ${whenText(a.detectedAt, nowMs, i)}` })),
    more: Math.max(0, warn.length - max), info: { count: info.length, text: titles.join(", ") },
  };
}

/** The hourly averages of the valid readings of the last 24 h (null = no reading in that hour). */
export function hourlyPoints(items: ApiMeasurement[], nowMs: number): (number | null)[] {
  const to = Math.floor(nowMs / 60_000) * 60_000;
  const from = to - 24 * 3_600_000;
  const sum = Array<number>(24).fill(0), n = Array<number>(24).fill(0);
  for (const m of items) {
    const t = Date.parse(m.observedAt);
    if (m.quality !== "valid" || m.value === null || t < from || t >= to) continue;
    const i = Math.floor((t - from) / 3_600_000);
    sum[i] += m.value;
    n[i] += 1;
  }
  return sum.map((s, i) => (n[i] ? Math.round(s / n[i]) : null));
}

/** “today 6:00 pm MYT”, “tomorrow 7:30 am MYT”, else the date and time — the next run of a schedule rule (IR260). */
export const nextRunText = (iso: string, nowMs: number, i: I18n = EN) => relativeTime(iso, nowMs, i);
