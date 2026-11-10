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
  airChartSeries, airGuidance, airMetricsOf, airPeriods, airRooms, airRows, airSelection, airWindow, allergenView, axisLabels, cleanNote, co2Now,
  metricCard, metricInfo, slotHeadings, ventRow, ventStrip, windowSub, windowTitle, type AirSlot, type AirWindow, type ApiAirSeries, type ApiVentilationLog,
} from "@ac/web/lib/air";
import { AirQualityDemo } from "./_components/air-quality-demo";
import { AirQualityView, type AirLive } from "./_components/air-quality-view";
import type { OpInput } from "@ac/web/lib/opTypes";
import { queryReader } from "@ac/web/lib/searchParams";
import { entryOf } from "@ac/web/lib/lookup";

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
  const metrics = airMetricsOf(one("metric")); // "co2,pm25": several metrics in one chart (IR689); a single name is the old link
  const period = airPeriods.find((p) => p === one("period")) ?? "24h";
  const w = airWindow(period, now);
  const [d, results, logs] = await Promise.all([
    coreOp<ApiUnitDetail>("units.get", { id: sel.unitId }),
    Promise.all(metrics.map((m) => readSeries(sel.unitId, m, w))),
    sel.room.spaceId ? coreOp<{ items: ApiVentilationLog[]; total: number }>("ventilation.list", { filters: { spaceId: sel.room.spaceId }, limit: 10 }) : null,
  ]);
  const latest = (m: string) => d.latestMeasurements.find((x) => x.metric === m);
  const sensors = new Set(d.capabilities.sensors.map((s) => s.metric));
  const isFreshAir = d.capabilities.ventilation && d.capabilities.ventilationLevels.includes("low");
  const guidance = airGuidance(latest("co2"), latest("pm25"), isFreshAir);
  const roomName = sel.room.spaceId ? spaces.find((s) => s.id === sel.room.spaceId)?.name ?? i.t("this room") : sel.room.path; // outside rooms: the property
  const byMetric = Object.fromEntries(metrics.map((m, k) => [m, results.at(k)]));
  const lines = airChartSeries(metrics, byMetric, sensors, i); // the whole window: never cut off, so no slot is "not loaded" (IR592 → IR607)
  const first = results.at(0);
  const metric = metrics[0] ?? "co2";
  const heading = metrics.map((m) => i.t(entryOf(metricInfo, m).label)).join(" · ");
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
    allergen: allergenView(first?.allergen ?? null, i),
    chart: {
      title: `${heading} — ${windowTitle(period, i.t)}`, sub: windowSub(w, [...new Set(lines.map((l) => l.unit))].join(" / "), roomName, i), labels: axisLabels(w, i), times: slotHeadings(first?.slots ?? [], w.slotMs, i),
      series: lines, gaps: results.some((r) => r.slots.some((s) => s.avg === null)),
    },
    tables: metrics.map((m, k) => ({ metric: m, label: i.t(entryOf(metricInfo, m).label), unit: entryOf(metricInfo, m).unit, rows: airRows(results.at(k)?.slots ?? [], m, w.slotMs, i) })),
    co2Now: co2Now(latest("co2"), display),
    history: logs ? { rows: logs.items.map((v) => ventRow(v, me.membershipId, names, now.getTime(), i)), total: logs.total } : null,
    metric,
    metrics,
    period,
  };
  return <AirQualityView live={live} />;
}
