"use server";

// Server Actions of the customer invoice page (FR-C11, FR-C12). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (client:self) and checks the invoice or payment version.
import { refresh } from "next/cache";
import { coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { demoReference, type ClientPayment, type PreviewParams } from "@ac/web/lib/clientBilling";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
const write = (version?: number) => (version === undefined ? { write: true } : { write: true, expectedVersion: version });

/** payments.simulate initiate with the chosen demo card; the demo PSP takes the card at once (processing), and the
 * invoice is paid only after a confirmation event (DD-C11). */
export async function startPayment(invoiceId: string, version: number, method: "demo_credit_card" | "demo_debit_card") {
  return run(async () => {
    const p = await coreOp<ClientPayment>("payments.simulate", { event: "initiate", invoiceId, method, demoConfirmed: true }, write(version));
    await coreOp("payments.simulate", { event: "processing", paymentId: p.id, eventId: crypto.randomUUID(), paymentReference: demoReference(p.id) }, write(p.version));
    return p.id;
  });
}
/** A demo PSP event for an open payment: processing, confirm (marks the invoice paid) or fail (stays unpaid). */
export async function demoOutcome(paymentId: string, version: number, event: "processing" | "confirm" | "fail") {
  return run(() => coreOp<ClientPayment>("payments.simulate", { event, paymentId, eventId: crypto.randomUUID(), paymentReference: demoReference(paymentId) }, write(version)).then((p) => p.status));
}
/** Payment instructions only: no Payment is created and the invoice stays unpaid. */
export async function paymentInstructions(invoiceId: string, version: number) {
  return run(() => coreOp<{ params: PreviewParams }>("payments.simulate", { event: "instructions", invoiceId, demoConfirmed: true }, write(version)).then((n) => n.params));
}
/** notifications.preview of the payment message for one channel (nothing is stored or sent). */
export async function previewMessage(invoiceId: string, channel: "email" | "whatsapp") {
  return run(async () => {
    const me = await corePrincipal();
    const n = await coreOp<{ params: PreviewParams; templateKey: string; channel: string }>("notifications.preview",
      { target: { kind: "invoice", id: invoiceId }, templateKey: "payment", channel, recipientMembershipId: me.membershipId });
    return { params: n.params, templateKey: n.templateKey, channel: n.channel };
  });
}
export async function sendInquiry(input: { subjectType: "payment"; invoiceId: string; message: string } | { subjectType: "restriction"; restrictionId: string; message: string }) {
  return run(() => coreOp<{ id: string }>("inquiries.create", { ...input, message: input.message.trim() }, write()).then((i) => i.id));
}
