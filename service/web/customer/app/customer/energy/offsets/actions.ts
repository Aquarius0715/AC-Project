"use server";

// Server Actions of the customer carbon offsets page (FR-C13). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (client:self; offsets.simulate only event=request or retry).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiOffsetRecord, ApiQuote } from "@ac/web/lib/offsets";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };
const failure = (e: unknown): ActionResult<never> => (e instanceof CoreError
  ? { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} }
  : { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} });

/** offsets.preview: a demo quote for the energy selection (no offset record is created). */
export async function getQuote(input: { purpose: string; amountKg: number; period: { from: string; to: string }; unitIds: string[] }): Promise<ActionResult<ApiQuote & { estimatedAmountMinor: number | null; currency: string | null }>> {
  try {
    return { ok: true, value: await coreOp("offsets.preview", input, { write: true }) };
  } catch (e) {
    return failure(e);
  }
}
/** offsets.simulate request: records a demo request for the quote (an expired or changed quote is CONFLICT). */
export async function requestOffset(quoteId: string, quoteVersion: number): Promise<ActionResult<ApiOffsetRecord>> {
  try {
    return { ok: true, value: await coreOp("offsets.simulate", { event: "request", quoteId, quoteVersion, demoConfirmed: true }, { write: true }) };
  } catch (e) {
    return failure(e);
  } finally {
    refresh();
  }
}
