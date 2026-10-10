// /customer/energy/offsets (FR-C13, SCR-C13): in API mode a Server Component reads energy.summary for the selection
// carried from Energy & cost (unitIds, period, from, to — the estimated emissions and savings) and the customer's
// offsets.list. The demo quote is offsets.preview (writes no record) and the demo request offsets.simulate request
// with the quote version and demoConfirmed; both are Server Actions. Nothing real is bought or retired. The Phase 1A
// demo keeps the fixtures. Texts in the user's display language (IR265).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { summaryView, type ApiEnergySummary } from "@ac/web/lib/energy";
import type { ApiOffsetRecord } from "@ac/web/lib/offsets";
import { periodKinds, periodRange, pickUnits, selectionQuery } from "@ac/web/lib/clientEnergy";
import { OffsetsDemo } from "./_components/offsets-demo";
import { OffsetsView } from "./_components/offsets-view";

export default async function CustomerOffsetsPage({ searchParams }: PageProps<"/customer/energy/offsets">) {
  await connection();
  if (!apiMode()) return <OffsetsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, units, records, display] = await Promise.all([
    coreNow(), coreAll<{ id: string; displayName: string }>("units.list"), coreAll<ApiOffsetRecord>("offsets.list", { sort: { field: "createdAt", direction: "desc" } }), coreDisplay(),
  ]);
  const i = i18nOf(display);
  const unitIds = pickUnits(units, one("unitIds")); // the same default unit as Energy & cost
  const kind = periodKinds.find((k) => k === one("period")) ?? "7d";
  const period = periodRange(kind, now, { from: one("from"), to: one("to") }, i);
  let estimate: { emissions: string; savedEmissions: string; factor: string; comparable: boolean } | null = null;
  if (unitIds.length && !period.error) {
    try {
      const s = await coreOp<ApiEnergySummary>("energy.summary", { from: period.from, to: period.to, unitIds, ...(one("baselineId") ? { baselineId: one("baselineId") } : {}) });
      const v = summaryView(s, i);
      estimate = { emissions: v.emissions, savedEmissions: v.savedEmissions, comparable: v.comparable, factor: s.factorSnapshot ? `${s.factorSnapshot.region} ${s.factorSnapshot.year}` : i.t("emission factor missing") };
    } catch (e) {
      if (!(e instanceof CoreError) || e.error.code !== "VALIDATION") throw e;
    }
  }
  const name = new Map(units.map((u) => [u.id, u.displayName]));
  return <OffsetsView live={{
    unitIds, subject: unitIds.map((u) => name.get(u) ?? u.slice(0, 8)).join(", "), period: { from: period.from, to: period.to, label: period.label }, estimate,
    back: selectionQuery({ unitIds, period: kind, from: one("from"), to: one("to"), baselineId: one("baselineId") }),
    records: records.map((r) => ({ id: r.id, amountKg: r.amountKg, state: r.state, proof: r.demoCertificateRef ?? r.retirementRef ?? null, createdAt: r.createdAt })),
  }} />;
}
