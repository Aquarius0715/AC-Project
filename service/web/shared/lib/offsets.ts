// Offset demo (FR-A15, DATA_SOURCE=api): demo quotes and offset records projected for /admin/offsets. Every quote,
// purchase and retirement is simulated (demo-only operations). Pure code shared by the Server Component and the view.
import { klInstant, klStamp } from "@ac/web/lib/energy";

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

export const stages = ["Quoted", "Demo requested", "Demo purchased", "Demo retired"];
const stageOf: Record<string, number> = { demo_requested: 1, demo_purchased: 2, demo_retired: 3 };
/** Three decimals for kgCO₂e of offsets (the quantity rule of DD-A15). */
export const kg3 = (x: number) => new Intl.NumberFormat("en-MY", { minimumFractionDigits: 3, maximumFractionDigits: 3 }).format(x);

export type RecordRow = {
  id: string; version: number; customer: string; amount: string; state: ApiOffsetRecord["state"]; stage: number; next: string; current: Attempt | null; r: ApiOffsetRecord;
};

export function recordRows(rs: ApiOffsetRecord[], customers: { id: string; name: string }[]): RecordRow[] {
  const name = new Map(customers.map((c) => [c.id, c.name]));
  return rs.map((r) => {
    const current = r.attempts.find((a) => a.id === r.currentAttemptId) ?? null;
    const failedStage = r.state === "failed" ? (r.previousState === "demo_purchased" ? "retirement" : "purchase") : null;
    return {
      id: r.id, version: r.version, customer: name.get(r.customerId) ?? "customer", amount: `${kg3(r.amountKg)} kgCO₂e`, state: r.state, current, r,
      stage: r.state === "failed" ? -1 : stageOf[r.state] ?? 0,
      next: r.state === "demo_requested" ? "Next: confirm the demo purchase" : r.state === "demo_purchased" ? "Next: demo retirement" : r.state === "demo_retired" ? `${r.demoCertificateRef ?? "DEMO-…"} · retired` : `${failedStage} failed · previous: ${r.previousState ?? "—"}`,
    };
  });
}

export const eventItem = (e: ApiOffsetRecord["eventHistory"][number]) => ({ time: klStamp(e.occurredAt), title: `${e.action} · ${e.result}`, detail: `${e.actorId.slice(0, 8)}${e.reason ? ` · ${e.reason}` : ""}` });

/** The demo quote form (DD-A15 fields). */
export type QuoteDraft = { customerId: string; unitIds: string[]; from: string; to: string; purpose: string; amountKg: string };
export function quoteErrors(d: QuoteDraft): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.customerId) e.customerId = "Choose a customer";
  if (d.unitIds.length < 1 || d.unitIds.length > 100) e.unitIds = "Choose 1–100 units";
  if (!d.from || !d.to || d.from >= d.to) e.period = "The end must be after the start";
  if (d.purpose.trim().length < 1 || d.purpose.trim().length > 1000) e.purpose = "1–1000 characters";
  const n = Number(d.amountKg);
  if (!/^\d+(\.\d{1,3})?$/.test(d.amountKg.trim()) || !(n > 0 && n <= 100000)) e.amountKg = "Above 0, at most 100000, up to 3 decimals";
  return e;
}
export const quoteInput = (d: QuoteDraft) => ({
  customerId: d.customerId, purpose: d.purpose.trim(), amountKg: Number(d.amountKg), period: { from: klInstant(d.from), to: klInstant(d.to) }, unitIds: d.unitIds,
});
