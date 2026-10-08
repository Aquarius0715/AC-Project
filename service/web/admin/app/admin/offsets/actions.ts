"use server";

// Server Actions of the offset demo (FR-A15): offsets.preview creates a demo quote and offsets.simulate moves a record
// through the simulated request, purchase, retirement, failure and retry (demo-only operations, IR operation catalog).
// Each one is a public endpoint: the DAL verifies the session and the Core API authorizes it (offset.write).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiQuote } from "@ac/web/lib/offsets";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>, rerender = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    if (rerender) refresh();
  }
}

/** offsets.preview: a demo quote valid for 15 minutes (no record yet). */
export async function quoteOffset(input: { customerId: string; purpose: string; amountKg: number; period: { from: string; to: string }; unitIds: string[] }) {
  return run(() => coreOp<ApiQuote>("offsets.preview", input, { write: true }), false);
}

/** offsets.simulate request: the quote becomes a demo_requested record. */
export async function requestOffset(quoteId: string, quoteVersion: number) {
  return run(() => coreOp<{ id: string }>("offsets.simulate", { event: "request", quoteId, quoteVersion, demoConfirmed: true }, { write: true }).then((r) => r.id));
}

/** offsets.simulate purchase_confirm / retire / fail of the current attempt (a fresh event ID per click). */
export async function simulateOffset(recordId: string, version: number, attemptId: string, event: "purchase_confirm" | "retire" | "fail") {
  return run(() => coreOp("offsets.simulate", { event, recordId, attemptId, eventId: crypto.randomUUID(), demoConfirmed: true }, { write: true, expectedVersion: version }).then(() => null));
}

/** offsets.simulate retry of the failed current attempt. */
export async function retryOffset(recordId: string, version: number, attemptId: string) {
  return run(() => coreOp("offsets.simulate", { event: "retry", recordId, attemptId, demoConfirmed: true }, { write: true, expectedVersion: version }).then(() => null));
}
