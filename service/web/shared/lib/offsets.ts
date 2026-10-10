// Offset demo (FR-A15, DATA_SOURCE=api): demo quotes and offset records projected for /admin/offsets. Every quote,
// purchase and retirement is simulated (demo-only operations). Pure code shared by the Server Component and the view;
// texts in the display language and instants in the display time zone, while a quote's period stays Kuala Lumpur time
// (IR298).
import { klInstant } from "@ac/web/lib/energy";
import { EN, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

/** OffsetQuote of service-contracts.ts. */
export type ApiQuote = {
  id: string; version: number; customerId: string; purpose: string; amountKg: number; period: { from: string; to: string }; unitIds: string[]; expiresAt: string;
  provider: "unselected"; scheme: "demo"; marketConcept: { stage: string; providerLabel: string; verificationStatus: string; ledgerStatus: string };
};
type Attempt = { id: string; stage: "purchase" | "retirement"; status: "pending" | "succeeded" | "failed"; startedAt: string; completedAt: string | null };
/** OffsetRecord of service-contracts.ts. */
export type ApiOffsetRecord = {
  id: string; version: number; createdAt: string; quoteId: string; customerId: string; amountKg: number; attempts: Attempt[]; currentAttemptId: string | null;
  state: "demo_requested" | "demo_purchased" | "demo_retired" | "failed"; previousState: ApiOffsetRecord["state"] | null; purchaseRef: string | null; retirementRef: string | null;
  demoCertificateRef: string | null; eventHistory: { action: string; occurredAt: string; actorId: string; result: string; reason: string | null }[];
};

const stageOf: Record<string, number> = { demo_requested: 1, demo_purchased: 2, demo_retired: 3 };
/** Three decimals for kgCO₂e of offsets (the quantity rule of DD-A15). */
export const kg3 = (x: number) => new Intl.NumberFormat("en-MY", { minimumFractionDigits: 3, maximumFractionDigits: 3 }).format(x);

export type TimelineItem = { time: string; title: string; detail: string };
export type RecordRow = {
  id: string; version: number; customer: string; amount: string; state: ApiOffsetRecord["state"]; stateText: string; stage: number; next: string;
  current: Attempt | null; currentText: string | null; events: TimelineItem[]; r: ApiOffsetRecord;
};

/** A record state as the screens word it (the customer offsets page uses the same words). */
export function stateWord(s: ApiOffsetRecord["state"] | null, t: T = en): string {
  const words: Record<ApiOffsetRecord["state"], string> = { demo_requested: t("Demo requested"), demo_purchased: t("Demo purchased"), demo_retired: t("Demo retired"), failed: t("Failed") };
  return s ? words[s] ?? s : "—";
}

/** An attempt: its stage, status and start in the display time zone. */
export function attemptText(a: Attempt, i: I18n = EN): string {
  const { t } = i;
  const status: Record<Attempt["status"], string> = { pending: t("pending"), succeeded: t("succeeded"), failed: t("failed") };
  return t("{stage} · {status} · started {time}", { stage: a.stage === "purchase" ? t("purchase") : t("retirement"), status: status[a.status] ?? a.status, time: showTime(a.startedAt, i.display) });
}

/** An event of the record's audit history: what was done, its audit result and when, in the display time zone. */
export function eventItem(e: ApiOffsetRecord["eventHistory"][number], i: I18n = EN): TimelineItem {
  const { t } = i;
  const action: Record<string, string> = {
    "offsets.request": t("Demo request"), "offsets.purchase_confirm": t("Demo purchase confirmed"), "offsets.retire": t("Demo retirement"), "offsets.fail": t("Simulated failure"), "offsets.retry": t("Retry started"),
  };
  const result: Record<string, string> = { success: t("done"), denied: t("denied"), failed: t("failed"), pending: t("pending") };
  return { time: showTime(e.occurredAt, i.display), title: `${action[e.action] ?? e.action} · ${result[e.result] ?? e.result}`, detail: `${e.actorId.slice(0, 8)}${e.reason ? ` · ${e.reason}` : ""}` };
}

export function recordRows(rs: ApiOffsetRecord[], customers: { id: string; name: string }[], i: I18n = EN): RecordRow[] {
  const { t } = i;
  const name = new Map(customers.map((c) => [c.id, c.name]));
  return rs.map((r) => {
    const current = r.attempts.find((a) => a.id === r.currentAttemptId) ?? null;
    return {
      id: r.id, version: r.version, customer: name.get(r.customerId) ?? t("customer"), amount: `${kg3(r.amountKg)} kgCO₂e`, state: r.state, stateText: stateWord(r.state, t), current, r,
      currentText: current ? attemptText(current, i) : null, events: r.eventHistory.map((e) => eventItem(e, i)),
      stage: r.state === "failed" ? -1 : stageOf[r.state] ?? 0,
      next: r.state === "demo_requested" ? t("Next: confirm the demo purchase") : r.state === "demo_purchased" ? t("Next: demo retirement")
        : r.state === "demo_retired" ? t("{certificate} · retired", { certificate: r.demoCertificateRef ?? "DEMO-…" })
        : r.previousState === "demo_purchased" ? t("Retirement failed · previous: {state}", { state: stateWord(r.previousState, t) }) : t("Purchase failed · previous: {state}", { state: stateWord(r.previousState, t) }),
    };
  });
}

/** The market concept of a quote: known codes worded, others as they are. */
export function conceptText(m: ApiQuote["marketConcept"], t: T = en): string {
  const word: Record<string, string> = { future_concept: t("future concept"), unverified: t("unverified"), not_connected: t("not connected") };
  return t("{stage} · verification {verification} · ledger {ledger}", { stage: word[m.stage] ?? m.stage, verification: word[m.verificationStatus] ?? m.verificationStatus, ledger: word[m.ledgerStatus] ?? m.ledgerStatus });
}

/** The demo quote form (DD-A15 fields). */
export type QuoteDraft = { customerId: string; unitIds: string[]; from: string; to: string; purpose: string; amountKg: string };
export function quoteErrors(d: QuoteDraft, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.customerId) e.customerId = t("Choose a customer");
  if (d.unitIds.length < 1 || d.unitIds.length > 100) e.unitIds = t("Choose 1–100 units");
  if (!d.from || !d.to || d.from >= d.to) e.period = t("The end must be after the start");
  if (d.purpose.trim().length < 1 || d.purpose.trim().length > 1000) e.purpose = t("1–1000 characters");
  const n = Number(d.amountKg);
  if (!/^\d+(\.\d{1,3})?$/.test(d.amountKg.trim()) || !(n > 0 && n <= 100000)) e.amountKg = t("Above 0, at most 100000, up to 3 decimals");
  return e;
}
export const quoteInput = (d: QuoteDraft) => ({
  customerId: d.customerId, purpose: d.purpose.trim(), amountKg: Number(d.amountKg), period: { from: klInstant(d.from), to: klInstant(d.to) }, unitIds: d.unitIds,
});
