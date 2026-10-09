// /customer/energy (FR-C06, FR-C16, SCR-C06): in API mode a Server Component reads the customer's units.list and
// baselines.list (only baselines whose units are all the customer's, IR90) and, for the URL selection, energy.summary
// for the period with the chosen baseline, one energy.summary per Kuala Lumpur day for the daily chart and, when
// comparing units, one per unit (and per unit and day for at most 7 days). The export modal is energy.exportReport and,
// for the monthly copy, preferences.update. URL keys: unitIds (up to 4), period (today, 7d, 30d, custom), from, to
// (custom, KL datetime-local), baselineId ("none" for no baseline). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { apiMode, coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { klLocal, summaryView, type ApiBaseline, type ApiEnergySummary } from "@ac/web/lib/energy";
import { baselineLabel, baselinesFor, compareRow, dayRanges, periodKinds, periodRange, selectionQuery } from "@ac/web/lib/clientEnergy";
import { EnergyDemo } from "./_components/energy-demo";
import { EnergyView, type EnergyLive } from "./_components/energy-view";

type Unit = { id: string; displayName: string };
type Preferences = { locale: string; timezone: string; monthlyReportEmail: boolean };

export default async function CustomerEnergyPage({ searchParams }: PageProps<"/customer/energy">) {
  await connection();
  if (!apiMode()) return <EnergyDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, units, baselines, properties, prefs] = await Promise.all([
    coreNow(), coreAll<Unit>("units.list"), coreAll<ApiBaseline>("baselines.list"), coreAll<{ id: string; name: string }>("properties.list"),
    coreOp<Preferences>("preferences.get", {}).catch(() => null),
  ]);
  const own = units.map((u) => ({ id: u.id, name: u.displayName })).sort((a, b) => a.name.localeCompare(b.name));
  const picked = [...new Set((one("unitIds") ?? "").split(","))].filter((id) => own.some((u) => u.id === id)).slice(0, 4);
  const unitIds = picked.length ? picked : own.slice(0, 1).map((u) => u.id);
  const kind = periodKinds.find((k) => k === one("period")) ?? "7d";
  const period = periodRange(kind, now, { from: one("from"), to: one("to") });
  const options = baselinesFor(baselines, unitIds);
  const baseline = one("baselineId") === "none" ? undefined : options.find((b) => b.id === one("baselineId")) ?? options[0];
  const name = new Map(own.map((u) => [u.id, u.name]));
  const lastMonth = (() => { const d = new Date(now); d.setUTCDate(1); d.setUTCMonth(d.getUTCMonth() - 1); return klLocal(d.toISOString()).slice(0, 7); })();
  const live: EnergyLive = {
    units: own, unitIds, period, baselines: options.map((b) => ({ id: b.id, label: baselineLabel(b) })), baselineId: baseline?.id ?? null,
    summary: null, daily: [], compare: null, properties, prefs, lastMonth,
    query: selectionQuery({ unitIds, period: kind, from: one("from"), to: one("to"), baselineId: baseline?.id }),
  };
  if (unitIds.length && !period.error) {
    try {
      const days = dayRanges(period.from, period.to);
      const sum = (u: string[], from: string, to: string, b?: string) => coreOp<ApiEnergySummary>("energy.summary", { from, to, unitIds: u, ...(b ? { baselineId: b } : {}) });
      const [total, perDay] = await Promise.all([sum(unitIds, period.from, period.to, baseline?.id), Promise.all(days.map((d) => sum(unitIds, d.from, d.to)))]);
      const minutes = (Date.parse(period.to) - Date.parse(period.from)) / 60000;
      // a comparable baseline is shown as an even daily share of its value (display only; savings come from the API)
      const base = total.totals.savedKWh !== null ? total.baselineSnapshot?.baselineKWh ?? null : null;
      live.summary = summaryView(total);
      live.raw = { kWh: total.totals.kWh, baseline: base, tariff: total.tariffVersion, factor: total.factorSnapshot ? `${total.factorSnapshot.region} ${total.factorSnapshot.year} · ${total.factorSnapshot.kgCO2ePerKWh} kgCO₂e/kWh (demo factor)` : null, coverage: total.coverage };
      live.daily = days.map((d, i) => ({ label: d.label, actual: perDay[i].totals.kWh, baseline: base === null ? null : (base * (Date.parse(d.to) - Date.parse(d.from))) / 60000 / minutes }));
      if (unitIds.length > 1) {
        const rows = await Promise.all(unitIds.map(async (u) => {
          const b = baselinesFor(baselines, [u])[0];
          const [s, byDay] = await Promise.all([sum([u], period.from, period.to, b?.id), days.length <= 7 ? Promise.all(days.map((d) => sum([u], d.from, d.to))) : Promise.resolve(null)]);
          return { row: compareRow(u, name.get(u) ?? u.slice(0, 8), s), byDay: byDay?.map((x) => x.totals.kWh ?? 0) ?? null };
        }));
        live.compare = { rows: rows.map((r) => r.row), series: rows.every((r) => r.byDay) ? rows.map((r) => r.byDay!) : null };
      }
    } catch (e) {
      if (!(e instanceof CoreError) || e.error.code !== "VALIDATION") throw e; // an unusable period or unit set shows on the page
      live.error = actionMessage({ code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors ?? {} });
    }
  }
  return <EnergyView live={live} />;
}
