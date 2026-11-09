"use server";

// Server Actions of the HQ billing screen (FR-A08, FR-A23). Each one is a public endpoint: the DAL verifies the session
// and the Core API authorizes the call (billing.write, billing.payment) and checks the expected version.
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run<T>(fn: () => Promise<T>, rerender = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (error) {
    return actionFailure(error);
  } finally {
    if (rerender) refresh();
  }
}

export interface NewInvoice { contractId: string; contractVersion: number; from: string; to: string; dueAt: string; amountMinor: number; currency: OpInput<"invoices.create">["currency"] }

/** invoices.create for a contract version (dates are Kuala Lumpur days). */
export async function createInvoice(n: NewInvoice) {
  return run(async () => { await coreOp("invoices.create", {
      contractId: n.contractId, contractVersion: n.contractVersion, amountMinor: n.amountMinor, currency: n.currency,
      period: { from: `${n.from}T00:00:00+08:00`, to: `${n.to}T00:00:00+08:00` }, dueAt: `${n.dueAt}T00:00:00+08:00`,
    }, { write: true }); return null; });
}

export async function recordManualPayment(invoiceId: string, version: number, paymentReference: string, amountMinor: number, currency: OpInput<"payments.recordManual">["currency"], reason: string) {
  return run(async () => { await coreOp("payments.recordManual", { confirmedAmountMinor: amountMinor, invoiceId, paymentReference, currency, reason }, { write: true, expectedVersion: version }); return null; });
}

export async function confirmCardPayment(paymentId: string, version: number, paymentReference: string, amountMinor: number, currency: OpInput<"payments.confirm">["currency"], reason: string) {
  return run(async () => { await coreOp("payments.confirm", { confirmedAmountMinor: amountMinor, paymentId, paymentReference, currency, reason }, { write: true, expectedVersion: version }); return null; });
}

/** notifications.preview of a payment reminder (a read: nothing is sent, the route is not re-rendered). */
export async function previewReminder(invoiceId: string, recipientMembershipId: string, channel: OpInput<"invoices.remind">["channel"], reason: string) {
  return run(async () => { const p = await coreOp<{ params: { targetName: string } }>("notifications.preview", {
      target: { kind: "invoice", id: invoiceId }, templateKey: "payment_reminder", channel, recipientMembershipId, reason,
    }); return p.params.targetName; }, false);
}

export async function sendReminder(invoiceId: string, version: number, recipientMembershipId: string, channel: OpInput<"invoices.remind">["channel"], reason: string) {
  return run(async () => { await coreOp("invoices.remind", { invoiceId, recipientMembershipId, channel, reason }, { write: true, expectedVersion: version }); return null; });
}

export async function answerInquiry(inquiryId: string, version: number, reply: string) {
  return run(async () => { await coreOp("inquiries.answer", { inquiryId, reply }, { write: true, expectedVersion: version }); return null; });
}

export async function generatePayouts(period: string) {
  return run(async () => { const s = await coreOp<unknown[]>("payouts.generate", { period }, { write: true }); return s.length; });
}

export async function transitionStatement(statementId: string, version: number, action: "approve" | "mark_paid") {
  return run(async () => { await coreOp("payouts.transition", { statementId, action }, { write: true, expectedVersion: version }); return null; });
}

export async function resolvePayoutQuery(statementId: string, version: number, queryId: string, reply: string, adjustmentMinor: number | null) {
  return run(async () => { await coreOp("payouts.resolveQuery", { statementId, queryId, reply, ...(adjustmentMinor && { adjustmentMinor }) }, { write: true, expectedVersion: version }); return null; });
}
