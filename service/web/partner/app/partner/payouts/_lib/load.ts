// The reads of the payouts screen (FR-P10) through the DAL: the company's approved and paid statements
// (payouts.list; drafts never reach a contractor), the selected statement's rate card (rateCards.list), the label of
// each line's job (jobs.get: a history snapshot after the access window, the detail with its unit while it is open)
// and the delegated jobs still in quality review (jobs.list status submitted) for the “Not included” rows.
import "server-only";
import { coreAll, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { jobTypeLabel, kpis, lineRows, questionRows, statementRows, type ApiPayoutStatement, type JobLabel, type ReviewingJob } from "@ac/web/lib/partnerPayouts";
import type { PayoutsLive } from "../_components/payouts-view";

type RateCard = { id: string; version: number; effectiveFrom: string };
type JobRow = { projection: string; id?: string; jobId?: string; type: string; status: string; unitId?: string; completedAt?: string | null };
const quiet = <T,>(p: Promise<T>) => p.catch((e) => {
  if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null;
  throw e;
});
const unitName = async (id: string | undefined, cache: Map<string, Promise<string | null>>) => {
  if (!id) return null;
  if (!cache.has(id)) cache.set(id, quiet(coreOp<ApiUnitDetail>("units.get", { id })).then((u) => (u ? `${u.displayName} · ${u.location.pathLabels[0] ?? ""}`.replace(/ · $/, "") : null)));
  return cache.get(id)!;
};

export async function loadPayouts(opts: { statementId?: string; period?: string; status?: string }): Promise<PayoutsLive> {
  const [all, reviewing] = await Promise.all([
    coreAll<ApiPayoutStatement>("payouts.list"),
    quiet(coreOp<{ items: JobRow[] }>("jobs.list", { limit: 50, filters: { statuses: ["submitted"] } })),
  ]);
  const status = opts.status === "approved" || opts.status === "paid" ? opts.status : null;
  const shown = all.filter((s) => (!opts.period || s.period === opts.period) && (!status || s.status === status));
  const selected = shown.find((s) => s.id === opts.statementId) ?? shown[0] ?? null;
  const units = new Map<string, Promise<string | null>>();
  const live: PayoutsLive = {
    periods: [...new Set(all.map((s) => s.period))].sort().reverse(), period: opts.period ?? "", status: status ?? "", statements: statementRows(shown), selected: null,
  };
  if (!selected) return live;
  const [cards, labels, inReview] = await Promise.all([
    quiet(coreOp<{ items: RateCard[] }>("rateCards.list", { limit: 20, sort: { field: "effectiveFrom", direction: "desc" } })),
    Promise.all([...new Set(selected.lines.map((l) => l.jobId))].map(async (jobId): Promise<[string, JobLabel] | null> => {
      const j = await quiet(coreOp<JobRow>("jobs.get", { jobId }));
      if (!j) return null;
      const unit = j.projection === "detail" ? await unitName(j.unitId, units) : null;
      return [jobId, { title: `${jobTypeLabel[j.type] ?? j.type}${unit ? ` · ${unit.split(" · ")[0]}` : ""}`, sub: unit ? unit.split(" · ").slice(1).join(" · ") || null : j.projection === "history" ? "customer details closed with the delegation" : null }];
    })),
    Promise.all((reviewing?.items ?? []).filter((j) => j.projection === "detail" || j.projection === "summary").slice(0, 10).map(async (j): Promise<ReviewingJob> => ({
      id: j.id ?? j.jobId ?? "", type: j.type, unitName: (await unitName(j.unitId, units))?.split(" · ")[0] ?? null,
    }))),
  ]);
  const card = cards?.items.find((c) => Date.parse(c.effectiveFrom) <= Date.parse(selected.payDate)) ?? cards?.items[0] ?? null;
  const rows = lineRows(selected, new Map(labels.filter((l): l is [string, JobLabel] => !!l)), inReview);
  live.selected = { statement: selected, kpis: kpis(selected, card ? `${card.id.slice(0, 8)} v${card.version}` : null), rows, questions: questionRows(selected, rows), rateCard: card ? `rc ${card.id.slice(0, 8)} v${card.version}` : "set by HQ" };
  return live;
}
