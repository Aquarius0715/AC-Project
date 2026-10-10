"use server";

// Server Actions of the HQ automation policies (FR-A11): policies.save (kind automation), automations.simulate (a read,
// nothing stored) and automations.fire (demo-only, one-time key). Public endpoints: the DAL verifies the session and
// the Core API authorizes them (automation.policy.write) and checks the policy version.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };
type Decision = { unitId: string; decision: string; ruleId: string | null; reason: string | null; commandId?: string | null };

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

export async function saveAutoPolicy(input: OpInput<"policies.save">, version?: number) {
  return run(() => coreOp<{ id: string; version: number }>("policies.save", input, { write: true, expectedVersion: input.id ? version : undefined }).then((p) => ({ id: p.id, version: p.version })));
}
/** automations.simulate: the selected rule or the suppression reason per unit; creates no Command. */
export async function simulateAuto(input: OpInput<"automations.simulate">) {
  return run(() => coreOp<{ results: Decision[] }>("automations.simulate", input).then((r) => r.results), false);
}
/** automations.fire (demo): the event ID is the one-time key, so a repeat returns the stored result. */
export async function fireAuto(input: OpInput<"automations.fire">) {
  return run(() => coreOp<{ results: Decision[] }>("automations.fire", input, { write: true, idempotencyKey: input.eventId }).then((r) => r.results));
}
