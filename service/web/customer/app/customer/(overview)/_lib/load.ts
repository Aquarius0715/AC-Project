// The reads of /customer (FR-C01, DD-C01, IR240) through the DAL: the customer's units with their latest readings and
// places (units.list, properties.list, spaces.list), the tiles (summaries.get for the property or unit), the unresolved
// alerts (alerts.list), the automations with the next run of the schedules shown (automations.list,
// automations.nextRuns, consents.get), energy.summary for the period, the previous period and each of the last 14 days
// (with a comparable baseline for the emissions saving, baselines.list), and the CO2 series of the last 24 h of the
// air-quality unit (telemetry.series). URL keys: propertyId, unitId, period (today, 7d, 30d). Texts and times in the
// user's display language and time zone (coreDisplay, IR260).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { byCount, i18nOf } from "@ac/web/lib/i18n";
import { dueNotice, unitPlaces, type ClientContract } from "@ac/web/lib/clientBilling";
import type { ApiInvoice } from "@ac/web/lib/billing";
import type { ApiRestriction } from "@ac/web/lib/restrictions";
import { baselinesFor, dayRanges, periodRange } from "@ac/web/lib/clientEnergy";
import { ruleCard, triggerOf, type ApiAutomation, type ApiConsent, type ApiOccurrence } from "@ac/web/lib/clientAutomations";
import { metricCard, type ApiAirSeries } from "@ac/web/lib/air";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiPropertyRow, ApiSpaceRow } from "@ac/web/lib/assets";
import type { ApiBaseline, ApiEnergySummary } from "@ac/web/lib/energy";
import {
  attentionCard, bucketPoints, emissionsCard, energyCard, nextRunText, overviewRows, overviewScope, periodLabel, periodNote, periodOf, previousRange, type OverviewUnit,
} from "@ac/web/lib/customerOverview";
import { orNull } from "@ac/web/lib/promises";

const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function loadOverview(sp: Record<string, string | string[] | undefined>) {
  interface Summary { counts: { total: number; powerOn: number; powerOff: number; powerUnknown: number; alertCount: number }; asOf: string }
  const kind = periodOf(one(sp.period));
  const [now, units, properties, spaces, alerts, automations, baselines, consent, display] = await Promise.all([
    coreNow(), coreAll<OverviewUnit>("units.list"), coreAll<ApiPropertyRow>("properties.list"), coreAll<ApiSpaceRow>("spaces.list"),
    coreAll<ApiAlert>("alerts.list"), coreAll<ApiAutomation>("automations.list"), coreAll<ApiBaseline>("baselines.list"),
    orNull(coreOp<ApiConsent>("consents.get", { purpose: "location_automation" })), coreDisplay(),
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
  const propertyFilter = sel.propertyId ? { propertyId: sel.propertyId } : {};
  const filters = sel.unitId ? { unitId: sel.unitId } : propertyFilter;
  const baseline = baselinesFor(baselines, ids)[0];
  const sum = (from: string, to: string, b?: string) => coreOp<ApiEnergySummary>("energy.summary", { from, to, unitIds: ids, ...(b && { baselineId: b }) });
  const energy = ids.length > 0 ? Promise.all([
    sum(period.from, period.to, baseline?.id), sum(prev.from, prev.to),
    Promise.all(week.map((d) => sum(d.from, d.to))), Promise.all(week.map((d) => sum(new Date(Date.parse(d.from) - 7 * 86_400_000).toISOString(), new Date(Date.parse(d.to) - 7 * 86_400_000).toISOString()))),
  ]) : Promise.resolve(null);
  // the air-quality card: the chosen unit, else the first unit shown with a CO2 reading
  const air = sel.scope.find((u) => u.id === sel.unitId) ?? sel.scope.find((u) => u.latestMeasurements.some((m) => m.metric === "co2")) ?? sel.scope[0] ?? null;
  const hasCo2 = !!air?.latestMeasurements.some((m) => m.metric === "co2");
  // the last 24 hours in hourly averages of the valid CO2 readings, whole (bucketMinutes, IR605, IR607)
  const series = async () => {
    const to = Math.floor(nowMs / 60_000) * 60_000;
    const page: ApiAirSeries = await coreOp("telemetry.series", { from: new Date(to - 86_400_000).toISOString(), to: new Date(to).toISOString(), unitIds: [air.id], metric: "co2", bucketMinutes: 60, query: {} });
    return bucketPoints(page.buckets ?? []);
  };
  // what is due or restricted (DEC-90, IR610): the overdue invoices and the restriction they caused, read as the payments
  // page reads them; a member without billing access sees no notice
  const due = (async () => {
    const [overdue, contracts] = await Promise.all([coreAll<ApiInvoice>("invoices.list", { filters: { overdueOnly: true } }), coreAll<ClientContract>("contracts.list")]);
    const k = contracts.find((c) => (c.activeRestrictionIds ?? []).length > 0 && overdue.some((x) => x.contractId === c.id));
    let restriction: { r: ApiRestriction; invoiceId: string } | null = null;
    if (k) {
      const invoiceId = overdue.find((x) => x.contractId === k.id)!.id;
      const page = await coreOp<{ items: ApiRestriction[] }>("restrictions.forInvoice", { invoiceId, query: { limit: 10 } });
      const r = page.items.find((x) => (k.activeRestrictionIds ?? []).includes(x.id));
      if (r) restriction = { r, invoiceId };
    }
    return dueNotice(overdue, restriction, i);
  })().catch((error: unknown) => { if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null; throw error; });
  const inScope = automations.filter((a) => a.unitIds.some((u) => ids.includes(u))).toSorted((a, b) => Number(b.enabled) - Number(a.enabled) || a.name.localeCompare(b.name));
  const shown = inScope.slice(0, 3);
  const [summary, sums, co2, nexts, dueLines] = await Promise.all([
    coreOp<Summary>("summaries.get", { kind: "customer", filters }), energy, hasCo2 ? series() : Promise.resolve(null),
    Promise.all(shown.map((a) => (a.kind === "schedule" && a.enabled ? orNull(coreOp<ApiOccurrence[]>("automations.nextRuns", { automationId: a.id, count: 8 }).then((o) => o.find((x) => x.phase === "schedule_start")?.at ?? null)) : Promise.resolve(null)))), due
  ]);
  const unitName = new Map(units.map((u) => [u.id, u.displayName]));
  const n = sel.scope.length;
  // the selected unit, the units of the selected property, or all units
  const scopeTextOf = (): string => {
    if (sel.unitId) return unitName.get(sel.unitId) ?? t("Unit");
    if (sel.propertyName) return byCount(n, { one: t("{n} unit in {property}", { n, property: sel.propertyName }), many: t("{n} units in {property}", { n, property: sel.propertyName }) });
    return byCount(n, { one: t("all {n} unit", { n }), many: t("all {n} units", { n }) });
  };
  const scopeText = scopeTextOf();
  const roomOf = (u: OverviewUnit) => (u.spaceId ? spaces.find((s) => s.id === u.spaceId)?.name : null) ?? properties.find((p) => p.id === u.propertyId)?.name ?? t("Room");
  return {
    due: dueLines,
    now: now.toISOString(), period: kind, periodLabel: t(periodLabel[kind]), note: periodNote(kind, period, i), asOf: summary.asOf, display,
    properties: properties.filter((p) => !p.archived).map((p) => ({ id: p.id, name: p.name })), propertyId: sel.propertyId, propertyName: sel.propertyName,
    units: sel.inProperty.map((u) => ({ id: u.id, name: u.displayName })), unitId: sel.unitId,
    counts: summary.counts,
    rows: overviewRows(sel.scope, (u) => place.get(u.id)?.place ?? "", i),
    energy: sums ? energyCard(kind, sums[0], sums[1], week.map((d, k) => ({ label: d.label.split(" ", 1)[0], kWh: sums[2][k].totals.kWh })), sums[3].map((s) => s.totals.kWh), scopeText, i) : null,
    emissions: sums ? { ...emissionsCard(sums[0], baseline ? baseline.method : null, i), scope: scopeText } : null,
    air: air ? {
      id: air.id, room: roomOf(air), unit: air.displayName, co2: metricCard("co2", air.latestMeasurements.find((m) => m.metric === "co2"), true, i),
      pm25: metricCard("pm25", air.latestMeasurements.find((m) => m.metric === "pm25"), true, i), points: co2 ?? [],
    } : null,
    attention: attentionCard(alerts, sel.scope, nowMs, 3, i),
    automations: {
      on: inScope.filter((a) => a.enabled).length, off: inScope.filter((a) => !a.enabled).length,
      items: shown.map((a, k) => {
        const u = units.find((x) => x.id === a.unitIds[0]);
        const card = ruleCard(a, u ? { name: u.displayName, path: place.get(u.id)?.place ?? "", property: properties.find((p) => p.id === u.propertyId)?.name ?? "" } : null, nexts[k], !!consent?.granted, i);
        const consentWord = t(consent?.granted ? "granted" : "not granted");
        const idle = triggerOf(a) === "location" ? t("{when} · location consent: {state}", { when: card.when, state: consentWord }) : card.when;
        const line = nexts[k] ? t("Next run: {when} · {unit}", { when: nextRunText(nexts[k], nowMs, i), unit: u?.displayName ?? "AC" }) : idle;
        return { id: a.id, name: a.name, status: card.status, line };
      }),
    },
  };
}

export type OverviewLive = Awaited<ReturnType<typeof loadOverview>>;
