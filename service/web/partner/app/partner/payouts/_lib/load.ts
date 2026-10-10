// The reads of the payouts screen (FR-P10) through the DAL: the company's approved and paid statements
// (payouts.list; drafts never reach a contractor), the selected statement's rate card (rateCards.list), the label of
// each line's job (jobs.get: a history snapshot after the access window, the detail with its unit while it is open)
// and the delegated jobs still in quality review (jobs.list status submitted) for the “Not included” rows. Texts in
// the user's display language; every date is formatted here, on the server (IR279).
import "server-only";
import { coreAll, coreDisplay, coreIdentity, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showDate } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import { kpis, lineRows, money, periodLabel, questionRows, statementRows, topicLabel, type ApiPayoutStatement, type JobLabel, type ReviewingJob } from "@ac/web/lib/partnerPayouts";
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
  const [all, reviewing, display, who] = await Promise.all([
    coreAll<ApiPayoutStatement>("payouts.list"),
    quiet(coreOp<{ items: JobRow[] }>("jobs.list", { limit: 50, filters: { statuses: ["submitted"] } })),
    coreDisplay(), coreIdentity(),
  ]);
  const i = i18nOf(display);
  const { t } = i;
  const status = opts.status === "approved" || opts.status === "paid" ? opts.status : null;
  const shown = all.filter((s) => (!opts.period || s.period === opts.period) && (!status || s.status === status));
  const selected = shown.find((s) => s.id === opts.statementId) ?? shown[0] ?? null;
  const units = new Map<string, Promise<string | null>>();
  const live: PayoutsLive = {
    periods: [...new Set(all.map((s) => s.period))].sort().reverse().map((p) => ({ value: p, text: periodLabel(p, display.locale) })), period: opts.period ?? "", status: status ?? "",
    statements: statementRows(shown, i), selected: null, company: who.organizationName || t("your company"),
    topics: (Object.keys(topicLabel) as (keyof typeof topicLabel)[]).map((x) => ({ id: x, text: t(topicLabel[x]) })),
  };
  if (!selected) return live;
  const [cards, labels, inReview] = await Promise.all([
    quiet(coreOp<{ items: RateCard[] }>("rateCards.list", { limit: 20, sort: { field: "effectiveFrom", direction: "desc" } })),
    Promise.all([...new Set(selected.lines.map((l) => l.jobId))].map(async (jobId): Promise<[string, JobLabel] | null> => {
      const j = await quiet(coreOp<JobRow>("jobs.get", { jobId }));
      if (!j) return null;
      const unit = j.projection === "detail" ? await unitName(j.unitId, units) : null;
      return [jobId, { title: `${typeLabel(j.type, t)}${unit ? ` · ${unit.split(" · ")[0]}` : ""}`, sub: unit ? unit.split(" · ").slice(1).join(" · ") || null : j.projection === "history" ? t("customer details closed with the delegation") : null }];
    })),
    Promise.all((reviewing?.items ?? []).filter((j) => j.projection === "detail" || j.projection === "summary").slice(0, 10).map(async (j): Promise<ReviewingJob> => ({
      id: j.id ?? j.jobId ?? "", type: j.type, unitName: (await unitName(j.unitId, units))?.split(" · ")[0] ?? null,
    }))),
  ]);
  const card = cards?.items.find((c) => Date.parse(c.effectiveFrom) <= Date.parse(selected.payDate)) ?? cards?.items[0] ?? null;
  const rows = lineRows(selected, new Map(labels.filter((l): l is [string, JobLabel] => !!l)), inReview, i);
  const s = selected;
  live.selected = {
    statement: s, kpis: kpis(s, card ? `${card.id.slice(0, 8)} v${card.version}` : null, i), rows, questions: questionRows(s, rows, t),
    title: t("{id} · {period} · lines", { id: s.id.slice(0, 8), period: periodLabel(s.period, display.locale) }),
    sub: t(s.status === "paid" ? "Rate card {card} · paid {date}" : "Rate card {card} · pays {date}", { card: card ? `rc ${card.id.slice(0, 8)} v${card.version}` : t("set by HQ"), date: s.status === "paid" ? (s.paidAt ? showDate(s.paidAt, display) : "—") : businessDay(s.payDate, display.locale) }),
    totals: [[t("Gross"), money(s.grossMinor, s.currency)], [t("Deductions"), money(-s.deductionsMinor, s.currency)], [t("Net payable"), money(s.netMinor, s.currency)]],
  };
  return live;
}
