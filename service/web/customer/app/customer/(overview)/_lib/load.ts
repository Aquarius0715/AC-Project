// The reads of /customer (FR-C01, DD-C01, IR240) through the DAL: the customer's units with their latest readings and
// places (units.list, properties.list, spaces.list), the tiles (summaries.get for the property or unit), the unresolved
// alerts (alerts.list), the automations with the next run of the schedules shown (automations.list,
// automations.nextRuns, consents.get), energy.summary for the period, the previous period and each of the last 14 days
// (with a comparable baseline for the emissions saving, baselines.list), and the CO2 series of the last 24 h of the
// air-quality unit (telemetry.series). URL keys: propertyId, unitId, period (today, 7d, 30d). Texts and times in the
// user's display language and time zone (coreDisplay, IR260).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { unitPlaces } from "@ac/web/lib/clientBilling";
import { baselinesFor, dayRanges, periodRange } from "@ac/web/lib/clientEnergy";
import { ruleCard, triggerOf, type ApiAutomation, type ApiConsent, type ApiOccurrence } from "@ac/web/lib/clientAutomations";
import { metricCard, type ApiAirSeries, type ApiMeasurement } from "@ac/web/lib/air";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiPropertyRow, ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiBaseline, ApiEnergySummary } from "@ac/web/lib/energy";
import {
  attentionCard, emissionsCard, energyCard, hourlyPoints, nextRunText, overviewRows, overviewScope, periodLabel, periodNote, periodOf, previousRange, type OverviewUnit,
} from "@ac/web/lib/customerOverview";

type Summary = { counts: { total: number; powerOn: number; powerOff: number; powerUnknown: number; alertCount: number }; asOf: string };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function loadOverview(sp: Record<string, string | string[] | undefined>) {
  const kind = periodOf(one(sp.period));
  const [now, units, properties, spaces, alerts, automations, baselines, consent, display] = await Promise.all([
    coreNow(), coreAll<OverviewUnit>("units.list"), coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"),
    coreAll<ApiAlert>("alerts.list"), coreAll<ApiAutomation>("automations.list"), coreAll<ApiBaseline>("baselines.list"),
    coreOp<ApiConsent>("consents.get", { purpose: "location_automation" }).catch(() => null), coreDisplay(),
  ]);
  const i = i18nOf(display);
  const { t } = i;
  const nowMs = now.getTime();
  const sel = overviewScope(units, properties, { propertyId: one(sp.propertyId), unitId: one(sp.unitId) });
  const ids = sel.scope.map((u) => u.id);
  const place = unitPlaces(units, properties, spaces);
  const period = periodRange(kind, now);
  const prev = previousRange(period);
  const week = dayRanges(periodRange("7d", now).from, period.to, display.locale).slice(-7);
  const filters = sel.unitId ? { unitId: sel.unitId } : sel.propertyId ? { propertyId: sel.propertyId } : {};
  const baseline = baselinesFor(baselines, ids)[0];
  const sum = (from: string, to: string, b?: string) => coreOp<ApiEnergySummary>("energy.summary", { from, to, unitIds: ids, ...(b ? { baselineId: b } : {}) });
  const energy = ids.length ? Promise.all([
    sum(period.from, period.to, baseline?.id), sum(prev.from, prev.to),
    Promise.all(week.map((d) => sum(d.from, d.to))), Promise.all(week.map((d) => sum(new Date(Date.parse(d.from) - 7 * 86_400_000).toISOString(), new Date(Date.parse(d.to) - 7 * 86_400_000).toISOString()))),
  ]) : Promise.resolve(null);
  // the air-quality card: the chosen unit, else the first unit shown with a CO2 reading
  const air = sel.scope.find((u) => u.id === sel.unitId) ?? sel.scope.find((u) => u.latestMeasurements.some((m) => m.metric === "co2")) ?? sel.scope[0] ?? null;
  const hasCo2 = !!air?.latestMeasurements.some((m) => m.metric === "co2");
  const series = async () => {
    const items: ApiMeasurement[] = [];
    let cursor: string | null = null;
    do { // newest first, at most 300 readings for the 24-hour chart
      const page: ApiAirSeries = await coreOp("telemetry.series", { from: new Date(nowMs - 86_400_000).toISOString(), to: now.toISOString(), unitIds: [air!.id], metric: "co2", query: { limit: 100, sort: { field: "observedAt", direction: "desc" }, ...(cursor ? { cursor } : {}) } });
      items.push(...page.items);
      cursor = page.nextCursor;
    } while (cursor && items.length < 300);
    return { items, cut: !!cursor };
  };
  const inScope = automations.filter((a) => a.unitIds.some((u) => ids.includes(u))).sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.name.localeCompare(b.name));
  const shown = inScope.slice(0, 3);
  const [summary, sums, co2, nexts] = await Promise.all([
    coreOp<Summary>("summaries.get", { kind: "customer", filters }), energy, hasCo2 ? series() : Promise.resolve(null),
    Promise.all(shown.map((a) => (a.kind === "schedule" && a.enabled ? coreOp<ApiOccurrence[]>("automations.nextRuns", { automationId: a.id, count: 8 }).then((o) => o.find((x) => x.phase === "schedule_start")?.at ?? null).catch(() => null) : Promise.resolve(null)))),
  ]);
  const unitName = new Map(units.map((u) => [u.id, u.displayName]));
  const n = sel.scope.length;
  const scopeText = sel.unitId ? unitName.get(sel.unitId)!
    : sel.propertyName ? t(n === 1 ? "{n} unit in {property}" : "{n} units in {property}", { n, property: sel.propertyName })
    : t(n === 1 ? "all {n} unit" : "all {n} units", { n });
  const roomOf = (u: OverviewUnit) => (u.spaceId ? spaces.find((s) => s.id === u.spaceId)?.name : null) ?? properties.find((p) => p.id === u.propertyId)?.name ?? t("Room");
  return {
    now: now.toISOString(), period: kind, periodLabel: t(periodLabel[kind]), note: periodNote(kind, period, i), asOf: summary.asOf, display,
    properties: properties.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name })), propertyId: sel.propertyId, propertyName: sel.propertyName,
    units: sel.inProperty.map((u) => ({ id: u.id, name: u.displayName })), unitId: sel.unitId,
    counts: summary.counts,
    rows: overviewRows(sel.scope, (u) => place.get(u.id)?.place ?? "", i),
    energy: sums ? energyCard(kind, sums[0], sums[1], week.map((d, k) => ({ label: d.label.split(" ")[0], kWh: sums[2][k].totals.kWh })), sums[3].map((s) => s.totals.kWh), scopeText, i) : null,
    emissions: sums ? { ...emissionsCard(sums[0], baseline ? baseline.method : null, i), scope: scopeText } : null,
    air: air ? {
      id: air.id, room: roomOf(air), unit: air.displayName, co2: metricCard("co2", air.latestMeasurements.find((m) => m.metric === "co2"), true, i),
      pm25: metricCard("pm25", air.latestMeasurements.find((m) => m.metric === "pm25"), true, i), points: co2 ? hourlyPoints(co2.items, nowMs) : [], cut: co2?.cut ?? false,
    } : null,
    attention: attentionCard(alerts, sel.scope, nowMs, 3, i),
    automations: {
      on: inScope.filter((a) => a.enabled).length, off: inScope.filter((a) => !a.enabled).length,
      items: shown.map((a, k) => {
        const u = units.find((x) => x.id === a.unitIds[0]);
        const card = ruleCard(a, u ? { name: u.displayName, path: place.get(u.id)?.place ?? "", property: properties.find((p) => p.id === u.propertyId)?.name ?? "" } : null, nexts[k], !!consent?.granted, i);
        const line = nexts[k] ? t("Next run: {when} · {unit}", { when: nextRunText(nexts[k]!, nowMs, i), unit: u?.displayName ?? "AC" })
          : triggerOf(a) === "location" ? t("{when} · location consent: {state}", { when: card.when, state: t(consent?.granted ? "granted" : "not granted") }) : card.when;
        return { id: a.id, name: a.name, line, status: card.status };
      }),
    },
  };
}

export type OverviewLive = Awaited<ReturnType<typeof loadOverview>>;
