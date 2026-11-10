// The reads of the payouts screen (FR-P10) through the DAL: the company's approved and paid statements
// (payouts.list; drafts never reach a contractor), the selected statement's rate card (rateCards.list), the label of
// each line's job (jobs.get: a history snapshot after the access window, the detail with its unit while it is open)
// and the delegated jobs still in quality review (jobs.list status submitted) for the “Not included” rows. Texts in
// the user's display language; every date is formatted here, on the server (IR279).
import "server-only";
import { coreAll, coreDisplay, coreIdentity, coreOp, orAbsent } from "@ac/web/lib/dal";
import { i18nOf, showDate } from "@ac/web/lib/i18n";
import { businessDay } from "@ac/web/lib/clientBilling";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import { kpis, lineRows, money, periodLabel, questionRows, statementRows, topicLabel, type ApiPayoutStatement, type JobLabel, type ReviewingJob } from "@ac/web/lib/partnerPayouts";
import type { PayoutsLive } from "../_components/payouts-view";
import { keysOf } from "@ac/web/lib/keysOf";
import { byText } from "@ac/web/lib/order";
import { entryOf } from "@ac/web/lib/lookup";

const quiet = orAbsent;
const unitName = async (id: string | undefined, cache: Map<string, Promise<string | null>>) => {
  if (!id) return null;
  const cached = cache.get(id);
  if (cached) return cached;
  const name = quiet(coreOp<ApiUnitDetail>("units.get", { id })).then((u) => (u ? `${u.displayName} · ${u.location.pathLabels[0] ?? ""}`.replace(/ · $/, "") : null));
  cache.set(id, name);
  return name;
};

export async function loadPayouts(options: { statementId?: string; period?: string; status?: string }): Promise<PayoutsLive> {
  interface JobRow { projection: string; id?: string; jobId?: string; type: string; status: string; unitId?: string; completedAt?: string | null }
  interface RateCard { id: string; version: number; effectiveFrom: string }
  const [all, reviewing, display, who] = await Promise.all([
    coreAll<ApiPayoutStatement>("payouts.list"),
    quiet(coreOp<{ items: JobRow[] }>("jobs.list", { limit: 50, filters: { statuses: ["submitted"] } })),
    coreDisplay(), coreIdentity(),
  ]);
  const i = i18nOf(display);
  const { t } = i;
  const status = options.status === "approved" || options.status === "paid" ? options.status : null;
  const shown = all.filter((s) => (!options.period || s.period === options.period) && (!status || s.status === status));
  const selected = shown.find((s) => s.id === options.statementId) ?? shown[0] ?? null;
  const units = new Map<string, Promise<string | null>>();
  const live: PayoutsLive = {
    periods: [...new Set(all.map((s) => s.period))].toSorted(byText).toReversed().map((p) => ({ value: p, text: periodLabel(p, display.locale) })), period: options.period ?? "", status: status ?? "",
    statements: statementRows(shown, i), selected: null, company: who.organizationName || t("your company"),
    topics: keysOf(topicLabel).map((x) => ({ id: x, text: t(entryOf(topicLabel, x)) })),
  };
  if (!selected) return live;
  const [cards, labels, inReview] = await Promise.all([
    quiet(coreOp<{ items: RateCard[] }>("rateCards.list", { limit: 20, sort: { field: "effectiveFrom", direction: "desc" } })),
    Promise.all([...new Set(selected.lines.map((l) => l.jobId))].map(async (jobId): Promise<[string, JobLabel] | null> => {
      const j = await quiet(coreOp<JobRow>("jobs.get", { jobId }));
      if (!j) return null;
      const unit = j.projection === "detail" ? await unitName(j.unitId, units) : null;
      const closed = j.projection === "history" ? t("customer details closed with the delegation") : null;
      return [jobId, { title: `${typeLabel(j.type, t)}${unit ? ` · ${unit.split(" · ", 1)[0] ?? unit}` : ""}`, sub: unit ? unit.split(" · ").slice(1).join(" · ") || null : closed }];
    })),
    Promise.all((reviewing?.items ?? []).filter((j) => j.projection === "detail" || j.projection === "summary").slice(0, 10).map(async (j): Promise<ReviewingJob> => {
      const name = await unitName(j.unitId, units);
      return {
        id: j.id ?? j.jobId ?? "", type: j.type, unitName: name?.split(" · ", 1)[0] ?? null,
      };
    })),
  ]);
  const card = cards?.items.find((c) => Date.parse(c.effectiveFrom) <= Date.parse(selected.payDate)) ?? cards?.items[0] ?? null;
  const rows = lineRows(selected, new Map(labels.filter((l): l is [string, JobLabel] => !!l)), inReview, i);
  const s = selected;
  const paidDate = s.paidAt ? showDate(s.paidAt, display) : "—";
  live.selected = {
    statement: s,
    kpis: kpis(s, card ? `${card.id.slice(0, 8)} v${String(card.version)}` : null, i),
    questions: questionRows(s, rows, t),
    title: t("{id} · {period} · lines", { id: s.id.slice(0, 8), period: periodLabel(s.period, display.locale) }),
    sub: t(s.status === "paid" ? "Rate card {card} · paid {date}" : "Rate card {card} · pays {date}", { card: card ? `rc ${card.id.slice(0, 8)} v${String(card.version)}` : t("set by HQ"), date: s.status === "paid" ? paidDate : businessDay(s.payDate, display.locale) }),
    totals: [[t("Gross"), money(s.grossMinor, s.currency)], [t("Deductions"), money(-s.deductionsMinor, s.currency)], [t("Net payable"), money(s.netMinor, s.currency)]],
    rows,
  };
  return live;
}
