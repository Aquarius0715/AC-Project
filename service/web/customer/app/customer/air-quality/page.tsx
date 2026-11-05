// /customer/air-quality (FR-C07, SCR-C07): in API mode a Server Component reads the customer's rooms (properties.list,
// spaces.list, units.list), the selected unit's units.get (latest readings with read-time quality, capability sensors and
// fresh-air function), telemetry.series of the selected metric and period — the whole window in the chart's slots,
// 5-minute or hourly averages of the valid readings (bucketMinutes, IR605, IR607), with the IR98 allergen
// observation — and the room's ventilation.list. Log ventilation is
// ventilation.log, a manual record that creates no Command and is not sent to HQ (IR110). URL keys: spaceId, unitId,
// metric, period. Texts and times in the user's display language and time zone (IR266). The Phase 1A demo keeps the
// fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf, showClock } from "@ac/web/lib/i18n";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import { setsTemperature, type ApiUnitDetail } from "@ac/web/lib/units";
import {
  airGuidance, airMetrics, airPeriods, airRooms, airRows, airSelection, airWindow, allergenView, axisLabels, axisRange, axisTicks, cleanNote, co2Now,
  metricCard, metricInfo, ventRow, ventStrip, windowSub, windowTitle, type AirSlot, type AirWindow, type ApiAirSeries, type ApiVentilationLog,
} from "@ac/web/lib/air";
import { AirQualityDemo } from "./_components/air-quality-demo";
import { AirQualityView, type AirLive } from "./_components/air-quality-view";
import type { OpInput } from "@ac/web/lib/opTypes";
import { queryReader } from "@ac/web/lib/searchParams";

/** telemetry.series of one unit over the window, whole: the chart's slots — 5-minute averages for an hour or a day, hourly
for 7 days — of the valid readings, with how many readings the window holds (IR605, IR607). */
async function readSeries(unitId: string, metric: OpInput<"telemetry.series">["metric"], w: AirWindow) {
  const page: ApiAirSeries = await coreOp("telemetry.series", { from: w.from, to: w.to, unitIds: [unitId], bucketMinutes: w.slotMs === 300_000 ? 5 : 60, query: {}, metric });
  const slots: AirSlot[] = (page.buckets ?? []).map((b) => ({ start: b.start, avg: b.value, n: b.count }));
  return { slots, total: page.total, allergen: page.allergenObservation };
}

export default async function CustomerAirQualityPage({ searchParams }: Readonly<PageProps<"/customer/air-quality">>) {
  await connection();
  if (!apiMode()) return <AirQualityDemo />;
  const sp = await searchParams;
  const one = queryReader(sp);
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
    readSeries(sel.unitId, metric, w),
    sel.room.spaceId ? coreOp<{ items: ApiVentilationLog[]; total: number }>("ventilation.list", { filters: { spaceId: sel.room.spaceId }, limit: 10 }) : null,
  ]);
  const latest = (m: string) => d.latestMeasurements.find((x) => x.metric === m);
  const sensors = new Set(d.capabilities.sensors.map((s) => s.metric));
  const isFreshAir = d.capabilities.ventilation && d.capabilities.ventilationLevels.includes("low");
  const guidance = airGuidance(latest("co2"), latest("pm25"), isFreshAir);
  const roomName = sel.room.spaceId ? spaces.find((s) => s.id === sel.room.spaceId)?.name ?? i.t("this room") : sel.room.path; // outside rooms: the property
  const slots = series.slots; // the whole window: never cut off, so no slot is "not loaded" (IR592 → IR607)
  const points = slots.map((s) => s.avg);
  const info = metricInfo[metric];
  const range = axisRange(points, info.guide);
  const names = new Map(units.map((u) => [u.id, u.displayName]));
  const live: AirLive = {
    rooms: rooms.map((r) => ({ key: r.key, spaceId: r.spaceId, label: r.label, units: r.units })),
    roomKey: sel.room.key,
    unitId: d.id,
    unitName: d.displayName,
    path: sel.room.path,
    spaceId: sel.room.spaceId,
    connection: d.connection,
    updated: showClock(now.toISOString(), display),
    freshAir: isFreshAir,
    cards: (["co2", "pm25", "temperature", "humidity"] as const).map((m) => metricCard(m, latest(m), sensors.has(m), i, setsTemperature(d.capabilities))),
    strip: ventStrip(guidance, latest("co2"), roomName, d.displayName, isFreshAir, i.t),
    clean: cleanNote(guidance, latest("pm25"), roomName, i.t),
    allergen: allergenView(series.allergen, i),
    chart: {
      title: `${i.t(info.label)} — ${windowTitle(period, i.t)}`, sub: windowSub(w, info.unit, roomName, i), points, labels: axisLabels(w, i), guide: info.guide, ...range, ticks: axisTicks(range.min, range.max),
      hasSensor: sensors.has(metric), readings: series.total, total: series.total, valid: slots.some((s) => s.avg !== null), gaps: slots.some((s) => s.avg === null),
    },
    rows: airRows(slots, metric, w.slotMs, i),
    co2Now: co2Now(latest("co2"), display),
    history: logs ? { rows: logs.items.map((v) => ventRow(v, me.membershipId, names, now.getTime(), i)), total: logs.total } : null,
    metric,
    period,
  };
  return <AirQualityView live={live} />;
}
