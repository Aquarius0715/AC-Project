// /customer/air-quality (FR-C07, SCR-C07): in API mode a Server Component reads the customer's rooms (properties.list,
// spaces.list, units.list), the selected unit's units.get (latest readings with read-time quality, capability sensors and
// fresh-air function), telemetry.series of the selected metric and period — newest first, at most 1000 readings in 10
// pages of 100 (D07), with the IR98 allergen observation — and the room's ventilation.list. Log ventilation is
// ventilation.log, a manual record that creates no Command and is not sent to HQ (IR110). URL keys: spaceId, unitId,
// metric, period. Texts and times in the user's display language and time zone (IR266). The Phase 1A demo keeps the
// fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf, showClock } from "@ac/web/lib/i18n";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import {
  airGuidance, airMetrics, airPeriods, airRooms, airRows, airSelection, airSlots, airWindow, allergenView, axisLabels, axisRange, axisTicks, cleanNote, co2Now,
  metricCard, metricInfo, ventRow, ventStrip, windowSub, windowTitle, type ApiAirSeries, type ApiMeasurement, type ApiVentilationLog,
} from "@ac/web/lib/air";
import { AirQualityDemo } from "./_components/air-quality-demo";
import { AirQualityView, type AirLive } from "./_components/air-quality-view";

const MAX_READINGS = 1000; // D07: 10 pages of 100; a cut-off series is labelled, never shown as complete

/** telemetry.series of one unit, newest first, following nextCursor up to MAX_READINGS. */
async function readSeries(unitId: string, metric: string, from: string, to: string) {
  const items: ApiMeasurement[] = [];
  let cursor: string | null = null;
  let total = 0;
  let allergen: ApiAirSeries["allergenObservation"] = null;
  do {
    const page: ApiAirSeries = await coreOp("telemetry.series", { from, to, unitIds: [unitId], metric, query: { limit: 100, sort: { field: "observedAt", direction: "desc" }, ...(cursor ? { cursor } : {}) } });
    items.push(...page.items);
    ({ total, nextCursor: cursor, allergenObservation: allergen } = page);
  } while (cursor && items.length < MAX_READINGS);
  return { items, total, allergen };
}

export default async function CustomerAirQualityPage({ searchParams }: PageProps<"/customer/air-quality">) {
  await connection();
  if (!apiMode()) return <AirQualityDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [me, now, properties, spaces, units, display] = await Promise.all([
    corePrincipal(), coreNow(), coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"), coreAll<ApiUnitRow>("units.list"), coreDisplay(),
  ]);
  const i = i18nOf(display);
  const rooms = airRooms(properties, spaces, units, i.t);
  const sel = airSelection(rooms, one("unitId"), one("spaceId"));
  if (!sel) return <AirQualityView live={null} />;
  const metric = airMetrics.find((m) => m === one("metric")) ?? "co2";
  const period = airPeriods.find((p) => p === one("period")) ?? "24h";
  const w = airWindow(period, now);
  const [d, series, logs] = await Promise.all([
    coreOp<ApiUnitDetail>("units.get", { id: sel.unitId }),
    readSeries(sel.unitId, metric, w.from, w.to),
    sel.room.spaceId ? coreOp<{ items: ApiVentilationLog[]; total: number }>("ventilation.list", { filters: { spaceId: sel.room.spaceId }, limit: 10 }) : null,
  ]);
  const latest = (m: string) => d.latestMeasurements.find((x) => x.metric === m);
  const sensors = new Set(d.capabilities.sensors.map((s) => s.metric));
  const freshAir = d.capabilities.ventilation && d.capabilities.ventilationLevels.includes("low");
  const guidance = airGuidance(latest("co2"), latest("pm25"), freshAir);
  const roomName = sel.room.spaceId ? spaces.find((s) => s.id === sel.room.spaceId)?.name ?? i.t("this room") : sel.room.path; // outside rooms: the property
  const slots = airSlots(series.items, w);
  const points = slots.map((s) => s.avg);
  const info = metricInfo[metric];
  const range = axisRange(points, info.guide);
  const names = new Map(units.map((u) => [u.id, u.displayName]));
  const live: AirLive = {
    rooms: rooms.map((r) => ({ key: r.key, spaceId: r.spaceId, label: r.label, units: r.units })), roomKey: sel.room.key, unitId: d.id, unitName: d.displayName,
    path: sel.room.path, spaceId: sel.room.spaceId, connection: d.connection, updated: showClock(now.toISOString(), display), freshAir,
    cards: (["co2", "pm25", "temperature", "humidity"] as const).map((m) => metricCard(m, latest(m), sensors.has(m), i)),
    strip: ventStrip(guidance, latest("co2"), roomName, d.displayName, freshAir, i.t),
    clean: cleanNote(guidance, latest("pm25"), roomName, i.t),
    allergen: allergenView(series.allergen, i), metric, period,
    chart: {
      title: `${i.t(info.label)} — ${windowTitle(period, i.t)}`, sub: windowSub(w, info.unit, roomName, i), points, labels: axisLabels(w, i), guide: info.guide, ...range, ticks: axisTicks(range.min, range.max),
      hasSensor: sensors.has(metric), readings: series.items.length, total: series.total, valid: slots.some((s) => s.avg !== null), gaps: slots.some((s) => s.avg === null),
    },
    rows: airRows(slots, metric, w.slotMs, i), co2Now: co2Now(latest("co2"), display),
    history: logs ? { rows: logs.items.map((v) => ventRow(v, me.membershipId, names, now.getTime(), i)), total: logs.total } : null,
  };
  return <AirQualityView live={live} />;
}
